package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	"github.com/734965549/aiops/internal/alert/infrastructure/ingest"
	"github.com/734965549/aiops/internal/alert/infrastructure/webhookidempotency"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
)

type fakeSourceRepo struct {
	byID map[string]*domain.AlertSource
}

func (r *fakeSourceRepo) Create(_ context.Context, source *domain.AlertSource) error {
	r.byID[source.ID] = source
	return nil
}

func (r *fakeSourceRepo) Update(_ context.Context, source *domain.AlertSource) error { return nil }
func (r *fakeSourceRepo) GetByID(_ context.Context, sourceID string) (*domain.AlertSource, error) {
	s, ok := r.byID[sourceID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return s, nil
}
func (r *fakeSourceRepo) List(_ context.Context) ([]domain.AlertSource, error) { return nil, nil }
func (r *fakeSourceRepo) Delete(_ context.Context, _ string) error             { return nil }

func newTestIngestService(alerts domain.AlertRepository, events domain.AlertEventRepository, sources domain.AlertSourceRepository) *IngestService {
	return NewIngestService(alerts, events, sources, nil, webhookidempotency.NewMemoryStore(), NoopAuditRecorder{})
}

func TestIngestService_FiringDedupAndRecover(t *testing.T) {
	secret := "test-webhook-secret-value"
	src := &domain.AlertSource{
		ID:          "prod-am",
		Name:        "Prod AM",
		Type:        domain.SourcePrometheusAlertmanager,
		Enabled:     true,
		SecretHash:  ingest.HashWebhookSecret(secret),
		Environment: "prod",
	}
	sources := &fakeSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
	alerts := newFakeAlertRepo()
	events := &fakeEventRepo{}
	svc := newTestIngestService(alerts, events, sources)
	ctx := IngestContext{SourceID: src.ID, Token: secret}

	payload := AlertmanagerWebhook{
		Status: "firing",
		Alerts: []AlertmanagerAlert{{
			Status:      "firing",
			Fingerprint: "fp-1",
			Labels:      map[string]string{"alertname": "HighCPU", "severity": "critical"},
			Annotations: map[string]string{"summary": "cpu high"},
			StartsAt:    time.Now().Format(time.RFC3339),
		}},
	}
	res, err := svc.IngestAlertmanager(context.Background(), ctx, payload)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("expected created=1, got %+v", res)
	}

	res, err = svc.IngestAlertmanager(context.Background(), ctx, payload)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if res.Updated != 1 {
		t.Fatalf("expected updated=1, got %+v", res)
	}

	payload.Alerts[0].Status = "resolved"
	payload.Status = "resolved"
	res, err = svc.IngestAlertmanager(context.Background(), ctx, payload)
	if err != nil {
		t.Fatalf("resolved ingest: %v", err)
	}
	if res.Recovered != 1 {
		t.Fatalf("expected recovered=1, got %+v", res)
	}
}

