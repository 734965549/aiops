package application

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/alert/domain"
)

type capturingAlertAudit struct {
	rows []AuditRecord
	err  error
}

func (c *capturingAlertAudit) Record(_ context.Context, rec AuditRecord) error {
	if c.err != nil {
		return c.err
	}
	c.rows = append(c.rows, rec)
	return nil
}

func TestSourceService_CreateWritesAudit(t *testing.T) {
	audit := &capturingAlertAudit{}
	svc := NewSourceService(&fakeSourceRepo{byID: map[string]*domain.AlertSource{}}, audit)
	out, err := svc.Create(context.Background(), Actor{UserID: "admin-1"}, CreateSourceInput{
		ID: "prod-am", Name: "Prod AM", Secret: "webhook-secret",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditSourceCreate {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
	if audit.rows[0].ResourceID != out.ID || audit.rows[0].Payload["result"] != "success" {
		t.Fatalf("unexpected audit payload: %+v", audit.rows[0])
	}
}

func TestSourceService_CreateAuditFailureDoesNotBlock(t *testing.T) {
	audit := &capturingAlertAudit{err: errors.New("audit down")}
	svc := NewSourceService(&fakeSourceRepo{byID: map[string]*domain.AlertSource{}}, audit)
	out, err := svc.Create(context.Background(), Actor{UserID: "admin-1"}, CreateSourceInput{
		ID: "prod-am", Name: "Prod AM", Secret: "webhook-secret",
	})
	if err != nil {
		t.Fatalf("create should succeed: %v", err)
	}
	if out == nil || out.Name != "Prod AM" {
		t.Fatalf("unexpected result: %+v", out)
	}
}
