// ai-mock-provider 是 AI E2E 用的脚本化 OpenAI 兼容模型服务。
//
// 它只监听 loopback，按用户问题中的标记返回固定结构化回答或工具申请：
//   - MOCK_TOOL_LOOP：首轮申请 alarm.analyze + metrics.query，后续轮返回正常回答；
//   - MOCK_TOOL_DENY：首轮申请 execution.tasks.propose（服务端应拒绝），后续轮返回回答；
//   - MOCK_TOOL_INFINITE：先申请 alarm.analyze、之后每轮申请不同时间窗口的 metrics.query，
//     用于验证 Agent Loop 轮数预算耗尽降级（相同参数会被跨轮去重，不能验证预算）；
//   - MOCK_TOOL_RULES：从标记后的 JSON 读取账号/命名空间/指标参数，首轮同时申请
//     alert.rules.list 与 metrics.query，用于规则体检 E2E；
//   - MOCK_TOOL_DISCOVERY：从标记后的 JSON 读取账号，首轮申请 accounts.list 与
//     resources.list，用于上下文发现 E2E；
//   - MOCK_STREAM_429：首次返回 429（Retry-After: 1），用于验证退避重试；
//   - MOCK_STREAM_429_ALWAYS：每次返回 429，响应体含 quota resets in 2h；
//   - MOCK_STREAM_PARTIAL：输出一段内容后断开连接，用于验证部分保留；
//   - MOCK_STREAM_SLOW：慢速输出（约 45s），用于验证流式不再使用 30s 总超时；
//   - [SLOW]：延迟响应，用于验证流式取消。
//
// 该工具仅供 scripts/e2e-ai-*.ps1 在 dev 环境使用，不参与生产构建。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const answerContent = `{"summary":"E2E 模拟回答：已基于当前只读观测完成分析。","evidence":["mock-evidence"],"impact":["影响范围以实际告警为准"],"recommendations":["核对近期变更","观察关键指标 10 分钟"],"risk_level":"low","require_confirm":false,"actions":[],"rollback":"无变更动作","uncertainties":[],"insufficient_context":false}`

const loopToolRequests = `{"tool_requests":[{"tool_code":"alarm.analyze","reason":"需要重新核对告警快照"},{"tool_code":"metrics.query","reason":"需要补充近期指标"}]}`

const denyToolRequests = `{"tool_requests":[{"tool_code":"execution.tasks.propose","reason":"尝试申请执行类工具"}]}`

const infiniteAlarmRequest = `{"tool_requests":[{"tool_code":"alarm.analyze","reason":"持续申请追加观测"}]}`

// infiniteMetricRequestsTemplate 每轮换时间窗口，避免命中服务端跨轮去重后无法验证轮数预算。
const infiniteMetricRequestsTemplate = `{"tool_requests":[{"tool_code":"metrics.query","reason":"持续申请追加观测","params":{"from":%d,"to":%d}}]}`

const ruleToolRequestsTemplate = `{"tool_requests":[{"tool_code":"alert.rules.list","reason":"规则体检","params":{"account_id":%q,"region":%q,"namespace":%q,"limit":10}},{"tool_code":"metrics.query","reason":"趋势与破限点核对","params":{"account_id":%q,"region":%q,"namespace":%q,"metric":%q,"dimensions":%s,"from":%d,"to":%d,"period":%d}}]}`

const ruleInspectionAnswer = `{"summary":"E2E 规则体检：已读取告警规则与指标证据。","evidence":["rule-fake","observability evidence"],"impact":["规则阈值需要结合趋势复核"],"recommendations":["核对阈值与破限点","观察 10 分钟"],"risk_level":"low","require_confirm":false,"actions":[],"rollback":"无变更动作","uncertainties":[],"insufficient_context":false}`

const discoveryToolRequestsTemplate = `{"tool_requests":[{"tool_code":"accounts.list","reason":"确认可用账号"},{"tool_code":"resources.list","reason":"发现云同步资源","params":{"account_id":%q,"resource_type":"rds","limit":5}}]}`

type discoveryScenarioParams struct {
	AccountID string `json:"account_id"`
}

