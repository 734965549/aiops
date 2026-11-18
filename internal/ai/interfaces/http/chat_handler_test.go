package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	statusErr   error
	deleteErr   error
	streamErr   error
	shareErr    error
	listLimit   int
	// streamed 记录流式入参；statusCalls 记录归档/恢复/删除调用。
	streamed    aiapp.SendChatMessageInput
	statusCalls []string
	// shareInput / shareToken 记录分享入参，便于断言 handler 绑定。
	shareInput aiapp.CreateChatShareInput
	shareToken string
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

func (f *fakeChatUseCase) SendMessageStream(ctx context.Context, in aiapp.SendChatMessageInput, emit aiapp.ChatEventSink) (*aiapp.SendChatMessageResult, error) {
	f.streamed = in
	if f.streamErr != nil {
		// 模拟「已推送部分事件后失败」：先推一个阶段事件再返回错误。
		_ = emit(aiapp.ChatEvent{Type: aiapp.ChatEventStage, Stage: aiapp.ChatStageGathering})
		return nil, f.streamErr
	}
	if err := emit(aiapp.ChatEvent{Type: aiapp.ChatEventStage, Stage: aiapp.ChatStageThinking}); err != nil {
		return nil, err
	}
	if err := emit(aiapp.ChatEvent{Type: aiapp.ChatEventTool, Tool: &aiapp.ChatToolEvent{
		ToolCode: "alarm.analyze", Allowed: true, Status: "ok",
	}}); err != nil {
		return nil, err
	}
	result := &aiapp.SendChatMessageResult{
		SessionID: in.SessionID, MessageID: "m-2", Status: "ok",
		Answer: aiapp.ChatAnswer{Summary: "CPU 持续升高", RiskLevel: "medium"},
	}
	if err := emit(aiapp.ChatEvent{Type: aiapp.ChatEventAnswer, Result: result}); err != nil {
		return nil, err
	}
	return result, nil
}

func (f *fakeChatUseCase) SubmitFeedback(_ context.Context, userID, sessionID, messageID, feedback string) (*aiapp.ChatMessageView, error) {
	f.feedback = [3]string{sessionID, messageID, feedback}
	if f.feedbackErr != nil {
		return nil, f.feedbackErr
	}
	return &aiapp.ChatMessageView{MessageID: messageID, Role: "assistant", Feedback: feedback}, nil
}

func (f *fakeChatUseCase) ArchiveSession(_ context.Context, _, sessionID string) (*aiapp.ChatSessionView, error) {
	f.statusCalls = append(f.statusCalls, "archive:"+sessionID)
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return &aiapp.ChatSessionView{SessionID: sessionID, Status: "archived"}, nil
}

func (f *fakeChatUseCase) UnarchiveSession(_ context.Context, _, sessionID string) (*aiapp.ChatSessionView, error) {
	f.statusCalls = append(f.statusCalls, "unarchive:"+sessionID)
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return &aiapp.ChatSessionView{SessionID: sessionID, Status: "active"}, nil
}

func (f *fakeChatUseCase) DeleteSession(_ context.Context, _, sessionID string) error {
	f.statusCalls = append(f.statusCalls, "delete:"+sessionID)
	return f.deleteErr
}

func (f *fakeChatUseCase) CreateShare(_ context.Context, in aiapp.CreateChatShareInput) (*aiapp.CreatedChatShare, error) {
	f.shareInput = in
	if f.shareErr != nil {
		return nil, f.shareErr
	}
	return &aiapp.CreatedChatShare{
		ChatShareView: aiapp.ChatShareView{ShareID: "sh-1", SessionID: in.SessionID, Status: "active"},
		Token:         "token-1",
		ShareURL:      "/ai-assistant/shared/token-1",
	}, nil
}

func (f *fakeChatUseCase) ListShares(_ context.Context, _, sessionID string) ([]aiapp.ChatShareView, error) {
	if f.shareErr != nil {
		return nil, f.shareErr
	}
	return []aiapp.ChatShareView{{ShareID: "sh-1", SessionID: sessionID, Status: "active"}}, nil
}

func (f *fakeChatUseCase) RevokeShare(_ context.Context, _, sessionID, shareID string) (*aiapp.ChatShareView, error) {
	if f.shareErr != nil {
		return nil, f.shareErr
	}
	return &aiapp.ChatShareView{ShareID: shareID, SessionID: sessionID, Status: "revoked"}, nil
}

