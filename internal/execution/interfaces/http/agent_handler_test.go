package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	execapp "github.com/734965549/aiops/internal/execution/application"
	"github.com/gin-gonic/gin"
)

func TestAgentHandler_Register_RejectsWhenExpectedTokenEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAgentHandler(&execapp.AgentService{}, nil, "")

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/executions/agents/register", strings.NewReader(`{"medium_id":"med-1"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.Register(ctx)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PERMISSION_DENIED") {
		t.Fatalf("expected PERMISSION_DENIED, body=%s", rec.Body.String())
	}
}

func TestAgentHandler_Register_RejectsMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAgentHandler(&execapp.AgentService{}, nil, "expected-register-token")

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/executions/agents/register", strings.NewReader(`{"medium_id":"med-1"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.Register(ctx)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgentHandler_Register_RejectsWrongToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAgentHandler(&execapp.AgentService{}, nil, "expected-register-token")

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/executions/agents/register", strings.NewReader(`{"medium_id":"med-1"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Register-Token", "wrong-token")

	h.Register(ctx)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
