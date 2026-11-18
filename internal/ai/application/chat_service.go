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
	// chatMaxClientMessageIDRunes 幂等键长度上限（与 ai_session_message.client_message_id 列宽对齐）。
	chatMaxClientMessageIDRunes = 64

	chatDegradedModelUnavailable = "model_unavailable"
	chatDegradedModelFailed      = "model_failed"
	chatDegradedOutputInvalid    = "model_output_invalid"

	chatToolAlarmAnalyze     = "alarm.analyze"
	chatToolMetricsQuery     = "metrics.query"
	chatToolExecutionPropose = "execution.tasks.propose"

	chatStatusOK                  = "ok"
	chatStatusDegraded            = "degraded"
	chatStatusInsufficientContext = "insufficient_context"

	// chatCancelAuditTimeout 取消审计的独立预算：请求 ctx 已取消时仍要尽量落审计。
	chatCancelAuditTimeout = 3 * time.Second
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
	// ClientMessageID 是可选幂等键：同一会话内重复提交相同值时返回首次结果，
	// 用于客户端超时重试（migration 0050 的部分唯一索引兜底并发）。
	ClientMessageID string
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
	// memory 为可选的相似案例记忆能力；nil 表示关闭（回答与未接入记忆时一致）。
	memory MemoryCaseStore
	// knowledge 为可选的知识沉淀能力；nil 表示关闭（回答与未接入知识时一致）。
	knowledge KnowledgeStore
	// shares 为可选的会话只读分享能力；nil 表示未接入（分享接口返回 503）。
	shares domain.SessionShareRepository
	now    func() time.Time
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

// SendMessage 发送用户消息并同步返回结构化回答（非流式）。
func (s *ChatService) SendMessage(ctx context.Context, in SendChatMessageInput) (*SendChatMessageResult, error) {
	return s.sendMessage(ctx, in, nil)
}

// SendMessageStream 以事件流方式执行同一编排：阶段、工具观测、最终答案与错误逐条推送。
//
// emit 返回错误（通常是客户端断开）时立即停止后续工作：已落库的用户消息保留，
// 但不写 assistant 消息，并记一条 chat_cancelled 审计。
func (s *ChatService) SendMessageStream(ctx context.Context, in SendChatMessageInput, emit ChatEventSink) (*SendChatMessageResult, error) {
	if emit == nil {
		return s.sendMessage(ctx, in, nil)
	}
	return s.sendMessage(withChatEventSink(ctx, emit), in, emit)
}

