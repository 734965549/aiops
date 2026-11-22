package application

import (
	"context"
	"strings"
	"testing"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/internal/ai/toolgateway"
)

type fakePostgresEvidencePort struct {
	query  PostgresEvidenceQuery
	result *PostgresEvidenceResult
	err    error
	calls  int
}

func (f *fakePostgresEvidencePort) Collect(_ context.Context, _ string, query PostgresEvidenceQuery) (*PostgresEvidenceResult, error) {
	f.query = query
	f.calls++
	return f.result, f.err
}

func longTransactionTestAlert() *AlertContext {
	alert := evidenceTestAlert()
	alert.Labels = map[string]string{
		"account_id": "acc-1", "region": "cn-north-4", "project_id": "p-1",
		"ces_namespace": "SYS.RDS", "metric_name": "oldest_transaction_duration",
		"cloud_resource_id": "30715ce0000000000000000000000000in03",
		"rds_cluster_id":    "30715ce0000000000000000000000000in03",
	}
	return alert
}

func TestSendMessageBuildsEvidenceBackedPostgresDiagnosis(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	svc.alerts = &fakeAnalyzeAlertReader{alert: longTransactionTestAlert()}
	port := &fakePostgresEvidencePort{result: &PostgresEvidenceResult{
		Status: EvidenceStatusOK,
		Sessions: []PostgresSession{{
			SessionID: 42, SessionStatus: "idle in transaction", TransactionExecutionTime: 900,
			QueryID: "query-42", AppName: "payment-worker", WaitEventType: "Client", WaitEventName: "ClientRead",
			SQLOperation: "UPDATE", SQLSummary: "UPDATE orders SET status = ? WHERE id = ?",
		}},
		TopSQL: []PostgresTopSQL{{QueryID: "query-42", SQLOperation: "UPDATE", SQLSummary: "UPDATE orders SET status = ? WHERE id = ?"}},
	}}
	store := &fakePostgresEvidenceStore{}
	svc.SetPostgresEvidence(port, store, 0)
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析这条长事务告警的可能原因",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if port.calls != 1 || port.query.InstanceID != "30715ce0000000000000000000000000in03" {
		t.Fatalf("unexpected postgres query: calls=%d query=%+v", port.calls, port.query)
	}
	if store.saved == nil || result.Answer.Diagnosis == nil {
		t.Fatalf("missing persisted evidence or diagnosis: %+v", result.Answer)
	}
	if result.Answer.Diagnosis.RootCause.Code != "idle_in_transaction" || result.Answer.Diagnosis.RootCause.Status != "confirmed" {
		t.Fatalf("unexpected root cause: %+v", result.Answer.Diagnosis.RootCause)
	}
	if result.Answer.InsufficientContext {
		t.Fatal("evidence-backed confirmed category must not be marked insufficient")
	}
	if !strings.Contains(result.Answer.Summary, "空闲长事务") || strings.Contains(result.Answer.Summary, "备份") {
		t.Fatalf("summary is not evidence bounded: %q", result.Answer.Summary)
	}
	foundFact := false
	foundTopSQL := false
	for _, fact := range result.Answer.Diagnosis.Facts {
		if strings.Contains(fact.Statement, "query-42") && fact.EvidenceID != "" {
			foundFact = true
		}
		if strings.Contains(fact.Statement, "关联 Top SQL") && strings.Contains(fact.Statement, "UPDATE orders") {
			foundTopSQL = true
		}
	}
	if !foundFact || !foundTopSQL {
		t.Fatalf("session or Top SQL fact with evidence id missing: %+v", result.Answer.Diagnosis.Facts)
	}
	if !hasToolCall(result.ToolCalls, chatToolPostgresActivity, EvidenceStatusOK) {
		t.Fatalf("postgres tool observation missing: %+v", result.ToolCalls)
	}
}

