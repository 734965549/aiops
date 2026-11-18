package http

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/gin-gonic/gin"
)

type bindTestPayload struct {
	Title string `json:"title"`
}

func newBindTestContext(raw []byte) *gin.Context {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	return c
}

func TestBindJSONAcceptsValidUTF8(t *testing.T) {
	c := newBindTestContext([]byte(`{"title":"生产告警分析"}`))
	var payload bindTestPayload
	if err := BindJSON(c, &payload); err != nil {
		t.Fatalf("expected valid utf-8 body accepted, got %v", err)
	}
	if payload.Title != "生产告警分析" {
		t.Fatalf("title not bound correctly: %q", payload.Title)
	}
}

func TestBindJSONRejectsInvalidUTF8(t *testing.T) {
	// "告警" 的 GBK 编码 B8 E6 BE AF，不是合法 UTF-8。
	raw := []byte(`{"title":"`)
	raw = append(raw, 0xB8, 0xE6, 0xBE, 0xAF)
	raw = append(raw, '"', '}')
	c := newBindTestContext(raw)

	var payload bindTestPayload
	err := BindJSON(c, &payload)
	if err == nil {
		t.Fatal("expected invalid utf-8 body to be rejected")
	}
	var typed *apperr.Error
	if !errors.As(err, &typed) || typed.Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
	}
	if payload.Title != "" {
		t.Fatalf("payload must not be bound on invalid utf-8, got %q", payload.Title)
	}
}

func TestBindJSONAcceptsASCIIAndEmptyBody(t *testing.T) {
	c := newBindTestContext([]byte(`{"title":"cpu high"}`))
	var payload bindTestPayload
	if err := BindJSON(c, &payload); err != nil || payload.Title != "cpu high" {
		t.Fatalf("expected ascii body accepted, got err=%v payload=%+v", err, payload)
	}

	// 空 body 通过 UTF-8 校验，随后交给 JSON 绑定按既有语义报 EOF。
	c = newBindTestContext(nil)
	var empty bindTestPayload
	err := BindJSON(c, &empty)
	if err == nil {
		t.Fatal("expected empty body to fail json binding")
	}
	var typed *apperr.Error
	if errors.As(err, &typed) {
		t.Fatalf("empty body must not be rejected as invalid utf-8, got %v", err)
	}
}
