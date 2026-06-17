package audit

import (
	"context"
	"errors"
	"testing"

	assetapp "github.com/734965549/aiops/internal/asset/application"
	auditapp "github.com/734965549/aiops/internal/audit/application"
	auditdomain "github.com/734965549/aiops/internal/audit/domain"
)

type memAuditRepo struct {
	rows []auditdomain.OperationAudit
	err  error
}

func (m *memAuditRepo) Create(_ context.Context, audit *auditdomain.OperationAudit) error {
	if m.err != nil {
		return m.err
	}
	m.rows = append(m.rows, *audit)
	return nil
}

func (m *memAuditRepo) List(context.Context, auditdomain.OperationAuditFilter) ([]auditdomain.OperationAudit, error) {
	return m.rows, nil
}

func (m *memAuditRepo) Count(context.Context, auditdomain.OperationAuditFilter) (int64, error) {
	return int64(len(m.rows)), nil
}

func TestRecorder_NilServiceNoOp(t *testing.T) {
	var r *Recorder
	if err := r.Record(context.Background(), assetapp.AuditRecord{}); err != nil {
		t.Fatalf("nil recorder should no-op: %v", err)
	}
}

func TestRecorder_WritesAuditRow(t *testing.T) {
	repo := &memAuditRepo{}
	r := NewRecorder(auditapp.NewOperationAuditService(repo))
	if err := r.Record(context.Background(), assetapp.AuditRecord{
		ResourceType: "application",
		ResourceID:   "app-1",
		Action:       assetapp.AuditCreateApplication,
		UserID:       "user-1",
		Payload:      map[string]any{"result": "success"},
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if len(repo.rows) != 1 || repo.rows[0].Action != "create" || repo.rows[0].ResourceType != "application" {
		t.Fatalf("unexpected rows: %+v", repo.rows)
	}
}

func TestRecorder_ReturnsError(t *testing.T) {
	repo := &memAuditRepo{err: errors.New("db down")}
	r := NewRecorder(auditapp.NewOperationAuditService(repo))
	if err := r.Record(context.Background(), assetapp.AuditRecord{
		ResourceType: "application", ResourceID: "app-1",
		Action: assetapp.AuditCreateApplication, UserID: "u1",
	}); err == nil {
		t.Fatal("expected error")
	}
}
