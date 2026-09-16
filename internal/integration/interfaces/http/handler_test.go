// Integration HTTP 层契约测试（ops/cloud-observability-contract.md §4）。
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
	integapp "github.com/734965549/aiops/internal/integration/application"
	"github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/internal/integration/infrastructure/credential"
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

type httpTestAccountRepo struct {
	byID map[string]*domain.IntegrationAccount
}

func newHTTPTestAccountRepo() *httpTestAccountRepo {
	return &httpTestAccountRepo{byID: map[string]*domain.IntegrationAccount{}}
}

func (r *httpTestAccountRepo) Create(_ context.Context, account *domain.IntegrationAccount) error {
	cp := *account
	r.byID[account.AccountID] = &cp
	return nil
}

func (r *httpTestAccountRepo) Update(_ context.Context, account *domain.IntegrationAccount) error {
	if _, ok := r.byID[account.AccountID]; !ok {
		return domain.ErrNotFound
	}
	cp := *account
	r.byID[account.AccountID] = &cp
	return nil
}

func (r *httpTestAccountRepo) GetByID(_ context.Context, accountID string) (*domain.IntegrationAccount, error) {
	acc, ok := r.byID[accountID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *acc
	return &cp, nil
}

func (r *httpTestAccountRepo) List(context.Context, domain.AccountFilter) ([]domain.IntegrationAccount, error) {
	return nil, nil
}

func (r *httpTestAccountRepo) Count(context.Context, domain.AccountFilter) (int64, error) {
	return 0, nil
}

func (r *httpTestAccountRepo) SoftDelete(context.Context, string) error { return nil }

type httpTestCredentialRepo struct {
	byAccount map[string]*domain.CredentialRef
}

func newHTTPTestCredentialRepo() *httpTestCredentialRepo {
	return &httpTestCredentialRepo{byAccount: map[string]*domain.CredentialRef{}}
}

func (r *httpTestCredentialRepo) Create(_ context.Context, ref *domain.CredentialRef) error {
	cp := *ref
	r.byAccount[ref.AccountID] = &cp
	return nil
}

func (r *httpTestCredentialRepo) Update(_ context.Context, ref *domain.CredentialRef) error {
	if _, ok := r.byAccount[ref.AccountID]; !ok {
		return domain.ErrNotFound
	}
	cp := *ref
	r.byAccount[ref.AccountID] = &cp
	return nil
}

func (r *httpTestCredentialRepo) GetByAccountID(_ context.Context, accountID string) (*domain.CredentialRef, error) {
	ref, ok := r.byAccount[accountID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *ref
	return &cp, nil
}

func (r *httpTestCredentialRepo) DeleteByAccountID(context.Context, string) error { return nil }

type httpTestCapabilityRepo struct {
	byAccount map[string][]domain.Capability
}

func newHTTPTestCapabilityRepo() *httpTestCapabilityRepo {
	return &httpTestCapabilityRepo{byAccount: map[string][]domain.Capability{}}
}

func (r *httpTestCapabilityRepo) ReplaceForAccount(_ context.Context, accountID string, caps []domain.Capability) error {
	cp := append([]domain.Capability(nil), caps...)
	r.byAccount[accountID] = cp
	return nil
}

func (r *httpTestCapabilityRepo) ListByAccountID(_ context.Context, accountID string) ([]domain.Capability, error) {
	return r.byAccount[accountID], nil
}

type httpTestCheckRepo struct{}

func (httpTestCheckRepo) Create(context.Context, *domain.ConnectivityCheck) error { return nil }
func (httpTestCheckRepo) LatestByAccountID(context.Context, string) (*domain.ConnectivityCheck, error) {
	return nil, domain.ErrNotFound
}

type httpTestUnitOfWork struct {
	repos domain.TransactionRepositories
}

func (u *httpTestUnitOfWork) WithinTransaction(ctx context.Context, fn func(context.Context, domain.TransactionRepositories) error) error {
	return fn(ctx, u.repos)
}

func newIntegrationHTTPEngine(t *testing.T, authz *fakeHTTPAuthorizer) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret:     "integration-http-test-secret-with-length",
		Issuer:     "aiops-test",
		AccessTTL:  time.Hour,
		RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt manager: %v", err)
	}
	token, _, err := jwtMgr.IssueAccess(auth.IssueOptions{UserID: "user-1", Username: "alice"})
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}

	accounts := newHTTPTestAccountRepo()
	creds := newHTTPTestCredentialRepo()
	caps := newHTTPTestCapabilityRepo()
	checks := httpTestCheckRepo{}
	vault, err := credential.NewVault("unit-test-credential-key", 1)
	if err != nil {
		t.Fatalf("new vault: %v", err)
	}
	uow := &httpTestUnitOfWork{repos: domain.TransactionRepositories{
		Accounts: accounts, Credentials: creds, Capabilities: caps, Checks: checks,
	}}
	svc := integapp.NewAccountService(accounts, creds, caps, checks, vault, nil, nil, uow)
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

type apiEnvelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	TraceID string          `json:"trace_id"`
	Data    json.RawMessage `json:"data"`
}

type accountResponse struct {
	AccountID     string   `json:"account_id"`
	Name          string   `json:"name"`
	Provider      string   `json:"provider"`
	AuthType      string   `json:"auth_type"`
	HasCredential bool     `json:"has_credential"`
	Capabilities  []string `json:"capabilities"`
}

func decodeAPIEnvelope(t *testing.T, body []byte) apiEnvelope {
	t.Helper()
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v body=%s", err, string(body))
	}
	return env
}

func TestCreateAccount_AuthNoneOmitsCredential(t *testing.T) {
	// §4.1：auth_type=none 时 credential 可省略；防止 DTO binding:"required" 回归。
	authz := &fakeHTTPAuthorizer{allowed: true}
	engine, token := newIntegrationHTTPEngine(t, authz)

	body := `{"name":"prom-no-auth","provider":"prometheus","auth_type":"none"}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/accounts", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	env := decodeAPIEnvelope(t, w.Body.Bytes())
	if env.Code != "OK" {
		t.Fatalf("expected code OK, got %q body=%s", env.Code, w.Body.String())
	}
	if env.TraceID == "" {
		t.Fatalf("trace_id missing: %s", w.Body.String())
	}

	var data accountResponse
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v body=%s", err, w.Body.String())
	}
	if data.AccountID == "" {
		t.Fatal("expected account_id in response")
	}
	if data.AuthType != string(domain.AuthNone) {
		t.Fatalf("expected auth_type none, got %q", data.AuthType)
	}
	if data.HasCredential {
		t.Fatal("expected has_credential=false when credential omitted")
	}
	if authz.last.Resource != integrationAuthResource || authz.last.Action != "create" {
		t.Fatalf("unexpected authz input: %+v", authz.last)
	}
}

func TestCreateAccount_AuthNoneEmptyCredentialObject(t *testing.T) {
	authz := &fakeHTTPAuthorizer{allowed: true}
	engine, token := newIntegrationHTTPEngine(t, authz)

	body := `{"name":"prom-empty-cred","provider":"prometheus","auth_type":"none","credential":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/accounts", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	env := decodeAPIEnvelope(t, w.Body.Bytes())
	if env.Code != "OK" {
		t.Fatalf("expected code OK, got %q body=%s", env.Code, w.Body.String())
	}
	var data accountResponse
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.HasCredential {
		t.Fatal("expected has_credential=false for empty credential object")
	}
}

func TestCreateAccount_NonNoneMissingCredential(t *testing.T) {
	// 非 none 类型缺 credential 应在 application 层拒绝，而非 handler binding 误判。
	authz := &fakeHTTPAuthorizer{allowed: true}
	engine, token := newIntegrationHTTPEngine(t, authz)

	body := `{"name":"prom-aksk","provider":"prometheus","auth_type":"ak_sk"}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/accounts", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	env := decodeAPIEnvelope(t, w.Body.Bytes())
	if env.Code != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT from application validation, got %q body=%s", env.Code, w.Body.String())
	}
}

func TestCreateAccount_MissingRequiredField(t *testing.T) {
	authz := &fakeHTTPAuthorizer{allowed: true}
	engine, token := newIntegrationHTTPEngine(t, authz)

	body := `{"provider":"prometheus","auth_type":"none"}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/accounts", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeAPIEnvelope(t, w.Body.Bytes()).Code != "INVALID_ARGUMENT" {
		t.Fatal("expected INVALID_ARGUMENT for missing name")
	}
}

func TestCreateAccount_Unauthenticated(t *testing.T) {
	authz := &fakeHTTPAuthorizer{allowed: true}
	engine, _ := newIntegrationHTTPEngine(t, authz)

	body := `{"name":"prom","provider":"prometheus","auth_type":"none"}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/accounts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeAPIEnvelope(t, w.Body.Bytes()).Code != "UNAUTHENTICATED" {
		t.Fatal("expected UNAUTHENTICATED")
	}
}

func TestCreateAccount_PermissionDenied(t *testing.T) {
	authz := &fakeHTTPAuthorizer{allowed: false}
	engine, token := newIntegrationHTTPEngine(t, authz)

	body := `{"name":"prom","provider":"prometheus","auth_type":"none"}`
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/accounts", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeAPIEnvelope(t, w.Body.Bytes()).Code != "PERMISSION_DENIED" {
		t.Fatal("expected PERMISSION_DENIED")
	}
}
