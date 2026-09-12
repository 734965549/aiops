package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/audit/domain"
	httpx "github.com/734965549/aiops/pkg/transport/http"
)

type mockAuditRepo struct {
	records []domain.OperationAudit
}

func (m *mockAuditRepo) Create(_ context.Context, audit *domain.OperationAudit) error {
	m.records = append(m.records, *audit)
	return nil
}

func (m *mockAuditRepo) List(_ context.Context, filter domain.OperationAuditFilter) ([]domain.OperationAudit, error) {
	var out []domain.OperationAudit
	for _, r := range m.records {
		if filter.ResourceType != "" && r.ResourceType != filter.ResourceType {
			continue
		}
		if filter.ResourceID != "" && r.ResourceID != filter.ResourceID {
			continue
		}
		if filter.UserID != "" && r.UserID != filter.UserID {
			continue
		}
		if filter.Action != "" && r.Action != filter.Action {
			continue
		}
		if filter.TraceID != "" {
			tid, _ := r.Payload["trace_id"].(string)
			if tid != filter.TraceID {
				continue
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func (m *mockAuditRepo) Count(ctx context.Context, filter domain.OperationAuditFilter) (int64, error) {
	rows, err := m.List(ctx, filter)
	return int64(len(rows)), err
}

func TestOperationAuditService_RecordAndList(t *testing.T) {
	repo := &mockAuditRepo{}
	svc := NewOperationAuditService(repo)

	ctx := httpx.ContextWithTraceID(context.Background(), "test-trace-123")
	err := svc.Record(ctx, RecordInput{
		UserID:       "usr_001",
		ResourceType: "alert",
		ResourceID:   "alt_001",
		Action:       "silence",
		Payload: map[string]any{
			"duration": "2h",
		},
		IP:        "127.0.0.1",
		UserAgent: "test-agent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(repo.records))
	}

	// Check trace_id was automatically injected
	if repo.records[0].Payload["trace_id"] != "test-trace-123" {
		t.Fatalf("expected trace_id in payload, got %v", repo.records[0].Payload["trace_id"])
	}

	// Test List by trace_id
	list, total, err := svc.List(context.Background(), ListQuery{
		TraceID: "test-trace-123",
	})
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("expected 1 item, got total=%d len=%d", total, len(list))
	}
	if list[0].UserID != "usr_001" {
		t.Fatalf("expected user usr_001, got %s", list[0].UserID)
	}

	// Test List with non-matching trace_id
	list2, total2, err := svc.List(context.Background(), ListQuery{
		TraceID: "non-existent-trace",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total2 != 0 || len(list2) != 0 {
		t.Fatalf("expected 0 items, got total=%d len=%d", total2, len(list2))
	}
}
