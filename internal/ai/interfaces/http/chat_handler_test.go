package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type fakeChatUseCase struct {
	created     aiapp.CreateChatSessionInput
	sent        aiapp.SendChatMessageInput
	feedback    [3]string
	createErr   error
	sendErr     error
	listErr     error
	detailErr   error
	feedbackErr error
	listLimit   int
}

func (f *fakeChatUseCase) CreateSession(_ context.Context, in aiapp.CreateChatSessionInput) (*aiapp.ChatSessionView, error) {
	f.created = in
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &aiapp.ChatSessionView{SessionID: "s-1", Title: "会话", Status: "active"}, nil
}

func (f *fakeChatUseCase) ListSessions(_ context.Context, _ string, limit int) ([]aiapp.ChatSessionView, error) {
	f.listLimit = limit
	if f.listErr != nil {
		return nil, f.listErr
	}
	return []aiapp.ChatSessionView{{SessionID: "s-1", Title: "会话", Status: "active"}}, nil
}

func (f *fakeChatUseCase) GetSession(_ context.Context, _ string, _ string) (*aiapp.ChatSessionDetail, error) {
	if f.detailErr != nil {
		return nil, f.detailErr
	}
	return &aiapp.ChatSessionDetail{
		Session:  aiapp.ChatSessionView{SessionID: "s-1"},
		Messages: []aiapp.ChatMessageView{{MessageID: "m-1", Role: "assistant", CreatedAt: time.Now()}},
	}, nil
}

func (f *fakeChatUseCase) SendMessage(_ context.Context, in aiapp.SendChatMessageInput) (*aiapp.SendChatMessageResult, error) {
	f.sent = in
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	return &aiapp.SendChatMessageResult{
		SessionID: in.SessionID, MessageID: "m-2", Status: "ok",
		Answer: aiapp.ChatAnswer{Summary: "CPU 持续升高", RiskLevel: "medium"},
	}, nil
}

func (f *fakeChatUseCase) SubmitFeedback(_ context.Context, userID, sessionID, messageID, feedback string) (*aiapp.ChatMessageView, error) {
	f.feedback = [3]string{sessionID, messageID, feedback}
	if f.feedbackErr != nil {
		return nil, f.feedbackErr
	}
	return &aiapp.ChatMessageView{MessageID: messageID, Role: "assistant", Feedback: feedback}, nil
}

func newChatHandlerTestContext(method, path string, body any, params gin.Params, userID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = params
	if userID != "" {
		c.Set(middleware.CtxKeyUserID, userID)
	}
	return c, w
}

