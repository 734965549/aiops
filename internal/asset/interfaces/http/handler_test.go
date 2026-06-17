package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	assetapp "github.com/734965549/aiops/internal/asset/application"
	assetdomain "github.com/734965549/aiops/internal/asset/domain"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/auth"
	"github.com/734965549/aiops/pkg/config"
	"github.com/gin-gonic/gin"
)

type fakeAssetHTTPAuthorizer struct {
	allowed bool
	last    identityapp.AuthorizationInput
}

func (f *fakeAssetHTTPAuthorizer) Authorize(_ context.Context, in identityapp.AuthorizationInput) (*identityapp.AuthorizationResult, error) {
	f.last = in
	return &identityapp.AuthorizationResult{Allowed: f.allowed}, nil
}

type assetHTTPTestAppRepo struct {
	apps []assetdomain.Application
}

func (r *assetHTTPTestAppRepo) Create(_ context.Context, app *assetdomain.Application) error {
	cp := *app
	if cp.CreatedAt.IsZero() {
		now := time.Now().UTC()
		cp.CreatedAt = now
		cp.UpdatedAt = now
	}
	r.apps = append(r.apps, cp)
	*app = cp
	return nil
}

func (r *assetHTTPTestAppRepo) List(_ context.Context) ([]assetdomain.Application, error) {
	out := make([]assetdomain.Application, len(r.apps))
	copy(out, r.apps)
	return out, nil
}

func (r *assetHTTPTestAppRepo) Count(_ context.Context) (int64, error) {
	return int64(len(r.apps)), nil
}

func (r *assetHTTPTestAppRepo) FindByNameEnv(_ context.Context, name, environment string) (*assetdomain.Application, error) {
	for i := range r.apps {
		app := r.apps[i]
		if strings.EqualFold(app.Name, name) {
			if environment == "" || app.Environment == "" || strings.EqualFold(app.Environment, environment) {
				cp := app
				return &cp, nil
			}
		}
	}
	return nil, assetdomain.ErrNotFound
}

