package application

import (
	"context"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	"github.com/734965549/aiops/internal/alert/infrastructure/ingest"
)

func TestIngestService_CreateWritesIngestAudit(t *testing.T) {
	audit := &capturingAlertAudit{}
	alerts := newIngestTestAlertRepo()
	events := &ingestTestEventRepo{}
	sources := &ingestTestSourceRepo{}
	src := &domain.AlertSource{
		ID: "src-1", Name: "AM", Type: domain.SourcePrometheusAlertmanager,
		Enabled: true, SecretHash: ingest.HashWebhookSecret("secret"),
	}
	_ = sources.Create(context.Background(), src)
	svc := NewIngestService(alerts, events, sources, nil, nil, audit)
	ctx := IngestContext{SourceID: src.ID, Token: "secret"}
	payload := AlertmanagerWebhook{
		Status: "firing",
		Alerts: []AlertmanagerAlert{{
			Labels: map[string]string{"alertname": "HighCPU", "service": "payment"},
		}},
	}
	if _, err := svc.IngestAlertmanager(context.Background(), ctx, payload); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditIngest {
		t.Fatalf("expected ingest audit, got %+v", audit.rows)
	}
	if audit.rows[0].Payload["result"] != "created" {
		t.Fatalf("expected created result, got %+v", audit.rows[0].Payload)
	}
}

type ingestTestSourceRepo struct {
	byID map[string]*domain.AlertSource
}

func (r *ingestTestSourceRepo) Create(_ context.Context, src *domain.AlertSource) error {
	if r.byID == nil {
		r.byID = map[string]*domain.AlertSource{}
	}
	cp := *src
	if cp.CreatedAt.IsZero() {
		now := time.Now().UTC()
		cp.CreatedAt = now
		cp.UpdatedAt = now
	}
	r.byID[src.ID] = &cp
	return nil
}

func (r *ingestTestSourceRepo) GetByID(_ context.Context, id string) (*domain.AlertSource, error) {
	src, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *src
	return &cp, nil
}

func (r *ingestTestSourceRepo) List(context.Context) ([]domain.AlertSource, error) { return nil, nil }
func (r *ingestTestSourceRepo) Update(context.Context, *domain.AlertSource) error  { return nil }
func (r *ingestTestSourceRepo) Delete(context.Context, string) error               { return nil }

type ingestTestEventRepo struct {
	rows []domain.AlertEvent
}

func (r *ingestTestEventRepo) Create(_ context.Context, ev *domain.AlertEvent) error {
	r.rows = append(r.rows, *ev)
	return nil
}

func (r *ingestTestEventRepo) ListByAlertID(context.Context, string) ([]domain.AlertEvent, error) {
	return r.rows, nil
}

func newIngestTestAlertRepo() *ingestTestAlertRepo {
	return &ingestTestAlertRepo{byDedup: map[string]*domain.Alert{}}
}

type ingestTestAlertRepo struct {
	byDedup map[string]*domain.Alert
	rows    []domain.Alert
}

func (r *ingestTestAlertRepo) Create(_ context.Context, alert *domain.Alert) error {
	cp := *alert
	r.rows = append(r.rows, cp)
	r.byDedup[alert.DedupKey] = &cp
	return nil
}

func (r *ingestTestAlertRepo) FindActiveByDedupKey(_ context.Context, _, dedupKey string) (*domain.Alert, error) {
	a, ok := r.byDedup[dedupKey]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (r *ingestTestAlertRepo) MaxLifecycleSeq(context.Context, string) (int, error) { return 0, nil }
func (r *ingestTestAlertRepo) Update(context.Context, *domain.Alert) error          { return nil }
func (r *ingestTestAlertRepo) GetByID(context.Context, string) (*domain.Alert, error) {
	return nil, domain.ErrNotFound
}
func (r *ingestTestAlertRepo) List(context.Context, domain.AlertFilter) ([]domain.Alert, error) {
	return nil, nil
}
func (r *ingestTestAlertRepo) Count(context.Context, domain.AlertFilter) (int64, error) {
	return 0, nil
}
func (r *ingestTestAlertRepo) CountByStatus(context.Context) (map[domain.AlertStatus]int64, error) {
	return nil, nil
}
