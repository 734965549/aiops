package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// ChatUseCase 是 AI 助手对话用例端口（*application.ChatService 实现），便于 HTTP 层单测。
type ChatUseCase interface {
	CreateSession(ctx context.Context, in aiapp.CreateChatSessionInput) (*aiapp.ChatSessionView, error)
	ListSessions(ctx context.Context, userID string, limit int) ([]aiapp.ChatSessionView, error)
	GetSession(ctx context.Context, userID, sessionID string) (*aiapp.ChatSessionDetail, error)
	SendMessage(ctx context.Context, in aiapp.SendChatMessageInput) (*aiapp.SendChatMessageResult, error)
	// SendMessageStream 以事件流方式执行同一编排（emit 收到 stage/tool/answer/error 事件）。
	SendMessageStream(ctx context.Context, in aiapp.SendChatMessageInput, emit aiapp.ChatEventSink) (*aiapp.SendChatMessageResult, error)
	SubmitFeedback(ctx context.Context, userID, sessionID, messageID, feedback string) (*aiapp.ChatMessageView, error)
	ArchiveSession(ctx context.Context, userID, sessionID string) (*aiapp.ChatSessionView, error)
	UnarchiveSession(ctx context.Context, userID, sessionID string) (*aiapp.ChatSessionView, error)
	DeleteSession(ctx context.Context, userID, sessionID string) error
	// 会话只读分享（ops/ai-contract.md §10.18）。
	CreateShare(ctx context.Context, in aiapp.CreateChatShareInput) (*aiapp.CreatedChatShare, error)
	ListShares(ctx context.Context, userID, sessionID string) ([]aiapp.ChatShareView, error)
	RevokeShare(ctx context.Context, userID, sessionID, shareID string) (*aiapp.ChatShareView, error)
	ViewSharedSession(ctx context.Context, userID, token string) (*aiapp.ChatSessionDetail, error)
}

// ChatHandler 是 AI 运维助手会话与对话的 HTTP 层。
type ChatHandler struct {
	chat ChatUseCase
}

func NewChatHandler(chat ChatUseCase) *ChatHandler {
	return &ChatHandler{chat: chat}
}

type chatContextRequest struct {
	Page          string `json:"page"`
	PageTitle     string `json:"page_title"`
	AlertID       string `json:"alert_id"`
	ApplicationID string `json:"application_id"`
	ResourceID    string `json:"resource_id"`
	ResourceType  string `json:"resource_type"`
	ExecutionID   string `json:"execution_id"`
	TimeRange     string `json:"time_range"`
}

func (r chatContextRequest) toDomain() domain.SessionContext {
	return domain.SessionContext{
		Page: r.Page, PageTitle: r.PageTitle, AlertID: r.AlertID,
		ApplicationID: r.ApplicationID, ResourceID: r.ResourceID,
		ResourceType: r.ResourceType, ExecutionID: r.ExecutionID, TimeRange: r.TimeRange,
	}.Normalize()
}

type createChatSessionRequest struct {
	Title   string              `json:"title"`
	Context *chatContextRequest `json:"context"`
}

type sendChatMessageRequest struct {
	Content string              `json:"content" binding:"required"`
	Context *chatContextRequest `json:"context"`
	// ClientMessageID 为可选幂等键，客户端超时重试时复用同一值（ops/ai-contract.md §10.6.5）。
	ClientMessageID string `json:"client_message_id"`
}

type chatFeedbackRequest struct {
	Feedback string `json:"feedback" binding:"required"`
}

func (h *ChatHandler) service() (ChatUseCase, bool) {
	if h == nil || h.chat == nil {
		return nil, false
	}
	return h.chat, true
}

