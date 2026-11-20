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
	// onAppend 在真正写入前调用，用于模拟并发请求抢先写入（返回错误则终止写入）。
	onAppend func(message domain.Message) error
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
	if f.onAppend != nil {
		if err := f.onAppend(message); err != nil {
			return err
		}
	}
	session, ok := f.sessions[message.SessionID]
	if !ok {
		return domain.ErrSessionNotFound
	}
	// 与 migration 0050 的部分唯一索引等价：同一会话内 user 消息的 client_message_id 唯一。
	if message.Role == domain.RoleUser && message.ClientMessageID != "" {
		for _, existing := range f.messages[message.SessionID] {
			if existing.Role == domain.RoleUser && existing.ClientMessageID == message.ClientMessageID {
				return domain.ErrDuplicateClientMessage
			}
		}
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

func (f *fakeChatSessionRepo) SetStatus(_ context.Context, sessionID string, status domain.SessionStatus, updatedAt time.Time) error {
	session, ok := f.sessions[sessionID]
	if !ok {
		return domain.ErrSessionNotFound
	}
	session.Status = status
	session.UpdatedAt = updatedAt
	f.sessions[sessionID] = session
	return nil
}

func (f *fakeChatSessionRepo) FindUserMessageByClientID(_ context.Context, sessionID, clientMessageID string) (*domain.Message, error) {
	if clientMessageID == "" {
		return nil, nil
	}
	for _, message := range f.messages[sessionID] {
		if message.Role == domain.RoleUser && message.ClientMessageID == clientMessageID {
			copied := message
			return &copied, nil
		}
	}
	return nil, nil
}

func (f *fakeChatSessionRepo) FindAssistantAnswer(_ context.Context, sessionID, userMessageID string) (*domain.Message, error) {
	rows := f.messages[sessionID]
	for i := range rows {
		if rows[i].MessageID != userMessageID || rows[i].Role != domain.RoleUser {
			continue
		}
		for j := i + 1; j < len(rows); j++ {
			if rows[j].Role == domain.RoleAssistant {
				copied := rows[j]
				return &copied, nil
			}
		}
		return nil, nil
	}
	return nil, nil
}

func (f *fakeChatSessionRepo) Delete(_ context.Context, sessionID string) error {
	if _, ok := f.sessions[sessionID]; !ok {
		return domain.ErrSessionNotFound
	}
	delete(f.sessions, sessionID)
	delete(f.messages, sessionID)
	return nil
}

type fakeChatClient struct {
	providerID string
	content    string
	// script 非空时按顺序返回（Agent Loop 多轮场景）；耗尽后回落到 content。
	script       []string
	err          error
	system       string
	user         string
	calls        int
	lastProvider string
	// block 为 true 时模拟模型挂起，直到 ctx 截止（回合预算测试）。
	block bool
	// partial 模拟流式中断但已收到部分输出。
	partial       bool
	partialReason string
}

func (f *fakeChatClient) ChatProviderID() (string, bool) {
	if f.providerID == "" {
		return "", false
	}
	return f.providerID, true
}

func (f *fakeChatClient) Complete(ctx context.Context, providerID, systemPrompt, userPrompt string) (*toolgateway.ChatCompletionResult, error) {
	f.calls++
	f.lastProvider = providerID
	f.system = systemPrompt
	f.user = userPrompt
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	content := f.content
	if len(f.script) > 0 {
		content = f.script[0]
		f.script = f.script[1:]
	}
	return &toolgateway.ChatCompletionResult{
		Content: content, Tokens: 42,
		Partial: f.partial, PartialReason: f.partialReason,
	}, nil
}

func (f *fakeChatClient) CompleteStream(ctx context.Context, providerID, systemPrompt, userPrompt string, onDelta toolgateway.ChatModelDelta) (*toolgateway.ChatCompletionResult, error) {
	if onDelta != nil {
		if err := onDelta("thinking", "正在分析上下文"); err != nil {
			return nil, apperr.Wrap(err, apperr.CodeAborted, "client closed the stream")
		}
	}
	result, err := f.Complete(ctx, providerID, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}
	if onDelta != nil {
		if err := onDelta("answer", result.Content); err != nil {
			return nil, apperr.Wrap(err, apperr.CodeAborted, "client closed the stream")
		}
	}
	return result, nil
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
		chatToolAlarmAnalyze: true, chatToolMetricsQuery: true, chatToolAlertRulesList: true,
		chatToolAccountsList: true, chatToolResourceAnalyze: true, chatToolResourcesList: true,
		chatToolExecutionPropose: true,
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
	// 工具权限校验使用固定权限码并跳过对象级数据范围（告警对象级范围由读取端口收口）。
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

func TestSendMessageRecordsDataScopeDeniedAlertAndStillAnswers(t *testing.T) {
	svc, repo, tools, chat, audit, evidence := newChatTestService(t)
	svc.alerts = &fakeAnalyzeAlertReader{err: apperr.New(apperr.CodePermissionDenied, "data scope denied")}
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析这个告警",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	// 告警上下文被拒：只留一条 denied 观测，不产生告警引用、不取指标、不提供告警跳转。
	var alertCall *domain.ToolCall
	for i := range result.ToolCalls {
		if result.ToolCalls[i].ToolCode == chatToolAlarmAnalyze {
			alertCall = &result.ToolCalls[i]
		}
	}
	if alertCall == nil || alertCall.Allowed || alertCall.Status != "denied" || alertCall.Reason != "data_scope_denied" {
		t.Fatalf("alert tool call = %+v", alertCall)
	}
	if len(result.References) != 0 {
		t.Fatalf("references must be empty on denied alert context: %+v", result.References)
	}
	if evidence.got.Metric != "" {
		t.Fatalf("metric evidence must not be collected: %+v", evidence.got)
	}
	for _, action := range result.Answer.Actions {
		if action.Code == "view.alert" {
			t.Fatalf("view.alert must be suppressed: %+v", action)
		}
	}
	// 数据范围拒绝是业务结果，仍需写工具审计（result=denied）。
	deniedAudits := 0
	for _, row := range audit.rows {
		if row.Action == AuditToolInvoke && row.Payload["reason"] == "data_scope_denied" && row.Payload["result"] == "denied" {
			deniedAudits++
		}
	}
	if deniedAudits != 1 {
		t.Fatalf("expected one denied tool audit, got %+v", audit.rows)
	}
	// RBAC/工具权限先于对象读取：告警工具已校验通过，但拒绝后不得再取指标证据。
	for _, req := range tools.requests {
		if req.ToolCode == chatToolMetricsQuery {
			t.Fatalf("denied alert must not trigger metric evidence: %+v", req)
		}
	}
	if chat.calls != 1 {
		t.Fatalf("model should still answer with degraded context, calls=%d", chat.calls)
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

func TestArchiveAndUnarchiveSession(t *testing.T) {
	svc, repo, _, _, audit, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{AlertID: "a1"})

	archived, err := svc.ArchiveSession(context.Background(), "u-1", "s-1")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived.Status != string(domain.SessionStatusArchived) {
		t.Fatalf("status = %q", archived.Status)
	}
	if repo.sessions["s-1"].Status != domain.SessionStatusArchived {
		t.Fatal("status not persisted")
	}
	// 归档后不可继续对话（读取历史仍允许）。
	if _, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "继续"}); err == nil ||
		apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected archived send rejection, got %v", err)
	}
	if _, err := svc.GetSession(context.Background(), "u-1", "s-1"); err != nil {
		t.Fatalf("archived session should stay readable: %v", err)
	}

	// 幂等：重复归档不报错，也不推进 updated_at。
	before := repo.sessions["s-1"].UpdatedAt
	again, err := svc.ArchiveSession(context.Background(), "u-1", "s-1")
	if err != nil || again.Status != string(domain.SessionStatusArchived) {
		t.Fatalf("idempotent archive: %v %+v", err, again)
	}
	if !repo.sessions["s-1"].UpdatedAt.Equal(before) {
		t.Fatal("idempotent archive must not touch updated_at")
	}

	restored, err := svc.UnarchiveSession(context.Background(), "u-1", "s-1")
	if err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if restored.Status != string(domain.SessionStatusActive) {
		t.Fatalf("status = %q", restored.Status)
	}
	if _, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "继续"}); err != nil {
		t.Fatalf("unarchived session should accept messages: %v", err)
	}

	var statusAudits []AuditRecord
	for _, row := range audit.rows {
		if row.Action == AuditChatSessionStatus {
			statusAudits = append(statusAudits, row)
		}
	}
	if len(statusAudits) != 3 {
		t.Fatalf("expected 3 status audits (archive/archive/unarchive), got %+v", statusAudits)
	}
	if statusAudits[0].Payload["status"] != "archived" || statusAudits[0].Payload["previous_status"] != "active" {
		t.Fatalf("unexpected archive audit payload: %+v", statusAudits[0].Payload)
	}
	if statusAudits[2].Payload["status"] != "active" || statusAudits[2].Payload["previous_status"] != "archived" {
		t.Fatalf("unexpected unarchive audit payload: %+v", statusAudits[2].Payload)
	}
}

