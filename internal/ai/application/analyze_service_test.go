package application

import (
	"context"
	"strings"
	"testing"

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
	captured toolgateway.ToolRequest
	resp     *toolgateway.ToolResponse
	err      error
}

func (f *fakeToolInvoker) Invoke(_ context.Context, _ string, req toolgateway.ToolRequest) (*toolgateway.ToolResponse, error) {
	f.captured = req
	return f.resp, f.err
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
	invoker := &fakeToolInvoker{resp: &toolgateway.ToolResponse{Data: map[string]any{}}}
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
