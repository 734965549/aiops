package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

const (
	// chatPermission 与 HTTP 路由 app:ai:chat 保持一致；工具调用二次校验使用同一权限码。
	chatPermission = "app:ai:chat"
	// chatMaxContentRunes 单条用户消息长度上限。
	chatMaxContentRunes = 4000

	chatDegradedModelUnavailable = "model_unavailable"
	chatDegradedModelFailed      = "model_failed"
	chatDegradedOutputInvalid    = "model_output_invalid"

	chatToolAlarmAnalyze     = "alarm.analyze"
	chatToolMetricsQuery     = "metrics.query"
	chatToolExecutionPropose = "execution.tasks.propose"

	chatStatusOK                  = "ok"
	chatStatusDegraded            = "degraded"
	chatStatusInsufficientContext = "insufficient_context"
)

// ChatToolGateway 是工具权限校验端口；ProviderRegistry 之外只暴露校验能力。
type ChatToolGateway interface {
	Validate(ctx context.Context, req toolgateway.ToolRequest) (*toolgateway.ToolResponse, error)
}

// CreateChatSessionInput 创建会话入参。
type CreateChatSessionInput struct {
	UserID  string
	Title   string
	Context domain.SessionContext
}

// ChatSessionView 会话摘要（列表 / 创建响应）。
type ChatSessionView struct {
	SessionID string                `json:"session_id"`
	Title     string                `json:"title"`
	Status    string                `json:"status"`
	Context   domain.SessionContext `json:"context"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
}

// ChatMessageView 单条消息（assistant 消息带结构化 answer）。
type ChatMessageView struct {
	MessageID  string             `json:"message_id"`
	SessionID  string             `json:"session_id"`
	Role       string             `json:"role"`
	Content    string             `json:"content"`
	Answer     *ChatAnswer        `json:"answer,omitempty"`
	ToolCalls  []domain.ToolCall  `json:"tool_calls"`
	References []domain.Reference `json:"references"`
	Feedback   string             `json:"feedback,omitempty"`
	CreatedAt  time.Time          `json:"created_at"`
}

// ChatSessionDetail 会话详情（含历史消息）。
type ChatSessionDetail struct {
	Session  ChatSessionView   `json:"session"`
	Messages []ChatMessageView `json:"messages"`
}

// SendChatMessageInput 发送消息入参。
type SendChatMessageInput struct {
	UserID    string
	SessionID string
	Content   string
	// Context 非 nil 时覆盖会话上下文（浮层在任意页面唤起）。
	Context *domain.SessionContext
}

// SendChatMessageResult 同步返回 AI 回答（流式为 P1+）。
type SendChatMessageResult struct {
	SessionID  string                `json:"session_id"`
	MessageID  string                `json:"message_id"`
	Status     string                `json:"status"`
	Answer     ChatAnswer            `json:"answer"`
	ToolCalls  []domain.ToolCall     `json:"tool_calls"`
	References []domain.Reference    `json:"references"`
	Context    domain.SessionContext `json:"context"`
	CreatedAt  time.Time             `json:"created_at"`
}

// ChatService 编排 AI 运维助手对话（流程图 §16）：上下文组装 -> 工具权限校验与只读取数
// -> 写工具审计 -> 模型汇总 -> 结构化回答落库。
type ChatService struct {
	sessions domain.SessionRepository
	alerts   AlertReader
	tools    ChatToolGateway
	chat     toolgateway.ChatClient
	evidence MetricEvidencePort
	audit    AuditRecorder
	now      func() time.Time
}

// NewChatService 构造对话服务；可选依赖为 nil 时对应能力降级。
func NewChatService(
	sessions domain.SessionRepository,
	alerts AlertReader,
	tools ChatToolGateway,
	chat toolgateway.ChatClient,
	evidence MetricEvidencePort,
	audit AuditRecorder,
) *ChatService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &ChatService{
		sessions: sessions, alerts: alerts, tools: tools, chat: chat,
		evidence: evidence, audit: audit, now: time.Now,
	}
}

// CreateSession 创建会话并写入 AI 审计。
func (s *ChatService) CreateSession(ctx context.Context, in CreateChatSessionInput) (*ChatSessionView, error) {
	if s == nil || s.sessions == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai chat service is not enabled")
	}
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "missing user identity")
	}
	now := s.now()
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "新会话"
	}
	session := domain.Session{
		SessionID: uuid.NewString(),
		UserID:    userID,
		Title:     domain.NewSessionTitle(title),
		Context:   in.Context.Normalize(),
		Status:    domain.SessionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "create ai chat session failed")
	}
	s.recordAudit(ctx, userID, "ai", session.SessionID, AuditChatSession, map[string]any{
		"session_id": session.SessionID,
		"page":       session.Context.Page,
		"alert_id":   session.Context.AlertID,
		"result":     "success",
	})
	view := toSessionView(session)
	return &view, nil
}

// ListSessions 返回当前用户最近的会话。
func (s *ChatService) ListSessions(ctx context.Context, userID string, limit int) ([]ChatSessionView, error) {
	if s == nil || s.sessions == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai chat service is not enabled")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "missing user identity")
	}
	rows, err := s.sessions.ListByUser(ctx, userID, limit)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list ai chat sessions failed")
	}
	out := make([]ChatSessionView, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSessionView(row))
	}
	return out, nil
}

// GetSession 返回会话与历史消息；会话不属于当前用户时按权限不足处理。
func (s *ChatService) GetSession(ctx context.Context, userID, sessionID string) (*ChatSessionDetail, error) {
	if s == nil || s.sessions == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai chat service is not enabled")
	}
	session, err := s.loadOwnedSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	rows, err := s.sessions.ListRecentMessages(ctx, session.SessionID, 200)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list ai chat messages failed")
	}
	messages := make([]ChatMessageView, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageView(row))
	}
	return &ChatSessionDetail{Session: toSessionView(*session), Messages: messages}, nil
}

// SendMessage 发送用户消息并同步返回结构化回答。
func (s *ChatService) SendMessage(ctx context.Context, in SendChatMessageInput) (*SendChatMessageResult, error) {
	if s == nil || s.sessions == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai chat service is not enabled")
	}
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "missing user identity")
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "content is required")
	}
	if len([]rune(content)) > chatMaxContentRunes {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "content exceeds %d characters", chatMaxContentRunes)
	}
	session, err := s.loadOwnedSession(ctx, userID, in.SessionID)
	if err != nil {
		return nil, err
	}
	if session.Status == domain.SessionStatusArchived {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "session is archived")
	}
	history, err := s.loadHistory(ctx, session.SessionID)
	if err != nil {
		return nil, err
	}

	now := s.now()
	userMessageID := uuid.NewString()
	touch := domain.SessionTouch{UpdatedAt: now}
	if strings.TrimSpace(session.Title) == "" || session.Title == "新会话" {
		touch.Title = domain.NewSessionTitle(content)
	}
	if in.Context != nil {
		normalized := in.Context.Normalize()
		session.Context = normalized
		touch.Context = &normalized
	}
	userMessage := domain.Message{
		MessageID: userMessageID, SessionID: session.SessionID, Role: domain.RoleUser,
		Content: content, Feedback: domain.FeedbackNone, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.sessions.AppendMessage(ctx, userMessage, touch); err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "session not found")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "persist ai chat message failed")
	}

	contextData := s.gatherContext(ctx, userID, session, userMessageID)
	generation := s.generateAnswer(ctx, content, session, history, contextData)
	s.finalizeAnswer(ctx, userID, session, userMessageID, &contextData, &generation)

	assistantID := uuid.NewString()
	answerJSON, err := json.Marshal(generation.Answer)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "encode ai chat answer failed")
	}
	assistantMessage := domain.Message{
		MessageID: assistantID, SessionID: session.SessionID, Role: domain.RoleAssistant,
		Content: string(answerJSON), ToolCalls: contextData.ToolCalls,
		References: contextData.References, Feedback: domain.FeedbackNone,
		CreatedAt: s.now(), UpdatedAt: s.now(),
	}
	if err := s.sessions.AppendMessage(ctx, assistantMessage, domain.SessionTouch{UpdatedAt: assistantMessage.UpdatedAt}); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "persist ai chat answer failed")
	}
	s.recordAudit(ctx, userID, "ai", session.SessionID, AuditChatMessage, map[string]any{
		"session_id":      session.SessionID,
		"message_id":      assistantID,
		"status":          generation.Status,
		"risk_level":      generation.Answer.RiskLevel,
		"require_confirm": generation.Answer.RequireConfirm,
		"tool_codes":      toolCodes(contextData.ToolCalls),
		"tool_denied":     deniedToolCodes(contextData.ToolCalls),
		"reference_count": len(contextData.References),
		"provider_id":     generation.ProviderID,
		"tokens":          generation.Tokens,
		"degraded_reason": generation.Answer.DegradedReason,
		"result":          auditResult(generation.Status),
	})
	return &SendChatMessageResult{
		SessionID: session.SessionID, MessageID: assistantID, Status: generation.Status,
		Answer: generation.Answer, ToolCalls: contextData.ToolCalls, References: contextData.References,
		Context: session.Context, CreatedAt: assistantMessage.CreatedAt,
	}, nil
}

// SubmitFeedback 记录用户对 AI 回答的有效/无效反馈。
func (s *ChatService) SubmitFeedback(ctx context.Context, userID, sessionID, messageID, feedback string) (*ChatMessageView, error) {
	if s == nil || s.sessions == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai chat service is not enabled")
	}
	target := domain.Feedback(strings.ToLower(strings.TrimSpace(feedback)))
	if target != domain.FeedbackValid && target != domain.FeedbackInvalid {
		return nil, apperr.New(apperr.CodeInvalidArgument, "feedback must be valid or invalid")
	}
	session, err := s.loadOwnedSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	message, err := s.sessions.GetMessage(ctx, session.SessionID, messageID)
	if err != nil {
		if errors.Is(err, domain.ErrMessageNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "message not found")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load ai chat message failed")
	}
	if message.Role != domain.RoleAssistant {
		return nil, apperr.New(apperr.CodeInvalidArgument, "feedback is only supported for assistant messages")
	}
	userID = strings.TrimSpace(userID)
	if err := s.sessions.SetFeedback(ctx, session.SessionID, message.MessageID, target, s.now()); err != nil {
		if errors.Is(err, domain.ErrMessageNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "message not found")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "update ai chat feedback failed")
	}
	message.Feedback = target
	view := toMessageView(*message)
	s.recordAudit(ctx, userID, "ai", session.SessionID, AuditChatFeedback, map[string]any{
		"session_id": session.SessionID,
		"message_id": message.MessageID,
		"feedback":   string(target),
		"result":     "success",
	})
	return &view, nil
}

func (s *ChatService) loadOwnedSession(ctx context.Context, userID, sessionID string) (*domain.Session, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "missing user identity")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "session_id is required")
	}
	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "session not found")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load ai chat session failed")
	}
	if !strings.EqualFold(session.UserID, userID) {
		return nil, apperr.New(apperr.CodePermissionDenied, "session does not belong to current user")
	}
	return session, nil
}

func (s *ChatService) loadHistory(ctx context.Context, sessionID string) ([]ChatHistoryMessage, error) {
	rows, err := s.sessions.ListRecentMessages(ctx, sessionID, chatHistoryLimit)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list ai chat history failed")
	}
	out := make([]ChatHistoryMessage, 0, len(rows))
	for _, row := range rows {
		content := row.Content
		if row.Role == domain.RoleAssistant {
			if answer, err := parseChatAnswer(row.Content); err == nil {
				content = answer.Summary
			}
		}
		out = append(out, ChatHistoryMessage{Role: string(row.Role), Content: content})
	}
	return out, nil
}