func TestArchiveSessionRejectsForeignSession(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "owner", domain.SessionContext{})

	for name, call := range map[string]func() error{
		"archive": func() error {
			_, err := svc.ArchiveSession(context.Background(), "attacker", "s-1")
			return err
		},
		"unarchive": func() error {
			_, err := svc.UnarchiveSession(context.Background(), "attacker", "s-1")
			return err
		},
		"delete": func() error {
			return svc.DeleteSession(context.Background(), "attacker", "s-1")
		},
	} {
		err := call()
		if err == nil || apperr.FromError(err).Code != apperr.CodePermissionDenied {
			t.Fatalf("%s: expected permission denied, got %v", name, err)
		}
	}
	if repo.sessions["s-1"].Status != domain.SessionStatusActive {
		t.Fatal("foreign session must not change status")
	}
}

func TestDeleteSession(t *testing.T) {
	svc, repo, _, _, audit, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{AlertID: "a1"})
	if _, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "分析告警"}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	if err := svc.DeleteSession(context.Background(), "u-1", "s-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := repo.sessions["s-1"]; ok {
		t.Fatal("session not deleted")
	}
	if len(repo.messages["s-1"]) != 0 {
		t.Fatal("messages not deleted")
	}
	if _, err := svc.GetSession(context.Background(), "u-1", "s-1"); err == nil ||
		apperr.FromError(err).Code != apperr.CodeNotFound {
		t.Fatalf("expected not found after delete, got %v", err)
	}
	// 重复删除按不存在处理。
	if err := svc.DeleteSession(context.Background(), "u-1", "s-1"); err == nil ||
		apperr.FromError(err).Code != apperr.CodeNotFound {
		t.Fatalf("expected not found on repeat delete, got %v", err)
	}
	deleted := 0
	for _, row := range audit.rows {
		if row.Action == AuditChatSessionDelete {
			deleted++
			if row.Payload["result"] != "success" || row.Payload["session_id"] != "s-1" {
				t.Fatalf("unexpected delete audit: %+v", row)
			}
		}
	}
	if deleted != 1 {
		t.Fatalf("expected one delete audit, got %+v", audit.rows)
	}
}

func TestSessionLifecycleValidatesInput(t *testing.T) {
	svc, _, _, _, _, _ := newChatTestService(t)
	if _, err := svc.ArchiveSession(context.Background(), "", "s-1"); err == nil ||
		apperr.FromError(err).Code != apperr.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
	if _, err := svc.UnarchiveSession(context.Background(), "u-1", ""); err == nil ||
		apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if err := svc.DeleteSession(context.Background(), "u-1", ""); err == nil ||
		apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestSendMessageReplaysSameClientMessageID(t *testing.T) {
	svc, repo, _, chat, audit, evidence := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	first, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警", ClientMessageID: "cm-1",
	})
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	if chat.calls != 1 {
		t.Fatalf("expected one model call, got %d", chat.calls)
	}
	evidenceCalls := evidence.got.Metric != ""

	// 超时重试：同一 client_message_id 必须原样返回首次结果，不新增消息、不再调模型。
	replay, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警", ClientMessageID: "cm-1",
	})
	if err != nil {
		t.Fatalf("replay send: %v", err)
	}
	if replay.MessageID != first.MessageID || replay.Answer.Summary != first.Answer.Summary || replay.Status != first.Status {
		t.Fatalf("replay mismatch: first=%+v replay=%+v", first, replay)
	}
	if chat.calls != 1 {
		t.Fatalf("replay must not call the model again, got %d calls", chat.calls)
	}
	if len(repo.messages["s-1"]) != 2 {
		t.Fatalf("replay must not append messages, got %d", len(repo.messages["s-1"]))
	}
	if (evidence.got.Metric != "") != evidenceCalls {
		t.Fatal("replay must not collect metric evidence")
	}
	// 幂等重放也写入审计，并标记 replayed。
	replayed := 0
	for _, row := range audit.rows {
		if row.Action == AuditChatMessage && row.Payload["replayed"] == true {
			replayed++
			if row.Payload["client_message_id"] != "cm-1" {
				t.Fatalf("unexpected replay audit payload: %+v", row.Payload)
			}
		}
	}
	if replayed != 1 {
		t.Fatalf("expected one replay audit, got %+v", audit.rows)
	}

	// 不同 client_message_id 仍然正常追加新消息。
	if _, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "继续分析", ClientMessageID: "cm-2",
	}); err != nil {
		t.Fatalf("second send: %v", err)
	}
	if len(repo.messages["s-1"]) != 4 || chat.calls != 2 {
		t.Fatalf("expected new message pair, messages=%d calls=%d", len(repo.messages["s-1"]), chat.calls)
	}
}

func TestSendMessageRejectsInFlightDuplicate(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	// 模拟上一次请求已落用户消息、但回答尚未写回（服务端中断）。
	repo.messages["s-1"] = append(repo.messages["s-1"], domain.Message{
		MessageID: "m-user-orphan", SessionID: "s-1", Role: domain.RoleUser,
		Content: "分析告警", ClientMessageID: "cm-inflight",
		CreatedAt: time.Date(2026, 1, 2, 3, 5, 0, 0, time.UTC),
	})

	_, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警", ClientMessageID: "cm-inflight",
	})
	if err == nil || apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
	if chat.calls != 0 {
		t.Fatal("in-flight duplicate must not call the model")
	}
	if len(repo.messages["s-1"]) != 1 {
		t.Fatalf("in-flight duplicate must not append messages, got %d", len(repo.messages["s-1"]))
	}
}

func TestSendMessageReplaysWhenConcurrentInsertLoses(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	// 并发：查重时还没有记录，插入前另一个请求抢先写入同一 client_message_id 并完成回答。
	repo.onAppend = func(message domain.Message) error {
		if message.Role != domain.RoleUser || message.ClientMessageID == "" {
			return nil
		}
		repo.messages["s-1"] = append(repo.messages["s-1"],
			domain.Message{MessageID: "m-u", SessionID: "s-1", Role: domain.RoleUser, Content: "分析告警", ClientMessageID: "cm-race"},
			domain.Message{MessageID: "m-a", SessionID: "s-1", Role: domain.RoleAssistant, Content: chatAnswerJSON("并发胜出")},
		)
		return domain.ErrDuplicateClientMessage
	}

	out, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警", ClientMessageID: "cm-race",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out.MessageID != "m-a" || out.Answer.Summary != "并发胜出" {
		t.Fatalf("expected replay of winner answer, got %+v", out)
	}
	if chat.calls != 0 {
		t.Fatal("losing duplicate must not call the model")
	}
	if len(repo.messages["s-1"]) != 2 {
		t.Fatalf("losing duplicate must not append rows, got %d", len(repo.messages["s-1"]))
	}
}

func TestSendMessageReplaysAfterSessionArchived(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	first, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警", ClientMessageID: "cm-arch",
	})
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	if _, err := svc.ArchiveSession(context.Background(), "u-1", "s-1"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	// 归档后重放已生成的回答仍可返回（只读）；新提问才被拒绝。
	replay, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警", ClientMessageID: "cm-arch",
	})
	if err != nil || replay.MessageID != first.MessageID {
		t.Fatalf("expected replay after archive, got %+v err=%v", replay, err)
	}
	if chat.calls != 1 {
		t.Fatalf("replay must not call the model, got %d", chat.calls)
	}
	if _, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "新问题", ClientMessageID: "cm-new",
	}); err == nil || apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("new message on archived session must be rejected, got %v", err)
	}
}

