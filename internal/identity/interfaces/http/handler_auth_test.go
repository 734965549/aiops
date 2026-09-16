package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/auth"
	"github.com/734965549/aiops/pkg/config"
	"github.com/gin-gonic/gin"
)

func TestHTTPLoginAndRefreshIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := newAuthTestEngine(t)

	loginBody := `{"username":"alice","password":"secret123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/identity/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	engine.ServeHTTP(loginW, loginReq)

	if loginW.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginW.Code, loginW.Body.String())
	}
	loginResp := decodeAPIEnvelope(t, loginW.Body.Bytes())
	if loginResp.Code != "OK" {
		t.Fatalf("login code=%q body=%s", loginResp.Code, loginW.Body.String())
	}
	tokens := decodeTokenPair(t, loginResp.Data)
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatalf("missing tokens: %+v", tokens)
	}

	refreshPayload, _ := json.Marshal(map[string]string{"refresh_token": tokens.RefreshToken})
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/identity/refresh", bytes.NewReader(refreshPayload))
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshW := httptest.NewRecorder()
	engine.ServeHTTP(refreshW, refreshReq)

	if refreshW.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", refreshW.Code, refreshW.Body.String())
	}
	refreshResp := decodeAPIEnvelope(t, refreshW.Body.Bytes())
	refreshed := decodeTokenPair(t, refreshResp.Data)
	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatalf("missing refreshed tokens: %+v", refreshed)
	}
	if refreshed.RefreshToken == tokens.RefreshToken {
		t.Fatal("expected new refresh token after refresh")
	}
}

func TestHTTPLoginWritesAuthAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	audits := &fakeAuthAuditService{}
	engine := newAuthTestEngineWithAudit(t, audits)

	okReq := httptest.NewRequest(http.MethodPost, "/api/identity/login", strings.NewReader(`{"username":"alice","password":"secret123"}`))
	okReq.Header.Set("Content-Type", "application/json")
	okReq.Header.Set("User-Agent", "audit-test")
	okW := httptest.NewRecorder()
	engine.ServeHTTP(okW, okReq)
	if okW.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", okW.Code, okW.Body.String())
	}

	badReq := httptest.NewRequest(http.MethodPost, "/api/identity/login", strings.NewReader(`{"username":"alice","password":"wrong"}`))
	badReq.Header.Set("Content-Type", "application/json")
	badW := httptest.NewRecorder()
	engine.ServeHTTP(badW, badReq)
	if badW.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status=%d body=%s", badW.Code, badW.Body.String())
	}

	if len(audits.rows) != 2 {
		t.Fatalf("expected 2 audit rows, got %d: %+v", len(audits.rows), audits.rows)
	}
	if audits.rows[0].Result != domain.AuthAuditResultSuccess || audits.rows[0].UserID != "user-1" || audits.rows[0].Method != domain.AuthAuditMethodLocal {
		t.Fatalf("unexpected success audit: %+v", audits.rows[0])
	}
	if audits.rows[1].Result != domain.AuthAuditResultFailure || audits.rows[1].Reason != "authentication failed" {
		t.Fatalf("unexpected failure audit: %+v", audits.rows[1])
	}
}

func TestHTTPLoginInvalidCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := newAuthTestEngine(t)

	body := `{"username":"alice","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/identity/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	if resp.Code != "UNAUTHENTICATED" {
		t.Fatalf("expected UNAUTHENTICATED, got %q", resp.Code)
	}
}

func TestHTTPLoginRejectsIPOutsideAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	audits := &fakeAuthAuditService{}
	allowlist, err := auth.NewIPAllowlist([]string{"198.51.100.0/24"})
	if err != nil {
		t.Fatalf("allowlist: %v", err)
	}
	engine := newAuthTestEngineWithAuditAndIP(t, audits, allowlist)

	req := httptest.NewRequest(http.MethodPost, "/api/identity/login", strings.NewReader(`{"username":"alice","password":"secret123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.10:12345"
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	if resp.Code != "PERMISSION_DENIED" {
		t.Fatalf("expected PERMISSION_DENIED, got %q", resp.Code)
	}
	if len(audits.rows) != 1 || audits.rows[0].Reason != "client ip is not allowed" {
		t.Fatalf("unexpected audit rows: %+v", audits.rows)
	}
}

func newAuthTestEngine(t *testing.T) *gin.Engine {
	return newAuthTestEngineWithAudit(t, nil)
}

func newAuthTestEngineWithAudit(t *testing.T, audit AuthAuditService) *gin.Engine {
	return newAuthTestEngineWithAuditAndIP(t, audit, nil)
}

func newAuthTestEngineWithAuditAndIP(t *testing.T, audit AuthAuditService, allowlist LoginIPAllowlist) *gin.Engine {
	t.Helper()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	repo := newFakeAuthUserRepo(&domain.User{
		ID: "user-1", Username: "alice", PasswordHash: hash, Status: domain.UserStatusActive,
	})
	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret:     "http-auth-test-secret-with-length",
		Issuer:     "aiops-test",
		AccessTTL:  time.Hour,
		RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt manager: %v", err)
	}
	authSvc := identityapp.NewAuthService(repo, jwtMgr, auth.NoopRefreshTokenStore{}, nil, nil, nil, nil, nil, "")
	userSvc := identityapp.NewUserService(repo)
	handler := NewHandler(userSvc, authSvc, nil, nil, auth.NoopLoginAttemptLimiter{}, audit, allowlist)

	return server.NewEngine(server.Options{
		Cfg: &config.Config{
			App:      config.AppConfig{Env: "dev", Timezone: "Asia/Shanghai"},
			Server:   config.ServerConfig{Port: 8080},
			Database: config.DatabaseConfig{Host: "127.0.0.1", Name: "aiops", SSLMode: "disable"},
			Auth:     config.AuthConfig{JWTSecret: config.DefaultJWTSecretPlaceholder},
		},
		Authenticator: auth.NewJWTAuthenticator(jwtMgr),
		Registrars:    []server.RouteRegistrar{NewRegistrar(handler, nil)},
		StartedAt:     time.Now(),
	})
}

type fakeAuthUserRepo struct {
	byID       map[string]*domain.User
	byUsername map[string]*domain.User
}

func newFakeAuthUserRepo(users ...*domain.User) *fakeAuthUserRepo {
	repo := &fakeAuthUserRepo{
		byID:       make(map[string]*domain.User),
		byUsername: make(map[string]*domain.User),
	}
	for _, u := range users {
		repo.byID[u.ID] = u
		repo.byUsername[strings.ToLower(u.Username)] = u
	}
	return repo
}

func (f *fakeAuthUserRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	return f.byID[id], nil
}

func (f *fakeAuthUserRepo) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	return f.byUsername[strings.ToLower(strings.TrimSpace(username))], nil
}

func (f *fakeAuthUserRepo) Create(_ context.Context, u *domain.User) error {
	key := strings.ToLower(u.Username)
	if _, ok := f.byUsername[key]; ok {
		return domain.ErrAlreadyExists
	}
	f.byUsername[key] = u
	f.byID[u.ID] = u
	return nil
}

func (f *fakeAuthUserRepo) Update(_ context.Context, u *domain.User) error {
	if existing, ok := f.byID[u.ID]; ok {
		existing.DisplayName = u.DisplayName
		existing.Email = u.Email
		existing.Status = u.Status
	}
	return nil
}

func (f *fakeAuthUserRepo) Reactivate(_ context.Context, userID, passwordHash string) error {
	if existing, ok := f.byID[userID]; ok {
		existing.PasswordHash = passwordHash
		existing.Status = domain.UserStatusActive
	}
	return nil
}

func (f *fakeAuthUserRepo) DeleteByID(_ context.Context, id string) error {
	u, ok := f.byID[id]
	if !ok {
		return nil
	}
	delete(f.byID, id)
	delete(f.byUsername, strings.ToLower(u.Username))
	return nil
}

type fakeAuthAuditService struct {
	rows []domain.AuthAudit
}

func (f *fakeAuthAuditService) Record(_ context.Context, audit domain.AuthAudit) error {
	f.rows = append(f.rows, audit)
	return nil
}

func (f *fakeAuthAuditService) List(_ context.Context, _ domain.AuthAuditFilter) ([]domain.AuthAudit, error) {
	return f.rows, nil
}

func (f *fakeAuthAuditService) Count(_ context.Context, _ domain.AuthAuditFilter) (int64, error) {
	return int64(len(f.rows)), nil
}

type apiEnvelope struct {
	Code string          `json:"code"`
	Data json.RawMessage `json:"data"`
}

type tokenPairResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func decodeAPIEnvelope(t *testing.T, body []byte) apiEnvelope {
	t.Helper()
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v body=%s", err, string(body))
	}
	return env
}

func decodeTokenPair(t *testing.T, raw json.RawMessage) tokenPairResponse {
	t.Helper()
	var pair tokenPairResponse
	if err := json.Unmarshal(raw, &pair); err != nil {
		t.Fatalf("unmarshal token pair: %v raw=%s", err, string(raw))
	}
	return pair
}
