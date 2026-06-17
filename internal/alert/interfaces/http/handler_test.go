// Alert HTTP 层契约测试。
//
// §2 统一响应：code / message / trace_id / data envelope 校验。
// §3.1 管理端：Bearer 必需（401 UNAUTHENTICATED）、RBAC（403 PERMISSION_DENIED）。
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	alertapp "github.com/734965549/aiops/internal/alert/application"
	"github.com/734965549/aiops/internal/alert/domain"
	identityapp "github.com/734965549/aiops/internal/identity/application"
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

type httpTestAlertRepo struct {
	mu   sync.Mutex
	byID map[string]*domain.Alert
}

func newHTTPTestAlertRepo(alerts ...*domain.Alert) *httpTestAlertRepo {
	r := &httpTestAlertRepo{byID: map[string]*domain.Alert{}}
	for _, a := range alerts {
		cp := *a
		r.byID[a.ID] = &cp
	}
	return r
}

func (r *httpTestAlertRepo) Create(_ context.Context, alert *domain.Alert) error { return nil }
func (r *httpTestAlertRepo) Update(_ context.Context, alert *domain.Alert) error { return nil }
func (r *httpTestAlertRepo) GetByID(_ context.Context, alertID string) (*domain.Alert, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[alertID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}
func (r *httpTestAlertRepo) FindActiveByDedupKey(_ context.Context, _, _ string) (*domain.Alert, error) {
	return nil, domain.ErrNotFound
}
func (r *httpTestAlertRepo) MaxLifecycleSeq(_ context.Context, _ string) (int, error) { return 0, nil }
func (r *httpTestAlertRepo) List(_ context.Context, filter domain.AlertFilter) ([]domain.Alert, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := make([]domain.Alert, 0, len(r.byID))
	for _, a := range r.byID {
		all = append(all, *a)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	offset := filter.Offset
	if offset >= len(all) {
		return []domain.Alert{}, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = len(all) - offset
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}
func (r *httpTestAlertRepo) Count(_ context.Context, _ domain.AlertFilter) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.byID)), nil
}

type httpTestEventRepo struct{}

func (httpTestEventRepo) Create(_ context.Context, _ *domain.AlertEvent) error { return nil }
func (httpTestEventRepo) ListByAlertID(_ context.Context, _ string) ([]domain.AlertEvent, error) {
	return nil, nil
}

func newAlertHTTPEngine(t *testing.T, authz *fakeHTTPAuthorizer, alerts *httpTestAlertRepo) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret:     "alert-http-test-secret-with-length",
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

	alertSvc := alertapp.NewAlertService(alerts, httpTestEventRepo{}, nil, alertapp.NoopAuditRecorder{})
	handler := NewHandler(alertSvc, nil)
	registrar := NewRegistrar(handler, nil, authz)

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

