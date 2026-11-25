package application

import (
	"strings"
	"testing"

	"github.com/734965549/aiops/internal/ai/domain"
)

func TestParseChatAnswerValid(t *testing.T) {
	content := "```json\n" + `{
		"summary": "CPU 持续升高",
		"evidence": ["告警 cpu_util > 90", "observability evidence ev-1", "  "],
		"impact": ["payment-service 3 个实例"],
		"recommendations": ["建议生成执行计划，由人工在执行确认页确认"],
		"risk_level": "CRITICAL",
		"require_confirm": true,
		"rollback": ["缩容回原实例数"],
		"uncertainties": ["缺少最近变更记录"],
		"insufficient_context": false
	}` + "\n```"
	answer, err := parseChatAnswer(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if answer.Summary != "CPU 持续升高" {
		t.Fatalf("summary = %q", answer.Summary)
	}
	if len(answer.Evidence) != 2 || answer.Evidence[1] != "observability evidence ev-1" {
		t.Fatalf("evidence = %#v", answer.Evidence)
	}
	if answer.RiskLevel != "high" {
		t.Fatalf("risk = %q, want high", answer.RiskLevel)
	}
	if !answer.RequireConfirm || answer.Rollback != "缩容回原实例数" {
		t.Fatalf("unexpected answer: %+v", answer)
	}
	if answer.Actions == nil || answer.InsufficientContext {
		t.Fatalf("expected empty actions and no insufficient flag: %+v", answer)
	}
}

func TestParseChatAnswerToleratesObjectLists(t *testing.T) {
	content := `{
		"summary": "阈值体检",
		"recommendations": [{"rule": "磁盘使用率", "current": 88, "suggested": 80}],
		"evidence": ["磁盘 31 点恒定 11.29%", {"metric": "mem_util", "peak": 60.3}],
		"risk_level": "info"
	}`
	answer, err := parseChatAnswer(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(answer.Recommendations) != 1 || !strings.Contains(answer.Recommendations[0], "磁盘使用率") {
		t.Fatalf("recommendations = %#v", answer.Recommendations)
	}
	if len(answer.Evidence) != 2 || !strings.Contains(answer.Evidence[1], "mem_util") {
		t.Fatalf("evidence = %#v", answer.Evidence)
	}
}

func TestParseChatAnswerRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"not json":        "抱歉，我无法回答",
		"missing summary": `{"summary":"","risk_level":"low"}`,
		"bad json":        `{"summary":`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseChatAnswer(content); err == nil {
				t.Fatalf("expected parse error for %q", content)
			}
		})
	}
}

func TestParseChatAnswerLimitsAndNormalizes(t *testing.T) {
	long := strings.Repeat("x", chatMaxListItemRunes+50)
	items := make([]string, 0, chatMaxListItems+5)
	for i := 0; i < chatMaxListItems+5; i++ {
		items = append(items, long)
	}
	payload := `{"summary":"s","risk_level":"weird","evidence":[` +
		quoteList(items) + `],"insufficient_context":true}`
	answer, err := parseChatAnswer(payload)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if answer.RiskLevel != "info" {
		t.Fatalf("risk = %q, want info", answer.RiskLevel)
	}
	if len(answer.Evidence) != 1 {
		// 全部条目前 500 字符相同，去重后只剩一条。
		t.Fatalf("expected deduped evidence, got %d", len(answer.Evidence))
	}
	if len([]rune(answer.Evidence[0])) != chatMaxListItemRunes {
		t.Fatalf("expected evidence item truncated, got %d runes", len([]rune(answer.Evidence[0])))
	}
	if !answer.InsufficientContext {
		t.Fatal("expected insufficient_context preserved")
	}
}

func TestBuildChatUserPrompt(t *testing.T) {
	prompt := buildChatUserPrompt(ChatPromptInput{
		Question:  "分析这个告警",
		Context:   domain.SessionContext{Page: "alerts", AlertID: "a1"},
		TimeRange: "30m",
		Alert:     map[string]any{"id": "a1", "name": "HighCPU"},
		Observations: []ChatObservation{{
			ToolCode: chatToolAlarmAnalyze, Allowed: true, Status: "ok",
			Data: map[string]any{"alert_id": "a1"},
		}},
		History: []ChatHistoryMessage{{Role: "user", Content: "之前的问题"}},
	})
	for _, want := range []string{`"question":"分析这个告警"`, `"alert_id":"a1"`, `"HighCPU"`, `"之前的问题"`, `"time_range":"30m"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %s: %s", want, prompt)
		}
	}
	if strings.Contains(prompt, "api_key") || strings.Contains(prompt, "secret") {
		t.Fatalf("prompt must not contain credentials: %s", prompt)
	}
}

func TestBuildChatUserPromptIncludesServerTime(t *testing.T) {
	prompt := buildChatUserPrompt(ChatPromptInput{Question: "分析当前时间附近的指标", Now: 1700005000})
	if !strings.Contains(prompt, `"now":1700005000`) {
		t.Fatalf("prompt missing server time: %s", prompt)
	}
}

func TestChatSystemPromptEnforcesTimeAndReadability(t *testing.T) {
	for _, want := range []string{
		"以 now 为基准",
		"不得从规则 ID",
		"metric_label",
		"未映射",
		"推荐操作必须逐条结构化",
		"根因优先",
		"breach_by_hour",
		"备份窗口",
		"不得把放宽阈值",
		"point_count 为 0",
		"收敛要求",
		"必须立即输出最终 answer",
		"重复申请会收到 denied",
	} {
		if !strings.Contains(chatSystemPrompt, want) {
			t.Fatalf("system prompt missing %q", want)
		}
	}
}

func TestDegradedChatAnswer(t *testing.T) {
	answer := degradedChatAnswer(chatDegradedModelUnavailable, nil)
	if !answer.Degraded || answer.DegradedReason != chatDegradedModelUnavailable {
		t.Fatalf("unexpected degraded answer: %+v", answer)
	}
	if len(answer.Evidence) == 0 || len(answer.Uncertainties) == 0 {
		t.Fatalf("expected evidence and uncertainty fallbacks: %+v", answer)
	}
	if answer.RequireConfirm {
		t.Fatal("degraded answer must not claim a confirmed action")
	}
	withEvidence := degradedChatAnswer(chatDegradedModelFailed, []string{"告警 HighCPU"})
	if len(withEvidence.Evidence) != 1 || withEvidence.Evidence[0] != "告警 HighCPU" {
		t.Fatalf("expected partial context retained: %+v", withEvidence.Evidence)
	}
}

func TestNormalizeChatRisk(t *testing.T) {
	cases := map[string]string{
		"low": "low", "Medium": "medium", "HIGH": "high", "critical": "high", "": "info", "unknown": "info",
	}
	for in, want := range cases {
		if got := normalizeChatRisk(in); got != want {
			t.Fatalf("normalizeChatRisk(%q) = %q, want %q", in, got, want)
		}
	}
	if chatRiskRank("high") != 3 || chatRiskRank("info") != 0 {
		t.Fatal("unexpected risk rank")
	}
}

func quoteList(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, `"`+item+`"`)
	}
	return strings.Join(quoted, ",")
}