// sendMessage 是同步与流式共用的编排实现；emit 为 nil 表示非流式。
func (s *ChatService) sendMessage(ctx context.Context, in SendChatMessageInput, emit ChatEventSink) (*SendChatMessageResult, error) {
	if err := emitChatStage(ctx, ChatStageGathering); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeAborted, "client closed the stream")
	}
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
	clientMessageID := strings.TrimSpace(in.ClientMessageID)
	if len([]rune(clientMessageID)) > chatMaxClientMessageIDRunes {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "client_message_id exceeds %d characters", chatMaxClientMessageIDRunes)
	}
	session, err := s.loadOwnedSession(ctx, userID, in.SessionID)
	if err != nil {
		return nil, err
	}
	// 幂等：同一 client_message_id 的重试直接复用上一次结果，不再落消息、不再调模型。
	// 重放是只读操作，因此放在归档校验之前：会话事后被归档也不影响已生成结果的重放。
	if clientMessageID != "" {
		existing, findErr := s.findUserMessageByClientID(ctx, session.SessionID, clientMessageID)
		if findErr != nil {
			return nil, findErr
		}
		if existing != nil {
			return s.replayChatMessage(ctx, session, existing)
		}
	}
	if !session.CanSendMessage() {
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
		Content: content, Feedback: domain.FeedbackNone, ClientMessageID: clientMessageID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.sessions.AppendMessage(ctx, userMessage, touch); err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "session not found")
		}
		// 并发重复提交同一 client_message_id：退回幂等分支，返回已有结果或冲突。
		if errors.Is(err, domain.ErrDuplicateClientMessage) {
			existing, findErr := s.findUserMessageByClientID(ctx, session.SessionID, clientMessageID)
			if findErr != nil {
				return nil, findErr
			}
			if existing != nil {
				return s.replayChatMessage(ctx, session, existing)
			}
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "persist ai chat message failed")
	}

	if canceledErr := chatCanceledErr(ctx); canceledErr != nil {
		s.recordChatCanceled(ctx, userID, session, userMessageID, "before_gather")
		return nil, canceledErr
	}
	contextData := s.gatherContext(ctx, userID, session, userMessageID)
	// 相似案例记忆：命中则注入 Prompt 与 references，失败降级不阻断回答。
	s.injectMemory(ctx, userID, session, content, &contextData)
	// 知识沉淀（阶段 2）：生效条目进 system prompt 附加段与 references。
	s.injectKnowledge(ctx, session, &contextData)
	if err := emitChatStage(ctx, ChatStageThinking); err != nil {
		s.recordChatCanceled(ctx, userID, session, userMessageID, "before_model")
		return nil, apperr.Wrap(err, apperr.CodeAborted, "client closed the stream")
	}
	generation := s.generateAnswer(ctx, userID, content, session, history, &contextData)
	// 客户端断开或请求超时：不落 assistant 消息，避免出现「无证据的半成品回答」。
	if canceledErr := chatCanceledErr(ctx); canceledErr != nil {
		s.recordChatCanceled(ctx, userID, session, userMessageID, "before_persist")
		return nil, canceledErr
	}
	if err := emitChatStage(ctx, ChatStageAnswering); err != nil {
		s.recordChatCanceled(ctx, userID, session, userMessageID, "before_answer")
		return nil, apperr.Wrap(err, apperr.CodeAborted, "client closed the stream")
	}
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
	// 命中统计：注入过的知识条目 hit_count +1（失败只记日志）。
	s.recordKnowledgeInjection(ctx, contextData.KnowledgeItems)
	s.recordAudit(ctx, userID, "ai", session.SessionID, AuditChatMessage, map[string]any{
		"session_id":       session.SessionID,
		"message_id":       assistantID,
		"status":           generation.Status,
		"risk_level":       generation.Answer.RiskLevel,
		"require_confirm":  generation.Answer.RequireConfirm,
		"tool_codes":       toolCodes(contextData.ToolCalls),
		"tool_denied":      deniedToolCodes(contextData.ToolCalls),
		"reference_count":  len(contextData.References),
		"provider_id":      generation.ProviderID,
		"tokens":           generation.Tokens,
		"degraded_reason":  generation.Answer.DegradedReason,
		"memory_used":      len(contextData.Memories),
		"memory_status":    contextData.MemoryStatus,
		"knowledge_used":   len(contextData.KnowledgeItems),
		"knowledge_status": contextData.KnowledgeStatus,
		"loop_rounds":      contextData.LoopRounds,
		"result":           auditResult(generation.Status),
	})
	result := &SendChatMessageResult{
		SessionID: session.SessionID, MessageID: assistantID, Status: generation.Status,
		Answer: generation.Answer, ToolCalls: contextData.ToolCalls, References: contextData.References,
		Context: session.Context, CreatedAt: assistantMessage.CreatedAt,
	}
	// 流式通道存在时推送最终答案；推送失败说明客户端已断开，但答案已落库，不再回滚。
	_ = emitChatEvent(ctx, ChatEvent{Type: ChatEventAnswer, Result: result})
	return result, nil
}

// chatCanceledErr 把 ctx 取消/超时统一转换为可返回的业务错误；未取消返回 nil。
func chatCanceledErr(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return apperr.Wrap(err, apperr.CodeAborted, "ai chat request was canceled")
	}
	return nil
}

// recordChatCanceled 记录一次被取消的对话（不写 assistant 消息，审计保留取消事实）。
func (s *ChatService) recordChatCanceled(ctx context.Context, userID string, session *domain.Session, userMessageID, phase string) {
	if s == nil || session == nil {
		return
	}
	// 取消后请求 ctx 已失效，审计必须脱链写入并带独立超时，否则 chat_canceled 会随取消一起丢失。
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), chatCancelAuditTimeout)
	defer cancel()
	s.recordAudit(auditCtx, userID, "ai", session.SessionID, AuditChatCanceled, map[string]any{
		"session_id":      session.SessionID,
		"user_message_id": userMessageID,
		"phase":           phase,
		"result":          "canceled",
	})
}

// findUserMessageByClientID 查询幂等键对应的用户消息；仓储错误统一转换为业务错误码。
func (s *ChatService) findUserMessageByClientID(ctx context.Context, sessionID, clientMessageID string) (*domain.Message, error) {
	message, err := s.sessions.FindUserMessageByClientID(ctx, sessionID, clientMessageID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load ai chat message by client id failed")
	}
	return message, nil
}

// replayChatMessage 幂等键命中时回放首次结果；流式通道存在时补推一条 answer 事件，
// 避免客户端只收到阶段事件后一直等待。
func (s *ChatService) replayChatMessage(ctx context.Context, session *domain.Session, userMessage *domain.Message) (*SendChatMessageResult, error) {
	result, err := s.replayOrConflict(ctx, session, userMessage)
	if err == nil && result != nil {
		_ = emitChatEvent(ctx, ChatEvent{Type: ChatEventAnswer, Result: result})
	}
	return result, err
}