func (r *assetHTTPTestAppRepo) ExistsByID(_ context.Context, id string) (bool, error) {
	for _, app := range r.apps {
		if app.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (r *assetHTTPTestAppRepo) GetByID(_ context.Context, id string) (*assetdomain.Application, error) {
	for i := range r.apps {
		if r.apps[i].ID == id {
			cp := r.apps[i]
			return &cp, nil
		}
	}
	return nil, assetdomain.ErrNotFound
}

func (r *assetHTTPTestAppRepo) Update(_ context.Context, app *assetdomain.Application) error {
	for i := range r.apps {
		if r.apps[i].ID == app.ID {
			cp := *app
			cp.UpdatedAt = time.Now().UTC()
			r.apps[i] = cp
			*app = cp
			return nil
		}
	}
	return assetdomain.ErrNotFound
}

func (r *assetHTTPTestAppRepo) Delete(_ context.Context, id string) error {
	for i := range r.apps {
		if r.apps[i].ID == id {
			r.apps = append(r.apps[:i], r.apps[i+1:]...)
			return nil
		}
	}
	return assetdomain.ErrNotFound
}

type assetHTTPTestResRepo struct {
	rows []assetdomain.Resource
}

func (r *assetHTTPTestResRepo) Create(_ context.Context, res *assetdomain.Resource) error {
	cp := *res
	if cp.CreatedAt.IsZero() {
		now := time.Now().UTC()
		cp.CreatedAt = now
		cp.UpdatedAt = now
	}
	r.rows = append(r.rows, cp)
	*res = cp
	return nil
}

func (r *assetHTTPTestResRepo) Count(_ context.Context) (int64, error) {
	return int64(len(r.rows)), nil
}

func (r *assetHTTPTestResRepo) ListByApplicationID(_ context.Context, applicationID string) ([]assetdomain.Resource, error) {
	out := make([]assetdomain.Resource, 0)
	for _, row := range r.rows {
		if row.ApplicationID == applicationID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *assetHTTPTestResRepo) FindBestMatch(_ context.Context, q assetdomain.ResourceMatchQuery) (*assetdomain.Resource, error) {
	for i := range r.rows {
		row := r.rows[i]
		if row.ApplicationID != q.ApplicationID {
			continue
		}
		if q.Pod != "" && strings.EqualFold(row.Pod, q.Pod) {
			cp := row
			return &cp, nil
		}
	}
	return nil, assetdomain.ErrNotFound
}

func (r *assetHTTPTestResRepo) GetByID(_ context.Context, id string) (*assetdomain.Resource, error) {
	for i := range r.rows {
		if r.rows[i].ID == id {
			cp := r.rows[i]
			return &cp, nil
		}
	}
	return nil, assetdomain.ErrNotFound
}

func (r *assetHTTPTestResRepo) Update(_ context.Context, res *assetdomain.Resource) error {
	for i := range r.rows {
		if r.rows[i].ID == res.ID {
			cp := *res
			cp.UpdatedAt = time.Now().UTC()
			r.rows[i] = cp
			*res = cp
			return nil
		}
	}
	return assetdomain.ErrNotFound
}

func (r *assetHTTPTestResRepo) Delete(_ context.Context, id string) error {
	for i := range r.rows {
		if r.rows[i].ID == id {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return assetdomain.ErrNotFound
}

func (r *assetHTTPTestResRepo) CountByApplicationID(_ context.Context, applicationID string) (int64, error) {
	var n int64
	for _, row := range r.rows {
		if row.ApplicationID == applicationID {
			n++
		}
	}
	return n, nil
}

type assetHTTPTestRuleRepo struct {
	rows []assetdomain.MatchRule
}

func (r *assetHTTPTestRuleRepo) Create(_ context.Context, rule *assetdomain.MatchRule) error {
	r.rows = append(r.rows, *rule)
	return nil
}

func (r *assetHTTPTestRuleRepo) List(_ context.Context) ([]assetdomain.MatchRule, error) {
	out := make([]assetdomain.MatchRule, len(r.rows))
	copy(out, r.rows)
	return out, nil
}

func (r *assetHTTPTestRuleRepo) ListEnabledByPriority(_ context.Context) ([]assetdomain.MatchRule, error) {
	out := make([]assetdomain.MatchRule, 0)
	for _, row := range r.rows {
		if row.Enabled {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *assetHTTPTestRuleRepo) GetByID(_ context.Context, id string) (*assetdomain.MatchRule, error) {
	for i := range r.rows {
		if r.rows[i].ID == id {
			cp := r.rows[i]
			return &cp, nil
		}
	}
	return nil, assetdomain.ErrNotFound
}

func (r *assetHTTPTestRuleRepo) Update(_ context.Context, rule *assetdomain.MatchRule) error {
	for i := range r.rows {
		if r.rows[i].ID == rule.ID {
			r.rows[i] = *rule
			return nil
		}
	}
	return assetdomain.ErrNotFound
}

func (r *assetHTTPTestRuleRepo) Delete(_ context.Context, id string) error {
	for i := range r.rows {
		if r.rows[i].ID == id {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return assetdomain.ErrNotFound
}

func (r *assetHTTPTestRuleRepo) CountByApplicationID(_ context.Context, applicationID string) (int64, error) {
	var n int64
	for _, row := range r.rows {
		if row.ApplicationID == applicationID {
			n++
		}
	}
	return n, nil
}

func (r *assetHTTPTestRuleRepo) CountByResourceID(_ context.Context, resourceID string) (int64, error) {
	var n int64
	for _, row := range r.rows {
		if row.ResourceID == resourceID {
			n++
		}
	}
	return n, nil
}

func newAssetHTTPEngine(t *testing.T, authz *fakeAssetHTTPAuthorizer) (*gin.Engine, string, *assetHTTPTestAppRepo, *assetHTTPTestResRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret: "asset-http-test-secret-with-length", Issuer: "aiops-test",
		AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt manager: %v", err)
	}
	token, _, err := jwtMgr.IssueAccess(auth.IssueOptions{UserID: "user-1", Username: "alice"})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	appRepo := &assetHTTPTestAppRepo{}
	resRepo := &assetHTTPTestResRepo{}
	ruleRepo := &assetHTTPTestRuleRepo{}
	svc := assetapp.NewAssetService(appRepo, resRepo, ruleRepo, assetapp.NoopAuditRecorder{})
	matchRules := assetapp.NewMatchRuleService(ruleRepo, appRepo, resRepo, assetapp.NoopAuditRecorder{})
	handler := NewHandler(svc, matchRules)
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
	return engine, token, appRepo, resRepo
}

func decodeAssetEnvelope(t *testing.T, body []byte) assetAPIEnvelope {
	t.Helper()
	var env assetAPIEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v body=%s", err, string(body))
	}
	return env
}

type assetAPIEnvelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	TraceID string          `json:"trace_id"`
	Data    json.RawMessage `json:"data"`
}

func TestListApplications_Unauthenticated(t *testing.T) {
	authz := &fakeAssetHTTPAuthorizer{allowed: true}
	engine, _, _, _ := newAssetHTTPEngine(t, authz)

	req := httptest.NewRequest(http.MethodGet, "/api/assets/applications", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeAssetEnvelope(t, w.Body.Bytes()).Code != "UNAUTHENTICATED" {
		t.Fatal("expected UNAUTHENTICATED")
	}
}

func TestListApplications_PermissionDenied(t *testing.T) {
	authz := &fakeAssetHTTPAuthorizer{allowed: false}
	engine, token, _, _ := newAssetHTTPEngine(t, authz)

	req := httptest.NewRequest(http.MethodGet, "/api/assets/applications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeAssetEnvelope(t, w.Body.Bytes()).Code != "PERMISSION_DENIED" {
		t.Fatal("expected PERMISSION_DENIED")
	}
	if authz.last.Resource != "assets" || authz.last.Action != "read" {
		t.Fatalf("unexpected authz: %+v", authz.last)
	}
}

func TestCreateApplication_Success(t *testing.T) {
	authz := &fakeAssetHTTPAuthorizer{allowed: true}
	engine, token, appRepo, _ := newAssetHTTPEngine(t, authz)

	body := `{"name":"payment-service","environment":"prod","namespace":"payment"}`
	req := httptest.NewRequest(http.MethodPost, "/api/assets/applications", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAssetEnvelope(t, w.Body.Bytes())
	if resp.Code != "OK" {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
	var app struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp.Data, &app); err != nil {
		t.Fatal(err)
	}
	if app.Name != "payment-service" || app.ID == "" {
		t.Fatalf("unexpected app: %+v", app)
	}
	if len(appRepo.apps) != 1 {
		t.Fatalf("expected 1 app in repo, got %d", len(appRepo.apps))
	}
}

func TestCreateApplication_InvalidBody(t *testing.T) {
	authz := &fakeAssetHTTPAuthorizer{allowed: true}
	engine, token, _, _ := newAssetHTTPEngine(t, authz)

	req := httptest.NewRequest(http.MethodPost, "/api/assets/applications", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeAssetEnvelope(t, w.Body.Bytes()).Code != "INVALID_ARGUMENT" {
		t.Fatal("expected INVALID_ARGUMENT")
	}
}

func TestCreateResource_Success(t *testing.T) {
	authz := &fakeAssetHTTPAuthorizer{allowed: true}
	engine, token, appRepo, resRepo := newAssetHTTPEngine(t, authz)
	_ = appRepo.Create(context.Background(), &assetdomain.Application{ID: "app-1", Name: "order-service", Environment: "prod"})

	body := `{"application_id":"app-1","name":"order-pod","resource_type":"pod","pod":"order-xxx-1","namespace":"order"}`
	req := httptest.NewRequest(http.MethodPost, "/api/assets/resources", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAssetEnvelope(t, w.Body.Bytes())
	var res struct {
		ID            string `json:"id"`
		ApplicationID string `json:"application_id"`
		Pod           string `json:"pod"`
	}
	if err := json.Unmarshal(resp.Data, &res); err != nil {
		t.Fatal(err)
	}
	if res.ApplicationID != "app-1" || res.Pod != "order-xxx-1" || res.ID == "" {
		t.Fatalf("unexpected resource: %+v", res)
	}
	if len(resRepo.rows) != 1 {
		t.Fatalf("expected 1 resource in repo, got %d", len(resRepo.rows))
	}
}

func TestCreateResource_ApplicationNotFound(t *testing.T) {
	authz := &fakeAssetHTTPAuthorizer{allowed: true}
	engine, token, _, _ := newAssetHTTPEngine(t, authz)

	body := `{"application_id":"missing-app","name":"order-pod","resource_type":"pod"}`
	req := httptest.NewRequest(http.MethodPost, "/api/assets/resources", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeAssetEnvelope(t, w.Body.Bytes()).Code != "NOT_FOUND" {
		t.Fatal("expected NOT_FOUND")
	}
}

func TestListResources_Success(t *testing.T) {
	authz := &fakeAssetHTTPAuthorizer{allowed: true}
	engine, token, appRepo, resRepo := newAssetHTTPEngine(t, authz)
	_ = appRepo.Create(context.Background(), &assetdomain.Application{ID: "app-1", Name: "svc", Environment: "prod"})
	_ = resRepo.Create(context.Background(), &assetdomain.Resource{ID: "res-1", ApplicationID: "app-1", Pod: "p1"})

	req := httptest.NewRequest(http.MethodGet, "/api/assets/applications/app-1/resources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAssetEnvelope(t, w.Body.Bytes())
	var data struct {
		Items []struct {
			ID  string `json:"id"`
			Pod string `json:"pod"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 1 || data.Items[0].Pod != "p1" {
		t.Fatalf("unexpected items: %+v", data.Items)
	}
}

func TestUpdateApplication_Success(t *testing.T) {
	authz := &fakeAssetHTTPAuthorizer{allowed: true}
	engine, token, appRepo, _ := newAssetHTTPEngine(t, authz)
	_ = appRepo.Create(context.Background(), &assetdomain.Application{ID: "app-1", Name: "svc", Environment: "prod"})

	body := `{"name":"svc-v2","environment":"staging","description":"updated"}`
	req := httptest.NewRequest(http.MethodPut, "/api/assets/applications/app-1", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAssetEnvelope(t, w.Body.Bytes())
	var app struct {
		Name        string `json:"name"`
		Environment string `json:"environment"`
	}
	if err := json.Unmarshal(resp.Data, &app); err != nil {
		t.Fatal(err)
	}
	if app.Name != "svc-v2" || app.Environment != "staging" {
		t.Fatalf("unexpected app: %+v", app)
	}
}

func TestDeleteApplication_BlockedWhenHasResources(t *testing.T) {
	authz := &fakeAssetHTTPAuthorizer{allowed: true}
	engine, token, appRepo, resRepo := newAssetHTTPEngine(t, authz)
	_ = appRepo.Create(context.Background(), &assetdomain.Application{ID: "app-1", Name: "svc"})
	_ = resRepo.Create(context.Background(), &assetdomain.Resource{ID: "res-1", ApplicationID: "app-1", Pod: "p1"})

	req := httptest.NewRequest(http.MethodDelete, "/api/assets/applications/app-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusPreconditionFailed {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeAssetEnvelope(t, w.Body.Bytes()).Code != "FAILED_PRECONDITION" {
		t.Fatal("expected FAILED_PRECONDITION")
	}
}

func TestDeleteResource_Success(t *testing.T) {
	authz := &fakeAssetHTTPAuthorizer{allowed: true}
	engine, token, appRepo, resRepo := newAssetHTTPEngine(t, authz)
	_ = appRepo.Create(context.Background(), &assetdomain.Application{ID: "app-1", Name: "svc"})
	_ = resRepo.Create(context.Background(), &assetdomain.Resource{ID: "res-1", ApplicationID: "app-1", Pod: "p1"})

	req := httptest.NewRequest(http.MethodDelete, "/api/assets/resources/res-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if len(resRepo.rows) != 0 {
		t.Fatal("resource should be deleted")
	}
}
