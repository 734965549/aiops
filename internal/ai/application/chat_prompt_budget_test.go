package application

import (
	"context"
	"strings"
	"testing"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/internal/ai/toolgateway"
)

func TestEstimateChatPromptTokens(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"abc", 1},
		{"abcd", 2},
		{"运维", 2},
	}
	for _, tc := range cases {
		if got := estimateChatPromptTokens(tc.text); got != tc.want {
			t.Fatalf("estimate(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestBuildChatUserPromptWithBudgetKeepsFullWhenFits(t *testing.T) {
	in := ChatPromptInput{
		Question: "最近一小时 CPU 为什么高？",
		Context:  domain.SessionContext{Page: "alerts", AlertID: "a1"},
		Now:      1700000000,
	}
	want := buildChatUserPrompt(in)
	prompt, trim, ok := buildChatUserPromptWithBudget(in, 100000, 0)
	if !ok || prompt != want {
		t.Fatalf("prompt changed on oversized budget: ok=%v", ok)
	}
	if trim.trimmed() || trim.EstimatedTokens > trim.Budget {
		t.Fatalf("unexpected trim = %+v", trim)
	}
}

func TestBuildChatUserPromptWithBudgetDropsOldestHistoryFirst(t *testing.T) {
	in := ChatPromptInput{
		Question: "继续分析",
		Now:      1700000000,
		History: []ChatHistoryMessage{
			{Role: "user", Content: strings.Repeat("旧", 1000)},
			{Role: "user", Content: "上一条问题"},
			{Role: "assistant", Content: "最近结论"},
		},
	}
	withoutOldest := in
	withoutOldest.History = in.History[1:]
	budget := estimateChatPromptTokens(buildChatUserPrompt(withoutOldest))

	prompt, trim, ok := buildChatUserPromptWithBudget(in, budget, 0)
	if !ok {
		t.Fatalf("expected prompt to fit after dropping oldest history, trim=%+v", trim)
	}
	if trim.DroppedHistory != 1 || !strings.Contains(prompt, "最近结论") {
		t.Fatalf("unexpected trim = %+v, prompt=%s", trim, prompt)
	}
	if strings.Contains(prompt, strings.Repeat("旧", 1000)) {
		t.Fatal("oldest history must be dropped first")
	}
}

func TestBuildChatUserPromptWithBudgetStripsMetricPoints(t *testing.T) {
	points := make([][2]float64, 288)
	for i := range points {
		points[i] = [2]float64{float64(1700000000 + i*60), 90 + float64(i%5)}
	}
	in := ChatPromptInput{
		Question: "看指标",
		Now:      1700000000,
		Observations: []ChatObservation{{
			ToolCode: chatToolMetricsQuery, Status: "ok", EvidenceID: "ev-1",
			Data: map[string]any{
				"metric":  "cpu_util",
				"metrics": []MetricEvidence{{Metric: "cpu_util", Points: points, EvidenceID: "ev-1"}},
			},
		}},
	}
	stripped := in
	stripped.Observations = copyChatObservations(in.Observations)
	if !stripChatObservationPoints(stripped.Observations) {
		t.Fatal("expected points to be stripped")
	}
	budget := estimateChatPromptTokens(buildChatUserPrompt(stripped))

	prompt, trim, ok := buildChatUserPromptWithBudget(in, budget, 0)
	if !ok || !trim.StrippedPoints {
		t.Fatalf("expected points trimmed, trim=%+v ok=%v", trim, ok)
	}
	if trim.DroppedHistory != 0 || trim.DroppedObservations != 0 {
		t.Fatalf("points should be stripped before dropping history/observations: %+v", trim)
	}
	if strings.Contains(prompt, `"points":[[`) {
		t.Fatal("metric points must not enter the trimmed prompt")
	}
	if !strings.Contains(prompt, `"points_stripped":true`) {
		t.Fatal("trimmed prompt must mark points_stripped")
	}
}

func TestBuildChatUserPromptWithBudgetDropsLowPriorityObservations(t *testing.T) {
	in := ChatPromptInput{
		Question: "看指标",
		Now:      1700000000,
		Observations: []ChatObservation{
			{ToolCode: chatToolAccountsList, Status: "ok", Summary: "accounts", Data: map[string]any{
				"accounts": strings.Repeat("a", 1200),
			}},
			{ToolCode: chatToolMetricsQuery, Status: "ok", Summary: "metrics", Data: map[string]any{
				"metric": "cpu_util",
			}},
		},
	}
	onlyHighPriority := in
	onlyHighPriority.Observations = in.Observations[1:]
	budget := estimateChatPromptTokens(buildChatUserPrompt(onlyHighPriority))

	prompt, trim, ok := buildChatUserPromptWithBudget(in, budget, 0)
	if !ok || trim.DroppedObservations != 1 {
		t.Fatalf("expected one observation dropped, trim=%+v ok=%v", trim, ok)
	}
	if strings.Contains(prompt, "accounts.list") {
		t.Fatal("accounts.list must be dropped before metrics.query")
	}
	if !strings.Contains(prompt, "metrics.query") {
		t.Fatal("metrics.query should be kept")
	}
}

func TestBuildChatUserPromptWithBudgetDropsMemory(t *testing.T) {
	in := ChatPromptInput{
		Question: "分析告警",
		Now:      1700000000,
		Memory: []chatMemoryCase{{
			CaseID: "case-1", Summary: strings.Repeat("经验", 300),
		}},
	}
	withoutMemory := in
	withoutMemory.Memory = nil
	budget := estimateChatPromptTokens(buildChatUserPrompt(withoutMemory))

	prompt, trim, ok := buildChatUserPromptWithBudget(in, budget, 0)
	if !ok || trim.DroppedMemory != 1 {
		t.Fatalf("expected memory dropped, trim=%+v ok=%v", trim, ok)
	}
	if strings.Contains(prompt, "memory_cases") {
		t.Fatal("memory_cases must not enter the trimmed prompt")
	}
}

func TestBuildChatUserPromptWithBudgetFailsWhenMinimumExceeds(t *testing.T) {
	in := ChatPromptInput{Question: "分析告警", Now: 1700000000}
	_, trim, ok := buildChatUserPromptWithBudget(in, 100, 500)
	if ok {
		t.Fatalf("expected budget failure, trim=%+v", trim)
	}
	if trim.EstimatedTokens <= trim.Budget {
		t.Fatalf("estimated tokens should exceed budget: %+v", trim)
	}
}

func TestLowestPriorityObservationIndex(t *testing.T) {
	observations := []ChatObservation{
		{ToolCode: chatToolMetricsQuery},
		{ToolCode: chatToolAccountsList},
		{ToolCode: "unknown.tool"},
	}
	if got := lowestPriorityObservationIndex(observations); got != 2 {
		t.Fatalf("unknown tool must be dropped first, got index %d", got)
	}
	observations = observations[1:]
	if got := lowestPriorityObservationIndex(observations); got != 1 {
		t.Fatalf("accounts.list must be dropped before metrics.query, got index %d", got)
	}
}

func TestEffectiveOutputTokens(t *testing.T) {
	cases := []struct {
		window, configured, fallback, want int
	}{
		{0, 0, 0, 0},
		{0, 1234, 2048, 1234},
		{0, 0, 2048, 2048},
		{8000, 0, 4096, 4000},
		{8000, 6000, 0, 4000},
		{8000, 1000, 4096, 1000},
		{1, 0, 4096, 1},
	}
	for _, tc := range cases {
		if got := toolgateway.EffectiveOutputTokens(tc.window, tc.configured, tc.fallback); got != tc.want {
			t.Fatalf("EffectiveOutputTokens(%d,%d,%d) = %d, want %d",
				tc.window, tc.configured, tc.fallback, got, tc.want)
		}
	}
}

func TestPromptBudgetFor(t *testing.T) {
	catalog := &fakeChatCatalog{fakeChatClient: &fakeChatClient{}, models: []toolgateway.ChatModel{
		{ProviderID: "p-global"},
		{ProviderID: "p-small", ContextWindowTokens: 8000},
		{ProviderID: "p-small-custom", ContextWindowTokens: 8000, MaxOutputTokens: 2000},
		{ProviderID: "p-large", ContextWindowTokens: 1000000},
	}}
	svc := &ChatService{chat: catalog, promptTokenBudget: 50000, maxOutputTokens: 3000}
	cases := []struct {
		providerID            string
		wantInput, wantOutput int
	}{
		{"p-global", 50000, 3000},
		// 窗口 8000：输出 3000，输入 5000。
		{"p-small", 5000, 3000},
		{"p-small-custom", 6000, 2000},
		{"p-large", 1000000 - 3000, 3000},
		{"missing", 50000, 3000},
	}
	for _, tc := range cases {
		input, output := svc.promptBudgetFor(tc.providerID)
		if input != tc.wantInput || output != tc.wantOutput {
			t.Fatalf("budget(%q) = (%d,%d), want (%d,%d)",
				tc.providerID, input, output, tc.wantInput, tc.wantOutput)
		}
	}
	// 全局默认：输入 32768，输出 4096；窗口足够大时输出不被截断。
	defaultSvc := &ChatService{chat: catalog}
	if input, output := defaultSvc.promptBudgetFor("p-global"); input != chatDefaultPromptTokenBudget || output != chatDefaultMaxOutputTokens {
		t.Fatalf("default budget = (%d,%d)", input, output)
	}
	if input, output := defaultSvc.promptBudgetFor("p-large"); input != 1000000-chatDefaultMaxOutputTokens || output != chatDefaultMaxOutputTokens {
		t.Fatalf("provider window must override global input budget, got (%d,%d)", input, output)
	}
	// 小窗口时输出上限被窗口一半截断，输入至少保留一半。
	tiny := &ChatService{chat: &fakeChatCatalog{fakeChatClient: &fakeChatClient{}, models: []toolgateway.ChatModel{
		{ProviderID: "p-tiny", ContextWindowTokens: 4000},
	}}}
	if input, output := tiny.promptBudgetFor("p-tiny"); input != 2000 || output != 2000 {
		t.Fatalf("tiny window budget = (%d,%d)", input, output)
	}
	noCatalog := &ChatService{chat: &fakeChatClient{}, promptTokenBudget: 50000, maxOutputTokens: 3000}
	if input, output := noCatalog.promptBudgetFor("p-any"); input != 50000 || output != 3000 {
		t.Fatalf("budget without catalog = (%d,%d)", input, output)
	}
}

func TestSendMessagePromptBudgetExceededSkipsModel(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	svc.SetPromptTokenBudget(1)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if chat.calls != 0 {
		t.Fatalf("model must not be called when prompt exceeds budget, calls=%d", chat.calls)
	}
	if result.Status != chatStatusDegraded || result.Answer.DegradedReason != chatDegradedPromptBudgetExceeded {
		t.Fatalf("unexpected result: %+v", result)
	}
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil ||
		chatAudit.Payload["prompt_token_budget"] != 1 ||
		chatAudit.Payload["prompt_output_limit"] != chatDefaultMaxOutputTokens {
		t.Fatalf("audit must record prompt input/output budget, got %+v", chatAudit)
	}
}