func TestIngestService_ExternalRecoverFromSilenced(t *testing.T) {
	secret := "test-webhook-secret-value"
	src := &domain.AlertSource{
		ID:          "prod-am",
		Name:        "Prod AM",
		Type:        domain.SourcePrometheusAlertmanager,
		Enabled:     true,
		SecretHash:  ingest.HashWebhookSecret(secret),
		Environment: "prod",
	}
	sources := &fakeSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
	alerts := newFakeAlertRepo()
	events := &fakeEventRepo{}
	svc := newTestIngestService(alerts, events, sources)
	ctx := IngestContext{SourceID: src.ID, Token: secret}
	bg := context.Background()

	labels := map[string]string{"alertname": "HighMem"}
	dedupKey := ingest.ComputeDedupKey(src.ID, "fp-silenced", "", "", labels)
	until := time.Now().Add(time.Hour)
	alert := &domain.Alert{
		ID: "a-silenced", SourceID: src.ID, Fingerprint: "fp-silenced", DedupKey: dedupKey, Name: "HighMem",
		Severity: domain.SeverityP2, Status: domain.StatusSilenced,
		Labels: labels, Annotations: map[string]string{},
		OccurrenceCount: 1, FirstSeenAt: time.Now(), LastSeenAt: time.Now(),
		SilencedUntil: &until,
	}
	if err := alerts.Create(bg, alert); err != nil {
		t.Fatalf("create silenced alert: %v", err)
	}

	resolved := AlertmanagerWebhook{
		Status: "resolved",
		Alerts: []AlertmanagerAlert{{
			Status:      "resolved",
			Fingerprint: "fp-silenced",
			Labels:      labels,
			StartsAt:    time.Now().Format(time.RFC3339),
		}},
	}
	res, err := svc.IngestAlertmanager(bg, ctx, resolved)
	if err != nil {
		t.Fatalf("resolved ingest: %v", err)
	}
	if res.Recovered != 1 {
		t.Fatalf("expected recovered=1, got %+v", res)
	}
	got, err := alerts.GetByID(bg, alert.ID)
	if err != nil {
		t.Fatalf("get alert: %v", err)
	}
	if got.Status != domain.StatusRecovered {
		t.Fatalf("expected recovered status, got %s", got.Status)
	}
	if got.SilencedUntil != nil {
		t.Fatal("expected silenced_until cleared after external recover")
	}
	if len(events.rows) == 0 || events.rows[len(events.rows)-1].EventType != domain.EventRecovered {
		t.Fatalf("expected recovered event, got %+v", events.rows)
	}
}

func TestIngestService_RefireAfterRecoveredReopensLifecycle(t *testing.T) {
	secret := "test-webhook-secret-value"
	src := &domain.AlertSource{
		ID:          "prod-am",
		Name:        "Prod AM",
		Type:        domain.SourcePrometheusAlertmanager,
		Enabled:     true,
		SecretHash:  ingest.HashWebhookSecret(secret),
		Environment: "prod",
	}
	sources := &fakeSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
	alerts := newFakeAlertRepo()
	events := &fakeEventRepo{}
	svc := newTestIngestService(alerts, events, sources)
	ctx := IngestContext{SourceID: src.ID, Token: secret}
	bg := context.Background()

	firingPayload := AlertmanagerWebhook{
		Status: "firing",
		Alerts: []AlertmanagerAlert{{
			Status:      "firing",
			Fingerprint: "fp-refire",
			Labels:      map[string]string{"alertname": "DiskFull", "severity": "warning"},
			Annotations: map[string]string{"summary": "disk usage high"},
			StartsAt:    time.Now().Format(time.RFC3339),
		}},
	}
	if _, err := svc.IngestAlertmanager(bg, ctx, firingPayload); err != nil {
		t.Fatalf("first firing: %v", err)
	}

	resolvedPayload := AlertmanagerWebhook{
		Status: "resolved",
		Alerts: []AlertmanagerAlert{{
			Status:      "resolved",
			Fingerprint: "fp-refire",
			Labels:      map[string]string{"alertname": "DiskFull", "severity": "warning"},
			Annotations: map[string]string{"summary": "disk usage high"},
			StartsAt:    firingPayload.Alerts[0].StartsAt,
		}},
	}
	res, err := svc.IngestAlertmanager(bg, ctx, resolvedPayload)
	if err != nil {
		t.Fatalf("resolved: %v", err)
	}
	if res.Recovered != 1 {
		t.Fatalf("expected recovered=1, got %+v", res)
	}

	res, err = svc.IngestAlertmanager(bg, ctx, firingPayload)
	if err != nil {
		t.Fatalf("refire after recovered: %v", err)
	}
	if res.Updated != 1 {
		t.Fatalf("expected updated=1, got %+v", res)
	}
	if res.Created != 0 {
		t.Fatalf("expected no new lifecycle, got created=%d", res.Created)
	}

	dedupKey := ingest.ComputeDedupKey(src.ID, "fp-refire", "", "", firingPayload.Alerts[0].Labels)
	active, err := alerts.FindActiveByDedupKey(bg, src.ID, dedupKey)
	if err != nil {
		t.Fatalf("find active alert: %v", err)
	}
	if active.Status != domain.StatusNew {
		t.Fatalf("expected status new after refire, got %s", active.Status)
	}
	if active.RecoveredAt != nil {
		t.Fatalf("expected recovered_at cleared, got %v", active.RecoveredAt)
	}
	if active.OccurrenceCount != 2 {
		t.Fatalf("expected occurrence_count=2, got %d", active.OccurrenceCount)
	}

	evs, err := events.ListByAlertID(bg, active.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("expected 3 events (triggered/recovered/triggered), got %d", len(evs))
	}
	if evs[2].EventType != domain.EventTriggered {
		t.Fatalf("expected last event triggered, got %s", evs[2].EventType)
	}
	if evs[2].Message != "告警恢复后再次触发" {
		t.Fatalf("unexpected last event message: %q", evs[2].Message)
	}
}

