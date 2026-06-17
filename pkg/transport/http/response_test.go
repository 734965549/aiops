package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/gin-gonic/gin"
)

// assertTraceIDFieldPresent 校验 §2 固定字段 trace_id 始终出现在 JSON 中（值可为 ""）。
func assertTraceIDFieldPresent(t *testing.T, body []byte) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, string(body))
	}
	if _, ok := raw["trace_id"]; !ok {
		t.Fatalf("trace_id field missing from response: %s", string(body))
	}
}

func TestOK_TraceIDAlwaysPresentWithoutMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	OK(c, gin.H{"x": 1})

	assertTraceIDFieldPresent(t, w.Body.Bytes())
	var env Response
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.TraceID != "" {
		t.Fatalf("expected empty trace_id without middleware, got %q", env.TraceID)
	}
	if env.Code != apperr.CodeOK || env.Message != "ok" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
}

func TestFailWith_TraceIDAlwaysPresentWithoutMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	FailWith(c, apperr.CodeUnavailable, "service down")

	assertTraceIDFieldPresent(t, w.Body.Bytes())
	var env Response
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.TraceID != "" {
		t.Fatalf("expected empty trace_id without middleware, got %q", env.TraceID)
	}
	if env.Code != apperr.CodeUnavailable {
		t.Fatalf("unexpected code: %q", env.Code)
	}
}

func TestFail_SanitizesPlainError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	Fail(c, errors.New("redis: connection refused"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	var env Response
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Code != apperr.CodeInternal {
		t.Fatalf("unexpected code: %q", env.Code)
	}
	if env.Message != apperr.InternalMessage {
		t.Fatalf("expected sanitized message, got %q", env.Message)
	}
	if env.Message == "redis: connection refused" {
		t.Fatal("internal error leaked into response")
	}
}

func TestFail_PreservesTypedErrorMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	Fail(c, apperr.New(apperr.CodeNotFound, "alert not found"))

	var env Response
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Message != "alert not found" {
		t.Fatalf("unexpected message: %q", env.Message)
	}
}
