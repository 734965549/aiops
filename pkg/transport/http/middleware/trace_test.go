package middleware

import (
	"net/http/httptest"
	"testing"

	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestNormalizeTraceIDAcceptsUUID(t *testing.T) {
	id := uuid.NewString()
	if got := normalizeTraceID(id); got != id {
		t.Fatalf("expected %q, got %q", id, got)
	}
}

func TestNormalizeTraceIDRejectsInvalid(t *testing.T) {
	for _, raw := range []string{"not-a-uuid", "", "   "} {
		got := normalizeTraceID(raw)
		if _, err := uuid.Parse(got); err != nil {
			t.Fatalf("expected generated uuid for %q, got %q: %v", raw, got, err)
		}
	}
	long := string(make([]byte, 129))
	if got := normalizeTraceID(long); got == long {
		t.Fatal("expected replacement for overlong trace id")
	}
}

func TestTraceMiddlewareUsesValidHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Trace())
	r.GET("/", func(c *gin.Context) {
		c.String(200, c.GetString(httpx.CtxKeyTraceID))
	})
	id := uuid.NewString()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(httpx.HeaderTraceID, id)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get(httpx.HeaderTraceID) != id {
		t.Fatalf("expected header %q, got %q", id, w.Header().Get(httpx.HeaderTraceID))
	}
}

func TestTraceMiddlewareReplacesInvalidHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Trace())
	r.GET("/", func(c *gin.Context) { c.Status(200) })
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(httpx.HeaderTraceID, "bad-trace")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if _, err := uuid.Parse(w.Header().Get(httpx.HeaderTraceID)); err != nil {
		t.Fatalf("expected valid uuid in response header, got %q", w.Header().Get(httpx.HeaderTraceID))
	}
}
