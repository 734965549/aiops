package application

import (
	"context"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/execution/domain"
)

type capturingExecAudit struct {
	rows []AuditRecord
}

func (c *capturingExecAudit) Record(_ context.Context, rec AuditRecord) error {
	c.rows = append(c.rows, rec)
	return nil
}

func TestTaskService_RejectWritesAudit(t *testing.T) {
	audit := &capturingExecAudit{}
	now := time.Now().UTC()
	taskRepo := newFakeTaskRepo()
	_ = taskRepo.Create(context.Background(), &domain.Task{
		ID: "t1", Status: domain.StatusPendingConfirm,
		CreatedAt: now, UpdatedAt: now,
	})
	svc := NewTaskService(taskRepo, nil, &fakeTaskCreator{tasks: taskRepo}, nil, nil, audit, nil, nil, nil)
	out, err := svc.Confirm(context.Background(), "t1", Actor{UserID: "user-1"}, ConfirmTaskInput{Confirm: false})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if out.Status != string(domain.StatusCancelled) {
		t.Fatalf("expected cancelled, got %s", out.Status)
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditReject {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
	if audit.rows[0].Payload["result"] != "success" {
		t.Fatalf("expected success result: %+v", audit.rows[0].Payload)
	}
}
