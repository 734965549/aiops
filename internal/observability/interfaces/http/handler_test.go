// Package http Observability HTTP 契约测试（ops/cloud-observability-contract.md §5）。
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	integdomain "github.com/734965549/aiops/internal/integration/domain"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	obsprovider "github.com/734965549/aiops/internal/observability/infrastructure/provider"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/auth"
	"github.com/734965549/aiops/pkg/config"
	"github.com/gin-gonic/gin"
)

type fakeHTTPAuthorizer struct {
	allowed bool
	last    identityapp.AuthorizationInput
}

func (f *fakeHTTPAuthorizer) Authorize(_ context.Context, in identityapp.AuthorizationInput) (*identityapp.AuthorizationResult, error) {
	f.last = in
	return &identityapp.AuthorizationResult{Allowed: f.allowed}, nil
}

type stubAccountPort struct{}

func (stubAccountPort) ResolveAccount(context.Context, string) (*domain.AccountSnapshot, error) {
	return &domain.AccountSnapshot{
		AccountID: "acc-test",
		Provider:  string(integdomain.ProviderHuaweiCloud),
		AuthType:  string(integdomain.AuthNone),
		Capabilities: []string{
			string(integdomain.CapabilityMetrics),
			string(integdomain.CapabilityLogs),
			string(integdomain.CapabilityTraces),
		},
	}, nil
}

func setupObsHTTPEngine(t *testing.T, allowed bool) (*gin.Engine, string, *fakeHTTPAuthorizer) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	authz := &fakeHTTPAuthorizer{allowed: allowed}
	svc := obsapp.NewQueryService(stubAccountPort{}, obsprovider.DefaultFakeRegistry(), nil, nil)
	handler := NewHandler(svc)
	registrar := NewRegistrar(handler, authz)

	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret:     "observability-http-test-secret-with-length",
		Issuer:     "aiops-test",
		AccessTTL:  time.Hour,
		RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := jwtMgr.IssueAccess(auth.IssueOptions{UserID: "user-1", Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		App:    config.AppConfig{Env: "dev"},
		Server: config.ServerConfig{Port: 8080},
		Auth:   config.AuthConfig{JWTSecret: "observability-http-test-secret-with-length"},
	}
	engine := server.NewEngine(server.Options{
		Cfg: cfg, Authenticator: auth.NewJWTAuthenticator(jwtMgr),
		Registrars: []server.RouteRegistrar{registrar}, StartedAt: time.Now(),
	})
	return engine, token, authz
}

func TestQueryMetrics_OK(t *testing.T) {
	engine, token, authz := setupObsHTTPEngine(t, true)
	body := bytes.NewBufferString(`{
		"account_id":"acc-test","provider":"huawei_cloud","metric":"cpu_util",
		"from":1710000000,"to":1710003600,"period":60
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/observability/metrics/query", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if authz.last.Resource != "observability" || authz.last.Action != "read" {
		t.Fatalf("unexpected authz: %+v", authz.last)
	}
	var env struct {
		Code string                   `json:"code"`
		Data obsapp.MetricQueryResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != "OK" || env.Data.EvidenceID == "" || len(env.Data.Series) == 0 {
		t.Fatalf("unexpected payload: %+v", env)
	}
}

func TestQueryMetrics_Forbidden(t *testing.T) {
	engine, token, _ := setupObsHTTPEngine(t, false)
	body := bytes.NewBufferString(`{"account_id":"acc-test","metric":"cpu_util","from":1,"to":2}`)
	req := httptest.NewRequest(http.MethodPost, "/api/observability/metrics/query", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSearchLogs_OK(t *testing.T) {
	engine, token, authz := setupObsHTTPEngine(t, true)
	body := bytes.NewBufferString(`{
		"account_id":"acc-test","provider":"huawei_cloud","service":"payment-service",
		"keyword":"timeout","from":1710000000,"to":1710003600,"limit":10
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/observability/logs/search", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if authz.last.Resource != "observability" || authz.last.Action != "read" {
		t.Fatalf("unexpected authz: %+v", authz.last)
	}
	var env struct {
		Code string                 `json:"code"`
		Data obsapp.LogSearchResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != "OK" || env.Data.EvidenceID == "" || len(env.Data.Entries) == 0 {
		t.Fatalf("unexpected payload: %+v", env)
	}
}

func TestQueryTraces_OK(t *testing.T) {
	engine, token, _ := setupObsHTTPEngine(t, true)
	body := bytes.NewBufferString(`{
		"account_id":"acc-test","provider":"huawei_cloud","service":"payment-service",
		"error_only":true,"min_latency_ms":500,"from":1710000000,"to":1710003600,"limit":10
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/observability/traces/query", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code string                  `json:"code"`
		Data obsapp.TraceQueryResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != "OK" || env.Data.EvidenceID == "" || len(env.Data.Spans) == 0 || !env.Data.Spans[0].Error {
		t.Fatalf("unexpected payload: %+v", env)
	}
}
