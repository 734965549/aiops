package application

import (
	"fmt"

	"github.com/734965549/aiops/internal/ai/toolgateway"
)

const (
	// chatDefaultPromptTokenBudget 是全局默认的单次 prompt token 预算。
	chatDefaultPromptTokenBudget = 32 * 1024
	// chatDefaultMaxOutputTokens 是 provider 未配置输出上限时的全局默认 max_tokens。
	chatDefaultMaxOutputTokens = toolgateway.DefaultMaxOutputTokens
	// chatEstimateBytesPerToken 是保守估算口径：UTF-8 3 字节 ≈ 1 token，宁多勿少。
	chatEstimateBytesPerToken = 3
	// chatImageTokenEstimate 是单张图片的保守 token 估算，避免图片消息绕过文本预算。
	chatImageTokenEstimate = 1600
)

// chatPromptTrim 记录一次 prompt 裁剪结果，用于日志与审计。
type chatPromptTrim struct {
	EstimatedTokens     int
	Budget              int
	DroppedHistory      int
	StrippedPoints      bool
	DroppedObservations int
	DroppedMemory       int
}

func (t chatPromptTrim) trimmed() bool {
	return t.DroppedHistory > 0 || t.StrippedPoints || t.DroppedObservations > 0 || t.DroppedMemory > 0
}

func (t chatPromptTrim) actions() []string {
	out := make([]string, 0, 4)
	if t.DroppedHistory > 0 {
		out = append(out, fmt.Sprintf("dropped_history:%d", t.DroppedHistory))
	}
	if t.StrippedPoints {
		out = append(out, "stripped_metric_points")
	}
	if t.DroppedObservations > 0 {
		out = append(out, fmt.Sprintf("dropped_observations:%d", t.DroppedObservations))
	}
	if t.DroppedMemory > 0 {
		out = append(out, fmt.Sprintf("dropped_memory:%d", t.DroppedMemory))
	}
	return out
}

// estimateChatPromptTokens 按字节数保守估算 token。
func estimateChatPromptTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + chatEstimateBytesPerToken - 1) / chatEstimateBytesPerToken
}

// promptBudgetFor 计算本次请求的输入预算与输出上限：
// provider 配置了 context_window_tokens 时输入预算 = 窗口 - max_tokens（provider 覆盖全局）；
// 未配置窗口时输入预算使用全局值。输出上限由 provider 配置或全局默认决定，
// 且不超过窗口的一半，保证输入至少还剩一半窗口。
func (s *ChatService) promptBudgetFor(providerID string) (inputBudget, outputTokens int) {
	inputBudget = s.promptTokenBudget
	if inputBudget <= 0 {
		inputBudget = chatDefaultPromptTokenBudget
	}
	fallbackOutput := s.maxOutputTokens
	if fallbackOutput <= 0 {
		fallbackOutput = chatDefaultMaxOutputTokens
	}
	window, configuredOutput := 0, 0
	if s.chat != nil {
		if catalog, ok := s.chat.(toolgateway.ChatModelCatalog); ok {
			if model, found := catalog.ChatProviderByID(providerID); found {
				window = model.ContextWindowTokens
				configuredOutput = model.MaxOutputTokens
			}
		}
	}
	outputTokens = toolgateway.EffectiveOutputTokens(window, configuredOutput, fallbackOutput)
	if window > 0 {
		inputBudget = window - outputTokens
		if inputBudget < 0 {
			inputBudget = 0
		}
	}
	return inputBudget, outputTokens
}

// recordPromptBudget 把最近一轮的估算与裁剪结果写入上下文，供审计与排查。
func recordPromptBudget(data *chatContextData, trim chatPromptTrim) {
	if data == nil {
		return
	}
	data.PromptTokenEstimate = trim.EstimatedTokens
	data.PromptTokenBudget = trim.Budget
	if trim.trimmed() {
		data.PromptTrims = trim.actions()
	} else {
		data.PromptTrims = nil
	}
}