// CreateSession POST /api/ai/chat/sessions
func (h *ChatHandler) CreateSession(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	var req createChatSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid chat session request")
		return
	}
	var context domain.SessionContext
	if req.Context != nil {
		context = req.Context.toDomain()
	}
	view, err := svc.CreateSession(c.Request.Context(), aiapp.CreateChatSessionInput{
		UserID: c.GetString(middleware.CtxKeyUserID), Title: req.Title, Context: context,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// ListSessions GET /api/ai/chat/sessions
func (h *ChatHandler) ListSessions(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 100 {
			httpx.FailWith(c, apperr.CodeInvalidArgument, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	items, err := svc.ListSessions(c.Request.Context(), c.GetString(middleware.CtxKeyUserID), limit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

// GetSession GET /api/ai/chat/sessions/:session_id
func (h *ChatHandler) GetSession(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	detail, err := svc.GetSession(c.Request.Context(), c.GetString(middleware.CtxKeyUserID), c.Param("session_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, detail)
}

// SendMessage POST /api/ai/chat/sessions/:session_id/messages
func (h *ChatHandler) SendMessage(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	var req sendChatMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "content is required")
		return
	}
	in := aiapp.SendChatMessageInput{
		UserID: c.GetString(middleware.CtxKeyUserID), SessionID: c.Param("session_id"), Content: req.Content,
		ClientMessageID: req.ClientMessageID,
	}
	if req.Context != nil {
		context := req.Context.toDomain()
		in.Context = &context
	}
	out, err := svc.SendMessage(c.Request.Context(), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// SendMessageStream POST /api/ai/chat/sessions/:session_id/messages/stream
//
// 以 SSE 推送事件：stage（阶段）、tool（工具观测）、answer（最终结构化回答）、error（业务错误）。
// 客户端断开（HTTP ctx 取消）时服务端停止后续工作，不落 assistant 消息，并记 chat_canceled 审计。
func (h *ChatHandler) SendMessageStream(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	var req sendChatMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "content is required")
		return
	}
	in := aiapp.SendChatMessageInput{
		UserID: c.GetString(middleware.CtxKeyUserID), SessionID: c.Param("session_id"), Content: req.Content,
		ClientMessageID: req.ClientMessageID,
	}
	if req.Context != nil {
		context := req.Context.toDomain()
		in.Context = &context
	}
	// SSE 头：禁用缓冲，避免 nginx 把事件攒到最后一次下发。
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, _ := c.Writer.(http.Flusher)

	writeEvent := func(event aiapp.ChatEvent) error {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := c.Writer.WriteString("data: " + string(raw) + "\n\n"); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}

	if _, err := svc.SendMessageStream(c.Request.Context(), in, writeEvent); err != nil {
		// 客户端已断开：HTTP 状态与响应体都已成形，直接结束。
		if c.Request.Context().Err() != nil {
			return
		}
		// 其它业务错误：状态码已是 200，只能把错误作为事件推给客户端。
		_ = writeEvent(aiapp.ChatEvent{
			Type: aiapp.ChatEventError, Code: string(apperr.CodeOf(err)), Message: err.Error(),
		})
	}
}

// SubmitFeedback POST /api/ai/chat/sessions/:session_id/messages/:message_id/feedback
func (h *ChatHandler) SubmitFeedback(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	var req chatFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "feedback is required")
		return
	}
	view, err := svc.SubmitFeedback(c.Request.Context(),
		c.GetString(middleware.CtxKeyUserID), c.Param("session_id"), c.Param("message_id"), req.Feedback)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// ArchiveSession POST /api/ai/chat/sessions/:session_id/archive
func (h *ChatHandler) ArchiveSession(c *gin.Context) {
	h.changeStatus(c, func(svc ChatUseCase, userID, sessionID string) (*aiapp.ChatSessionView, error) {
		return svc.ArchiveSession(c.Request.Context(), userID, sessionID)
	})
}

// UnarchiveSession POST /api/ai/chat/sessions/:session_id/unarchive
func (h *ChatHandler) UnarchiveSession(c *gin.Context) {
	h.changeStatus(c, func(svc ChatUseCase, userID, sessionID string) (*aiapp.ChatSessionView, error) {
		return svc.UnarchiveSession(c.Request.Context(), userID, sessionID)
	})
}

// DeleteSession DELETE /api/ai/chat/sessions/:session_id
func (h *ChatHandler) DeleteSession(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	if err := svc.DeleteSession(c.Request.Context(), c.GetString(middleware.CtxKeyUserID), c.Param("session_id")); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}

// changeStatus 收敛归档/恢复两个接口的公共处理，避免重复鉴权分支。
func (h *ChatHandler) changeStatus(c *gin.Context, apply func(ChatUseCase, string, string) (*aiapp.ChatSessionView, error)) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	view, err := apply(svc, c.GetString(middleware.CtxKeyUserID), c.Param("session_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// ---- 会话只读分享（ops/ai-contract.md §10.18）----

type createChatShareRequest struct {
	// ExpiresInHours 可选；缺省 168（7 天），允许 1–720 小时。
	ExpiresInHours int `json:"expires_in_hours"`
}

// CreateShare POST /api/ai/chat/sessions/:session_id/shares
func (h *ChatHandler) CreateShare(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	var req createChatShareRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid share payload")
			return
		}
	}
	created, err := svc.CreateShare(c.Request.Context(), aiapp.CreateChatShareInput{
		UserID:         c.GetString(middleware.CtxKeyUserID),
		SessionID:      c.Param("session_id"),
		ExpiresInHours: req.ExpiresInHours,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, created)
}

// ListShares GET /api/ai/chat/sessions/:session_id/shares
func (h *ChatHandler) ListShares(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	shares, err := svc.ListShares(c.Request.Context(), c.GetString(middleware.CtxKeyUserID), c.Param("session_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"shares": shares})
}

// RevokeShare DELETE /api/ai/chat/sessions/:session_id/shares/:share_id
func (h *ChatHandler) RevokeShare(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	view, err := svc.RevokeShare(c.Request.Context(),
		c.GetString(middleware.CtxKeyUserID), c.Param("session_id"), c.Param("share_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

// ViewSharedSession GET /api/ai/chat/shares/:token
//
// 只读：返回会话与历史消息，不提供发消息、反馈、归档等写入口。
func (h *ChatHandler) ViewSharedSession(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai chat service is not enabled")
		return
	}
	detail, err := svc.ViewSharedSession(c.Request.Context(),
		c.GetString(middleware.CtxKeyUserID), c.Param("token"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, detail)
}
