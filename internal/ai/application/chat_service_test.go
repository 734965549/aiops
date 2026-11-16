package application

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// ---- fakes ----

type fakeChatSessionRepo struct {
	sessions  map[string]domain.Session
	messages  map[string][]domain.Message
	appendErr error
}

func newFakeChatSessionRepo() *fakeChatSessionRepo {
	return &fakeChatSessionRepo{sessions: map[string]domain.Session{}, messages: map[string][]domain.Message{}}
}

func (f *fakeChatSessionRepo) Create(_ context.Context, session domain.Session) error {
	f.sessions[session.SessionID] = session
	return nil
}

func (f *fakeChatSessionRepo) Get(_ context.Context, sessionID string) (*domain.Session, error) {
	session, ok := f.sessions[sessionID]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}
	copied := session
	return &copied, nil
}

func (f *fakeChatSessionRepo) ListByUser(_ context.Context, userID string, limit int) ([]domain.Session, error) {
	out := make([]domain.Session, 0, len(f.sessions))
	for _, session := range f.sessions {
		if session.UserID == userID {
			out = append(out, session)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeChatSessionRepo) AppendMessage(_ context.Context, message domain.Message, touch domain.SessionTouch) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	session, ok := f.sessions[message.SessionID]
	if !ok {
		return domain.ErrSessionNotFound
	}
	if touch.Title != "" {
		session.Title = touch.Title
	}
	if touch.Context != nil {
		session.Context = *touch.Context
	}
	if !touch.UpdatedAt.IsZero() {
		session.UpdatedAt = touch.UpdatedAt
	}
	f.sessions[message.SessionID] = session
	f.messages[message.SessionID] = append(f.messages[message.SessionID], message)
	return nil
}

func (f *fakeChatSessionRepo) ListMessages(_ context.Context, sessionID string, limit int) ([]domain.Message, error) {
	rows := f.messages[sessionID]
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return append([]domain.Message{}, rows...), nil
}

func (f *fakeChatSessionRepo) ListRecentMessages(_ context.Context, sessionID string, limit int) ([]domain.Message, error) {
	rows := f.messages[sessionID]
	if limit > 0 && len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	return append([]domain.Message{}, rows...), nil
}

func (f *fakeChatSessionRepo) GetMessage(_ context.Context, sessionID, messageID string) (*domain.Message, error) {
	for _, message := range f.messages[sessionID] {
		if message.MessageID == messageID {
			copied := message
			return &copied, nil
		}
	}
	return nil, domain.ErrMessageNotFound
}

func (f *fakeChatSessionRepo) SetFeedback(_ context.Context, sessionID, messageID string, feedback domain.Feedback, _ time.Time) error {
	rows := f.messages[sessionID]
	for i := range rows {
		if rows[i].MessageID == messageID {
			rows[i].Feedback = feedback
			f.messages[sessionID] = rows
			return nil
		}
	}
	return domain.ErrMessageNotFound
}

type fakeChatClient struct {
	providerID   string
	content      string
	err          error
	system       string
	user         string
	calls        int
	lastProvider string
}

func (f *fakeChatClient) ChatProviderID() (string, bool) {
	if f.providerID == "" {
		return "", false
	}
	return f.providerID, true
}

func (f *fakeChatClient) Complete(_ context.Context, providerID, systemPrompt, userPrompt string) (*toolgateway.ChatCompletionResult, error) {
	f.calls++
	f.lastProvider = providerID
	f.system = systemPrompt
	f.user = userPrompt
	if f.err != nil {
		return nil, f.err
	}
	return &toolgateway.ChatCompletionResult{Content: f.content, Tokens: 42}, nil
}

type fakeChatToolGateway struct {
	allowed  map[string]bool
	reason   string
	requests []toolgateway.ToolRequest
}

func (f *fakeChatToolGateway) Validate(_ context.Context, req toolgateway.ToolRequest) (*toolgateway.ToolResponse, error) {
	f.requests = append(f.requests, req)
	if f.allowed != nil && !f.allowed[req.ToolCode] {
		reason := f.reason
		if reason == "" {
			reason = "tool permission denied"
		}
		return &toolgateway.ToolResponse{Allowed: false, Reason: reason}, nil
	}
	mode := "read_only"
	if req.ToolCode == chatToolExecutionPropose {
		mode = "require_confirm"
	}
	return &toolgateway.ToolResponse{Allowed: true, Mode: mode}, nil
}

func allowAllChatTools() *fakeChatToolGateway {
	return &fakeChatToolGateway{allowed: map[string]bool{
		chatToolAlarmAnalyze: true, chatToolMetricsQuery: true, chatToolExecutionPropose: true,
	}}
}

func chatAnswerJSON(summary string) string {
	return `{"summary":"` + summary + `","evidence":["告警 cpu_util > 90"],` +
		`"impact":["payment-service"],"recommendations":["建议生成执行计划，由人工在执行确认页确认"],` +
		`"risk_level":"medium","require_confirm":true,"rollback":"缩容回原实例数",` +
		`"uncertainties":["缺少最近变更记录"],"insufficient_context":false}`
}

func newChatTestService(t *testing.T) (*ChatService, *fakeChatSessionRepo, *fakeChatToolGateway, *fakeChatClient, *capturingAIAudit, *fakeEvidenceCollector) {
	t.Helper()
	repo := newFakeChatSessionRepo()
	tools := allowAllChatTools()
	chat := &fakeChatClient{providerID: "provider-1", content: chatAnswerJSON("CPU 持续升高")}
	audit := &capturingAIAudit{}
	evidence := &fakeEvidenceCollector{result: &MetricEvidenceResult{
		Status: EvidenceStatusOK,
		Metrics: []MetricEvidence{{
			Metric: "cpu_util", Peak: floatPtr(97.4), EvidenceID: "ev-1",
		}},
	}}
	alerts := &fakeAnalyzeAlertReader{alert: &AlertContext{
		ID: "a1", Name: "HighCPU", Severity: "p1", Status: "firing",
		ApplicationName: "payment-service", ResourceName: "ecs-1",
		Labels:      evidenceTestAlert().Labels,
		Annotations: evidenceTestAlert().Annotations,
		FirstSeenAt: 1700000000, LastSeenAt: 1700000600,
	}}
	svc := NewChatService(repo, alerts, tools, chat, evidence, audit)
	svc.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	return svc, repo, tools, chat, audit, evidence
}

func floatPtr(v float64) *float64 { return &v }

func seedChatSession(t *testing.T, repo *fakeChatSessionRepo, userID string, pageContext domain.SessionContext) domain.Session {
	t.Helper()
	now := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	session := domain.Session{
		SessionID: "s-1", UserID: userID, Title: "生产告警分析", Context: pageContext,
		Status: domain.SessionStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(context.Background(), session); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return session
}

// ---- tests ----

func TestCreateChatSession(t *testing.T) {
	svc, repo, _, _, audit, _ := newChatTestService(t)
	view, err := svc.CreateSession(context.Background(), CreateChatSessionInput{
		UserID: "u-1", Context: domain.SessionContext{Page: "alerts", AlertID: "a1"},
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if view.Title != "新会话" || view.Status != "active" || view.Context.AlertID != "a1" {
		t.Fatalf("unexpected session view: %+v", view)
	}
	if _, ok := repo.sessions[view.SessionID]; !ok {
		t.Fatal("session not persisted")
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditChatSession || audit.rows[0].UserID != "u-1" {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
	if _, err := svc.CreateSession(context.Background(), CreateChatSessionInput{}); err == nil {
		t.Fatal("expected unauthenticated error")
	}
}

func TestSendMessageHappyPath(t *testing.T) {
	svc, repo, tools, chat, audit, evidence := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1", TimeRange: "30m"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "帮我分析这个告警",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusOK {
		t.Fatalf("status = %q", result.Status)
	}
	if result.Answer.Summary != "CPU 持续升高" || !result.Answer.RequireConfirm || result.Answer.RiskLevel != "medium" {
		t.Fatalf("unexpected answer: %+v", result.Answer)
	}
	// 服务端补齐动作：生成执行计划 + 查看告警 + 查看指标。
	codes := map[string]ChatAction{}
	for _, action := range result.Answer.Actions {
		codes[action.Code] = action
	}
	if !codes["execution.propose"].Allowed || codes["execution.propose"].Target != "/executions" {
		t.Fatalf("execution action = %+v", codes["execution.propose"])
	}
	if codes["view.alert"].Target != "/alerts?alert_id=a1" {
		t.Fatalf("view.alert target = %q", codes["view.alert"].Target)
	}
	if _, ok := codes["view.metrics"]; !ok {
		t.Fatalf("expected view.metrics action: %+v", result.Answer.Actions)
	}
	// 工具调用：告警快照 + 指标证据 + 执行计划权限校验。
	toolByCode := map[string]domain.ToolCall{}
	for _, call := range result.ToolCalls {
		toolByCode[call.ToolCode] = call
	}
	if !toolByCode[chatToolAlarmAnalyze].Allowed || toolByCode[chatToolAlarmAnalyze].Status != "ok" {
		t.Fatalf("alert tool call = %+v", toolByCode[chatToolAlarmAnalyze])
	}
	if !toolByCode[chatToolMetricsQuery].Allowed {
		t.Fatalf("metrics tool call = %+v", toolByCode[chatToolMetricsQuery])
	}
	if !toolByCode[chatToolExecutionPropose].Allowed || toolByCode[chatToolExecutionPropose].Mode != "require_confirm" {
		t.Fatalf("propose tool call = %+v", toolByCode[chatToolExecutionPropose])
	}
	// 工具权限校验使用固定权限码并跳过对象级数据范围（由数据端口各自保证）。
	for _, req := range tools.requests {
		if req.RequiredPermission != chatPermission || !req.SkipDataScope {
			t.Fatalf("unexpected tool request: %+v", req)
		}
	}
	// 引用来源：告警 + 指标证据。
	if len(result.References) != 2 {
		t.Fatalf("references = %+v", result.References)
	}
	if result.References[0].Type != "alert" || result.References[1].ID != "ev-1" {
		t.Fatalf("references = %+v", result.References)
	}
	if evidence.userID != "u-1" || evidence.got.Metric != "cpu_util" {
		t.Fatalf("evidence query not propagated: %+v", evidence.got)
	}
	// 模型收到服务端固定 system prompt 与完整 user prompt。
	if !strings.Contains(chat.system, "只依据") || !strings.Contains(chat.user, "帮我分析这个告警") {
		t.Fatalf("unexpected prompts: system=%q user=%q", chat.system, chat.user)
	}
	if strings.Contains(chat.user, "api_key") || strings.Contains(chat.user, "secret") {
		t.Fatalf("prompt must not carry credentials: %s", chat.user)
	}
	// 落库：用户消息 + 助手消息。
	rows := repo.messages["s-1"]
	if len(rows) != 2 || rows[0].Role != domain.RoleUser || rows[1].Role != domain.RoleAssistant {
		t.Fatalf("unexpected persisted messages: %+v", rows)
	}
	var answer ChatAnswer
	if err := json.Unmarshal([]byte(rows[1].Content), &answer); err != nil {
		t.Fatalf("assistant content is not answer json: %v", err)
	}
	if answer.Summary != "CPU 持续升高" || len(rows[1].ToolCalls) != len(result.ToolCalls) {
		t.Fatalf("unexpected assistant message: %+v", rows[1])
	}
	// 审计：工具调用 + 对话消息。
	actions := map[AuditAction]int{}
	for _, row := range audit.rows {
		actions[row.Action]++
	}
	if actions[AuditToolInvoke] != 3 || actions[AuditChatMessage] != 1 {
		t.Fatalf("unexpected audit actions: %+v", actions)
	}
}

func TestSendMessageRejectsNonOwner(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "owner", domain.SessionContext{})
	_, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "attacker", SessionID: "s-1", Content: "看看其它人的会话",
	})
	if err == nil || apperr.FromError(err).Code != apperr.CodePermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if chat.calls != 0 {
		t.Fatal("model must not be called for a foreign session")
	}
	if len(repo.messages["s-1"]) != 0 {
		t.Fatal("no message should be persisted for a foreign session")
	}
}

func TestSendMessageValidatesInput(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	cases := []SendChatMessageInput{
		{UserID: "u-1", SessionID: "s-1", Content: "   "},
		{UserID: "u-1", SessionID: "s-1", Content: strings.Repeat("长", chatMaxContentRunes+1)},
		{UserID: "", SessionID: "s-1", Content: "hi"},
		{UserID: "u-1", SessionID: "", Content: "hi"},
	}
	for _, in := range cases {
		if _, err := svc.SendMessage(context.Background(), in); err == nil {
			t.Fatalf("expected validation error for %+v", in)
		}
	}
	if _, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "missing", Content: "hi",
	}); err == nil || apperr.FromError(err).Code != apperr.CodeNotFound {
		t.Fatalf("expected not found for missing session, got %v", err)
	}
}