// raceSimulatingAlertRepo 模拟并发 ingest：FindActive 先查不到，Create 时唯一索引已冲突。
type raceSimulatingAlertRepo struct {
	*fakeAlertRepo
	skipFindOnce bool
	winner       *domain.Alert
}

func (r *raceSimulatingAlertRepo) FindActiveByDedupKey(ctx context.Context, sourceID, dedupKey string) (*domain.Alert, error) {
	if r.skipFindOnce {
		r.skipFindOnce = false
		return nil, domain.ErrNotFound
	}
	return r.fakeAlertRepo.FindActiveByDedupKey(ctx, sourceID, dedupKey)
}

func (r *raceSimulatingAlertRepo) Create(ctx context.Context, alert *domain.Alert) error {
	if r.winner != nil {
		if err := r.fakeAlertRepo.Create(ctx, r.winner); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
			return err
		}
		return domain.ErrAlreadyExists
	}
	return r.fakeAlertRepo.Create(ctx, alert)
}

func TestIngestService_CreateConflictRetriesAsUpdate(t *testing.T) {
	secret := "test-webhook-secret-value"
	src := &domain.AlertSource{
		ID:          "prod-am",
		Name:        "Prod AM",
		Type:        domain.SourcePrometheusAlertmanager,
		Enabled:     true,
		SecretHash:  ingest.HashWebhookSecret(secret),
		Environment: "prod",
	}
	now := time.Now()
	dedupKey := ingest.ComputeDedupKey(src.ID, "fp-race", "", "", map[string]string{"alertname": "HighCPU"})
	winner := &domain.Alert{
		ID:              "winner-alert",
		SourceID:        src.ID,
		DedupKey:        dedupKey,
		LifecycleSeq:    1,
		Name:            "HighCPU",
		Severity:        domain.SeverityP1,
		Status:          domain.StatusNew,
		Labels:          map[string]string{"alertname": "HighCPU"},
		Annotations:     map[string]string{},
		OccurrenceCount: 1,
		FirstSeenAt:     now,
		LastSeenAt:      now,
	}
	alerts := &raceSimulatingAlertRepo{
		fakeAlertRepo: newFakeAlertRepo(),
		skipFindOnce:  true,
		winner:        winner,
	}
	sources := &fakeSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
	svc := newTestIngestService(alerts, &fakeEventRepo{}, sources)
	ctx := IngestContext{SourceID: src.ID, Token: secret}

	payload := AlertmanagerWebhook{
		Status: "firing",
		Alerts: []AlertmanagerAlert{{
			Status:      "firing",
			Fingerprint: "fp-race",
			Labels:      map[string]string{"alertname": "HighCPU"},
			StartsAt:    now.Format(time.RFC3339),
		}},
	}
	res, err := svc.IngestAlertmanager(context.Background(), ctx, payload)
	if err != nil {
		t.Fatalf("ingest after create conflict: %v", err)
	}
	if res.Updated != 1 {
		t.Fatalf("expected updated=1, got %+v", res)
	}
	if res.Created != 0 {
		t.Fatalf("expected created=0, got %+v", res)
	}

	active, err := alerts.FindActiveByDedupKey(context.Background(), src.ID, dedupKey)
	if err != nil {
		t.Fatalf("find active: %v", err)
	}
	if active.OccurrenceCount != 2 {
		t.Fatalf("expected occurrence_count=2, got %d", active.OccurrenceCount)
	}
}