func TestChatHandlerCreateSession(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions", map[string]any{
		"title": " 生产告警分析 ",
		"context": map[string]any{
			"page": "alerts", "alert_id": " a1 ", "time_range": "30m",
		},
	}, nil, "u-1")

	h.CreateSession(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if usecase.created.UserID != "u-1" || usecase.created.Context.AlertID != "a1" {
		t.Fatalf("unexpected input: %+v", usecase.created)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"code":"OK"`)) {
		t.Fatalf("expected OK response: %s", w.Body.String())
	}
}

func TestChatHandlerCreateSessionInvalidBody(t *testing.T) {
	h := NewChatHandler(&fakeChatUseCase{})
	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions", nil, nil, "u-1")
	c.Request = httptest.NewRequest(http.MethodPost, "/chat/sessions", bytes.NewReader([]byte("{bad")))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateSession(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChatHandlerListSessions(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	c, w := newChatHandlerTestContext(http.MethodGet, "/chat/sessions?limit=5", nil, nil, "u-1")

	h.ListSessions(c)

	if w.Code != http.StatusOK || usecase.listLimit != 5 {
		t.Fatalf("expected 200 with limit 5, got %d limit=%d", w.Code, usecase.listLimit)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"session_id":"s-1"`)) {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestChatHandlerListSessionsRejectsInvalidLimit(t *testing.T) {
	h := NewChatHandler(&fakeChatUseCase{})
	for _, limit := range []string{"0", "101", "abc"} {
		c, w := newChatHandlerTestContext(http.MethodGet, "/chat/sessions?limit="+limit, nil, nil, "u-1")
		h.ListSessions(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("limit %s: expected 400, got %d", limit, w.Code)
		}
	}
}

func TestChatHandlerGetSession(t *testing.T) {
	h := NewChatHandler(&fakeChatUseCase{})
	c, w := newChatHandlerTestContext(http.MethodGet, "/chat/sessions/s-1", nil, gin.Params{{Key: "session_id", Value: "s-1"}}, "u-1")

	h.GetSession(c)

	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"m-1"`)) {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
}

func TestChatHandlerSendMessage(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/messages", map[string]any{
		"content": "帮我分析这个告警",
		"context": map[string]any{"page": "alerts", "alert_id": "a1"},
	}, gin.Params{{Key: "session_id", Value: "s-1"}}, "u-1")

	h.SendMessage(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if usecase.sent.Content != "帮我分析这个告警" || usecase.sent.SessionID != "s-1" {
		t.Fatalf("unexpected input: %+v", usecase.sent)
	}
	if usecase.sent.Context == nil || usecase.sent.Context.AlertID != "a1" {
		t.Fatalf("context not propagated: %+v", usecase.sent.Context)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"summary":"CPU 持续升高"`)) {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestChatHandlerSendMessageRequiresContent(t *testing.T) {
	h := NewChatHandler(&fakeChatUseCase{})
	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/messages", map[string]any{}, gin.Params{{Key: "session_id", Value: "s-1"}}, "u-1")
	h.SendMessage(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChatHandlerSubmitFeedback(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/messages/m-2/feedback",
		map[string]any{"feedback": "valid"},
		gin.Params{{Key: "session_id", Value: "s-1"}, {Key: "message_id", Value: "m-2"}}, "u-1")

	h.SubmitFeedback(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if usecase.feedback[0] != "s-1" || usecase.feedback[1] != "m-2" || usecase.feedback[2] != "valid" {
		t.Fatalf("unexpected feedback input: %+v", usecase.feedback)
	}
}

func TestChatHandlerServiceUnavailable(t *testing.T) {
	h := NewChatHandler(nil)
	c, w := newChatHandlerTestContext(http.MethodGet, "/chat/sessions", nil, nil, "u-1")
	h.ListSessions(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestChatHandlerMapsServiceErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"unauthenticated", apperr.New(apperr.CodeUnauthenticated, "missing user identity"), http.StatusUnauthorized},
		{"not found", apperr.New(apperr.CodeNotFound, "session not found"), http.StatusNotFound},
		{"permission", apperr.New(apperr.CodePermissionDenied, "session does not belong to current user"), http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			usecase := &fakeChatUseCase{sendErr: tc.err}
			h := NewChatHandler(usecase)
			c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/messages",
				map[string]any{"content": "hi"}, gin.Params{{Key: "session_id", Value: "s-1"}}, "u-1")
			h.SendMessage(c)
			if w.Code != tc.code {
				t.Fatalf("expected %d, got %d: %s", tc.code, w.Code, w.Body.String())
			}
		})
	}
}

func TestChatHandlerSubmitFeedbackRequiresFeedback(t *testing.T) {
	h := NewChatHandler(&fakeChatUseCase{})
	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/messages/m-1/feedback",
		map[string]any{},
		gin.Params{{Key: "session_id", Value: "s-1"}, {Key: "message_id", Value: "m-1"}}, "u-1")
	h.SubmitFeedback(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChatHandlerCreateSessionContextNormalized(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	c, _ := newChatHandlerTestContext(http.MethodPost, "/chat/sessions", map[string]any{
		"context": map[string]any{"alert_id": "  a1  ", "page": " alerts "},
	}, nil, "u-1")
	h.CreateSession(c)
	if usecase.created.Context.AlertID != "a1" || usecase.created.Context.Page != "alerts" {
		t.Fatalf("context not normalized: %+v", usecase.created.Context)
	}
	// 会话上下文类型仍可安全序列化，用于前端展示。
	if _, err := json.Marshal(domain.SessionContext{Page: "alerts"}); err != nil {
		t.Fatalf("marshal context: %v", err)
	}
}