func TestGetAlert_Unauthenticated(t *testing.T) {
	// §3.1：管理端缺少 Bearer → 401 UNAUTHENTICATED
	authz := &fakeHTTPAuthorizer{allowed: true}
	engine, _ := newAlertHTTPEngine(t, authz, newHTTPTestAlertRepo())

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/a1", nil)
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

func TestGetAlert_PermissionDenied(t *testing.T) {
	// §3.1：无 app:alerts:read → 403 PERMISSION_DENIED
	authz := &fakeHTTPAuthorizer{allowed: false}
	now := time.Now()
	engine, token := newAlertHTTPEngine(t, authz, newHTTPTestAlertRepo(&domain.Alert{
		ID: "a1", SourceID: "src1", DedupKey: "dk1", Name: "HighCPU",
		Severity: domain.SeverityP1, Status: domain.StatusNew,
		Labels: map[string]string{}, Annotations: map[string]string{},
		OccurrenceCount: 1, FirstSeenAt: now, LastSeenAt: now,
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/a1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	if resp.Code != "PERMISSION_DENIED" {
		t.Fatalf("expected PERMISSION_DENIED, got %q", resp.Code)
	}
	if authz.last.Resource != "alerts" || authz.last.Action != "read" {
		t.Fatalf("unexpected authz input: %+v", authz.last)
	}
}

func TestGetAlert_Success(t *testing.T) {
	authz := &fakeHTTPAuthorizer{allowed: true}
	now := time.Now()
	engine, token := newAlertHTTPEngine(t, authz, newHTTPTestAlertRepo(&domain.Alert{
		ID: "a1", SourceID: "src1", DedupKey: "dk1", Name: "HighCPU",
		Severity: domain.SeverityP1, Status: domain.StatusNew,
		Labels: map[string]string{}, Annotations: map[string]string{},
		OccurrenceCount: 1, FirstSeenAt: now, LastSeenAt: now,
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/a1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	if resp.Code != "OK" {
		t.Fatalf("expected OK, got %q body=%s", resp.Code, w.Body.String())
	}
	if resp.Message != "ok" {
		t.Fatalf("expected message ok, got %q", resp.Message)
	}
	if resp.TraceID == "" {
		t.Fatalf("expected trace_id in response, body=%s", w.Body.String())
	}
	var detail struct {
		Alert struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"alert"`
		Events []any `json:"events"`
	}
	if err := json.Unmarshal(resp.Data, &detail); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if detail.Alert.ID != "a1" || detail.Alert.Name != "HighCPU" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
}

func TestGetAlert_NotFound(t *testing.T) {
	authz := &fakeHTTPAuthorizer{allowed: true}
	engine, token := newAlertHTTPEngine(t, authz, newHTTPTestAlertRepo())

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/missing", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	if resp.Code != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND, got %q", resp.Code)
	}
}

func TestAcknowledge_PermissionDenied(t *testing.T) {
	// §3.1：认领需 app:alerts:acknowledge
	authz := &fakeHTTPAuthorizer{allowed: false}
	now := time.Now()
	engine, token := newAlertHTTPEngine(t, authz, newHTTPTestAlertRepo(&domain.Alert{
		ID: "a1", SourceID: "src1", DedupKey: "dk1", Name: "HighCPU",
		Severity: domain.SeverityP1, Status: domain.StatusNew,
		Labels: map[string]string{}, Annotations: map[string]string{},
		OccurrenceCount: 1, FirstSeenAt: now, LastSeenAt: now,
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/a1/acknowledge", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	if resp.Code != "PERMISSION_DENIED" {
		t.Fatalf("expected PERMISSION_DENIED, got %q", resp.Code)
	}
	if authz.last.Resource != "alerts" || authz.last.Action != "acknowledge" {
		t.Fatalf("unexpected authz input: %+v", authz.last)
	}
}

func TestListAlerts_Success(t *testing.T) {
	// §2：成功 envelope + PageData（items/total/page/page_size）+ trace_id 回显
	authz := &fakeHTTPAuthorizer{allowed: true}
	now := time.Now()
	repo := newHTTPTestAlertRepo(
		&domain.Alert{
			ID: "a1", SourceID: "src1", DedupKey: "dk1", Name: "HighCPU",
			Severity: domain.SeverityP1, Status: domain.StatusNew,
			Labels: map[string]string{}, Annotations: map[string]string{},
			OccurrenceCount: 1, FirstSeenAt: now, LastSeenAt: now,
		},
		&domain.Alert{
			ID: "a2", SourceID: "src1", DedupKey: "dk2", Name: "HighMem",
			Severity: domain.SeverityP2, Status: domain.StatusNew,
			Labels: map[string]string{}, Annotations: map[string]string{},
			OccurrenceCount: 1, FirstSeenAt: now, LastSeenAt: now,
		},
	)
	engine, token := newAlertHTTPEngine(t, authz, repo)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?page=1&page_size=10", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Trace-Id", "550e8400-e29b-41d4-a716-446655440001")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	if resp.Code != "OK" || resp.Message != "ok" {
		t.Fatalf("unexpected envelope: %+v body=%s", resp, w.Body.String())
	}
	if resp.TraceID != "550e8400-e29b-41d4-a716-446655440001" {
		t.Fatalf("expected trace_id echo, got %q", resp.TraceID)
	}
	var page struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
	}
	if err := json.Unmarshal(resp.Data, &page); err != nil {
		t.Fatalf("unmarshal page data: %v", err)
	}
	if page.Total != 2 || page.Page != 1 || page.PageSize != 10 || len(page.Items) != 2 {
		t.Fatalf("unexpected page data: %+v", page)
	}
	if page.Items[0].ID != "a1" || page.Items[1].ID != "a2" {
		t.Fatalf("unexpected items order: %+v", page.Items)
	}
	if authz.last.Resource != "alerts" || authz.last.Action != "read" {
		t.Fatalf("unexpected authz input: %+v", authz.last)
	}
}

func TestListAlerts_DefaultPagination(t *testing.T) {
	// §2：未传分页参数时默认 page=1、page_size=20
	authz := &fakeHTTPAuthorizer{allowed: true}
	now := time.Now()
	engine, token := newAlertHTTPEngine(t, authz, newHTTPTestAlertRepo(&domain.Alert{
		ID: "a1", SourceID: "src1", DedupKey: "dk1", Name: "HighCPU",
		Severity: domain.SeverityP1, Status: domain.StatusNew,
		Labels: map[string]string{}, Annotations: map[string]string{},
		OccurrenceCount: 1, FirstSeenAt: now, LastSeenAt: now,
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/alerts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	var page struct {
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
	}
	if err := json.Unmarshal(resp.Data, &page); err != nil {
		t.Fatalf("unmarshal page data: %v", err)
	}
	if page.Page != 1 || page.PageSize != 20 {
		t.Fatalf("expected default page=1 page_size=20, got page=%d page_size=%d", page.Page, page.PageSize)
	}
}

func TestListAlerts_PageSizeCapped(t *testing.T) {
	// §2：page_size 超过 100 时 Normalize 截断为 100
	authz := &fakeHTTPAuthorizer{allowed: true}
	engine, token := newAlertHTTPEngine(t, authz, newHTTPTestAlertRepo())

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?page=1&page_size=200", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	var page struct {
		PageSize int `json:"page_size"`
	}
	if err := json.Unmarshal(resp.Data, &page); err != nil {
		t.Fatalf("unmarshal page data: %v", err)
	}
	if page.PageSize != 100 {
		t.Fatalf("expected page_size capped to 100, got %d", page.PageSize)
	}
}

func TestListAlerts_ServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/alerts", nil)

	h.ListAlerts(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	if resp.Code != "UNAVAILABLE" {
		t.Fatalf("expected UNAVAILABLE, got %q", resp.Code)
	}
	assertEnvelopeTraceIDKey(t, w.Body.Bytes()) // §2：直调 handler 无 Trace 中间件时 trace_id 仍输出 ""
}

// assertEnvelopeTraceIDKey 校验响应 JSON 含 trace_id 键（§2 固定字段，值可为空串）。
func assertEnvelopeTraceIDKey(t *testing.T, body []byte) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v body=%s", err, string(body))
	}
	if _, ok := raw["trace_id"]; !ok {
		t.Fatalf("trace_id field missing: %s", string(body))
	}
}

type apiEnvelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	TraceID string          `json:"trace_id"`
	Data    json.RawMessage `json:"data"`
}

// decodeAPIEnvelope 解析 §2 统一响应 envelope，供契约测试断言 code/message/trace_id/data。
func decodeAPIEnvelope(t *testing.T, body []byte) apiEnvelope {
	t.Helper()
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v body=%s", err, string(body))
	}
	return env
}