// buildChatUserPromptWithBudget 在预算内组装 user prompt：
// 超限时按 历史（最旧优先）→ 指标 points → 低优先级观测 → 记忆案例 的顺序裁剪。
// fixedTokens 是 system prompt、图片等不可裁剪输入的估算占用。
// ok=false 表示裁剪到最小仍超预算，调用方不得再请求模型。
func buildChatUserPromptWithBudget(in ChatPromptInput, budget, fixedTokens int) (string, chatPromptTrim, bool) {
	trim := chatPromptTrim{Budget: budget}
	if budget <= 0 {
		trim.Budget = chatDefaultPromptTokenBudget
		budget = trim.Budget
	}
	history := append([]ChatHistoryMessage(nil), in.History...)
	observations := copyChatObservations(in.Observations)
	memory := append([]chatMemoryCase(nil), in.Memory...)

	build := func() (string, int) {
		attempt := in
		attempt.History = history
		attempt.Observations = observations
		attempt.Memory = memory
		prompt := buildChatUserPrompt(attempt)
		return prompt, estimateChatPromptTokens(prompt) + fixedTokens
	}

	prompt, tokens := build()
	trim.EstimatedTokens = tokens
	if tokens <= budget {
		return prompt, trim, true
	}
	// 1) 从最旧开始逐条丢弃历史。
	for len(history) > 0 {
		history = history[1:]
		trim.DroppedHistory++
		prompt, tokens = build()
		trim.EstimatedTokens = tokens
		if tokens <= budget {
			return prompt, trim, true
		}
	}
	// 2) 指标证据只保留统计，丢弃逐点序列。
	if stripChatObservationPoints(observations) {
		trim.StrippedPoints = true
		prompt, tokens = build()
		trim.EstimatedTokens = tokens
		if tokens <= budget {
			return prompt, trim, true
		}
	}
	// 3) 按工具优先级逐条丢弃观测（未知工具最先丢）。
	for len(observations) > 0 {
		idx := lowestPriorityObservationIndex(observations)
		observations = append(observations[:idx], observations[idx+1:]...)
		trim.DroppedObservations++
		prompt, tokens = build()
		trim.EstimatedTokens = tokens
		if tokens <= budget {
			return prompt, trim, true
		}
	}
	// 4) 逐条丢弃历史案例记忆。
	for len(memory) > 0 {
		memory = memory[1:]
		trim.DroppedMemory++
		prompt, tokens = build()
		trim.EstimatedTokens = tokens
		if tokens <= budget {
			return prompt, trim, true
		}
	}
	return prompt, trim, false
}

// copyChatObservations 复制观测切片及 Data 顶层 map，避免裁剪污染原始观测。
func copyChatObservations(in []ChatObservation) []ChatObservation {
	if len(in) == 0 {
		return nil
	}
	out := make([]ChatObservation, len(in))
	copy(out, in)
	for i := range out {
		if in[i].Data == nil {
			continue
		}
		data := make(map[string]any, len(in[i].Data))
		for key, value := range in[i].Data {
			data[key] = value
		}
		out[i].Data = data
	}
	return out
}

// stripChatObservationPoints 丢弃指标证据的逐点序列，仅保留统计摘要。
func stripChatObservationPoints(observations []ChatObservation) bool {
	stripped := false
	for i := range observations {
		data := observations[i].Data
		if data == nil {
			continue
		}
		metrics, ok := data["metrics"].([]MetricEvidence)
		if !ok || len(metrics) == 0 {
			continue
		}
		changed := false
		cloned := make([]MetricEvidence, len(metrics))
		copy(cloned, metrics)
		for j := range cloned {
			if len(cloned[j].Points) > 0 {
				cloned[j].Points = nil
				changed = true
			}
		}
		if !changed {
			continue
		}
		data["metrics"] = cloned
		data["points_stripped"] = true
		stripped = true
	}
	return stripped
}

// chatObservationKeepRank 是观测裁剪优先级：数值越小越先丢弃。
var chatObservationKeepRank = map[string]int{
	chatToolAlarmAnalyze:     90,
	chatToolMetricsQuery:     80,
	chatToolPostgresActivity: 70,
	chatToolAlertRulesHealth: 60,
	chatToolAlertRulesList:   50,
	chatToolRDSBackupQuery:   40,
	chatToolResourcesList:    30,
	chatToolResourceAnalyze:  20,
	chatToolAccountsList:     10,
}

// lowestPriorityObservationIndex 返回应最先丢弃的观测下标；同优先级取最早一条。
func lowestPriorityObservationIndex(observations []ChatObservation) int {
	idx := 0
	lowest := chatObservationKeepRank[observations[0].ToolCode]
	for i := 1; i < len(observations); i++ {
		rank := chatObservationKeepRank[observations[i].ToolCode]
		if rank < lowest {
			lowest = rank
			idx = i
		}
	}
	return idx
}
