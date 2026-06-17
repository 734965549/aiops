package application

import (
	"context"
	"testing"

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
