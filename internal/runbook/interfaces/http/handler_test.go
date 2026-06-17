package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	rbapp "github.com/734965549/aiops/internal/runbook/application"
	rbdomain "github.com/734965549/aiops/internal/runbook/domain"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/auth"
	"github.com/734965549/aiops/pkg/config"
	"github.com/gin-gonic/gin"
)

type fakeRBHTTPAuthorizer struct {
	allowed bool
	last    identityapp.AuthorizationInput
}

func (f *fakeRBHTTPAuthorizer) Authorize(_ context.Context, in identityapp.AuthorizationInput) (*identityapp.AuthorizationResult, error) {
	f.last = in
	return &identityapp.AuthorizationResult{Allowed: f.allowed}, nil
}

type rbHTTPTestTemplateRepo struct {
	templates []*rbdomain.Template
	steps     map[string][]rbdomain.Step
}

func (r *rbHTTPTestTemplateRepo) Create(_ context.Context, tpl *rbdomain.Template) error {
	cp := *tpl
	r.templates = append(r.templates, &cp)
	return nil
}

func (r *rbHTTPTestTemplateRepo) CreateWithSteps(_ context.Context, tpl *rbdomain.Template, steps []rbdomain.Step) error {
	if err := r.Create(context.Background(), tpl); err != nil {
		return err
	}
	r.steps[tpl.ID] = append([]rbdomain.Step(nil), steps...)
	return nil
}

func (r *rbHTTPTestTemplateRepo) Update(_ context.Context, tpl *rbdomain.Template) error {
	for i, t := range r.templates {
		if t.ID == tpl.ID {
			cp := *tpl
			r.templates[i] = &cp
			return nil
		}
	}
	return rbdomain.ErrNotFound
}

func (r *rbHTTPTestTemplateRepo) ReplaceWithSteps(_ context.Context, tpl *rbdomain.Template, steps []rbdomain.Step) error {
	if err := r.Update(context.Background(), tpl); err != nil {
		return err
	}
	r.steps[tpl.ID] = append([]rbdomain.Step(nil), steps...)
	return nil
}

func (r *rbHTTPTestTemplateRepo) Delete(_ context.Context, templateID string) error {
	for i, t := range r.templates {
		if t.ID == templateID {
			r.templates[i].Enabled = false
			return nil
		}
	}
	return rbdomain.ErrNotFound
}

func (r *rbHTTPTestTemplateRepo) GetByID(_ context.Context, templateID string) (*rbdomain.Template, error) {
	for _, t := range r.templates {
		if t.ID == templateID {
			cp := *t
			return &cp, nil
		}
	}
	return nil, rbdomain.ErrNotFound
}

func (r *rbHTTPTestTemplateRepo) List(_ context.Context, _ rbdomain.TemplateFilter) ([]rbdomain.Template, error) {
	out := make([]rbdomain.Template, 0, len(r.templates))
	for _, t := range r.templates {
		out = append(out, *t)
	}
	return out, nil
}

func (r *rbHTTPTestTemplateRepo) Count(_ context.Context, _ rbdomain.TemplateFilter) (int64, error) {
	return int64(len(r.templates)), nil
}

func (r *rbHTTPTestTemplateRepo) ListEnabled(_ context.Context) ([]rbdomain.Template, error) {
	out := make([]rbdomain.Template, 0)
	for _, t := range r.templates {
		if t.Enabled {
			out = append(out, *t)
		}
	}
	return out, nil
}

type rbHTTPTestStepRepo struct {
	parent *rbHTTPTestTemplateRepo
}

func (r *rbHTTPTestStepRepo) Create(_ context.Context, step *rbdomain.Step) error {
	r.parent.steps[step.TemplateID] = append(r.parent.steps[step.TemplateID], *step)
	return nil
}

func (r *rbHTTPTestStepRepo) Update(_ context.Context, _ *rbdomain.Step) error { return nil }
func (r *rbHTTPTestStepRepo) DeleteByTemplateID(_ context.Context, templateID string) error {
	delete(r.parent.steps, templateID)
	return nil
}

func (r *rbHTTPTestStepRepo) ListByTemplateID(_ context.Context, templateID string) ([]rbdomain.Step, error) {
	rows := r.parent.steps[templateID]
	out := make([]rbdomain.Step, len(rows))
	copy(out, rows)
	return out, nil
}

type rbHTTPTestAlertReader struct{}

func (rbHTTPTestAlertReader) GetForExecution(_ context.Context, _ string) (*rbapp.AlertContext, error) {
	return &rbapp.AlertContext{
		ID: "a1", Name: "HighCPU", Status: "processing",
		Environment: "prod", ResourceType: "pod",
	}, nil
}

