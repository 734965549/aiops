package http

import (
	"context"
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
	SubmitFeedback(ctx context.Context, userID, sessionID, messageID, feedback string) (*aiapp.ChatMessageView, error)
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
	TimeRange     string `json:"time_range"`
}

func (r chatContextRequest) toDomain() domain.SessionContext {
	return domain.SessionContext{
		Page: r.Page, PageTitle: r.PageTitle, AlertID: r.AlertID,
		ApplicationID: r.ApplicationID, ResourceID: r.ResourceID,
		ResourceType: r.ResourceType, TimeRange: r.TimeRange,
	}.Normalize()
}

type createChatSessionRequest struct {
	Title   string              `json:"title"`
	Context *chatContextRequest `json:"context"`
}

type sendChatMessageRequest struct {
	Content string              `json:"content" binding:"required"`
	Context *chatContextRequest `json:"context"`
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