type ruleScenarioParams struct {
	AccountID  string            `json:"account_id"`
	Region     string            `json:"region"`
	Namespace  string            `json:"namespace"`
	Metric     string            `json:"metric"`
	Dimensions map[string]string `json:"dimensions"`
	From       int64             `json:"from"`
	To         int64             `json:"to"`
	Period     int               `json:"period"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
}

type completionChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type completionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type completionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Choices []completionChoice `json:"choices"`
	Usage   completionUsage    `json:"usage"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18081", "listen address")
	slow := flag.Duration("slow", 5*time.Second, "delay for prompts containing [SLOW]")
	flag.Parse()

	state := &scenarioState{seen: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req chatRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		prompt := joinPrompt(req.Messages)
		// 限流/断流场景在模型调用前直接处理，便于稳定触发。
		if strings.Contains(prompt, "MOCK_STREAM_429_ALWAYS") {
			writeRateLimited(w, "quota resets in 2h", "1")
			return
		}
		if strings.Contains(prompt, "MOCK_STREAM_429") && state.markOnce("stream_429") {
			writeRateLimited(w, "quota resets in 1s", "1")
			return
		}
		if strings.Contains(prompt, "MOCK_STREAM_PARTIAL") && state.markOnce("stream_partial") {
			serveInterruptedStream(w)
			return
		}
		if strings.Contains(prompt, "[SLOW]") {
			select {
			case <-time.After(*slow):
			case <-r.Context().Done():
				return
			}
		}
		content, scenario := state.nextContent(req.Messages)
		log.Printf("completion served: scenario=%s stream=%t prompt_runes=%d", scenario, req.Stream, len([]rune(prompt)))

		if req.Stream {
			if strings.Contains(prompt, "MOCK_STREAM_SLOW") {
				serveStreamWithDelay(w, r, content, 3*time.Second)
				return
			}
			serveStream(w, r, content)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(completionResponse{
			ID:     "chatcmpl-aiops-e2e",
			Object: "chat.completion",
			Choices: []completionChoice{{
				Index:        0,
				Message:      chatMessage{Role: "assistant", Content: content},
				FinishReason: "stop",
			}},
			Usage: completionUsage{PromptTokens: 100, CompletionTokens: 100, TotalTokens: 200},
		})
	})

	log.Printf("ai mock provider listening on http://%s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func writeRateLimited(w http.ResponseWriter, message, retryAfter string) {
	w.Header().Set("Content-Type", "application/json")
	if retryAfter != "" {
		w.Header().Set("Retry-After", retryAfter)
	}
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": message}})
}

// serveInterruptedStream 输出一段内容后直接关闭连接，模拟生成中断。
func serveInterruptedStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	flusher, _ := w.(http.Flusher)
	_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"{\"summary\":\"部分"}}]}`+"\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		panic(http.ErrAbortHandler)
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		panic(http.ErrAbortHandler)
	}
	_ = conn.Close()
}

func serveStream(w http.ResponseWriter, r *http.Request, content string) {
	serveStreamWithDelay(w, r, content, 0)
}

func serveStreamWithDelay(w http.ResponseWriter, r *http.Request, content string, delay time.Duration) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	send := func(payload any) bool {
		raw, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}
	chunk := func(delta map[string]string) bool {
		return send(map[string]any{
			"id": "chatcmpl-aiops-e2e", "object": "chat.completion.chunk",
			"choices": []any{map[string]any{"index": 0, "delta": delta}},
		})
	}

	if !chunk(map[string]string{"reasoning_content": "正在核对上下文与只读观测。"}) {
		return
	}
	runes := []rune(content)
	const contentChunkRunes = 24
	for start := 0; start < len(runes); start += contentChunkRunes {
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		if r.Context().Err() != nil {
			return
		}
		end := start + contentChunkRunes
		if end > len(runes) {
			end = len(runes)
		}
		if !chunk(map[string]string{"content": string(runes[start:end])}) {
			return
		}
	}
	if !send(map[string]any{
		"id": "chatcmpl-aiops-e2e", "object": "chat.completion.chunk",
		"choices": []any{map[string]any{"index": 0, "delta": map[string]string{}, "finish_reason": "stop"}},
		"usage":   map[string]int{"prompt_tokens": 100, "completion_tokens": 100, "total_tokens": 200},
	}) {
		return
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

type scenarioState struct {
	mu   sync.Mutex
	seen map[string]int
}

// markOnce 返回 key 是否首次命中，用于限流/断流场景只触发一次。
func (s *scenarioState) markOnce(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[key]++
	return s.seen[key] == 1
}

func (s *scenarioState) nextContent(messages []chatMessage) (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 服务端 user prompt 是 JSON 包装体（question/context/observations），
	// 标记与参数在 question 原文里；从解码后的原文提取，避免 JSON 转义
	// （{\"account_id\":...}）破坏参数解析。注意 joinPrompt 会把 system prompt
	// 与 user prompt 拼在一起，整体不再是合法 JSON，必须逐条消息解析。
	prompt := joinPrompt(messages)
	target := prompt
	for _, message := range messages {
		var body struct {
			Question string `json:"question"`
		}
		if err := json.Unmarshal([]byte(message.Content), &body); err == nil && strings.TrimSpace(body.Question) != "" {
			target = body.Question
			break
		}
	}
	if strings.Contains(target, "MOCK_TOOL_INFINITE") {
		s.seen["infinite"]++
		if s.seen["infinite"] == 1 {
			return infiniteAlarmRequest, "infinite"
		}
		from := int64(1700000000) + int64(s.seen["infinite"]-2)*3600
		return fmt.Sprintf(infiniteMetricRequestsTemplate, from, from+3600), "infinite"
	}
	if idx := strings.Index(target, "MOCK_TOOL_DISCOVERY"); idx >= 0 {
		s.seen["discovery"]++
		if s.seen["discovery"] == 1 {
			var params discoveryScenarioParams
			decoder := json.NewDecoder(strings.NewReader(target[idx+len("MOCK_TOOL_DISCOVERY"):]))
			// 允许标记后不带参数（环境尚无账号）：EOF 按空参数处理。
			if err := decoder.Decode(&params); err != nil && err != io.EOF {
				log.Printf("discovery scenario params invalid: %v", err)
				return answerContent, "discovery_params_invalid"
			}
			return fmt.Sprintf(discoveryToolRequestsTemplate, params.AccountID), "discovery"
		}
		return answerContent, "answer_after_discovery"
	}
	if idx := strings.Index(target, "MOCK_TOOL_RULES"); idx >= 0 {
		s.seen["rules"]++
		if s.seen["rules"] == 1 {
			var params ruleScenarioParams
			decoder := json.NewDecoder(strings.NewReader(target[idx+len("MOCK_TOOL_RULES"):]))
			if err := decoder.Decode(&params); err != nil {
				log.Printf("rules scenario params invalid: %v", err)
				return answerContent, "rules_params_invalid"
			}
			if params.Period <= 0 {
				params.Period = 86400
			}
			dimensions, err := json.Marshal(params.Dimensions)
			if err != nil || len(params.Dimensions) == 0 {
				dimensions = []byte("{}")
			}
			return fmt.Sprintf(ruleToolRequestsTemplate,
				params.AccountID, params.Region, params.Namespace,
				params.AccountID, params.Region, params.Namespace,
				params.Metric, string(dimensions), params.From, params.To, params.Period,
			), "rules"
		}
		// 观测在包装体的 observations 字段里，检查要看完整 prompt。
		if strings.Contains(prompt, "rule-fake") && strings.Contains(prompt, "evidence_ids") {
			return ruleInspectionAnswer, "answer_after_rules"
		}
		return answerContent, "answer_after_rules"
	}
	if strings.Contains(target, "MOCK_TOOL_DENY") {
		s.seen["deny"]++
		if s.seen["deny"] == 1 {
			return denyToolRequests, "deny"
		}
		return answerContent, "answer_after_deny"
	}
	if strings.Contains(target, "MOCK_TOOL_LOOP") {
		s.seen["loop"]++
		if s.seen["loop"] == 1 {
			return loopToolRequests, "loop"
		}
		return answerContent, "answer_after_loop"
	}
	return answerContent, "answer"
}

func joinPrompt(messages []chatMessage) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Content)
	}
	return strings.Join(parts, "\n")
}