func TestSendMessageValidatesClientMessageID(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	_, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "hi", ClientMessageID: strings.Repeat("x", chatMaxClientMessageIDRunes+1),
	})
	if err == nil || apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestChatResultStatusFromAnswer(t *testing.T) {
	if got := chatResultStatus(ChatAnswer{Summary: "s"}); got != chatStatusOK {
		t.Fatalf("status = %q", got)
	}
	if got := chatResultStatus(ChatAnswer{Degraded: true, DegradedReason: "model_failed"}); got != chatStatusDegraded {
		t.Fatalf("status = %q", got)
	}
	if got := chatResultStatus(ChatAnswer{Degraded: true, Incomplete: true}); got != chatStatusIncomplete {
		t.Fatalf("incomplete status = %q", got)
	}
	if got := chatResultStatus(ChatAnswer{InsufficientContext: true}); got != chatStatusInsufficientContext {
		t.Fatalf("status = %q", got)
	}
	// 落库的降级回答必须能被解析回来（重放时 status 不能丢）。
	raw, err := json.Marshal(ChatAnswer{Summary: "降级", Degraded: true, DegradedReason: "model_unavailable"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := parseChatAnswer(string(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !parsed.Degraded || parsed.DegradedReason != "model_unavailable" {
		t.Fatalf("degraded flags lost: %+v", parsed)
	}
}

func TestSendMessageInjectsMemoryCases(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	memoryRepo := &fakeMemoryRepo{}
	svc.SetMemory(NewMemoryService(memoryRepo, &fakeAnalyzeAlertReader{alert: &AlertContext{
		ID: "a1", Name: "HighCPU", Labels: map[string]string{"metric_name": "cpu_util"},
	}}))
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	// 预置一条同用户历史案例（应用匹配）。
	if _, err := memoryRepo.Create(context.Background(), domain.MemoryCase{
		CaseID: "case-1", SourceType: domain.MemoryCaseSourceChatMessage, SourceMessageID: "m-old",
		UserID: "u-1", ApplicationID: "app-1", AlertSignature: "", Keywords: "cpu 告警",
		Summary: "历史结论：发布导致 CPU 升高", Recommendations: []string{"先查发布记录"},
		RiskLevel: "medium", QualityScore: 1, Status: domain.MemoryCaseStatusActive,
	}); err != nil {
		t.Fatalf("seed memory case: %v", err)
	}

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "这个告警怎么回事", Context: &domain.SessionContext{Page: "alerts", AlertID: "a1", ApplicationID: "app-1"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	// Prompt 注入 memory_cases（含 case_id）。
	if !strings.Contains(chat.user, "memory_cases") || !strings.Contains(chat.user, "case-1") {
		t.Fatalf("memory cases not injected into prompt: %s", chat.user)
	}
	// references 回显 memory_case。
	found := false
	for _, ref := range result.References {
		if ref.Type == "memory_case" && ref.ID == "case-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("memory_case reference missing: %+v", result.References)
	}
	// 审计记录条数与状态，不含案例原文。
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil {
		t.Fatal("chat_message audit missing")
	}
	if chatAudit.Payload["memory_used"] != 1 || chatAudit.Payload["memory_status"] != MemoryStatusOK {
		t.Fatalf("unexpected memory audit fields: %+v", chatAudit.Payload)
	}
	if _, ok := chatAudit.Payload["summary"]; ok {
		t.Fatal("audit must not contain case summary text")
	}
}

func TestSendMessageMemoryFailureDoesNotBlock(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	svc.SetMemory(NewMemoryService(&fakeMemoryRepo{searchErr: errors.New("db down")}, nil))
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警", Context: &domain.SessionContext{AlertID: "a1", ApplicationID: "app-1"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusOK {
		t.Fatalf("status = %q", result.Status)
	}
	if strings.Contains(chat.user, "memory_cases") {
		t.Fatal("failed memory search must not inject cases")
	}
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil || chatAudit.Payload["memory_status"] != MemoryStatusError || chatAudit.Payload["memory_used"] != 0 {
		t.Fatalf("unexpected memory audit: %+v", chatAudit)
	}
}

func TestSubmitFeedbackSyncsMemory(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	memoryRepo := &fakeMemoryRepo{}
	svc.SetMemory(NewMemoryService(memoryRepo, nil))
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts"})

	sent, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "分析告警"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if _, err := svc.SubmitFeedback(context.Background(), "u-1", "s-1", sent.MessageID, "valid"); err != nil {
		t.Fatalf("feedback: %v", err)
	}
	if len(memoryRepo.cases) != 1 || memoryRepo.cases[0].SourceMessageID != sent.MessageID {
		t.Fatalf("valid feedback must create a memory case: %+v", memoryRepo.cases)
	}
	if _, err := svc.SubmitFeedback(context.Background(), "u-1", "s-1", sent.MessageID, "invalid"); err != nil {
		t.Fatalf("invalid feedback: %v", err)
	}
	if memoryRepo.cases[0].QualityScore != domain.MemoryCaseValidDelta+domain.MemoryCaseInvalidDelta {
		t.Fatalf("invalid feedback must lower quality: %+v", memoryRepo.cases[0])
	}
}

func TestSubmitFeedbackMemoryFailureDoesNotBlock(t *testing.T) {
	svc, repo, _, _, audit, _ := newChatTestService(t)
	svc.SetMemory(NewMemoryService(&fakeMemoryRepo{createErr: errors.New("db down")}, nil))
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	sent, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "分析告警"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	view, err := svc.SubmitFeedback(context.Background(), "u-1", "s-1", sent.MessageID, "valid")
	if err != nil {
		t.Fatalf("feedback must succeed even if memory write fails: %v", err)
	}
	if view.Feedback != string(domain.FeedbackValid) {
		t.Fatalf("feedback = %q", view.Feedback)
	}
	found := false
	for _, row := range audit.rows {
		if row.Action == AuditChatFeedback {
			found = true
		}
	}
	if !found {
		t.Fatal("feedback audit missing")
	}
}

func TestSendMessageInjectsKnowledgeAppendix(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	knowledgeRepo := newFakeKnowledgeRepo(knowledgeTestItem("item-1", domain.KnowledgeStatusActive))
	svc.SetKnowledge(NewKnowledgeService(knowledgeRepo, NoopAuditRecorder{}))
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "这个告警怎么回事",
		Context: &domain.SessionContext{Page: "alerts", AlertID: "a1", ApplicationID: "app-1"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	// 知识进 system prompt 附加段，固定安全约束仍在最前面。
	if !strings.Contains(chat.system, "只依据") {
		t.Fatalf("fixed system prompt lost: %s", chat.system)
	}
	if !strings.Contains(chat.system, "item-1") || !strings.Contains(chat.system, "先查发布记录") {
		t.Fatalf("knowledge appendix missing: %s", chat.system)
	}
	if strings.Index(chat.system, "只依据") > strings.Index(chat.system, "item-1") {
		t.Fatal("knowledge appendix must not precede the fixed safety prompt")
	}
	// references 回显 knowledge_item。
	found := false
	for _, ref := range result.References {
		if ref.Type == "knowledge_item" && ref.ID == "item-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("knowledge_item reference missing: %+v", result.References)
	}
	// 命中计数 +1。
	if knowledgeRepo.items["item-1"].HitCount != 1 {
		t.Fatalf("hit_count = %d", knowledgeRepo.items["item-1"].HitCount)
	}
	// 审计字段。
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil || chatAudit.Payload["knowledge_used"] != 1 || chatAudit.Payload["knowledge_status"] != KnowledgeStatusOK {
		t.Fatalf("unexpected knowledge audit: %+v", chatAudit)
	}
}

func TestSendMessageSkipsDraftKnowledge(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	knowledgeRepo := newFakeKnowledgeRepo(knowledgeTestItem("item-1", domain.KnowledgeStatusDraft))
	svc.SetKnowledge(NewKnowledgeService(knowledgeRepo, NoopAuditRecorder{}))
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "分析告警"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if strings.Contains(chat.system, "item-1") {
		t.Fatal("draft knowledge must not be injected")
	}
	for _, ref := range result.References {
		if ref.Type == "knowledge_item" {
			t.Fatalf("draft knowledge must not be referenced: %+v", ref)
		}
	}
	if knowledgeRepo.items["item-1"].HitCount != 0 {
		t.Fatalf("draft hit_count = %d", knowledgeRepo.items["item-1"].HitCount)
	}
}

func TestSubmitFeedbackMarksKnowledgeUseful(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	knowledgeRepo := newFakeKnowledgeRepo(knowledgeTestItem("item-1", domain.KnowledgeStatusActive))
	svc.SetKnowledge(NewKnowledgeService(knowledgeRepo, NoopAuditRecorder{}))
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	sent, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "分析告警"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if _, err := svc.SubmitFeedback(context.Background(), "u-1", "s-1", sent.MessageID, "valid"); err != nil {
		t.Fatalf("feedback: %v", err)
	}
	// 有效反馈把注入过的条目记为 useful（hit_count 已在注入时累加）。
	if knowledgeRepo.items["item-1"].UsefulCount != 1 {
		t.Fatalf("useful_count = %d", knowledgeRepo.items["item-1"].UsefulCount)
	}
	if knowledgeRepo.items["item-1"].Status != domain.KnowledgeStatusActive {
		t.Fatalf("useful item must stay active, got %s", knowledgeRepo.items["item-1"].Status)
	}
}

func TestSendMessageKnowledgeFailureDoesNotBlock(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	failing := newFakeKnowledgeRepo()
	failing.listErr = errors.New("db down")
	svc.SetKnowledge(NewKnowledgeService(failing, NoopAuditRecorder{}))
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "分析告警"})
	if err != nil {
		t.Fatalf("knowledge failure must not block: %v", err)
	}
	if result.Status != chatStatusOK {
		t.Fatalf("status = %q", result.Status)
	}
	if strings.Contains(chat.system, "知识沉淀") {
		t.Fatal("failed lookup must not add an appendix")
	}
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil || chatAudit.Payload["knowledge_status"] != KnowledgeStatusError {
		t.Fatalf("unexpected knowledge audit: %+v", chatAudit)
	}
}

func TestSendMessageAgentLoopRequestsReadOnlyTool(t *testing.T) {
	svc, repo, _, chat, audit, evidence := newChatTestService(t)
	// 第一轮：模型申请追加指标证据；第二轮：给出结构化回答。
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"metrics.query","reason":"缺少近期指标"}]}`,
		chatAnswerJSON("指标证据显示 CPU 峰值 97%"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "为什么 CPU 高",
		Context: &domain.SessionContext{Page: "alerts", AlertID: "a1"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if chat.calls != 2 {
		t.Fatalf("expected two model calls, got %d", chat.calls)
	}
	if result.Status != chatStatusOK || !strings.Contains(result.Answer.Summary, "97%") {
		t.Fatalf("unexpected answer: %+v", result)
	}
	// 循环内的工具调用进入 tool_calls，并被审计。
	metricsCalls := 0
	for _, call := range result.ToolCalls {
		if call.ToolCode == chatToolMetricsQuery {
			metricsCalls++
		}
	}
	if metricsCalls < 1 {
		t.Fatalf("loop tool call missing: %+v", result.ToolCalls)
	}
	if evidence.got.Metric == "" {
		t.Fatal("loop must actually collect evidence")
	}
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil || chatAudit.Payload["loop_rounds"] != 1 {
		t.Fatalf("unexpected loop audit: %+v", chatAudit)
	}
}

func TestSendMessageAgentLoopRejectsExecutionTools(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"execution.command.run","reason":"直接执行"}]}`,
		chatAnswerJSON("无法直接执行，建议生成执行计划"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "帮我执行"})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	denied := false
	for _, call := range result.ToolCalls {
		if call.ToolCode == "execution.command.run" {
			if call.Allowed || call.Status != "denied" || call.Reason != "tool is not allowed in agent loop" {
				t.Fatalf("execution tool must be denied: %+v", call)
			}
			denied = true
		}
	}
	if !denied {
		t.Fatalf("execution tool request must be recorded as denied: %+v", result.ToolCalls)
	}
	if result.Status != chatStatusOK {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestSendMessageAgentLoopExhaustsRounds(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	// 模型每轮申请不同时间窗口的指标证据：轮数用尽后强制收敛一轮，
	// 强制轮仍申请工具（无 answer）才降级，不能悬挂。
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700000000,"to":1700003600}}]}`,
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700003600,"to":1700007200}}]}`,
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700007200,"to":1700010800}}]}`,
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700010800,"to":1700014400}}]}`,
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700014400,"to":1700018000}}]}`,
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{AlertID: "a1", Page: "alerts"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "继续查",
		Context: &domain.SessionContext{AlertID: "a1", Page: "alerts"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusDegraded || result.Answer.DegradedReason != chatDegradedLoopExhausted {
		t.Fatalf("expected agent_loop_exhausted, got %+v", result)
	}
	if chat.calls != chatMaxLoopRounds+2 {
		t.Fatalf("expected %d model calls, got %d", chatMaxLoopRounds+2, chat.calls)
	}
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil || chatAudit.Payload["loop_rounds"] != chatMaxLoopRounds {
		t.Fatalf("unexpected loop audit: %+v", chatAudit)
	}
}

func TestSendMessageAgentLoopDuplicateDoesNotConsumeRounds(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	// 每轮「先重复上一轮申请、再换新窗口」：重复申请回 denied 且不占有效轮数，
	// 3 个不同窗口后仍留有轮次让模型输出结论。
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700000000,"to":1700003600}}]}`,
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700000000,"to":1700003600}}]}`,
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700003600,"to":1700007200}}]}`,
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700003600,"to":1700007200}}]}`,
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700007200,"to":1700010800}}]}`,
		chatAnswerJSON("基于三个窗口得出结论"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{AlertID: "a1", Page: "alerts"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "连续对比三段窗口",
		Context: &domain.SessionContext{AlertID: "a1", Page: "alerts"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusOK || !strings.Contains(result.Answer.Summary, "三个窗口") {
		t.Fatalf("duplicate rounds must not exhaust budget: %+v", result)
	}
	if chat.calls != 6 {
		t.Fatalf("expected 6 model calls, got %d", chat.calls)
	}
	if auditChatLoopRounds(audit) != chatMaxLoopRounds {
		t.Fatalf("loop_rounds = %d", auditChatLoopRounds(audit))
	}
	if !hasToolCallReason(result.ToolCalls, chatObservationReasonDuplicateRequest) {
		t.Fatalf("missing duplicate denied observation: %+v", result.ToolCalls)
	}
}

func TestSendMessageAgentLoopStopsOnRepeatedIdenticalRequests(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	// 模型反复申请同一查询：第一次执行，后续回 denied；连续两轮无进展即收敛。
	chat.content = `{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700000000,"to":1700003600}}]}`
	seedChatSession(t, repo, "u-1", domain.SessionContext{AlertID: "a1", Page: "alerts"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "继续查",
		Context: &domain.SessionContext{AlertID: "a1", Page: "alerts"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusDegraded || result.Answer.DegradedReason != chatDegradedLoopExhausted {
		t.Fatalf("expected agent_loop_exhausted, got %+v", result)
	}
	// 两次无进展轮 + 一次强制收敛轮（仍输出 tool_requests，无 answer）。
	if chat.calls != 4 {
		t.Fatalf("expected 4 model calls, got %d", chat.calls)
	}
	if !hasToolCallReason(result.ToolCalls, chatObservationReasonDuplicateRequest) {
		t.Fatalf("missing duplicate denied observation: %+v", result.ToolCalls)
	}
}

func TestSendMessageAgentLoopStopsAfterOneRetryWithoutProgress(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	// 模型持续申请未授权工具：先给一次纠正机会（拒绝原因已反馈），仍无实际取数才收敛。
	chat.content = `{"tool_requests":[{"tool_code":"execution.command.run"}]}`
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessage(context.Background(), SendMessageInput("继续"))
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusDegraded || result.Answer.DegradedReason != chatDegradedLoopExhausted {
		t.Fatalf("expected agent_loop_exhausted, got %+v", result)
	}
	// 一轮申请 + 一轮纠正机会 + 一次强制收敛轮（仍申请未授权工具），共三次模型调用；不无限重试。
	if chat.calls != 3 {
		t.Fatalf("expected three model calls (one retry after denial + forced convergence), got %d", chat.calls)
	}
}

func TestSendMessageAgentLoopForcedConvergenceAnswers(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	// 轮数用尽后强制收敛轮必须基于已有观测产出回答，而不是丢弃证据降级。
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700000000,"to":1700003600}}]}`,
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700003600,"to":1700007200}}]}`,
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700007200,"to":1700010800}}]}`,
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700010800,"to":1700014400}}]}`,
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{AlertID: "a1", Page: "alerts"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "继续查",
		Context: &domain.SessionContext{AlertID: "a1", Page: "alerts"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusOK || result.Answer.Summary != "CPU 持续升高" {
		t.Fatalf("forced convergence must answer: %+v", result)
	}
	if chat.calls != chatMaxLoopRounds+2 {
		t.Fatalf("expected %d model calls, got %d", chatMaxLoopRounds+2, chat.calls)
	}
	if !strings.Contains(chat.user, "取数预算已耗尽") || !strings.Contains(chat.user, "禁止再输出 tool_requests") {
		t.Fatalf("forced prompt missing convergence instruction: %s", chat.user)
	}
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil || chatAudit.Payload["forced_convergence"] != true || chatAudit.Payload["loop_rounds"] != chatMaxLoopRounds {
		t.Fatalf("unexpected forced convergence audit: %+v", chatAudit)
	}
}

type fakeAlertRuleListPort struct {
	got    AlertRuleQuery
	userID string
	result *AlertRuleListResult
	err    error
	calls  int
}

func (f *fakeAlertRuleListPort) List(_ context.Context, userID string, q AlertRuleQuery) (*AlertRuleListResult, error) {
	f.got = q
	f.userID = userID
	f.calls++
	return f.result, f.err
}

func TestSendMessageAgentLoopUsesFreeMetricParams(t *testing.T) {
	svc, repo, _, chat, audit, evidence := newChatTestService(t)
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"metrics.query","reason":"自由参数","params":{"namespace":"SYS.RDS","metric":"mem_util","dimensions":{"instance_id":"i-9"},"from":1700000000,"to":1700003600,"period":60}}]}`,
		chatAnswerJSON("自由参数取数完成"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "看下 RDS 内存指标",
		Context: &domain.SessionContext{Page: "alerts", AlertID: "a1"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusOK {
		t.Fatalf("status = %q", result.Status)
	}
	if evidence.got.Namespace != "SYS.RDS" || evidence.got.Metric != "rds002_mem_util" {
		t.Fatalf("model params not used (mem_util must normalize): %+v", evidence.got)
	}
	if evidence.got.AccountID != "acc-1" || evidence.got.Region != "cn-north-4" {
		t.Fatalf("alert identity fallback missing: %+v", evidence.got)
	}
	if evidence.got.From != 1700000000 || evidence.got.To != 1700003600 || evidence.got.Period != 60 {
		t.Fatalf("time params not used: %+v", evidence.got)
	}
	if evidence.got.Threshold != nil {
		t.Fatalf("threshold must be dropped when metric changes: %+v", evidence.got)
	}
	if !hasToolCall(result.ToolCalls, chatToolMetricsQuery, "ok") {
		t.Fatalf("missing metrics.query ok call: %+v", result.ToolCalls)
	}
	if !hasReference(result.References, "ev-1") {
		t.Fatalf("missing metric evidence reference: %+v", result.References)
	}
	if auditChatLoopRounds(audit) != 1 {
		t.Fatalf("loop_rounds = %d", auditChatLoopRounds(audit))
	}
}

func TestSendMessageAgentLoopQueriesAlertRules(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	threshold := 80.0
	rulesPort := &fakeAlertRuleListPort{result: &AlertRuleListResult{
		Status: EvidenceStatusOK, EvidenceID: "rules-ev-1",
		Rules: []AlertRuleSummary{{
			RuleID: "rule-1", Name: "High CPU", Namespace: "SYS.RDS", Severity: "major",
			Enabled: true, Metric: "cpu_util", Operator: ">", Threshold: &threshold, WindowSeconds: 300,
		}},
	}}
	svc.SetAlertRules(rulesPort)
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"alert.rules.list","reason":"规则体检","params":{"namespace":"SYS.RDS","limit":10}}]}`,
		chatAnswerJSON("规则阈值偏高"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "SYS.RDS 规则阈值合理吗",
		Context: &domain.SessionContext{Page: "alerts", AlertID: "a1"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusOK {
		t.Fatalf("status = %q", result.Status)
	}
	if rulesPort.got.AccountID != "acc-1" || rulesPort.got.Namespace != "SYS.RDS" || rulesPort.got.Limit != 10 {
		t.Fatalf("rule query = %+v", rulesPort.got)
	}
	if !hasToolCall(result.ToolCalls, chatToolAlertRulesList, "ok") {
		t.Fatalf("missing alert.rules.list call: %+v", result.ToolCalls)
	}
	if !hasReference(result.References, "rules-ev-1") {
		t.Fatalf("missing rule evidence reference: %+v", result.References)
	}
	var ruleAudit *AuditRecord
	for i := range audit.rows {
		row := audit.rows[i]
		if row.Action == AuditToolInvoke && row.Payload["tool_code"] == chatToolAlertRulesList {
			ruleAudit = &audit.rows[i]
		}
	}
	if ruleAudit == nil || ruleAudit.Payload["evidence_id"] != "rules-ev-1" || ruleAudit.Payload["result"] != "success" {
		t.Fatalf("unexpected rule audit: %+v", ruleAudit)
	}
	if auditChatLoopRounds(audit) != 1 {
		t.Fatalf("loop_rounds = %d", auditChatLoopRounds(audit))
	}
}

func TestSendMessageAgentLoopInvalidMetricParamsDoesNotQuery(t *testing.T) {
	svc, repo, _, chat, _, evidence := newChatTestService(t)
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"metrics.query","params":{"from":1700000000,"to":1702678400,"period":86400}}]}`,
		chatAnswerJSON("参数无效，改用现有证据"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "看 31 天趋势",
		Context: &domain.SessionContext{Page: "alerts", AlertID: "a1"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusOK {
		t.Fatalf("status = %q", result.Status)
	}
	if evidence.calls != 1 {
		t.Fatalf("invalid params must not query evidence, calls=%d", evidence.calls)
	}
	if !hasToolCall(result.ToolCalls, chatToolMetricsQuery, chatObservationStatusInvalidArgument) {
		t.Fatalf("missing invalid_argument observation: %+v", result.ToolCalls)
	}
}

func TestSendMessageAgentLoopRulesUnsupportedDoesNotBlock(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	svc.SetAlertRules(&fakeAlertRuleListPort{result: &AlertRuleListResult{
		Status: EvidenceStatusUnsupported, Reason: "alert rules are not supported by the provider",
	}})
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"alert.rules.list","params":{"namespace":"SYS.RDS"}}]}`,
		chatAnswerJSON("当前 provider 不支持规则查询"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "查规则",
		Context: &domain.SessionContext{Page: "alerts", AlertID: "a1"},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusOK {
		t.Fatalf("status = %q", result.Status)
	}
	if !hasToolCall(result.ToolCalls, chatToolAlertRulesList, EvidenceStatusUnsupported) {
		t.Fatalf("missing unsupported rule call: %+v", result.ToolCalls)
	}
}

