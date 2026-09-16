package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/734965549/aiops/pkg/config"
	"github.com/gin-gonic/gin"
)

func TestCORS_SameOriginEmptyOriginsWithCredentialsDoesNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(CORS(config.CORSConfig{AllowCredentials: true}))

	engine.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS headers for same-origin mode, got %q", got)
	}
}

func TestCORS_ExplicitOriginsWithCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(CORS(config.CORSConfig{
		AllowOrigins:     []string{"https://app.example.com"},
		AllowCredentials: true,
	}))

	engine.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("unexpected allow-origin header: %q", got)
	}
}
