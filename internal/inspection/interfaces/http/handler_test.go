// Package http Inspection HTTP 契约测试（ops/cloud-observability-contract.md §6）。
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
	inspectionapp "github.com/734965549/aiops/internal/inspection/application"
	"github.com/734965549/aiops/internal/inspection/domain"
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

type memPolicyRepo struct {
	items map[string]*domain.InspectionPolicy
}

func (m *memPolicyRepo) Create(_ context.Context, p *domain.InspectionPolicy) error {
	m.items[p.PolicyID] = p
	return nil
}
func (m *memPolicyRepo) Update(_ context.Context, p *domain.InspectionPolicy) error {
	m.items[p.PolicyID] = p
	return nil
}
func (m *memPolicyRepo) GetByID(_ context.Context, id string) (*domain.InspectionPolicy, error) {
	if p, ok := m.items[id]; ok {
		return p, nil
	}
	return nil, domain.ErrNotFound
}
func (m *memPolicyRepo) List(context.Context, domain.PolicyFilter) ([]domain.InspectionPolicy, error) {
	return nil, nil
}
func (m *memPolicyRepo) Count(context.Context, domain.PolicyFilter) (int64, error) { return 0, nil }
func (m *memPolicyRepo) SoftDelete(_ context.Context, id string) error {
	delete(m.items, id)
	return nil
}

type memRunRepo struct {
	items map[string]*domain.InspectionRun
}

func (m *memRunRepo) Create(_ context.Context, r *domain.InspectionRun) error {
	m.items[r.RunID] = r
	return nil
}
func (m *memRunRepo) Update(_ context.Context, r *domain.InspectionRun) error {
	m.items[r.RunID] = r
	return nil
}
func (m *memRunRepo) GetByID(_ context.Context, id string) (*domain.InspectionRun, error) {
	if r, ok := m.items[id]; ok {
		return r, nil
	}
	return nil, domain.ErrNotFound
}
func (m *memRunRepo) List(context.Context, domain.RunFilter) ([]domain.InspectionRun, error) {
	return nil, nil
}
func (m *memRunRepo) Count(context.Context, domain.RunFilter) (int64, error) { return 0, nil }

type memFindingRepo struct{}

func (memFindingRepo) CreateBatch(context.Context, []domain.InspectionFinding) error { return nil }
func (memFindingRepo) List(context.Context, domain.FindingFilter) ([]domain.InspectionFinding, error) {
	return nil, nil
}
func (memFindingRepo) Count(context.Context, domain.FindingFilter) (int64, error) { return 0, nil }
func (memFindingRepo) GetByID(context.Context, string) (*domain.InspectionFinding, error) {
	return nil, domain.ErrNotFound
}

type memRecRepo struct{}

func (memRecRepo) CreateBatch(context.Context, []domain.Recommendation) error { return nil }
func (memRecRepo) ListByRunID(context.Context, string) ([]domain.Recommendation, error) {
	return nil, nil
}
func (memRecRepo) ListByFindingID(context.Context, string) ([]domain.Recommendation, error) {
	return nil, nil
}
func (memRecRepo) GetByID(context.Context, string) (*domain.Recommendation, error) {
	return nil, domain.ErrNotFound
}
func (memRecRepo) Update(context.Context, *domain.Recommendation) error { return nil }

type stubAnalyzer struct{}

func (stubAnalyzer) CollectEvidence(context.Context, inspectionapp.Actor, inspectionapp.CheckEvidenceInput) (*inspectionapp.EvidenceSummary, error) {
	return &inspectionapp.EvidenceSummary{Type: "metrics", EvidenceID: "ev-test", Metric: "mem_util", MaxValue: 80}, nil
}
func (stubAnalyzer) Analyze(context.Context, []string, []inspectionapp.EvidenceSummary) ([]inspectionapp.AnalysisResult, error) {
	return []inspectionapp.AnalysisResult{{
		Category: "metrics.memory", RiskLevel: "high", Summary: "test finding",
		EvidenceRefs: []string{"ev-test"}, Confidence: 0.8,
		Recommendations: []inspectionapp.RecommendationDraft{{Title: "test rec", RiskLevel: "medium"}},
	}}, nil
}

func setupInspectionHTTPEngine(t *testing.T, allowed bool) (*gin.Engine, string, *fakeHTTPAuthorizer) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	authz := &fakeHTTPAuthorizer{allowed: allowed}
	policyRepo := &memPolicyRepo{items: map[string]*domain.InspectionPolicy{
		"pol-test": {
			PolicyID: "pol-test", Name: "test", Enabled: true,
			Scope: domain.PolicyScope{AccountID: "acc-test", Provider: "huawei_cloud"},
			Checks: []string{"metrics.memory"},
		},
	}}
	runRepo := &memRunRepo{items: map[string]*domain.InspectionRun{}}
	policySvc := inspectionapp.NewPolicyService(policyRepo, nil)
	runSvc := inspectionapp.NewRunService(policyRepo, runRepo, memFindingRepo{}, memRecRepo{}, stubAnalyzer{}, nil)
	handler := NewHandler(policySvc, runSvc, nil)
	registrar := NewRegistrar(handler, authz)

	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret: "inspection-http-test-secret-with-length", Issuer: "aiops-test",
		AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := jwtMgr.IssueAccess(auth.IssueOptions{UserID: "user-1", Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		App: config.AppConfig{Env: "dev"}, Server: config.ServerConfig{Port: 8080},
		Auth: config.AuthConfig{JWTSecret: "inspection-http-test-secret-with-length"},
	}
	engine := server.NewEngine(server.Options{
		Cfg: cfg, Authenticator: auth.NewJWTAuthenticator(jwtMgr),
		Registrars: []server.RouteRegistrar{registrar}, StartedAt: time.Now(),
	})
	return engine, token, authz
}

func TestCreatePolicy_OK(t *testing.T) {
	engine, token, authz := setupInspectionHTTPEngine(t, true)
	body := bytes.NewBufferString(`{
		"name":"prod check",
		"scope":{"account_id":"acc-1","provider":"huawei_cloud","environment":"prod"},
		"checks":["metrics.cpu","metrics.memory"]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/inspections/policies", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if authz.last.Resource != "inspections" || authz.last.Action != "write" {
		t.Fatalf("unexpected authz: %+v", authz.last)
	}
}

func TestTriggerRun_Forbidden(t *testing.T) {
	engine, token, _ := setupInspectionHTTPEngine(t, false)
	req := httptest.NewRequest(http.MethodPost, "/api/inspections/policies/pol-test/runs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestTriggerRun_OK(t *testing.T) {
	engine, token, _ := setupInspectionHTTPEngine(t, true)
	req := httptest.NewRequest(http.MethodPost, "/api/inspections/policies/pol-test/runs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code string                `json:"code"`
		Data inspectionapp.RunDTO  `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != "OK" || env.Data.RunID == "" {
		t.Fatalf("unexpected: %+v", env)
	}
}