func hasToolCall(calls []domain.ToolCall, code, status string) bool {
	for _, call := range calls {
		if call.ToolCode == code && call.Status == status {
			return true
		}
	}
	return false
}

func hasToolCallReason(calls []domain.ToolCall, reason string) bool {
	for _, call := range calls {
		if call.Reason == reason {
			return true
		}
	}
	return false
}

func hasReference(refs []domain.Reference, id string) bool {
	for _, ref := range refs {
		if ref.ID == id {
			return true
		}
	}
	return false
}

func auditChatLoopRounds(audit *capturingAIAudit) int {
	for i := len(audit.rows) - 1; i >= 0; i-- {
		if audit.rows[i].Action == AuditChatMessage {
			if v, ok := audit.rows[i].Payload["loop_rounds"].(int); ok {
				return v
			}
			return 0
		}
	}
	return -1
}

// SendMessageInput 是收敛测试的固定入参。
func SendMessageInput(content string) SendChatMessageInput {
	return SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: content}
}

func TestSendMessageStreamEmitsStagesAndAnswer(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	var events []ChatEvent
	result, err := svc.SendMessageStream(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警",
		Context: &domain.SessionContext{Page: "alerts", AlertID: "a1"},
	}, func(event ChatEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("stream message: %v", err)
	}
	if result == nil || result.MessageID == "" {
		t.Fatal("stream must return the persisted result")
	}
	// 顺序：gathering → thinking →（工具事件）→ answering → answer。
	stages := make([]string, 0, 3)
	var sawTool, sawAnswer bool
	for _, event := range events {
		switch event.Type {
		case ChatEventStage:
			stages = append(stages, event.Stage)
		case ChatEventTool:
			sawTool = true
			if event.Tool == nil || event.Tool.ToolCode == "" {
				t.Fatalf("tool event missing payload: %+v", event)
			}
		case ChatEventAnswer:
			sawAnswer = true
			if event.Result == nil || event.Result.MessageID != result.MessageID {
				t.Fatalf("answer event mismatch: %+v", event.Result)
			}
		}
	}
	if len(stages) < 3 || stages[0] != ChatStageGathering || stages[len(stages)-1] != ChatStageAnswering {
		t.Fatalf("unexpected stages: %+v", stages)
	}
	if !sawTool {
		t.Fatal("expected at least one tool event")
	}
	if !sawAnswer {
		t.Fatal("expected a final answer event")
	}
}