func newRunbookHTTPEngine(t *testing.T, authz *fakeRBHTTPAuthorizer, seed ...*rbdomain.Template) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret: "runbook-http-test-secret-with-length", Issuer: "aiops-test",
		AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt manager: %v", err)
	}
	token, _, err := jwtMgr.IssueAccess(auth.IssueOptions{UserID: "user-1", Username: "alice"})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	tplRepo := &rbHTTPTestTemplateRepo{steps: map[string][]rbdomain.Step{}}
	stepRepo := &rbHTTPTestStepRepo{parent: tplRepo}
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	for _, tpl := range seed {
		cp := *tpl
		if cp.CreatedAt.IsZero() {
			cp.CreatedAt = now
			cp.UpdatedAt = now
		}
		_ = tplRepo.Create(context.Background(), &cp)
	}
	svc := rbapp.NewTemplateService(tplRepo, stepRepo, rbHTTPTestAlertReader{}, rbapp.NoopAuditRecorder{})
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

func decodeRBEnvelope(t *testing.T, body []byte) apiEnvelope {
	t.Helper()
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v body=%s", err, string(body))
	}
	return env
}

type apiEnvelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	TraceID string          `json:"trace_id"`
	Data    json.RawMessage `json:"data"`
}

func TestRecommend_Unauthenticated(t *testing.T) {
	authz := &fakeRBHTTPAuthorizer{allowed: true}
	engine, _ := newRunbookHTTPEngine(t, authz)

	req := httptest.NewRequest(http.MethodGet, "/api/runbooks/recommendations?alert_id=a1", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeRBEnvelope(t, w.Body.Bytes()).Code != "UNAUTHENTICATED" {
		t.Fatal("expected UNAUTHENTICATED")
	}
}

func TestRecommend_PermissionDenied(t *testing.T) {
	authz := &fakeRBHTTPAuthorizer{allowed: false}
	engine, token := newRunbookHTTPEngine(t, authz)

	req := httptest.NewRequest(http.MethodGet, "/api/runbooks/recommendations?alert_id=a1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeRBEnvelope(t, w.Body.Bytes()).Code != "PERMISSION_DENIED" {
		t.Fatal("expected PERMISSION_DENIED")
	}
	if authz.last.Resource != "runbooks" || authz.last.Action != "read" {
		t.Fatalf("unexpected authz: %+v", authz.last)
	}
}

func TestRecommend_MissingAlertID(t *testing.T) {
	authz := &fakeRBHTTPAuthorizer{allowed: true}
	engine, token := newRunbookHTTPEngine(t, authz)

	req := httptest.NewRequest(http.MethodGet, "/api/runbooks/recommendations", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeRBEnvelope(t, w.Body.Bytes()).Code != "INVALID_ARGUMENT" {
		t.Fatal("expected INVALID_ARGUMENT")
	}
}

func TestRecommend_Success(t *testing.T) {
	authz := &fakeRBHTTPAuthorizer{allowed: true}
	engine, token := newRunbookHTTPEngine(t, authz, &rbdomain.Template{
		ID: "tpl-1", Name: "HighCPU Pod", Enabled: true,
		OperationType: rbdomain.OpRunbook, RiskLevel: rbdomain.RiskMedium,
		MatchAlertName: "HighCPU", MatchResourceType: "pod", MatchEnvironment: "prod",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/runbooks/recommendations?alert_id=a1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeRBEnvelope(t, w.Body.Bytes())
	if resp.Code != "OK" || resp.Message != "ok" {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
	var data struct {
		Items []struct {
			TemplateID    string `json:"template_id"`
			Name          string `json:"name"`
			MatchedReason string `json:"matched_reason"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 1 || data.Items[0].TemplateID != "tpl-1" {
		t.Fatalf("unexpected items: %+v", data.Items)
	}
}

func TestCreateTemplate_InvalidBody(t *testing.T) {
	authz := &fakeRBHTTPAuthorizer{allowed: true}
	engine, token := newRunbookHTTPEngine(t, authz)

	req := httptest.NewRequest(http.MethodPost, "/api/runbooks/templates", strings.NewReader(`{"name":"x"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeRBEnvelope(t, w.Body.Bytes()).Code != "INVALID_ARGUMENT" {
		t.Fatal("expected INVALID_ARGUMENT")
	}
}

func TestCreateTemplate_Success(t *testing.T) {
	authz := &fakeRBHTTPAuthorizer{allowed: true}
	engine, token := newRunbookHTTPEngine(t, authz)

	body := `{
		"name":"E2E 预案",
		"operation_type":"runbook",
		"risk_level":"medium",
		"enabled":true,
		"steps":[{"step_order":1,"name":"重启","action_type":"restart","risk_level":"medium"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/runbooks/templates", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeRBEnvelope(t, w.Body.Bytes())
	var detail struct {
		Template struct {
			ID   string `json:"template_id"`
			Name string `json:"name"`
		} `json:"template"`
		Steps []any `json:"steps"`
	}
	if err := json.Unmarshal(resp.Data, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Template.Name != "E2E 预案" || len(detail.Steps) != 1 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}