func TestSendMessageDegradesWhenModelUnavailable(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	svc.chat = &fakeChatClient{}
	seedChatSession(t, repo, "u-1", domain.SessionContext{AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "分析告警"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusDegraded {
		t.Fatalf("status = %q", result.Status)
	}
	if !result.Answer.Degraded || result.Answer.DegradedReason != chatDegradedModelUnavailable {
		t.Fatalf("unexpected degraded answer: %+v", result.Answer)
	}
	if len(result.Answer.Evidence) == 0 {
		t.Fatal("expected partial context evidence in degraded answer")
	}
	// 降级结果仍落库，便于用户回看。
	if len(repo.messages["s-1"]) != 2 {
		t.Fatalf("expected persisted degraded answer, got %+v", repo.messages["s-1"])
	}
}

func TestSendMessageDegradesOnModelFailureAndInvalidOutput(t *testing.T) {
	for name, chat := range map[string]*fakeChatClient{
		"failure":        {providerID: "provider-1", err: errors.New("upstream timeout")},
		"invalid output": {providerID: "provider-1", content: "不是 JSON"},
	} {
		t.Run(name, func(t *testing.T) {
			svc, repo, _, _, _, _ := newChatTestService(t)
			svc.chat = chat
			seedChatSession(t, repo, "u-1", domain.SessionContext{})
			result, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "系统状态如何"})
			if err != nil {
				t.Fatalf("send message: %v", err)
			}
			if result.Status != chatStatusDegraded || !result.Answer.Degraded {
				t.Fatalf("expected degraded result, got %+v", result)
			}
			want := chatDegradedModelFailed
			if name == "invalid output" {
				want = chatDegradedOutputInvalid
			}
			if result.Answer.DegradedReason != want {
				t.Fatalf("degraded reason = %q, want %q", result.Answer.DegradedReason, want)
			}
		})
	}
}