func TestSendMessageStreamCancelSkipsPersistingAnswer(t *testing.T) {
	svc, repo, _, _, audit, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{AlertID: "a1"})
	canceled := false

	_, err := svc.SendMessageStream(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警",
		Context: &domain.SessionContext{AlertID: "a1"},
	}, func(event ChatEvent) error {
		// gathering 阶段放行，thinking 事件后模拟客户端断开：
		// 取消点落在模型调用前，命中 before_model 分支并写 chat_canceled 审计。
		if event.Type == ChatEventStage && event.Stage == ChatStageThinking && !canceled {
			canceled = true
			return errors.New("client gone")
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected canceled error")
	}
	if apperr.CodeOf(err) != apperr.CodeAborted {
		t.Fatalf("expected aborted, got %v", err)
	}
	// 取消后不落 assistant 消息。
	assistant := 0
	for _, message := range repo.messages["s-1"] {
		if message.Role == domain.RoleAssistant {
			assistant++
		}
	}
	if assistant != 0 {
		t.Fatalf("canceled stream must not persist assistant message, got %d", assistant)
	}
	// 取消事实写审计。
	found := false
	for _, row := range audit.rows {
		if row.Action == AuditChatCanceled {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected chat_canceled audit, got %+v", audit.rows)
	}
}

func TestSendMessageCanceledContextStillWritesCancelAudit(t *testing.T) {
	svc, repo, _, _, audit, _ := newChatTestService(t)
	// 模拟真实仓储：ctx 已取消时写入失败。取消审计必须脱链写入才能保留。
	audit.requireLiveCtx = true
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.SendMessage(ctx, SendChatMessageInput{UserID: "u-1", SessionID: "s-1", Content: "分析告警"})
	if err == nil || apperr.CodeOf(err) != apperr.CodeAborted {
		t.Fatalf("expected aborted, got %v", err)
	}
	found := false
	for _, row := range audit.rows {
		if row.Action == AuditChatCanceled && row.Payload["phase"] == "before_gather" {
			found = true
		}
	}
	if !found {
		t.Fatalf("canceled request must still write chat_canceled audit, got %+v", audit.rows)
	}
}

func TestSendMessageStreamReplaysAnswerEvent(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	first, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警", ClientMessageID: "cm-stream-1",
	})
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	if chat.calls != 1 {
		t.Fatalf("expected one model call, got %d", chat.calls)
	}

	var events []ChatEvent
	replay, err := svc.SendMessageStream(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警", ClientMessageID: "cm-stream-1",
	}, func(event ChatEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("stream replay: %v", err)
	}
	if replay.MessageID != first.MessageID || replay.Status != first.Status {
		t.Fatalf("replay mismatch: first=%+v replay=%+v", first, replay)
	}
	if chat.calls != 1 {
		t.Fatalf("stream replay must not call the model, got %d", chat.calls)
	}
	// 幂等重放也必须推 answer 事件，否则流式客户端只收到阶段事件后会一直等待。
	answerEvents := 0
	for _, event := range events {
		if event.Type == ChatEventAnswer && event.Result != nil {
			answerEvents++
			if event.Result.MessageID != first.MessageID {
				t.Fatalf("unexpected replayed answer: %+v", event.Result)
			}
		}
	}
	if answerEvents != 1 {
		t.Fatalf("expected one answer event on replay, got %+v", events)
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

func TestSendMessageStreamPartialRetention(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	chat.partial = true
	chat.partialReason = "stream_idle_timeout"
	chat.content = `{"summary":"半成品结论"` // 中断的 JSON 无法解析，退回保守回答
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	result, err := svc.SendMessageStream(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警",
		Context: &domain.SessionContext{Page: "alerts", AlertID: "a1"},
	}, func(ChatEvent) error { return nil })
	if err != nil {
		t.Fatalf("stream message: %v", err)
	}
	if result.Status != chatStatusIncomplete {
		t.Fatalf("status = %q", result.Status)
	}
	if !result.Answer.Incomplete || result.Answer.IncompleteReason != "stream_idle_timeout" {
		t.Fatalf("unexpected answer: %+v", result.Answer)
	}
	if !result.Answer.Degraded || result.Answer.DegradedReason != chatDegradedStreamInterrupted {
		t.Fatalf("partial answer must keep degraded context: %+v", result.Answer)
	}
	if len(result.Answer.Evidence) == 0 {
		t.Fatal("partial answer must retain observed evidence")
	}
	// 不完整回答仍需落库，重放时保留 incomplete 标记。
	rows := repo.messages["s-1"]
	if len(rows) != 2 || rows[1].Role != domain.RoleAssistant {
		t.Fatalf("partial answer must be persisted: %+v", rows)
	}
	detail, err := svc.GetSession(context.Background(), "u-1", "s-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if detail.Messages[1].Answer == nil || !detail.Messages[1].Answer.Incomplete {
		t.Fatalf("incomplete flag lost on replay: %+v", detail.Messages[1].Answer)
	}
	found := false
	for _, row := range audit.rows {
		if row.Action == AuditChatMessage && row.Payload["status"] == chatStatusIncomplete {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected incomplete chat audit, got %+v", audit.rows)
	}
}

func TestSendMessageStreamPartialKeepsParseableAnswer(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	chat.partial = true
	chat.partialReason = "stream_read_error"
	chat.content = chatAnswerJSON("中断前已生成的结论")
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessageStream(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警",
	}, func(ChatEvent) error { return nil })
	if err != nil {
		t.Fatalf("stream message: %v", err)
	}
	if result.Status != chatStatusIncomplete || result.Answer.Summary != "中断前已生成的结论" {
		t.Fatalf("parseable partial answer lost: %+v", result)
	}
	if !result.Answer.Incomplete || result.Answer.IncompleteReason != "stream_read_error" {
		t.Fatalf("incomplete flags = %+v", result.Answer)
	}
}

