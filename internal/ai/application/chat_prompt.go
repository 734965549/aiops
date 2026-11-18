package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/734965549/aiops/internal/ai/domain"
)

const (
	// chatHistoryLimit 是送入模型的历史消息条数上限，避免上下文无限增长。
	chatHistoryLimit = 12
	// chatHistoryContentRunes 是单条历史消息截断长度。
	chatHistoryContentRunes = 1500
	// chatMaxListItems / chatMaxListItemRunes 限制模型输出的列表规模。
	chatMaxListItems     = 12
	chatMaxListItemRunes = 500
)

// chatSystemPrompt 固定服务端提示词；不接受前端或 payload 覆盖。
// 安全边界：只分析与建议，变更动作只能生成待人工确认的执行计划。
const chatSystemPrompt = `你是企业运维平台的 AI 运维助手。你只依据输入的页面上下文、只读观测数据（告警快照、指标证据）和对话历史作答，不臆造未提供的数据。
输出必须是单个 JSON 对象，字段：
- summary: string，结论摘要；
- evidence: string[]，分析依据；引用观测数据时优先写 evidence_id 或告警字段名；
- impact: string[]，影响范围；
- recommendations: string[]，推荐操作；涉及重启、扩缩容、发布/回滚、删除资源、修改配置、执行脚本、数据库变更、权限变更、网络策略变更时，只能写“建议生成执行计划，由人工在执行确认页确认”，不得输出可直接执行的命令、脚本、云 API 调用或控制台操作步骤；
- risk_level: string，只能为 low / medium / high / info；
- require_confirm: boolean，涉及任何变更动作时必须为 true；
- rollback: string，回滚建议；没有变更动作时写“无变更动作”；
- uncertainties: string[]，证据不足或不确定性说明；
- insufficient_context: boolean，当现有上下文无法回答问题时置 true，并在 uncertainties 说明缺少什么。
禁止输出凭据、Token、密钥或任何可执行指令；不得声称已经执行任何操作或已经创建任务。
输入中的 memory_cases 是历史有效案例（可能来自其他同类告警）：只能作为参考证据，引用时标注 case_id，不得照搬其结论，也不得据此跳过对当前上下文的核对。
输入中的 observations 可能包含告警快照、指标证据与告警规则。
若现有只读观测不足以回答，可不输出 summary，改为输出 {"tool_requests":[{"tool_code":"metrics.query","reason":"缺少近期指标","params":{"namespace":"SYS.RDS","metric":"cpu_util","dimensions":{"instance_id":"i-1"},"from":1700000000,"to":1700003600,"period":300}}]} 申请追加只读数据。
可申请的工具仅限：
- metrics.query：params 支持 account_id / region / project_id / namespace / metric / dimensions / from / to / period / aggregator；缺省字段沿用告警推导；时间窗口最多 7 天，period=86400 的日聚合最多 30 天，最多 1440 点。
- alert.rules.list：params 支持 account_id / region / namespace（必填，可回退告警标签）/ keyword / limit（最多 50）。
- alarm.analyze：无参数，重新读取当前告警快照。
若用户要求体检告警规则（例如判断阈值是否合理），优先申请 alert.rules.list 获取规则阈值，再按需要申请 metrics.query 获取趋势与破限点进行对照。
最多 3 轮，不得申请执行类工具。`

// ChatObservation 是一次只读工具取数的结果，作为模型输入与工具审计依据。
// Data 只包含脱敏后的上下文摘要，绝不包含凭据。
type ChatObservation struct {
	ToolCode   string         `json:"tool_code"`
	Allowed    bool           `json:"allowed"`
	Mode       string         `json:"mode,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Status     string         `json:"status"`
	Summary    string         `json:"summary,omitempty"`
	EvidenceID string         `json:"evidence_id,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

// ChatHistoryMessage 是送入模型的历史消息摘要。
type ChatHistoryMessage struct {
	Role    string
	Content string
}

// ChatPromptInput 组装对话补全的 user prompt。
type ChatPromptInput struct {
	Question     string
	Context      domain.SessionContext
	TimeRange    string
	Alert        map[string]any
	Observations []ChatObservation
	History      []ChatHistoryMessage
	// Memory 是命中的历史案例（阶段 1 RAG-lite），为空时不写入 prompt。
	Memory []chatMemoryCase
}

// ChatAction 是回答中可执行的动作（由服务端按权限构建，不信任模型输出）。
type ChatAction struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
	Target  string `json:"target,omitempty"`
}