func TestSendMessageMarksInsufficientContext(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	chat.content = `{"summary":"上下文不足","evidence":[],"impact":[],"recommendations":[],"risk_level":"info","require_confirm":false,"rollback":"无变更动作","uncertainties":["缺少告警信息"],"insufficient_context":true}`
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "这个告警怎么回事"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusInsufficientContext {
		t.Fatalf("status = %q", result.Status)
	}
	if !result.Answer.InsufficientContext {
		t.Fatal("insufficient_context must be preserved")
	}
}

func TestSendMessageRecordsDeniedToolAndStillAnswers(t *testing.T) {
	svc, repo, tools, _, audit, _ := newChatTestService(t)
	tools.allowed = map[string]bool{
		chatToolAlarmAnalyze: false, chatToolMetricsQuery: true, chatToolExecutionPropose: false,
	}
	tools.reason = "tool permission denied"
	seedChatSession(t, repo, "u-1", domain.SessionContext{AlertID: "a1"})
	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "分析告警"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	denied := 0
	for _, call := range result.ToolCalls {
		if !call.Allowed && call.Reason != "tool permission denied" {
			t.Fatalf("denied tool call = %+v", call)
		}
		if !call.Allowed {
			denied++
		}
	}
	if denied != 2 {
		t.Fatalf("expected 2 denied tool calls, got %+v", result.ToolCalls)
	}
	execAction := ChatAction{}
	for _, action := range result.Answer.Actions {
		if action.Code == "execution.propose" {
			execAction = action
		}
	}
	if execAction.Allowed || execAction.Reason == "" {
		t.Fatalf("execution action must be denied with reason: %+v", execAction)
	}
	// 拒绝是业务结果，仍需写工具审计。
	deniedAudits := 0
	for _, row := range audit.rows {
		if row.Action == AuditToolInvoke && row.Payload["result"] == "denied" {
			deniedAudits++
		}
	}
	if deniedAudits != 2 {
		t.Fatalf("expected denied tool audits, got %+v", deniedAudits)
	}
}