type fakeChatCatalog struct {
	*fakeChatClient
	models []toolgateway.ChatModel
}

func (f *fakeChatCatalog) ChatModels() []toolgateway.ChatModel { return f.models }

func (f *fakeChatCatalog) ChatProviderByID(providerID string) (toolgateway.ChatModel, bool) {
	for _, model := range f.models {
		if model.ProviderID == providerID {
			return model, true
		}
	}
	return toolgateway.ChatModel{}, false
}

func TestListChatModels(t *testing.T) {
	svc, _, _, chat, _, _ := newChatTestService(t)
	svc.chat = &fakeChatCatalog{fakeChatClient: chat, models: []toolgateway.ChatModel{
		{ProviderID: "provider-1", Name: "默认模型", Model: "model-1"},
		{ProviderID: "provider-2", Name: "备用模型", Model: "model-2"},
	}}
	models := svc.ListChatModels()
	if len(models) != 2 || models[1].ProviderID != "provider-2" || models[1].Model != "model-2" {
		t.Fatalf("models = %+v", models)
	}
}

func TestSendMessageUsesRequestedProviderAndLocksSession(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	svc.chat = &fakeChatCatalog{fakeChatClient: chat, models: []toolgateway.ChatModel{
		{ProviderID: "provider-1", Name: "默认模型", Model: "model-1"},
		{ProviderID: "provider-2", Name: "备用模型", Model: "model-2"},
	}}
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警", ProviderID: "provider-2",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if chat.lastProvider != "provider-2" {
		t.Fatalf("provider = %q", chat.lastProvider)
	}
	if result.Status != chatStatusOK || len(result.Answer.Uncertainties) != 1 {
		t.Fatalf("unexpected answer: %+v", result.Answer)
	}
	// 首次消息显式选择会写入会话上下文，后续轮次沿用。
	if repo.sessions["s-1"].Context.ProviderID != "provider-2" {
		t.Fatalf("provider not locked in session context: %+v", repo.sessions["s-1"].Context)
	}
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil || chatAudit.Payload["provider_id"] != "provider-2" || chatAudit.Payload["requested_provider_id"] != "provider-2" {
		t.Fatalf("unexpected provider audit: %+v", chatAudit)
	}
}

func TestSendMessageFallsBackWhenProviderUnavailable(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	svc.chat = &fakeChatCatalog{fakeChatClient: chat, models: []toolgateway.ChatModel{
		{ProviderID: "provider-1", Name: "默认模型", Model: "model-1"},
	}}
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析告警", ProviderID: "missing-provider",
	})
	if err != nil {
		t.Fatalf("fallback must not fail the request: %v", err)
	}
	if chat.lastProvider != "provider-1" || result.Status != chatStatusOK {
		t.Fatalf("fallback provider = %q status=%q", chat.lastProvider, result.Status)
	}
	if len(result.Answer.Uncertainties) == 0 || !strings.Contains(result.Answer.Uncertainties[0], "已回退 默认模型") {
		t.Fatalf("missing fallback uncertainty: %+v", result.Answer.Uncertainties)
	}
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil || chatAudit.Payload["provider_fallback"] != true ||
		chatAudit.Payload["requested_provider_id"] != "missing-provider" {
		t.Fatalf("unexpected fallback audit: %+v", chatAudit)
	}
}

func TestSendMessageRateLimitShowsResetHint(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	chat.err = apperr.Wrap(&toolgateway.RateLimitError{
		StatusCode: 429, RetryAfter: time.Second, ResetAfter: 2 * time.Hour,
	}, apperr.CodeUnavailable, "model provider rate limited")
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析系统状态",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusDegraded || result.Answer.DegradedReason != chatDegradedRateLimited {
		t.Fatalf("unexpected answer: %+v", result)
	}
	if len(result.Answer.Uncertainties) == 0 || !strings.Contains(result.Answer.Uncertainties[0], "约 2 小时后恢复") {
		t.Fatalf("missing reset hint: %+v", result.Answer.Uncertainties)
	}
	found := false
	for _, row := range audit.rows {
		if row.Action == AuditChatMessage && row.Payload["degraded_reason"] == chatDegradedRateLimited {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing rate-limited audit: %+v", audit.rows)
	}
}

func TestHumanizeRetryAfter(t *testing.T) {
	cases := map[time.Duration]string{
		0:                "",
		45 * time.Second: "约 45 秒后",
		90 * time.Second: "约 2 分钟后",
		2 * time.Hour:    "约 2 小时后",
		61 * time.Minute: "约 2 小时后",
	}
	for delay, want := range cases {
		if got := humanizeRetryAfter(delay); got != want {
			t.Fatalf("humanizeRetryAfter(%v) = %q, want %q", delay, got, want)
		}
	}
}

func TestSendMessageTurnBudgetExceeded(t *testing.T) {
	svc, repo, _, chat, audit, _ := newChatTestService(t)
	chat.block = true
	svc.SetTurnBudget(50 * time.Millisecond)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	started := time.Now()
	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "需要长时间推理的问题",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatalf("turn budget did not stop the model call")
	}
	if result.Status != chatStatusDegraded || result.Answer.DegradedReason != chatDegradedTurnBudgetExceeded {
		t.Fatalf("unexpected result: %+v", result)
	}
	if chat.calls != 1 {
		t.Fatalf("model calls = %d", chat.calls)
	}
	var chatAudit *AuditRecord
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatMessage {
			chatAudit = &audit.rows[i]
		}
	}
	if chatAudit == nil || chatAudit.Payload["degraded_reason"] != chatDegradedTurnBudgetExceeded {
		t.Fatalf("unexpected audit: %+v", chatAudit)
	}
}

