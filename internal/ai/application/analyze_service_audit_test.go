package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/ai/toolgateway"
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

func TestAnalyzeService_WritesEvidenceAudit(t *testing.T) {
	audit := &capturingAIAudit{}
	invoker := &fakeToolInvoker{resp: &toolgateway.ToolResponse{Allowed: true, Data: map[string]any{}}}
	collector := &fakeEvidenceCollector{result: &MetricEvidenceResult{
		Status:  EvidenceStatusOK,
		Metrics: []MetricEvidence{{Metric: "cpu_util", EvidenceID: "ev-1"}},
	}}
	svc := NewAnalyzeService(&fakeAnalyzeAlertReader{alert: evidenceTestAlert()}, invoker, "provider-1", audit)
	svc.SetMetricEvidence(collector)

	if _, err := svc.AnalyzeAlert(context.Background(), "user-1", AnalyzeAlertInput{AlertID: "a1", IncludeMetrics: true}); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(audit.rows) != 1 {
		t.Fatalf("unexpected audit rows: %+v", audit.rows)
	}
	if got := audit.rows[0].Payload["evidence_status"]; got != EvidenceStatusOK {
		t.Fatalf("evidence_status = %v", got)
	}
	ids, ok := audit.rows[0].Payload["evidence_ids"].([]string)
	if !ok || len(ids) != 1 || ids[0] != "ev-1" {
		t.Fatalf("evidence_ids = %#v", audit.rows[0].Payload["evidence_ids"])
	}
}

type capturingAIAudit struct {
	rows []AuditRecord
}

func (c *capturingAIAudit) Record(_ context.Context, rec AuditRecord) error {
	c.rows = append(c.rows, rec)
	return nil
}