func (f *fakeChatUseCase) ViewSharedSession(_ context.Context, _, token string) (*aiapp.ChatSessionDetail, error) {
	f.shareToken = token
	if f.shareErr != nil {
		return nil, f.shareErr
	}
	return &aiapp.ChatSessionDetail{
		Session:  aiapp.ChatSessionView{SessionID: "s-1", Title: "共享会话"},
		Messages: []aiapp.ChatMessageView{{MessageID: "m-1", Role: "assistant"}},
	}, nil
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
		"content":           "帮我分析这个告警",
		"context":           map[string]any{"page": "alerts", "alert_id": "a1"},
		"client_message_id": "cm-1",
	}, gin.Params{{Key: "session_id", Value: "s-1"}}, "u-1")

	h.SendMessage(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if usecase.sent.Content != "帮我分析这个告警" || usecase.sent.SessionID != "s-1" {
		t.Fatalf("unexpected input: %+v", usecase.sent)
	}
	if usecase.sent.ClientMessageID != "cm-1" {
		t.Fatalf("client_message_id not propagated: %+v", usecase.sent)
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

func TestChatHandlerSendMessageStream(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/messages/stream", map[string]any{
		"content":           "帮我分析这个告警",
		"client_message_id": "cm-1",
	}, gin.Params{{Key: "session_id", Value: "s-1"}}, "u-1")

	h.SendMessageStream(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}
	if w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("cache-control = %q", w.Header().Get("Cache-Control"))
	}
	if usecase.streamed.SessionID != "s-1" || usecase.streamed.ClientMessageID != "cm-1" {
		t.Fatalf("unexpected stream input: %+v", usecase.streamed)
	}
	body := w.Body.String()
	for _, want := range []string{`"type":"stage"`, `"stage":"thinking"`, `"type":"tool"`, `"tool_code":"alarm.analyze"`, `"type":"answer"`, `"summary":"CPU 持续升高"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in SSE body: %s", want, body)
		}
	}
	// 每条事件都以 data: 前缀 + 空行结尾。
	if strings.Count(body, "data: ") != 3 {
		t.Fatalf("expected 3 SSE events, got body: %s", body)
	}
}

func TestChatHandlerSendMessageStreamEmitsErrorEvent(t *testing.T) {
	usecase := &fakeChatUseCase{streamErr: apperr.New(apperr.CodeFailedPrecondition, "session is archived")}
	h := NewChatHandler(usecase)
	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/messages/stream", map[string]any{
		"content": "继续",
	}, gin.Params{{Key: "session_id", Value: "s-1"}}, "u-1")

	h.SendMessageStream(c)

	// 状态码已经是 200，业务错误只能作为事件下发。
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"type":"error"`) || !strings.Contains(body, "FAILED_PRECONDITION") {
		t.Fatalf("expected error event, got: %s", body)
	}
}

func TestChatHandlerSendMessageStreamRequiresContent(t *testing.T) {
	h := NewChatHandler(&fakeChatUseCase{})
	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/messages/stream", map[string]any{}, gin.Params{{Key: "session_id", Value: "s-1"}}, "u-1")
	h.SendMessageStream(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChatHandlerArchiveAndUnarchiveSession(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	params := gin.Params{{Key: "session_id", Value: "s-1"}}

	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/archive", nil, params, "u-1")
	h.ArchiveSession(c)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"status":"archived"`)) {
		t.Fatalf("unexpected archive response: %d %s", w.Code, w.Body.String())
	}

	c, w = newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/unarchive", nil, params, "u-1")
	h.UnarchiveSession(c)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"status":"active"`)) {
		t.Fatalf("unexpected unarchive response: %d %s", w.Code, w.Body.String())
	}

	if len(usecase.statusCalls) != 2 || usecase.statusCalls[0] != "archive:s-1" || usecase.statusCalls[1] != "unarchive:s-1" {
		t.Fatalf("unexpected status calls: %+v", usecase.statusCalls)
	}
}