func TestSendMessageStreamEmitsModelDeltas(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	var events []ChatEvent
	_, err := svc.SendMessageStream(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析当前告警",
	}, func(event ChatEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("send stream: %v", err)
	}
	phases := map[string]bool{}
	answerEvents := 0
	for _, event := range events {
		switch event.Type {
		case ChatEventDelta:
			if event.Text == "" {
				t.Fatalf("delta event without text: %+v", event)
			}
			phases[event.Phase] = true
		case ChatEventAnswer:
			answerEvents++
		}
	}
	if !phases[ChatDeltaPhaseThinking] || !phases[ChatDeltaPhaseAnswer] {
		t.Fatalf("missing delta phases: %+v", phases)
	}
	if answerEvents != 1 {
		t.Fatalf("answer events = %d", answerEvents)
	}
}

func TestSendMessageStreamAbortsWhenDeltaSinkFails(t *testing.T) {
	svc, repo, _, _, audit, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := svc.SendMessageStream(ctx, SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析当前告警",
	}, func(event ChatEvent) error {
		if event.Type == ChatEventDelta {
			return errors.New("client closed")
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected aborted error")
	}
	detail, detailErr := svc.GetSession(context.Background(), "u-1", "s-1")
	if detailErr != nil {
		t.Fatalf("get session: %v", detailErr)
	}
	for _, message := range detail.Messages {
		if message.Role == string(domain.RoleAssistant) {
			t.Fatalf("canceled stream must not persist assistant message: %+v", message)
		}
	}
	canceled := false
	for i := range audit.rows {
		if audit.rows[i].Action == AuditChatCanceled {
			canceled = true
		}
	}
	if !canceled {
		t.Fatal("missing chat_canceled audit")
	}
}

type fakeAccountListPort struct {
	result *AccountListResult
	err    error
	userID string
	calls  int
}

func (f *fakeAccountListPort) List(_ context.Context, userID string) (*AccountListResult, error) {
	f.userID = userID
	f.calls++
	return f.result, f.err
}

type fakeAssetReader struct {
	resource   *AssetContext
	err        error
	userID     string
	resourceID string
	calls      int
}

func (f *fakeAssetReader) GetForAnalysis(_ context.Context, userID, resourceID string) (*AssetContext, error) {
	f.userID = userID
	f.resourceID = resourceID
	f.calls++
	return f.resource, f.err
}

type fakeResourceDiscoveryPort struct {
	got    ResourceQuery
	userID string
	result *ResourceListResult
	err    error
	calls  int
}

func (f *fakeResourceDiscoveryPort) List(_ context.Context, userID string, q ResourceQuery) (*ResourceListResult, error) {
	f.userID = userID
	f.got = q
	f.calls++
	return f.result, f.err
}

func testAssetContext() *AssetContext {
	return &AssetContext{
		ResourceID: "res-1", Name: "orders-rds", ApplicationID: "app-1",
		ResourceType: "database", CloudResourceType: "rds", CloudResourceID: "cluster-1",
		Region: "cn-north-4", AccountID: "acc-1",
		Labels: map[string]string{"namespace": "SYS.RDS", "dim_name": "rds_cluster_id"},
	}
}

func TestSendMessageGathersAccountsWithoutAlert(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	svc.SetAccounts(&fakeAccountListPort{result: &AccountListResult{
		Status: EvidenceStatusOK, Total: 2,
		Accounts: []AccountSummary{{
			AccountID: "acc-1", Name: "生产账号", Provider: "huawei_cloud",
			Regions: []string{"cn-north-4"},
		}},
	}})
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "dashboard"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "有哪些可用账号",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if !hasToolCall(result.ToolCalls, chatToolAccountsList, EvidenceStatusOK) {
		t.Fatalf("missing accounts.list observation: %+v", result.ToolCalls)
	}
	if !strings.Contains(chat.user, "acc-1") || !strings.Contains(chat.user, "accounts") {
		t.Fatalf("account list not injected into prompt: %s", chat.user)
	}
}

func TestSendMessageGathersResourceContext(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	reader := &fakeAssetReader{resource: testAssetContext()}
	svc.SetAssets(reader)
	seedChatSession(t, repo, "u-1", domain.SessionContext{
		Page: "assets", ResourceID: "res-1", ResourceType: "rds",
	})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "这个 RDS 指标正常吗",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if reader.calls != 1 || reader.resourceID != "res-1" || reader.userID != "u-1" {
		t.Fatalf("resource reader not used: %+v", reader)
	}
	if !hasToolCall(result.ToolCalls, chatToolResourceAnalyze, EvidenceStatusOK) {
		t.Fatalf("missing resource.analyze observation: %+v", result.ToolCalls)
	}
	for _, want := range []string{"acc-1", "cn-north-4", "SYS.RDS", "rds_cluster_id", "cluster-1"} {
		if !strings.Contains(chat.user, want) {
			t.Fatalf("resource context %q not injected into prompt: %s", want, chat.user)
		}
	}
}

func TestSendMessageResourceScopeDenied(t *testing.T) {
	svc, repo, _, _, audit, _ := newChatTestService(t)
	svc.SetAssets(&fakeAssetReader{err: apperr.New(apperr.CodePermissionDenied, "no access to resource")})
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "assets", ResourceID: "res-1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析这个资源",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	var call *domain.ToolCall
	for i := range result.ToolCalls {
		if result.ToolCalls[i].ToolCode == chatToolResourceAnalyze {
			call = &result.ToolCalls[i]
		}
	}
	if call == nil || call.Status != "denied" || call.Reason != "data_scope_denied" {
		t.Fatalf("resource tool call = %+v", call)
	}
	denied := 0
	for _, row := range audit.rows {
		// 数据范围拒绝属于业务降级：allowed=true、status=denied、result=partial。
		if row.Action == AuditToolInvoke && row.Payload["tool_code"] == chatToolResourceAnalyze &&
			row.Payload["status"] == "denied" && row.Payload["reason"] == "data_scope_denied" {
			denied++
		}
	}
	if denied != 1 {
		t.Fatalf("expected one denied resource audit, got %+v", audit.rows)
	}
}

func TestSendMessageAgentLoopDiscoversResources(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	port := &fakeResourceDiscoveryPort{result: &ResourceListResult{
		Status: EvidenceStatusOK,
		Resources: []ResourceSummary{{
			ResourceID: "res-1", Name: "orders-rds", CloudResourceType: "rds",
			CloudResourceID: "cluster-1", Region: "cn-north-4", AccountID: "acc-1",
			Labels: map[string]string{"namespace": "SYS.RDS", "dim_name": "rds_cluster_id"},
		}},
	}}
	svc.SetResources(port)
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"resources.list","reason":"发现 RDS 实例","params":{"account_id":"acc-1","resource_type":"rds"}}]}`,
		chatAnswerJSON("已找到 RDS 实例"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "dashboard"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "有哪些 RDS 实例",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if port.calls != 1 || port.got.AccountID != "acc-1" || port.got.ResourceType != "rds" {
		t.Fatalf("resource discovery query = %+v", port.got)
	}
	if !hasToolCall(result.ToolCalls, chatToolResourcesList, EvidenceStatusOK) {
		t.Fatalf("missing resources.list observation: %+v", result.ToolCalls)
	}
	if !strings.Contains(chat.user, "SYS.RDS") || !strings.Contains(chat.user, "rds_cluster_id") {
		t.Fatalf("resource list not enriched in prompt: %s", chat.user)
	}
}

func TestSendMessageAgentLoopUsesResourceMetricFallback(t *testing.T) {
	svc, repo, _, chat, _, evidence := newChatTestService(t)
	svc.SetAssets(&fakeAssetReader{resource: testAssetContext()})
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"metrics.query","reason":"看 CPU","params":{"metric":"cpu_util"}}]}`,
		chatAnswerJSON("已按资源上下文取数"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "assets", ResourceID: "res-1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "看下这个实例 CPU",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusOK {
		t.Fatalf("status = %q", result.Status)
	}
	if evidence.got.AccountID != "acc-1" || evidence.got.Region != "cn-north-4" ||
		evidence.got.Namespace != "SYS.RDS" || evidence.got.Metric != "rds001_cpu_util" {
		t.Fatalf("resource fallback not used (cpu_util must normalize): %+v", evidence.got)
	}
	if evidence.got.Dimensions["rds_cluster_id"] != "cluster-1" {
		t.Fatalf("dimensions = %+v", evidence.got.Dimensions)
	}
	if evidence.got.Threshold != nil {
		t.Fatalf("resource fallback must not invent thresholds: %+v", evidence.got)
	}
	if !hasToolCall(result.ToolCalls, chatToolMetricsQuery, "ok") {
		t.Fatalf("missing metrics.query call: %+v", result.ToolCalls)
	}
}

