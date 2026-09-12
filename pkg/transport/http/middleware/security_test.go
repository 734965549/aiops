package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(SecurityHeaders())

	engine.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff, got %q", got)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("expected X-Frame-Options: DENY, got %q", got)
	}
	if got := w.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Errorf("expected Referrer-Policy: strict-origin-when-cross-origin, got %q", got)
	}
	if got := w.Header().Get("Permissions-Policy"); got != "geolocation=(), microphone=(), camera=()" {
		t.Errorf("expected Permissions-Policy: geolocation=(), microphone=(), camera=(), got %q", got)
	}
}

func TestRequestBodyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestBodyLimit(10)) // 10 bytes limit

	type payload struct {
		Name string `json:"name"`
	}

	engine.POST("/test", func(c *gin.Context) {
		var p payload
		if err := c.ShouldBindJSON(&p); err != nil {
			_ = c.Error(err)
			return
		}
		c.String(http.StatusOK, "ok")
	})

	// 1. Small body - should pass
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(`{"a":"b"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Large body - should trigger 413 Payload Too Large
	reqLarge := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(`{"name":"this is a very long string exceeding limit"}`))
	reqLarge.Header.Set("Content-Type", "application/json")
	wLarge := httptest.NewRecorder()
	engine.ServeHTTP(wLarge, reqLarge)
	if wLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", wLarge.Code, wLarge.Body.String())
	}
}
