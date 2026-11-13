package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeAnalyzeAlertReader struct {
	alert *AlertContext
	err   error
}

func (f *fakeAnalyzeAlertReader) GetForAnalysis(_ context.Context, _ string) (*AlertContext, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.alert, nil
}

func TestAnalyzeService_LocalSummary(t *testing.T) {
	svc := NewAnalyzeService(&fakeAnalyzeAlertReader{
		alert: &AlertContext{Name: "HighCPU", Summary: "CPU > 85%", Severity: "p1"},
	}, nil, "", NoopAuditRecorder{})
	out, err := svc.AnalyzeAlert(context.Background(), "u1", AnalyzeAlertInput{AlertID: "a1"})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if out.Summary == "" || out.RiskLevel != "high" || out.ConversationID == "" {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestAnalyzeService_NotFound(t *testing.T) {
	svc := NewAnalyzeService(&fakeAnalyzeAlertReader{err: apperr.New(apperr.CodeNotFound, "alert not found")}, nil, "", NoopAuditRecorder{})
	_, err := svc.AnalyzeAlert(context.Background(), "u1", AnalyzeAlertInput{AlertID: "missing"})
	if err == nil || apperr.FromError(err).Code != apperr.CodeNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestAnalyzeService_InvalidArgument(t *testing.T) {
	svc := NewAnalyzeService(&fakeAnalyzeAlertReader{}, nil, "", NoopAuditRecorder{})
	_, err := svc.AnalyzeAlert(context.Background(), "u1", AnalyzeAlertInput{})
	if err == nil || apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

type fakeToolInvoker struct {
	providerID string
	captured   toolgateway.ToolRequest
	resp       *toolgateway.ToolResponse
	err        error
}

func (f *fakeToolInvoker) Invoke(_ context.Context, providerID string, req toolgateway.ToolRequest) (*toolgateway.ToolResponse, error) {
	f.providerID = providerID
	f.captured = req
	return f.resp, f.err
}

type fakeProviderResolver struct {
	id string
	ok bool
}

func (f fakeProviderResolver) AnalysisProviderID() (string, bool) { return f.id, f.ok }

func TestAnalyzeServiceUsesDynamicProviderAndServerPrompt(t *testing.T) {
	reader := &fakeAnalyzeAlertReader{alert: &AlertContext{ID: "a1", Name: "HighCPU", Severity: "p1"}}
	invoker := &fakeToolInvoker{resp: &toolgateway.ToolResponse{Allowed: true, Data: map[string]any{"summary": "模型结论"}}}
	svc := NewAnalyzeService(reader, invoker, "static-provider", NoopAuditRecorder{})
	svc.SetProviderResolver(fakeProviderResolver{id: "dynamic-provider", ok: true})

	out, err := svc.AnalyzeAlert(context.Background(), "u1", AnalyzeAlertInput{AlertID: "a1"})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if invoker.providerID != "dynamic-provider" {
		t.Fatalf("expected dynamic provider, got %q", invoker.providerID)
	}
	if invoker.captured.SystemPrompt == "" || invoker.captured.UserPrompt == "" {
		t.Fatalf("expected server-side prompts, got %+v", invoker.captured)
	}
	if invoker.captured.RequiredPermission != "app:ai.analysis:analyze" {
		t.Fatalf("expected stable analysis permission, got %q", invoker.captured.RequiredPermission)
	}
	if out.Summary != "模型结论" {
		t.Fatalf("expected model summary, got %q", out.Summary)
	}
}

func TestAnalyzeServiceSurfacesAuthorizationDenial(t *testing.T) {
	reader := &fakeAnalyzeAlertReader{alert: &AlertContext{ID: "a1", Name: "HighCPU", Severity: "p1"}}
	invoker := &fakeToolInvoker{resp: &toolgateway.ToolResponse{
		Allowed: false,
		Reason:  "missing operation permission",
	}}
	audit := &capturingAIAudit{}
	svc := NewAnalyzeService(reader, invoker, "provider-1", audit)

	if _, err := svc.AnalyzeAlert(context.Background(), "u1", AnalyzeAlertInput{AlertID: "a1"}); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(audit.rows) != 1 {
		t.Fatalf("unexpected audit rows: %+v", audit.rows)
	}
	payload := audit.rows[0].Payload
	if payload["result"] != "partial" {
		t.Fatalf("expected partial result, got %+v", payload)
	}
	toolError, _ := payload["tool_error"].(string)
	if !strings.Contains(toolError, "authorization_denied") || !strings.Contains(toolError, "missing operation permission") {
		t.Fatalf("expected denial reason in audit, got %q", toolError)
	}
}

func TestAnalyzeServiceMarksDegradedProviderAsPartial(t *testing.T) {
	reader := &fakeAnalyzeAlertReader{alert: &AlertContext{ID: "a1", Name: "HighCPU", Severity: "p1"}}
	invoker := &fakeToolInvoker{resp: &toolgateway.ToolResponse{Allowed: true, Data: map[string]any{
		"summary":  "降级结论",
		"degraded": true,
	}}}
	audit := &capturingAIAudit{}
	svc := NewAnalyzeService(reader, invoker, "provider-1", audit)

	if _, err := svc.AnalyzeAlert(context.Background(), "u1", AnalyzeAlertInput{AlertID: "a1"}); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(audit.rows) != 1 || audit.rows[0].Payload["result"] != "partial" {
		t.Fatalf("expected partial audit, got %+v", audit.rows)
	}
}

func TestAnalyzeService_PayloadIncludesFullAlertSnapshot(t *testing.T) {
	reader := &fakeAnalyzeAlertReader{alert: &AlertContext{
		ID: "a1", Name: "HighCPU", Summary: "ecs-prod-01", Description: "SYS.ECS cpu_util > 90%",
		Severity: "p1", Status: "processing", Source: "huawei_ces", RuleName: "CPU rule",
		BusinessLine: "payments", Environment: "prod", ResourceID: "i-1", ResourceName: "ecs-prod-01",
		ResourceType: "ecs", OccurrenceCount: 3, FirstSeenAt: 1700000000, LastSeenAt: 1700001800,
		Labels:      map[string]string{"account_id": "acc-1", "region": "cn-north-4"},
		Annotations: map[string]any{"ces_condition": "> 90% (average, 300s x3)"},
	}}
	invoker := &fakeToolInvoker{resp: &toolgateway.ToolResponse{Allowed: true, Data: map[string]any{}}}
	svc := NewAnalyzeService(reader, invoker, "provider-1", NoopAuditRecorder{})

	if _, err := svc.AnalyzeAlert(context.Background(), "u1", AnalyzeAlertInput{
		AlertID: "a1", TimeRange: "30m", IncludeMetrics: true,
	}); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	snapshot, ok := invoker.captured.Payload["alert"].(map[string]any)
	if !ok {
		t.Fatalf("payload.alert missing or wrong type: %#v", invoker.captured.Payload["alert"])
	}
	for key, want := range map[string]string{
		"description":   "SYS.ECS cpu_util > 90%",
		"status":        "processing",
		"source":        "huawei_ces",
		"rule_name":     "CPU rule",
		"business_line": "payments",
		"resource_type": "ecs",
	} {
		if got, _ := snapshot[key].(string); got != want {
			t.Fatalf("snapshot[%s] = %q, want %q", key, got, want)
		}
	}
	if got := snapshot["occurrence_count"]; got != float64(3) {
		t.Fatalf("snapshot occurrence_count = %#v", got)
	}
	if got := snapshot["first_seen_at"]; got != float64(1700000000) {
		t.Fatalf("snapshot first_seen_at = %#v", got)
	}
	annotations, ok := snapshot["annotations"].(map[string]any)
	if !ok || annotations["ces_condition"] != "> 90% (average, 300s x3)" {
		t.Fatalf("snapshot annotations = %#v", snapshot["annotations"])
	}
	if got, _ := invoker.captured.Payload["alert_name"].(string); got != "HighCPU" {
		t.Fatalf("legacy alert_name = %q", got)
	}
}

func TestBuildLocalSummaryPrefersDescription(t *testing.T) {
	summary := buildLocalSummary(&AlertContext{
		Name: "HighCPU", Summary: "ecs-prod-01", Description: "SYS.ECS cpu_util > 90% · ecs-prod-01",
	})
	if !strings.Contains(summary, "cpu_util") {
		t.Fatalf("expected description in summary, got %q", summary)
	}
}

type fakeEvidenceCollector struct {
	got    MetricEvidenceQuery
	userID string
	result *MetricEvidenceResult
	err    error
}

func (f *fakeEvidenceCollector) Collect(_ context.Context, userID string, q MetricEvidenceQuery) (*MetricEvidenceResult, error) {
	f.got = q
	f.userID = userID
	return f.result, f.err
}

func evidenceTestAlert() *AlertContext {
	return &AlertContext{
		Name: "HighCPU", Severity: "p1",
		Labels: map[string]string{
			"account_id": "acc-1", "region": "cn-north-4", "project_id": "p-1",
			"record_id": "r-1", "alarm_id": "al-1", "rule_name": "cpu-rule",
			"metric_name": "cpu_util", "resource_type": "ecs", "ces_namespace": "SYS.ECS",
			"cloud_resource_id": "i-1", "instance_id": "i-1",
		},
		Annotations: map[string]any{
			"ces_filter": "max", "ces_threshold": "90", "ces_operator": ">",
		},
		FirstSeenAt: 1700000000, LastSeenAt: 1700000600,
	}
}

func TestDeriveMetricEvidenceQuery(t *testing.T) {
	q, skip := deriveMetricEvidenceQuery(evidenceTestAlert())
	if skip != "" {
		t.Fatalf("unexpected skip: %s", skip)
	}
	if q.AccountID != "acc-1" || q.Region != "cn-north-4" || q.ProjectID != "p-1" ||
		q.Namespace != "SYS.ECS" || q.Metric != "cpu_util" {
		t.Fatalf("unexpected query identity: %+v", q)
	}
	if len(q.Dimensions) != 1 || q.Dimensions["instance_id"] != "i-1" {
		t.Fatalf("unexpected dimensions: %+v", q.Dimensions)
	}
	if q.From != 1700000000-1800 || q.To != 1700000600+600 {
		t.Fatalf("unexpected window: %d..%d", q.From, q.To)
	}
	if q.Period != 300 || q.Aggregator != "max" || q.Operator != ">" || q.Threshold == nil || *q.Threshold != 90 {
		t.Fatalf("unexpected query options: %+v", q)
	}
}

func TestEvidenceTimeWindowClampsLongRunningAlert(t *testing.T) {
	now := time.Now().Unix()
	alert := &AlertContext{FirstSeenAt: now - 10*24*3600, LastSeenAt: now - 1200}
	from, to := evidenceTimeWindow(alert)
	if to != alert.LastSeenAt+evidenceWindowAfter {
		t.Fatalf("to = %d, want %d", to, alert.LastSeenAt+evidenceWindowAfter)
	}
	if to-from != evidenceMaxWindow {
		t.Fatalf("window = %d, want %d", to-from, evidenceMaxWindow)
	}
	if from <= alert.FirstSeenAt {
		t.Fatalf("from = %d should focus on recent window", from)
	}
}

func TestDeriveMetricEvidenceQuerySkipReasons(t *testing.T) {
	cases := []struct {
		name  string
		alert *AlertContext
		want  string
	}{
		{name: "nil alert", alert: nil, want: "no_alert"},
		{name: "no account", alert: &AlertContext{}, want: "no_account"},
		{name: "no region", alert: &AlertContext{Labels: map[string]string{"account_id": "acc"}}, want: "no_region"},
		{name: "no namespace", alert: &AlertContext{Labels: map[string]string{"account_id": "acc", "region": "cn-north-4"}}, want: "no_namespace"},
		{name: "no metric", alert: &AlertContext{Labels: map[string]string{"account_id": "acc", "region": "cn-north-4", "ces_namespace": "SYS.ECS"}}, want: "no_metric"},
		{name: "no dimensions", alert: &AlertContext{Labels: map[string]string{
			"account_id": "acc", "region": "cn-north-4", "ces_namespace": "SYS.ECS", "metric_name": "cpu_util",
		}}, want: "no_dimensions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, got := deriveMetricEvidenceQuery(tc.alert); got != tc.want {
				t.Fatalf("skip = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAnalyzeService_InjectsMetricEvidenceAndReferences(t *testing.T) {
	invoker := &fakeToolInvoker{resp: &toolgateway.ToolResponse{Allowed: true, Data: map[string]any{}}}
	collector := &fakeEvidenceCollector{result: &MetricEvidenceResult{
		Status: EvidenceStatusOK,
		Metrics: []MetricEvidence{{
			Metric: "cpu_util", Unit: "%", EvidenceID: "ev-1",
		}},
	}}
	svc := NewAnalyzeService(&fakeAnalyzeAlertReader{alert: evidenceTestAlert()}, invoker, "provider-1", NoopAuditRecorder{})
	svc.SetMetricEvidence(collector)

	out, err := svc.AnalyzeAlert(context.Background(), "u-1", AnalyzeAlertInput{AlertID: "a1", IncludeMetrics: true})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if collector.userID != "u-1" || collector.got.Metric != "cpu_util" {
		t.Fatalf("unexpected collector call: user=%s query=%+v", collector.userID, collector.got)
	}
	evidence, ok := invoker.captured.Payload["evidence"].(*MetricEvidenceResult)
	if !ok || evidence.Status != EvidenceStatusOK || len(evidence.Metrics) != 1 {
		t.Fatalf("unexpected payload evidence: %#v", invoker.captured.Payload["evidence"])
	}
	if len(out.References) != 1 || out.References[0] != "observability evidence ev-1" {
		t.Fatalf("unexpected references: %+v", out.References)
	}
}

func TestAnalyzeService_EvidenceErrorDoesNotBlockAnalysis(t *testing.T) {
	invoker := &fakeToolInvoker{resp: &toolgateway.ToolResponse{Allowed: true, Data: map[string]any{}}}
	collector := &fakeEvidenceCollector{result: &MetricEvidenceResult{Status: EvidenceStatusPermissionDenied, Reason: "denied"}}
	svc := NewAnalyzeService(&fakeAnalyzeAlertReader{alert: evidenceTestAlert()}, invoker, "provider-1", NoopAuditRecorder{})
	svc.SetMetricEvidence(collector)

	out, err := svc.AnalyzeAlert(context.Background(), "u-1", AnalyzeAlertInput{AlertID: "a1", IncludeMetrics: true})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if out.Summary == "" || out.RiskLevel != "high" {
		t.Fatalf("analysis should still succeed, got %+v", out)
	}
	evidence, _ := invoker.captured.Payload["evidence"].(*MetricEvidenceResult)
	if evidence == nil || evidence.Status != EvidenceStatusPermissionDenied {
		t.Fatalf("unexpected evidence status: %#v", evidence)
	}
}

func TestAnalyzeService_CollectorDisabledSkipsEvidence(t *testing.T) {
	invoker := &fakeToolInvoker{resp: &toolgateway.ToolResponse{Allowed: true, Data: map[string]any{}}}
	svc := NewAnalyzeService(&fakeAnalyzeAlertReader{alert: evidenceTestAlert()}, invoker, "provider-1", NoopAuditRecorder{})

	if _, err := svc.AnalyzeAlert(context.Background(), "u-1", AnalyzeAlertInput{AlertID: "a1", IncludeMetrics: true}); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	evidence, _ := invoker.captured.Payload["evidence"].(*MetricEvidenceResult)
	if evidence == nil || evidence.Status != EvidenceStatusSkipped || evidence.Reason != "collector_disabled" {
		t.Fatalf("unexpected evidence: %#v", evidence)
	}
}

func TestCapEvidencePayloadTruncatesPoints(t *testing.T) {
	points := make([][2]float64, 0, 3000)
	for i := 0; i < 3000; i++ {
		points = append(points, [2]float64{float64(1700000000 + i), 88.88})
	}
	evidence := &MetricEvidenceResult{
		Status:  EvidenceStatusOK,
		Metrics: []MetricEvidence{{Metric: "cpu_util", Points: points, EvidenceID: "ev-1"}},
	}
	capped := capEvidencePayload(evidence)
	if capped == nil || capped.Reason != "truncated" {
		t.Fatalf("expected truncation, got %+v", capped)
	}
	if len(capped.Metrics) != 1 || len(capped.Metrics[0].Points) != 0 {
		t.Fatalf("expected points dropped, got %+v", capped.Metrics)
	}
	if capped.Metrics[0].EvidenceID != "ev-1" {
		t.Fatalf("summary metadata should be kept, got %+v", capped.Metrics[0])
	}
}