func TestSendMessageAgentLoopAccountsWithoutPortReturnsSkipped(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"accounts.list","reason":"确认账号"}]}`,
		chatAnswerJSON("当前未接入账号发现能力"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "有哪些账号",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	var call *domain.ToolCall
	for i := range result.ToolCalls {
		if result.ToolCalls[i].ToolCode == chatToolAccountsList && result.ToolCalls[i].Status == "skipped" {
			call = &result.ToolCalls[i]
		}
	}
	if call == nil || call.Reason != "account discovery is not configured" {
		t.Fatalf("skipped accounts call = %+v", call)
	}
}

func TestSendMessageAgentLoopResourceWithoutContextReturnsSkipped(t *testing.T) {
	svc, repo, _, chat, _, _ := newChatTestService(t)
	svc.SetAssets(&fakeAssetReader{resource: testAssetContext()})
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"resource.analyze","reason":"读取资源"}]}`,
		chatAnswerJSON("当前页面没有资源上下文"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "dashboard"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析资源",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	var call *domain.ToolCall
	for i := range result.ToolCalls {
		if result.ToolCalls[i].ToolCode == chatToolResourceAnalyze && result.ToolCalls[i].Status == "skipped" {
			call = &result.ToolCalls[i]
		}
	}
	if call == nil || call.Reason != "no_resource" {
		t.Fatalf("skipped resource call = %+v", call)
	}
}

func TestExecuteMetricObservationReportsPointCountAndLabel(t *testing.T) {
	svc, _, _, _, _, evidence := newChatTestService(t)
	evidence.result = &MetricEvidenceResult{Status: EvidenceStatusOK, Metrics: []MetricEvidence{{
		Metric: "rds002_mem_util",
		Points: [][2]float64{{1700000000, 60}, {1700000300, 75}, {1700000600, 91}},
	}}}

	obs, result := svc.executeMetricObservation(context.Background(), "u-1", MetricEvidenceQuery{
		AccountID: "acc-1", Region: "cn-north-4", Namespace: "SYS.RDS",
		Metric: "rds002_mem_util", Dimensions: map[string]string{"rds_cluster_id": "c-1"},
	})
	if obs == nil || result == nil {
		t.Fatal("expected observation and evidence")
	}
	if obs.Data["point_count"] != 3 || obs.Data["metric_label"] != "内存使用率" {
		t.Fatalf("observation data = %+v", obs.Data)
	}
	metrics, ok := obs.Data["metrics"].([]MetricEvidence)
	if !ok || len(metrics) != 1 || len(metrics[0].Points) != 3 {
		t.Fatalf("observation metrics = %+v", obs.Data["metrics"])
	}
	if !strings.Contains(obs.Summary, "3 点") || !strings.Contains(obs.Summary, "内存使用率") {
		t.Fatalf("summary = %q", obs.Summary)
	}

	evidence.result = &MetricEvidenceResult{Status: EvidenceStatusOK, Metrics: []MetricEvidence{{
		Metric: "rds002_mem_util",
	}}}
	zero, _ := svc.executeMetricObservation(context.Background(), "u-1", MetricEvidenceQuery{Metric: "rds002_mem_util"})
	if zero == nil || zero.Data["point_count"] != 0 {
		t.Fatalf("zero-point observation = %+v", zero)
	}
	if !strings.Contains(zero.Summary, "无数据点") {
		t.Fatalf("zero-point summary = %q", zero.Summary)
	}
}

func TestSendMessageAgentLoopPromptCarriesMetricValues(t *testing.T) {
	svc, repo, _, chat, _, evidence := newChatTestService(t)
	evidence.result = &MetricEvidenceResult{Status: EvidenceStatusOK, Metrics: []MetricEvidence{{
		Metric: "cpu_util", Peak: floatPtr(97.4), Last: floatPtr(95.2),
		Points: [][2]float64{{1700000000, 62.1}, {1700000300, 97.4}}, EvidenceID: "ev-1",
	}}}
	chat.script = []string{
		`{"tool_requests":[{"tool_code":"metrics.query","reason":"缺少近期指标","params":{"account_id":"acc-1","region":"cn-north-4","namespace":"SYS.RDS","metric":"cpu_util","dimensions":{"instance_id":"i-1"},"from":1700000000,"to":1700003600,"period":300}}]}`,
		chatAnswerJSON("CPU 趋势上升"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "看下最近趋势",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusOK || chat.calls != 2 {
		t.Fatalf("status = %q, model calls = %d", result.Status, chat.calls)
	}
	// 第二轮 prompt 必须带上第一轮取回的数值，否则模型只看到元数据无法收敛。
	if !strings.Contains(chat.user, `"points"`) || !strings.Contains(chat.user, "97.4") || !strings.Contains(chat.user, `"from":1700000000`) {
		t.Fatalf("round-1 prompt missing metric values: %s", chat.user)
	}
}

func TestExecuteMetricObservationDownsamplesPointsForPrompt(t *testing.T) {
	svc, _, _, _, _, evidence := newChatTestService(t)
	points := make([][2]float64, 1440)
	for i := range points {
		points[i] = [2]float64{float64(1700000000 + i*300), float64(i % 100)}
	}
	evidence.result = &MetricEvidenceResult{Status: EvidenceStatusOK, Metrics: []MetricEvidence{{
		Metric: "cpu_util", Points: points, EvidenceID: "ev-1",
	}}}

	obs, result := svc.executeMetricObservation(context.Background(), "u-1", MetricEvidenceQuery{Metric: "cpu_util"})
	if obs == nil || result == nil {
		t.Fatal("expected observation and evidence")
	}
	if obs.Data["point_count"] != 1440 {
		t.Fatalf("point_count must report pre-cap fact, got %v", obs.Data["point_count"])
	}
	metrics, ok := obs.Data["metrics"].([]MetricEvidence)
	if !ok || len(metrics) != 1 {
		t.Fatalf("observation metrics = %+v", obs.Data["metrics"])
	}
	got := metrics[0].Points
	if len(got) != chatMetricPromptMaxPoints {
		t.Fatalf("prompt points = %d, want %d", len(got), chatMetricPromptMaxPoints)
	}
	if got[0] != points[0] || got[len(got)-1] != points[len(points)-1] {
		t.Fatalf("downsample must keep first/last: %v .. %v", got[0], got[len(got)-1])
	}
	// 返回的 evidence 保留全量点，供引用与动作使用。
	if len(result.Metrics[0].Points) != 1440 {
		t.Fatalf("evidence points = %d, want full 1440", len(result.Metrics[0].Points))
	}
}

func TestSendMessageAgentLoopSameRoundDistinctMetricsBothExecute(t *testing.T) {
	svc, repo, _, chat, _, evidence := newChatTestService(t)
	evidence.result = &MetricEvidenceResult{Status: EvidenceStatusOK, Metrics: []MetricEvidence{{
		Metric: "cpu_util", Peak: floatPtr(97.4),
		Points: [][2]float64{{1700000000, 62.1}, {1700000300, 97.4}}, EvidenceID: "ev-1",
	}}}
	chat.script = []string{
		`{"tool_requests":[` +
			`{"tool_code":"metrics.query","reason":"cpu 趋势","params":{"account_id":"acc-1","region":"cn-north-4","namespace":"SYS.RDS","metric":"cpu_util","dimensions":{"instance_id":"i-1"},"from":1700000000,"to":1700003600,"period":300}},` +
			`{"tool_code":"metrics.query","reason":"mem 趋势","params":{"account_id":"acc-1","region":"cn-north-4","namespace":"SYS.RDS","metric":"mem_util","dimensions":{"instance_id":"i-1"},"from":1700000000,"to":1700003600,"period":300}}` +
			`]}`,
		chatAnswerJSON("CPU 与内存趋势均上升"),
	}
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "看下 cpu 和内存两个指标趋势",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Status != chatStatusOK || chat.calls != 2 || evidence.calls != 2 {
		t.Fatalf("status = %q, model calls = %d, evidence calls = %d", result.Status, chat.calls, evidence.calls)
	}
	metricOK := 0
	for _, call := range result.ToolCalls {
		if call.ToolCode == chatToolMetricsQuery && call.Status == "ok" {
			metricOK++
		}
	}
	if metricOK != 2 {
		t.Fatalf("metrics ok calls = %d, want 2: %+v", metricOK, result.ToolCalls)
	}
	// 同轮不同参数的两个 metrics 都必须执行，且下一轮 prompt 带两组序列。
	if !strings.Contains(chat.user, `"metric":"rds001_cpu_util"`) || !strings.Contains(chat.user, `"metric":"rds002_mem_util"`) {
		t.Fatalf("round-1 prompt missing one of the metric series: %s", chat.user)
	}
}

func TestSendMessageSkipsAccountsWhenPortMissing(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "你好",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	for _, call := range result.ToolCalls {
		if call.ToolCode == chatToolAccountsList || call.ToolCode == chatToolResourceAnalyze {
			t.Fatalf("unexpected discovery tool call without ports: %+v", call)
		}
	}
}
