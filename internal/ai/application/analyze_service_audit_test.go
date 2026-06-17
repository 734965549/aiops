package application

import (
	"context"
	"testing"
)

func TestAnalyzeService_WritesAuditOnSuccess(t *testing.T) {
	audit := &capturingAIAudit{}
	svc := NewAnalyzeService(&fakeAnalyzeAlertReader{
		alert: &AlertContext{Name: "HighCPU", Summary: "CPU > 85%", Severity: "p1"},
	}, nil, "", audit)
	out, err := svc.AnalyzeAlert(context.Background(), "user-1", AnalyzeAlertInput{AlertID: "a1"})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditAnalyzeAlert {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
	if audit.rows[0].UserID != "user-1" || audit.rows[0].Payload["alert_id"] != "a1" {
		t.Fatalf("unexpected audit payload: %+v", audit.rows[0])
	}
	if audit.rows[0].ResourceID != out.ConversationID {
		t.Fatalf("expected resource_id=conversation_id, got %s", audit.rows[0].ResourceID)
	}
}

type capturingAIAudit struct {
	rows []AuditRecord
}

func (c *capturingAIAudit) Record(_ context.Context, rec AuditRecord) error {
	c.rows = append(c.rows, rec)
	return nil
}
