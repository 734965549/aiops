// ai-mock-provider 是 AI E2E 用的脚本化 OpenAI 兼容模型服务。
//
// 它只监听 loopback，按用户问题中的标记返回固定结构化回答或工具申请：
//   - MOCK_TOOL_LOOP：首轮申请 alarm.analyze + metrics.query，后续轮返回正常回答；
//   - MOCK_TOOL_DENY：首轮申请 execution.tasks.propose（服务端应拒绝），后续轮返回回答；
//   - MOCK_TOOL_INFINITE：每轮都申请 alarm.analyze，用于验证 Agent Loop 预算耗尽降级；
//   - MOCK_TOOL_RULES：从标记后的 JSON 读取账号/命名空间/指标参数，首轮同时申请
//     alert.rules.list 与 metrics.query，用于规则体检 E2E；
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

const infiniteToolRequests = `{"tool_requests":[{"tool_code":"alarm.analyze","reason":"持续申请追加观测"}]}`

const ruleToolRequestsTemplate = `{"tool_requests":[{"tool_code":"alert.rules.list","reason":"规则体检","params":{"account_id":%q,"region":%q,"namespace":%q,"limit":10}},{"tool_code":"metrics.query","reason":"趋势与破限点核对","params":{"account_id":%q,"region":%q,"namespace":%q,"metric":%q,"dimensions":%s,"from":%d,"to":%d,"period":%d}}]}`

const ruleInspectionAnswer = `{"summary":"E2E 规则体检：已读取告警规则与指标证据。","evidence":["rule-fake","observability evidence"],"impact":["规则阈值需要结合趋势复核"],"recommendations":["核对阈值与破限点","观察 10 分钟"],"risk_level":"low","require_confirm":false,"actions":[],"rollback":"无变更动作","uncertainties":[],"insufficient_context":false}`

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
		if strings.Contains(prompt, "[SLOW]") {
			select {
			case <-time.After(*slow):
			case <-r.Context().Done():
				return
			}
		}
		content, scenario := state.nextContent(prompt)
		log.Printf("completion served: scenario=%s prompt_runes=%d", scenario, len([]rune(prompt)))

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

type scenarioState struct {
	mu   sync.Mutex
	seen map[string]int
}

func (s *scenarioState) nextContent(prompt string) (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.Contains(prompt, "MOCK_TOOL_INFINITE") {
		return infiniteToolRequests, "infinite"
	}
	if idx := strings.Index(prompt, "MOCK_TOOL_RULES"); idx >= 0 {
		s.seen["rules"]++
		if s.seen["rules"] == 1 {
			var params ruleScenarioParams
			decoder := json.NewDecoder(strings.NewReader(prompt[idx+len("MOCK_TOOL_RULES"):]))
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
		if strings.Contains(prompt, "rule-fake") && strings.Contains(prompt, "evidence_ids") {
			return ruleInspectionAnswer, "answer_after_rules"
		}
		return answerContent, "answer_after_rules"
	}
	if strings.Contains(prompt, "MOCK_TOOL_DENY") {
		s.seen["deny"]++
		if s.seen["deny"] == 1 {
			return denyToolRequests, "deny"
		}
		return answerContent, "answer_after_deny"
	}
	if strings.Contains(prompt, "MOCK_TOOL_LOOP") {
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
