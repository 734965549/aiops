package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeAuthorizationService struct {
	result *AuthorizationResult
	err    error
	last   AuthorizationInput
}

func (f *fakeAuthorizationService) Authorize(ctx context.Context, in AuthorizationInput) (*AuthorizationResult, error) {
	f.last = in
	return f.result, f.err
}

func TestAuthorizeStaticAllows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &fakeAuthorizationService{result: &AuthorizationResult{Allowed: true}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxKeyUserID, "u1"); c.Next() })
	r.GET("/x", AuthorizeStatic(svc, "alarm", "read"), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if svc.last.Resource != "alarm" || svc.last.Action != "read" || svc.last.UserID != "u1" || !svc.last.SkipDataScope {
		t.Fatalf("unexpected auth input: %+v", svc.last)
	}
}

func TestAuthorizeRequiredDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &fakeAuthorizationService{result: &AuthorizationResult{Allowed: false, Reason: "denied"}}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(CtxKeyUserID, "u1"); c.Next() })
	r.GET("/x", AuthorizeStatic(svc, "alarm", "read"), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}
