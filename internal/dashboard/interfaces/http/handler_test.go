package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	alertdomain "github.com/734965549/aiops/internal/alert/domain"
	dashapp "github.com/734965549/aiops/internal/dashboard/application"
	execdomain "github.com/734965549/aiops/internal/execution/domain"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	rbdomain "github.com/734965549/aiops/internal/runbook/domain"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/auth"
	"github.com/734965549/aiops/pkg/config"
	"github.com/gin-gonic/gin"
)

type fakeDashHTTPAuthorizer struct {
	allowed bool
	last    identityapp.AuthorizationInput
}

func (f *fakeDashHTTPAuthorizer) Authorize(_ context.Context, in identityapp.AuthorizationInput) (*identityapp.AuthorizationResult, error) {
	f.last = in
	return &identityapp.AuthorizationResult{Allowed: f.allowed}, nil
}

type dashHTTPStats struct {
	active int64
}

func (s dashHTTPStats) CountAlerts(_ context.Context, filter alertdomain.AlertFilter) (int64, error) {
	if filter.ActiveOnly && len(filter.Severities) == 0 {
		return s.active, nil
	}
	return 0, nil
}

func (dashHTTPStats) ListAlerts(_ context.Context, _ alertdomain.AlertFilter) ([]alertdomain.Alert, error) {
	return nil, nil
}

func (dashHTTPStats) CountTasks(_ context.Context, _ execdomain.TaskFilter) (int64, error) {
	return 0, nil
}

func (dashHTTPStats) ListTasks(_ context.Context, _ execdomain.TaskFilter) ([]execdomain.Task, error) {
	return nil, nil
}

func (dashHTTPStats) CountApplications(context.Context) (int64, error) { return 2, nil }
func (dashHTTPStats) CountResources(context.Context) (int64, error)    { return 7, nil }

func (dashHTTPStats) CountRunbookTemplates(_ context.Context, _ rbdomain.TemplateFilter) (int64, error) {
	return 1, nil
}

func newDashboardHTTPEngine(t *testing.T, authz *fakeDashHTTPAuthorizer, stats dashapp.StatsReader) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret: "dashboard-http-test-secret-length", Issuer: "aiops-test",
		AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt manager: %v", err)
	}
	token, _, err := jwtMgr.IssueAccess(auth.IssueOptions{UserID: "user-1", Username: "alice"})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	svc := dashapp.NewSummaryService(stats)
	handler := NewHandler(svc)
	registrar := NewRegistrar(handler, authz)

	engine := server.NewEngine(server.Options{
		Cfg: &config.Config{
			App:      config.AppConfig{Env: "dev", Timezone: "Asia/Shanghai"},
			Server:   config.ServerConfig{Port: 8080},
			Database: config.DatabaseConfig{Host: "127.0.0.1", Name: "aiops", SSLMode: "disable"},
			Auth:     config.AuthConfig{JWTSecret: config.DefaultJWTSecretPlaceholder},
		},
		Authenticator: auth.NewJWTAuthenticator(jwtMgr),
		Registrars:    []server.RouteRegistrar{registrar},
		StartedAt:     time.Now(),
	})
	return engine, token
}

type dashAPIEnvelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func decodeDashEnvelope(t *testing.T, body []byte) dashAPIEnvelope {
	t.Helper()
	var env dashAPIEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v body=%s", err, string(body))
	}
	return env
}

func TestGetSummary_Unauthenticated(t *testing.T) {
	authz := &fakeDashHTTPAuthorizer{allowed: true}
	engine, _ := newDashboardHTTPEngine(t, authz, dashHTTPStats{active: 3})

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/summary", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetSummary_PermissionDenied(t *testing.T) {
	authz := &fakeDashHTTPAuthorizer{allowed: false}
	engine, token := newDashboardHTTPEngine(t, authz, dashHTTPStats{active: 3})

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/summary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if authz.last.Resource != "dashboard" || authz.last.Action != "read" {
		t.Fatalf("unexpected authz: %+v", authz.last)
	}
}

func TestGetSummary_Success(t *testing.T) {
	authz := &fakeDashHTTPAuthorizer{allowed: true}
	engine, token := newDashboardHTTPEngine(t, authz, dashHTTPStats{active: 9})

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/summary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeDashEnvelope(t, w.Body.Bytes())
	if resp.Code != "OK" {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
	var data struct {
		Alerts struct {
			ActiveTotal int64 `json:"active_total"`
		} `json:"alerts"`
		Assets struct {
			Applications int64 `json:"applications"`
			Resources    int64 `json:"resources"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Alerts.ActiveTotal != 9 || data.Assets.Applications != 2 || data.Assets.Resources != 7 {
		t.Fatalf("unexpected summary: %+v", data)
	}
}
