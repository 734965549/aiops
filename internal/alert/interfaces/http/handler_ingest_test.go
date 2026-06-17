package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	alertapp "github.com/734965549/aiops/internal/alert/application"
	"github.com/734965549/aiops/internal/alert/domain"
	"github.com/734965549/aiops/internal/alert/infrastructure/ingest"
	"github.com/734965549/aiops/internal/alert/infrastructure/webhookidempotency"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/config"
	"github.com/gin-gonic/gin"
)

type ingestHTTPSourceRepo struct {
	byID map[string]*domain.AlertSource
}

func (r *ingestHTTPSourceRepo) Create(_ context.Context, source *domain.AlertSource) error {
	r.byID[source.ID] = source
	return nil
}
func (r *ingestHTTPSourceRepo) Update(_ context.Context, _ *domain.AlertSource) error { return nil }
func (r *ingestHTTPSourceRepo) GetByID(_ context.Context, sourceID string) (*domain.AlertSource, error) {
	s, ok := r.byID[sourceID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return s, nil
}
func (r *ingestHTTPSourceRepo) List(_ context.Context) ([]domain.AlertSource, error) { return nil, nil }
func (r *ingestHTTPSourceRepo) Delete(_ context.Context, _ string) error             { return nil }

type ingestHTTPAlertRepo struct {
	mu     sync.Mutex
	byID   map[string]*domain.Alert
	active map[string]*domain.Alert
}

func newIngestHTTPAlertRepo() *ingestHTTPAlertRepo {
	return &ingestHTTPAlertRepo{
		byID:   map[string]*domain.Alert{},
		active: map[string]*domain.Alert{},
	}
}

func (r *ingestHTTPAlertRepo) key(sourceID, dedupKey string) string {
	return sourceID + "\x00" + dedupKey
}

func (r *ingestHTTPAlertRepo) Create(_ context.Context, alert *domain.Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.byID {
		if a.DedupKey == alert.DedupKey && a.LifecycleSeq == alert.LifecycleSeq {
			return domain.ErrAlreadyExists
		}
	}
	if alert.Status != domain.StatusClosed {
		if _, ok := r.active[r.key(alert.SourceID, alert.DedupKey)]; ok {
			return domain.ErrAlreadyExists
		}
	}
	cp := *alert
	r.byID[alert.ID] = &cp
	if alert.Status != domain.StatusClosed {
		r.active[r.key(alert.SourceID, alert.DedupKey)] = &cp
	}
	return nil
}

func (r *ingestHTTPAlertRepo) Update(_ context.Context, alert *domain.Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *alert
	r.byID[alert.ID] = &cp
	if alert.Status == domain.StatusClosed {
		delete(r.active, r.key(alert.SourceID, alert.DedupKey))
	} else {
		r.active[r.key(alert.SourceID, alert.DedupKey)] = &cp
	}
	return nil
}

func (r *ingestHTTPAlertRepo) GetByID(_ context.Context, alertID string) (*domain.Alert, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[alertID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (r *ingestHTTPAlertRepo) FindActiveByDedupKey(_ context.Context, sourceID, dedupKey string) (*domain.Alert, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.active[r.key(sourceID, dedupKey)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (r *ingestHTTPAlertRepo) MaxLifecycleSeq(_ context.Context, dedupKey string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	max := 0
	for _, a := range r.byID {
		if a.DedupKey == dedupKey && a.LifecycleSeq > max {
			max = a.LifecycleSeq
		}
	}
	return max, nil
}

func (r *ingestHTTPAlertRepo) List(_ context.Context, _ domain.AlertFilter) ([]domain.Alert, error) {
	return nil, nil
}
func (r *ingestHTTPAlertRepo) Count(_ context.Context, _ domain.AlertFilter) (int64, error) {
	return 0, nil
}

type ingestHTTPEventRepo struct {
	mu   sync.Mutex
	rows []domain.AlertEvent
}

func (r *ingestHTTPEventRepo) Create(_ context.Context, event *domain.AlertEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *event
	r.rows = append(r.rows, cp)
	return nil
}

func (r *ingestHTTPEventRepo) ListByAlertID(_ context.Context, alertID string) ([]domain.AlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.AlertEvent, 0)
	for _, e := range r.rows {
		if e.AlertID == alertID {
			out = append(out, e)
		}
	}
	return out, nil
}

func newIngestHTTPEngine(t *testing.T, secret string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	src := &domain.AlertSource{
		ID:         "prod-am",
		Name:       "Prod AM",
		Type:       domain.SourcePrometheusAlertmanager,
		Enabled:    true,
		SecretHash: ingest.HashWebhookSecret(secret),
	}
	sources := &ingestHTTPSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
	alerts := newIngestHTTPAlertRepo()
	events := &ingestHTTPEventRepo{}
	svc := alertapp.NewIngestService(alerts, events, sources, nil, webhookidempotency.NewMemoryStore(), alertapp.NoopAuditRecorder{})
	registrar := NewRegistrar(nil, NewIngestHandler(svc), nil)

	return server.NewEngine(server.Options{
		Cfg: &config.Config{
			App:      config.AppConfig{Env: "dev", Timezone: "Asia/Shanghai"},
			Server:   config.ServerConfig{Port: 8080},
			Database: config.DatabaseConfig{Host: "127.0.0.1", Name: "aiops", SSLMode: "disable"},
			Auth:     config.AuthConfig{JWTSecret: config.DefaultJWTSecretPlaceholder},
		},
		Registrars: []server.RouteRegistrar{registrar},
		StartedAt:  time.Now(),
	})
}

func TestIngestAlertmanager_Unauthenticated(t *testing.T) {
	// §3.2：缺少 X-AIOPS-Webhook-Token → 401 UNAUTHENTICATED
	engine := newIngestHTTPEngine(t, "webhook-secret")
	body := `{"status":"firing","alerts":[{"status":"firing","fingerprint":"fp-1","labels":{"alertname":"HighCPU"},"startsAt":"2026-01-01T00:00:00Z"}]}`

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/ingest/alertmanager/prod-am", strings.NewReader(body))
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

func TestIngestAlertmanager_InvalidToken(t *testing.T) {
	// §3.2：token 不匹配 → 401 UNAUTHENTICATED
	engine := newIngestHTTPEngine(t, "webhook-secret")
	body := `{"status":"firing","alerts":[{"status":"firing","fingerprint":"fp-1","labels":{"alertname":"HighCPU"},"startsAt":"2026-01-01T00:00:00Z"}]}`

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/ingest/alertmanager/prod-am", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhookTokenHeader, "wrong-secret")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestIngestAlertmanager_InvalidPayload(t *testing.T) {
	engine := newIngestHTTPEngine(t, "webhook-secret")

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/ingest/alertmanager/prod-am", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhookTokenHeader, "webhook-secret")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	if resp.Code != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %q", resp.Code)
	}
}

func TestIngestAlertmanager_Success(t *testing.T) {
	engine := newIngestHTTPEngine(t, "webhook-secret")
	body := `{"status":"firing","alerts":[{"status":"firing","fingerprint":"fp-http","labels":{"alertname":"HighCPU"},"startsAt":"2026-01-01T00:00:00Z"}]}`

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/ingest/alertmanager/prod-am", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhookTokenHeader, "webhook-secret")
	req.Header.Set("X-Trace-Id", "550e8400-e29b-41d4-a716-446655440000")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	if resp.Code != "OK" {
		t.Fatalf("expected OK, got %q body=%s", resp.Code, w.Body.String())
	}
	if resp.TraceID == "" {
		t.Fatalf("expected trace_id in response, body=%s", w.Body.String())
	}
	var data struct {
		Created int `json:"created"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.Created != 1 {
		t.Fatalf("expected created=1, got %+v", data)
	}
}

func TestIngestAlertmanager_PayloadTooLarge(t *testing.T) {
	// §3.2：请求体超过 1MB → 413 PAYLOAD_TOO_LARGE
	engine := newIngestHTTPEngine(t, "webhook-secret")
	body := strings.Repeat("x", maxWebhookBodyBytes+1)

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/ingest/alertmanager/prod-am", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhookTokenHeader, "webhook-secret")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	resp := decodeAPIEnvelope(t, w.Body.Bytes())
	if resp.Code != "PAYLOAD_TOO_LARGE" {
		t.Fatalf("expected PAYLOAD_TOO_LARGE, got %q", resp.Code)
	}
}

func TestIngestAlertmanager_IdempotentRequestID(t *testing.T) {
	// §3.2 / §7.2：相同 X-Request-ID 重复提交返回首次结果，不重复创建
	engine := newIngestHTTPEngine(t, "webhook-secret")
	body := `{"status":"firing","alerts":[{"status":"firing","fingerprint":"fp-idem","labels":{"alertname":"HighCPU"},"startsAt":"2026-01-01T00:00:00Z"}]}`

	doReq := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/alerts/ingest/alertmanager/prod-am", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(webhookTokenHeader, "webhook-secret")
		req.Header.Set(requestIDHeader, "idem-req-1")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		resp := decodeAPIEnvelope(t, w.Body.Bytes())
		var data struct {
			Created int `json:"created"`
			Updated int `json:"updated"`
		}
		_ = json.Unmarshal(resp.Data, &data)
		return data.Created
	}

	if got := doReq(); got != 1 {
		t.Fatalf("first request created=%d", got)
	}
	if got := doReq(); got != 1 {
		t.Fatalf("idempotent replay should return cached created=1, got created=%d", got)
	}
}

func TestIngestAlertmanager_DisabledSource(t *testing.T) {
	// §3.2：接入源未启用视为不存在
	gin.SetMode(gin.TestMode)
	src := &domain.AlertSource{
		ID: "disabled-am", Name: "Disabled", Type: domain.SourcePrometheusAlertmanager,
		Enabled: false, SecretHash: ingest.HashWebhookSecret("webhook-secret"),
	}
	sources := &ingestHTTPSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
	svc := alertapp.NewIngestService(newIngestHTTPAlertRepo(), &ingestHTTPEventRepo{}, sources, nil, webhookidempotency.NewMemoryStore(), alertapp.NoopAuditRecorder{})
	engine := server.NewEngine(server.Options{
		Cfg:        &config.Config{App: config.AppConfig{Env: "dev"}, Server: config.ServerConfig{Port: 8080}, Database: config.DatabaseConfig{Host: "127.0.0.1", Name: "aiops", SSLMode: "disable"}, Auth: config.AuthConfig{JWTSecret: config.DefaultJWTSecretPlaceholder}},
		Registrars: []server.RouteRegistrar{NewRegistrar(nil, NewIngestHandler(svc), nil)},
		StartedAt:  time.Now(),
	})
	body := `{"status":"firing","alerts":[{"status":"firing","fingerprint":"fp-1","labels":{"alertname":"HighCPU"},"startsAt":"2026-01-01T00:00:00Z"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/ingest/alertmanager/disabled-am", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhookTokenHeader, "webhook-secret")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestIngestAlertmanager_ServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewIngestHandler(nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/alerts/ingest/alertmanager/prod-am", strings.NewReader(`{}`))
	c.Params = gin.Params{{Key: "source_id", Value: "prod-am"}}

	h.IngestAlertmanager(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