func TestIngestService_IntegrationEventIncludesTraceID(t *testing.T) {
	const traceID = "550e8400-e29b-41d4-a716-446655440000"
	secret := "test-webhook-secret-value"
	src := &domain.AlertSource{
		ID:          "prod-am",
		Name:        "Prod AM",
		Type:        domain.SourcePrometheusAlertmanager,
		Enabled:     true,
		SecretHash:  ingest.HashWebhookSecret(secret),
		Environment: "prod",
	}
	sources := &fakeSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
	alerts := newFakeAlertRepo()
	events := &fakeEventRepo{}
	svc := newTestIngestService(alerts, events, sources)
	ingestCtx := IngestContext{SourceID: src.ID, Token: secret}
	bg := httpx.ContextWithTraceID(context.Background(), traceID)

	payload := AlertmanagerWebhook{
		Status: "firing",
		Alerts: []AlertmanagerAlert{{
			Status:      "firing",
			Fingerprint: "fp-trace",
			Labels:      map[string]string{"alertname": "HighCPU"},
			StartsAt:    time.Now().Format(time.RFC3339),
		}},
	}
	if _, err := svc.IngestAlertmanager(bg, ingestCtx, payload); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	dedupKey := ingest.ComputeDedupKey(src.ID, "fp-trace", "", "", payload.Alerts[0].Labels)
	active, err := alerts.FindActiveByDedupKey(bg, src.ID, dedupKey)
	if err != nil {
		t.Fatalf("find active: %v", err)
	}
	evs, err := events.ListByAlertID(bg, active.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	got, _ := evs[0].Payload["trace_id"].(string)
	if got != traceID {
		t.Fatalf("expected trace_id=%q, got %q", traceID, got)
	}
}

func TestIngestService_IntegrationEventIncludesRequestMetadata(t *testing.T) {
	secret := "test-webhook-secret-value"
	src := &domain.AlertSource{
		ID:         "prod-am",
		Name:       "Prod AM",
		Type:       domain.SourcePrometheusAlertmanager,
		Enabled:    true,
		SecretHash: ingest.HashWebhookSecret(secret),
	}
	sources := &fakeSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
	alerts := newFakeAlertRepo()
	events := &fakeEventRepo{}
	svc := newTestIngestService(alerts, events, sources)
	ingestCtx := IngestContext{
		SourceID:  src.ID,
		Token:     secret,
		RequestID: "req-001",
		IP:        "10.0.0.1",
		UserAgent: "Alertmanager/0.27",
	}
	payload := AlertmanagerWebhook{
		Status: "firing",
		Alerts: []AlertmanagerAlert{{
			Status:      "firing",
			Fingerprint: "fp-meta",
			Labels:      map[string]string{"alertname": "HighCPU"},
			StartsAt:    time.Now().Format(time.RFC3339),
		}},
	}
	if _, err := svc.IngestAlertmanager(context.Background(), ingestCtx, payload); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	dedupKey := ingest.ComputeDedupKey(src.ID, "fp-meta", "", "", payload.Alerts[0].Labels)
	active, err := alerts.FindActiveByDedupKey(context.Background(), src.ID, dedupKey)
	if err != nil {
		t.Fatalf("find active: %v", err)
	}
	evs, err := events.ListByAlertID(context.Background(), active.ID)
	if err != nil || len(evs) != 1 {
		t.Fatalf("events: %v len=%d", err, len(evs))
	}
	p := evs[0].Payload
	if p["request_id"] != "req-001" || p["ip"] != "10.0.0.1" || p["user_agent"] != "Alertmanager/0.27" {
		t.Fatalf("unexpected payload: %+v", p)
	}
}

func TestIngestService_RequestIdempotency(t *testing.T) {
	// §3.2 / §7.2：相同 source_id + X-Request-ID 短期返回缓存结果，不重复写库
	secret := "test-webhook-secret-value"
	src := &domain.AlertSource{
		ID:         "prod-am",
		Name:       "Prod AM",
		Type:       domain.SourcePrometheusAlertmanager,
		Enabled:    true,
		SecretHash: ingest.HashWebhookSecret(secret),
	}
	sources := &fakeSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
	alerts := newFakeAlertRepo()
	events := &fakeEventRepo{}
	svc := newTestIngestService(alerts, events, sources)
	ingestCtx := IngestContext{SourceID: src.ID, Token: secret, RequestID: "dup-req-1"}
	payload := AlertmanagerWebhook{
		Status: "firing",
		Alerts: []AlertmanagerAlert{{
			Status:      "firing",
			Fingerprint: "fp-dup",
			Labels:      map[string]string{"alertname": "HighCPU"},
			StartsAt:    time.Now().Format(time.RFC3339),
		}},
	}
	res1, err := svc.IngestAlertmanager(context.Background(), ingestCtx, payload)
	if err != nil || res1.Created != 1 {
		t.Fatalf("first ingest: err=%v res=%+v", err, res1)
	}
	res2, err := svc.IngestAlertmanager(context.Background(), ingestCtx, payload)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if res2.Created != 1 || res2.Updated != 0 {
		t.Fatalf("expected cached first result, got %+v", res2)
	}
	if len(alerts.byID) != 1 {
		t.Fatalf("expected 1 alert in repo, got %d", len(alerts.byID))
	}
}

func TestIngestService_RequestIdempotencyConcurrent(t *testing.T) {
	// §3.2：相同 X-Request-ID 并发重放应合并为一次 ingest（singleflight），只创建一条告警
	const workers = 16
	secret := "test-webhook-secret-value"
	src := &domain.AlertSource{
		ID:         "prod-am",
		Name:       "Prod AM",
		Type:       domain.SourcePrometheusAlertmanager,
		Enabled:    true,
		SecretHash: ingest.HashWebhookSecret(secret),
	}
	sources := &fakeSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
	alerts := newFakeAlertRepo()
	events := &fakeEventRepo{}
	svc := newTestIngestService(alerts, events, sources)
	ingestCtx := IngestContext{SourceID: src.ID, Token: secret, RequestID: "concurrent-req-1"}
	payload := AlertmanagerWebhook{
		Status: "firing",
		Alerts: []AlertmanagerAlert{{
			Status:      "firing",
			Fingerprint: "fp-concurrent",
			Labels:      map[string]string{"alertname": "HighCPU"},
			StartsAt:    time.Now().Format(time.RFC3339),
		}},
	}

	var wg sync.WaitGroup
	results := make([]*IngestResultDTO, workers)
	errs := make([]error, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.IngestAlertmanager(context.Background(), ingestCtx, payload)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
		if results[i].Created != 1 || results[i].Updated != 0 {
			t.Fatalf("worker %d: expected created=1 updated=0, got %+v", i, results[i])
		}
	}
	if len(alerts.byID) != 1 {
		t.Fatalf("expected exactly 1 alert after concurrent ingest, got %d", len(alerts.byID))
	}
}

func TestVerifySource_InvalidAndDisabled(t *testing.T) {
	secret := "test-webhook-secret-value"
	enabled := &domain.AlertSource{
		ID: "prod-am", Enabled: true, SecretHash: ingest.HashWebhookSecret(secret),
	}
	disabled := &domain.AlertSource{
		ID: "disabled", Enabled: false, SecretHash: ingest.HashWebhookSecret(secret),
	}
	sources := &fakeSourceRepo{byID: map[string]*domain.AlertSource{
		enabled.ID:  enabled,
		disabled.ID: disabled,
	}}
	svc := NewIngestService(nil, nil, sources, nil, nil, NoopAuditRecorder{})

	_, err := svc.VerifySource(context.Background(), IngestContext{SourceID: disabled.ID, Token: secret})
	if err == nil {
		t.Fatal("expected error for disabled source")
	}
	if appErr := apperr.FromError(err); appErr.Code != apperr.CodeNotFound {
		t.Fatalf("expected NOT_FOUND for disabled source, got %v", err)
	}

	_, err = svc.VerifySource(context.Background(), IngestContext{SourceID: enabled.ID, Token: "wrong"})
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if appErr := apperr.FromError(err); appErr.Code != apperr.CodeUnauthenticated {
		t.Fatalf("expected UNAUTHENTICATED, got %v", err)
	}
}

func TestIngestService_RefireAfterClosedCreatesNewLifecycle(t *testing.T) {
	secret := "test-webhook-secret-value"
	src := &domain.AlertSource{
		ID:          "prod-am",
		Name:        "Prod AM",
		Type:        domain.SourcePrometheusAlertmanager,
		Enabled:     true,
		SecretHash:  ingest.HashWebhookSecret(secret),
		Environment: "prod",
	}
	sources := &fakeSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
	alerts := newFakeAlertRepo()
	events := &fakeEventRepo{}
	ingestSvc := newTestIngestService(alerts, events, sources)
	alertSvc := NewAlertService(alerts, events, nil, NoopAuditRecorder{})
	ingestCtx := IngestContext{SourceID: src.ID, Token: secret}
	bg := context.Background()

	payload := AlertmanagerWebhook{
		Status: "firing",
		Alerts: []AlertmanagerAlert{{
			Status:      "firing",
			Fingerprint: "fp-closed-refire",
			Labels:      map[string]string{"alertname": "DiskFull"},
			StartsAt:    time.Now().Format(time.RFC3339),
		}},
	}
	res, err := ingestSvc.IngestAlertmanager(bg, ingestCtx, payload)
	if err != nil {
		t.Fatalf("first firing: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("expected created=1, got %+v", res)
	}

	dedupKey := ingest.ComputeDedupKey(src.ID, "fp-closed-refire", "", "", payload.Alerts[0].Labels)
	first, err := alerts.FindActiveByDedupKey(bg, src.ID, dedupKey)
	if err != nil {
		t.Fatalf("find first active: %v", err)
	}
	if first.LifecycleSeq != 1 {
		t.Fatalf("expected lifecycle_seq=1, got %d", first.LifecycleSeq)
	}

	if _, err := alertSvc.Close(bg, first.ID, Actor{UserID: "u1", DisplayName: "alice"}, "resolved"); err != nil {
		t.Fatalf("close alert: %v", err)
	}
	if _, err := alerts.FindActiveByDedupKey(bg, src.ID, dedupKey); err == nil {
		t.Fatal("expected no active alert after close")
	}

	res, err = ingestSvc.IngestAlertmanager(bg, ingestCtx, payload)
	if err != nil {
		t.Fatalf("refire after closed: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("expected created=1 after closed refire, got %+v", res)
	}
	if res.Updated != 0 {
		t.Fatalf("expected updated=0, got %+v", res)
	}

	second, err := alerts.FindActiveByDedupKey(bg, src.ID, dedupKey)
	if err != nil {
		t.Fatalf("find second active: %v", err)
	}
	if second.LifecycleSeq != 2 {
		t.Fatalf("expected lifecycle_seq=2, got %d", second.LifecycleSeq)
	}
	if second.ID == first.ID {
		t.Fatal("expected new alert record after closed refire")
	}

	closedFirst, err := alerts.GetByID(bg, first.ID)
	if err != nil {
		t.Fatalf("load closed alert: %v", err)
	}
	if closedFirst.Status != domain.StatusClosed || closedFirst.LifecycleSeq != 1 {
		t.Fatalf("expected first lifecycle remains closed seq=1, got status=%s seq=%d", closedFirst.Status, closedFirst.LifecycleSeq)
	}

	evs, err := events.ListByAlertID(bg, second.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("expected 1 triggered event on new lifecycle, got %d", len(evs))
	}
	if evs[0].Message != "告警再次触发（新生命周期）" {
		t.Fatalf("unexpected event message: %q", evs[0].Message)
	}
}