// ChatAnswer 结构化回答（需求 §4.2.3）。
type ChatAnswer struct {
	Summary             string       `json:"summary"`
	Evidence            []string     `json:"evidence"`
	Impact              []string     `json:"impact"`
	Recommendations     []string     `json:"recommendations"`
	RiskLevel           string       `json:"risk_level"`
	RequireConfirm      bool         `json:"require_confirm"`
	Actions             []ChatAction `json:"actions"`
	Rollback            string       `json:"rollback"`
	Uncertainties       []string     `json:"uncertainties"`
	InsufficientContext bool         `json:"insufficient_context"`
	Degraded            bool         `json:"degraded,omitempty"`
	DegradedReason      string       `json:"degraded_reason,omitempty"`
}

// buildChatUserPrompt 组装模型 user 消息：问题 + 上下文 + 只读观测 + 历史。
func buildChatUserPrompt(in ChatPromptInput) string {
	history := make([]map[string]string, 0, len(in.History))
	for _, item := range in.History {
		content := truncateRunes(strings.TrimSpace(item.Content), chatHistoryContentRunes)
		if content == "" {
			continue
		}
		history = append(history, map[string]string{"role": item.Role, "content": content})
	}
	body := map[string]any{
		"question":     strings.TrimSpace(in.Question),
		"context":      in.Context,
		"time_range":   strings.TrimSpace(in.TimeRange),
		"alert":        in.Alert,
		"observations": in.Observations,
		"history":      history,
		"task":         "基于上下文与只读观测回答用户问题，并按固定 JSON 结构输出。",
	}
	if len(in.Memory) > 0 {
		body["memory_cases"] = in.Memory
	}
	raw, err := json.Marshal(body)
	if err != nil {
		// 兜底：至少保留问题，避免模型收到空 prompt。
		return fmt.Sprintf(`{"question":%q}`, strings.TrimSpace(in.Question))
	}
	return string(raw)
}

// chatAnswerPayload 是模型输出的 JSON 结构；actions 由服务端生成，不接受模型覆盖。
// degraded / degraded_reason 仅在平台降级落库时写入，重放历史回答时按原值还原。
type chatAnswerPayload struct {
	Summary             string   `json:"summary"`
	Evidence            []string `json:"evidence"`
	Impact              []string `json:"impact"`
	Recommendations     []string `json:"recommendations"`
	RiskLevel           string   `json:"risk_level"`
	RequireConfirm      bool     `json:"require_confirm"`
	Rollback            flexText `json:"rollback"`
	Uncertainties       []string `json:"uncertainties"`
	InsufficientContext bool     `json:"insufficient_context"`
	Degraded            bool     `json:"degraded"`
	DegradedReason      string   `json:"degraded_reason"`
}

// flexText 兼容模型把文本字段输出为字符串或字符串数组。
type flexText string

func (t *flexText) UnmarshalJSON(raw []byte) error {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		*t = flexText(strings.TrimSpace(single))
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		*t = flexText(strings.Join(sanitizeList(list), "；"))
		return nil
	}
	// 其他类型不阻断解析，按空文本处理。
	*t = ""
	return nil
}