func TestSendMessageRejectsArchivedSession(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	session := seedChatSession(t, repo, "u-1", domain.SessionContext{})
	session.Status = domain.SessionStatusArchived
	repo.sessions[session.SessionID] = session
	if _, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: session.SessionID, Content: "hi"}); err == nil {
		t.Fatal("expected archived session rejection")
	}
}

func TestSendMessageUpdatesContextAndTitle(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "dashboard"})
	pageContext := domain.SessionContext{Page: "alerts", AlertID: "a2"}
	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "看看这个告警", Context: &pageContext,
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Context.AlertID != "a2" {
		t.Fatalf("context = %+v", result.Context)
	}
	updated := repo.sessions["s-1"]
	if updated.Context.AlertID != "a2" {
		t.Fatalf("session context not touched: %+v", updated)
	}
	// 已有明确标题的会话不被首条消息覆盖。
	if updated.Title != "生产告警分析" {
		t.Fatalf("session title should be preserved, got %q", updated.Title)
	}
}

func TestGetSessionAndFeedback(t *testing.T) {
	svc, repo, _, _, audit, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{AlertID: "a1"})
	sent, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "分析告警"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	detail, err := svc.GetSession(context.Background(), "u-1", "s-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if len(detail.Messages) != 2 || detail.Messages[1].Answer == nil || detail.Messages[1].Answer.Summary != "CPU 持续升高" {
		t.Fatalf("unexpected detail: %+v", detail.Messages)
	}

	updated, err := svc.SubmitFeedback(context.Background(), "u-1", "s-1", sent.MessageID, "valid")
	if err != nil {
		t.Fatalf("feedback: %v", err)
	}
	if updated.Feedback != string(domain.FeedbackValid) {
		t.Fatalf("feedback = %q", updated.Feedback)
	}
	if repo.messages["s-1"][1].Feedback != domain.FeedbackValid {
		t.Fatalf("feedback not persisted: %+v", repo.messages["s-1"][1])
	}
	found := false
	for _, row := range audit.rows {
		if row.Action == AuditChatFeedback && row.Payload["feedback"] == "valid" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected feedback audit: %+v", audit.rows)
	}

	if _, err := svc.SubmitFeedback(context.Background(), "u-1", "s-1", sent.MessageID, "maybe"); err == nil {
		t.Fatal("expected invalid feedback rejection")
	}
	if _, err := svc.SubmitFeedback(context.Background(), "u-1", "s-1", "missing", "valid"); err == nil {
		t.Fatal("expected missing message rejection")
	}
	userMessageID := repo.messages["s-1"][0].MessageID
	if _, err := svc.SubmitFeedback(context.Background(), "u-1", "s-1", userMessageID, "valid"); err == nil {
		t.Fatal("expected user message feedback rejection")
	}
	if _, err := svc.SubmitFeedback(context.Background(), "other", "s-1", sent.MessageID, "valid"); err == nil {
		t.Fatal("expected foreign session rejection")
	}
}

func TestListSessions(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	views, err := svc.ListSessions(context.Background(), "u-1", 10)
	if err != nil || len(views) != 1 {
		t.Fatalf("list sessions = %+v, err=%v", views, err)
	}
	empty, err := svc.ListSessions(context.Background(), "nobody", 10)
	if err != nil || len(empty) != 0 {
		t.Fatalf("expected empty list, got %+v, err=%v", empty, err)
	}
	if _, err := svc.ListSessions(context.Background(), "", 10); err == nil {
		t.Fatal("expected unauthenticated error")
	}
}