// replayOrConflict 处理幂等键命中：
//   - 已有对应回答：原样返回首次结果（不落库、不调模型、不重复写审计之外的副作用）；
//   - 尚未产生回答：返回 409，提示上一次请求仍在处理或已中断，客户端应刷新会话。
func (s *ChatService) replayOrConflict(ctx context.Context, session *domain.Session, userMessage *domain.Message) (*SendChatMessageResult, error) {
	assistant, err := s.sessions.FindAssistantAnswer(ctx, session.SessionID, userMessage.MessageID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load ai chat answer failed")
	}
	if assistant == nil {
		return nil, apperr.New(apperr.CodeFailedPrecondition,
			"the same client_message_id is still being processed; refresh the session before retrying")
	}
	answer, parseErr := parseChatAnswer(assistant.Content)
	if parseErr != nil {
		return nil, apperr.New(apperr.CodeInternal, "stored ai chat answer is invalid")
	}
	status := chatResultStatus(answer)
	s.recordAudit(ctx, session.UserID, "ai", session.SessionID, AuditChatMessage, map[string]any{
		"session_id":        session.SessionID,
		"message_id":        assistant.MessageID,
		"client_message_id": userMessage.ClientMessageID,
		"status":            status,
		"risk_level":        answer.RiskLevel,
		"require_confirm":   answer.RequireConfirm,
		"tool_codes":        toolCodes(assistant.ToolCalls),
		"tool_denied":       deniedToolCodes(assistant.ToolCalls),
		"reference_count":   len(assistant.References),
		"degraded_reason":   answer.DegradedReason,
		"replayed":          true,
		"result":            auditResult(status),
	})
	return &SendChatMessageResult{
		SessionID: session.SessionID, MessageID: assistant.MessageID, Status: status,
		Answer: answer, ToolCalls: assistant.ToolCalls, References: assistant.References,
		Context: session.Context, CreatedAt: assistant.CreatedAt,
	}, nil
}

// ArchiveSession 归档会话：归档后只读，仍可查看历史，再次发消息返回 409。
// 已归档时幂等返回当前视图。审计 action=chat_session_status。
func (s *ChatService) ArchiveSession(ctx context.Context, userID, sessionID string) (*ChatSessionView, error) {
	return s.setSessionStatus(ctx, userID, sessionID, domain.SessionStatusArchived)
}

// UnarchiveSession 恢复归档会话，使其可以继续对话；未归档时幂等返回当前视图。
func (s *ChatService) UnarchiveSession(ctx context.Context, userID, sessionID string) (*ChatSessionView, error) {
	return s.setSessionStatus(ctx, userID, sessionID, domain.SessionStatusActive)
}

// DeleteSession 删除会话及其全部消息；仅会话归属用户可删除，审计 action=chat_session_delete。
func (s *ChatService) DeleteSession(ctx context.Context, userID, sessionID string) error {
	if s == nil || s.sessions == nil {
		return apperr.New(apperr.CodeUnavailable, "ai chat service is not enabled")
	}
	session, err := s.loadOwnedSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	if err := s.sessions.Delete(ctx, session.SessionID); err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			return apperr.New(apperr.CodeNotFound, "session not found")
		}
		return apperr.Wrap(err, apperr.CodeInternal, "delete ai chat session failed")
	}
	s.recordAudit(ctx, strings.TrimSpace(userID), "ai", session.SessionID, AuditChatSessionDelete, map[string]any{
		"session_id": session.SessionID,
		"status":     string(session.Status),
		"result":     "success",
	})
	return nil
}

// setSessionStatus 统一处理归档/恢复：归属校验 -> 状态流转（幂等）-> 审计 -> 返回最新视图。
func (s *ChatService) setSessionStatus(ctx context.Context, userID, sessionID string, target domain.SessionStatus) (*ChatSessionView, error) {
	if s == nil || s.sessions == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai chat service is not enabled")
	}
	if !target.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "unsupported session status")
	}
	session, err := s.loadOwnedSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	previous := session.Status
	if session.Status != target {
		if err := s.sessions.SetStatus(ctx, session.SessionID, target, now); err != nil {
			if errors.Is(err, domain.ErrSessionNotFound) {
				return nil, apperr.New(apperr.CodeNotFound, "session not found")
			}
			return nil, apperr.Wrap(err, apperr.CodeInternal, "update ai chat session status failed")
		}
		session.Status = target
		session.UpdatedAt = now
	}
	view := toSessionView(*session)
	s.recordAudit(ctx, strings.TrimSpace(userID), "ai", session.SessionID, AuditChatSessionStatus, map[string]any{
		"session_id":      session.SessionID,
		"status":          string(session.Status),
		"previous_status": string(previous),
		"result":          "success",
	})
	return &view, nil
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
	// 反馈回流记忆：有效反馈沉淀案例、无效反馈降分淘汰（失败只记 Warn）。
	s.syncMemoryFeedback(ctx, session, message, target)
	// 反馈回流知识：有效 → useful_count +1；无效 → 低效评估（可能自动停用）。
	s.syncKnowledgeFeedback(ctx, message, target)
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