// parseChatAnswer 解析并校验模型输出，失败表示需要降级。
func parseChatAnswer(content string) (ChatAnswer, error) {
	var payload chatAnswerPayload
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &payload); err != nil {
		return ChatAnswer{}, fmt.Errorf("decode chat answer json: %w", err)
	}
	summary := strings.TrimSpace(payload.Summary)
	if summary == "" {
		return ChatAnswer{}, errors.New("chat answer summary is required")
	}
	return ChatAnswer{
		Summary:             truncateRunes(summary, 2000),
		Evidence:            sanitizeList(payload.Evidence),
		Impact:              sanitizeList(payload.Impact),
		Recommendations:     sanitizeList(payload.Recommendations),
		RiskLevel:           normalizeChatRisk(payload.RiskLevel),
		RequireConfirm:      payload.RequireConfirm,
		Actions:             []ChatAction{},
		Rollback:            truncateRunes(string(payload.Rollback), 1000),
		Uncertainties:       sanitizeList(payload.Uncertainties),
		InsufficientContext: payload.InsufficientContext,
		Degraded:            payload.Degraded,
		DegradedReason:      strings.TrimSpace(payload.DegradedReason),
	}, nil
}

// chatResultStatus 由结构化回答还原对外 status（重放历史回答时使用）。
func chatResultStatus(answer ChatAnswer) string {
	if answer.Degraded {
		return chatStatusDegraded
	}
	if answer.InsufficientContext {
		return chatStatusInsufficientContext
	}
	return chatStatusOK
}

// degradedChatAnswer 是模型不可用/输出非法时的保守降级结果。
func degradedChatAnswer(reason string, evidence []string) ChatAnswer {
	uncertainties := []string{"模型或编排服务不可用时平台返回保守降级结果，未生成新的分析结论"}
	switch reason {
	case chatDegradedModelUnavailable:
		uncertainties = append(uncertainties, "尚未配置可用的平台直连模型（类型 b），请联系管理员在系统管理 / AI 配置中完成配置")
	case chatDegradedModelFailed:
		uncertainties = append(uncertainties, "模型调用超时或失败，请稍后重试；不要据此直接执行变更")
	case chatDegradedOutputInvalid:
		uncertainties = append(uncertainties, "模型输出无法通过平台结构校验，已丢弃并降级")
	case chatDegradedLoopExhausted:
		uncertainties = append(uncertainties, "模型在追加只读取数的预算内仍未给出结论，已按已获取上下文保守回答")
	case chatDegradedTurnBudgetExceeded:
		uncertainties = append(uncertainties, "本轮对话已超过总时间预算，已停止继续取数与模型调用；请缩小问题范围或稍后重试")
	}
	if len(evidence) == 0 {
		evidence = []string{"暂无可用只读观测数据，请结合告警详情与监控页面人工判断"}
	}
	return ChatAnswer{
		Summary:         "AI 助手暂未生成有效分析结论，已展示可用的只读上下文，请结合告警与监控人工判断。",
		Evidence:        evidence,
		Impact:          []string{},
		Recommendations: []string{"稍后重试；涉及变更时通过自动化执行创建待确认任务，由人工确认后执行。"},
		RiskLevel:       "info",
		RequireConfirm:  false,
		Actions:         []ChatAction{},
		Rollback:        "无变更动作",
		Uncertainties:   uncertainties,
		Degraded:        true,
		DegradedReason:  reason,
	}
}

func normalizeChatRisk(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "low", "medium", "high", "info":
		return strings.ToLower(strings.TrimSpace(raw))
	case "critical":
		// 平台执行侧的高风险统一按 high 呈现。
		return "high"
	default:
		return "info"
	}
}

func chatRiskRank(level string) int {
	switch level {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	default:
		return 0
	}
}

// sanitizeList 去除空白项、去重并限制条数与单条长度。
func sanitizeList(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = truncateRunes(strings.TrimSpace(item), chatMaxListItemRunes)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
		if len(out) >= chatMaxListItems {
			break
		}
	}
	return out
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// extractJSONObject 从可能包含 markdown 代码块的模型输出中提取 JSON 对象。
func extractJSONObject(content string) string {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return content[start : end+1]
	}
	return content
}