func TestChatHandlerDeleteSession(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	c, w := newChatHandlerTestContext(http.MethodDelete, "/chat/sessions/s-1", nil,
		gin.Params{{Key: "session_id", Value: "s-1"}}, "u-1")

	h.DeleteSession(c)

	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"deleted":true`)) {
		t.Fatalf("unexpected delete response: %d %s", w.Code, w.Body.String())
	}
	if len(usecase.statusCalls) != 1 || usecase.statusCalls[0] != "delete:s-1" {
		t.Fatalf("unexpected calls: %+v", usecase.statusCalls)
	}
}

func TestChatHandlerSessionLifecycleErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"not found", apperr.New(apperr.CodeNotFound, "session not found"), http.StatusNotFound},
		{"permission", apperr.New(apperr.CodePermissionDenied, "session does not belong to current user"), http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := gin.Params{{Key: "session_id", Value: "s-1"}}

			h := NewChatHandler(&fakeChatUseCase{statusErr: tc.err})
			c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/archive", nil, params, "u-1")
			h.ArchiveSession(c)
			if w.Code != tc.code {
				t.Fatalf("archive: expected %d, got %d: %s", tc.code, w.Code, w.Body.String())
			}

			h = NewChatHandler(&fakeChatUseCase{deleteErr: tc.err})
			c, w = newChatHandlerTestContext(http.MethodDelete, "/chat/sessions/s-1", nil, params, "u-1")
			h.DeleteSession(c)
			if w.Code != tc.code {
				t.Fatalf("delete: expected %d, got %d: %s", tc.code, w.Code, w.Body.String())
			}
		})
	}
}

func TestChatHandlerLifecycleUnavailable(t *testing.T) {
	h := NewChatHandler(nil)
	params := gin.Params{{Key: "session_id", Value: "s-1"}}
	for name, call := range map[string]func(*gin.Context){
		"archive":   h.ArchiveSession,
		"unarchive": h.UnarchiveSession,
		"delete":    h.DeleteSession,
	} {
		c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1", nil, params, "u-1")
		call(c)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503, got %d", name, w.Code)
		}
	}
}

func TestChatHandlerCreateShare(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/shares", map[string]any{
		"expires_in_hours": 24,
	}, gin.Params{{Key: "session_id", Value: "s-1"}}, "u-1")

	h.CreateShare(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if usecase.shareInput.SessionID != "s-1" || usecase.shareInput.ExpiresInHours != 24 {
		t.Fatalf("unexpected share input: %+v", usecase.shareInput)
	}
	body := w.Body.String()
	for _, want := range []string{`"share_id":"sh-1"`, `"token":"token-1"`, `"share_url":"/ai-assistant/shared/token-1"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s in body: %s", want, body)
		}
	}
}

func TestChatHandlerCreateShareAcceptsEmptyBody(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	c, w := newChatHandlerTestContext(http.MethodPost, "/chat/sessions/s-1/shares", nil,
		gin.Params{{Key: "session_id", Value: "s-1"}}, "u-1")

	h.CreateShare(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if usecase.shareInput.ExpiresInHours != 0 {
		t.Fatalf("empty body should use default ttl, got %d", usecase.shareInput.ExpiresInHours)
	}
}

func TestChatHandlerListAndRevokeShares(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	params := gin.Params{{Key: "session_id", Value: "s-1"}}

	c, w := newChatHandlerTestContext(http.MethodGet, "/chat/sessions/s-1/shares", nil, params, "u-1")
	h.ListShares(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"shares"`) {
		t.Fatalf("list shares failed: %d %s", w.Code, w.Body.String())
	}

	c, w = newChatHandlerTestContext(http.MethodDelete, "/chat/sessions/s-1/shares/sh-1", nil,
		gin.Params{{Key: "session_id", Value: "s-1"}, {Key: "share_id", Value: "sh-1"}}, "u-1")
	h.RevokeShare(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"revoked"`) {
		t.Fatalf("revoke share failed: %d %s", w.Code, w.Body.String())
	}
}

func TestChatHandlerViewSharedSession(t *testing.T) {
	usecase := &fakeChatUseCase{}
	h := NewChatHandler(usecase)
	c, w := newChatHandlerTestContext(http.MethodGet, "/chat/shares/token-1", nil,
		gin.Params{{Key: "token", Value: "token-1"}}, "u-2")

	h.ViewSharedSession(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if usecase.shareToken != "token-1" {
		t.Fatalf("unexpected token: %q", usecase.shareToken)
	}
	if !strings.Contains(w.Body.String(), `"session"`) || !strings.Contains(w.Body.String(), `"messages"`) {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestChatHandlerShareErrorsMapToStatus(t *testing.T) {
	usecase := &fakeChatUseCase{shareErr: apperr.New(apperr.CodeNotFound, "share link is not available")}
	h := NewChatHandler(usecase)
	c, w := newChatHandlerTestContext(http.MethodGet, "/chat/shares/token-1", nil,
		gin.Params{{Key: "token", Value: "token-1"}}, "u-1")
	h.ViewSharedSession(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
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