func TestAnalyzeAlertIncludesPostgresDiagnosis(t *testing.T) {
	invoker := &fakeToolInvoker{resp: &toolgateway.ToolResponse{Allowed: true, Data: map[string]any{
		"summary": "疑似未提交",
	}}}
	svc := NewAnalyzeService(&fakeAnalyzeAlertReader{alert: longTransactionTestAlert()}, invoker, "provider-1", NoopAuditRecorder{})
	svc.SetMetricEvidence(&fakeEvidenceCollector{result: &MetricEvidenceResult{
		Status: EvidenceStatusOK, Metrics: []MetricEvidence{{Metric: "oldest_transaction_duration", EvidenceID: "ev-metric-1"}},
	}})
	svc.SetEvidenceSnapshots(&fakeEvidenceSnapshotStore{}, 0)
	port := &fakePostgresEvidencePort{result: &PostgresEvidenceResult{
		Status:   EvidenceStatusOK,
		Sessions: []PostgresSession{{SessionID: 42, SessionStatus: "idle in transaction", TransactionExecutionTime: 900, QueryID: "query-42"}},
	}}
	svc.SetPostgresEvidence(port, &fakePostgresEvidenceStore{}, 0)

	out, err := svc.AnalyzeAlert(context.Background(), "u-1", AnalyzeAlertInput{AlertID: "a1", IncludeMetrics: true})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if _, ok := invoker.captured.Payload["postgres_activity"].(*PostgresEvidenceResult); !ok {
		t.Fatalf("provider payload missing postgres activity: %#v", invoker.captured.Payload)
	}
	if out.Diagnosis == nil || out.Diagnosis.RootCause.Code != "idle_in_transaction" || out.Summary == "疑似未提交" {
		t.Fatalf("analysis diagnosis was not applied: %+v", out)
	}
	foundActivityRef := false
	for _, ref := range out.References {
		if strings.Contains(ref, "observability evidence ev-") && ref != "observability evidence ev-metric-1" {
			foundActivityRef = true
		}
	}
	if !foundActivityRef {
		t.Fatalf("activity evidence reference missing: %+v", out.References)
	}
}

func TestAnalyzeAlertPostgresToolDeniedDoesNotCollect(t *testing.T) {
	invoker := &fakeToolInvoker{
		resp:         &toolgateway.ToolResponse{Allowed: true, Data: map[string]any{}},
		validateResp: &toolgateway.ToolResponse{Allowed: false, Reason: "missing tool permission"},
	}
	svc := NewAnalyzeService(&fakeAnalyzeAlertReader{alert: longTransactionTestAlert()}, invoker, "provider-1", NoopAuditRecorder{})
	port := &fakePostgresEvidencePort{}
	svc.SetPostgresEvidence(port, &fakePostgresEvidenceStore{}, 0)

	out, err := svc.AnalyzeAlert(context.Background(), "u-1", AnalyzeAlertInput{AlertID: "a1", IncludeMetrics: true})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if port.calls != 0 {
		t.Fatal("tool denied must not call RDS history API")
	}
	if out.Diagnosis == nil || out.Diagnosis.RootCause.Status != "undetermined" {
		t.Fatalf("tool denied must not produce a root cause: %+v", out.Diagnosis)
	}
}

func TestSendMessagePostgresFailureProducesMissingEvidence(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	svc.alerts = &fakeAnalyzeAlertReader{alert: longTransactionTestAlert()}
	port := &fakePostgresEvidencePort{result: &PostgresEvidenceResult{
		Status: EvidenceStatusPermissionDenied, Reason: "denied",
	}}
	svc.SetPostgresEvidence(port, &fakePostgresEvidenceStore{}, 0)
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts", AlertID: "a1"})

	result, err := svc.SendMessage(context.Background(), SendChatMessageInput{
		UserID: "u-1", SessionID: "s-1", Content: "分析根因",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if result.Answer.Diagnosis == nil || result.Answer.Diagnosis.RootCause.Status != "undetermined" {
		t.Fatalf("failed evidence must be undetermined: %+v", result.Answer.Diagnosis)
	}
	if !result.Answer.InsufficientContext || len(result.Answer.Diagnosis.MissingEvidence) == 0 {
		t.Fatalf("missing evidence must be explicit: %+v", result.Answer)
	}
	if strings.Contains(result.Answer.Summary, "未提交") || strings.Contains(result.Answer.Summary, "持续阻塞") {
		t.Fatalf("fallback summary contains speculation: %q", result.Answer.Summary)
	}
}
