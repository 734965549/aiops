package application

import (
	"context"
	"testing"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
)

type nilObservabilityPort struct{}

func (nilObservabilityPort) QueryMetrics(context.Context, obsapp.Actor, obsdomain.MetricQuery) (*obsapp.MetricQueryResult, error) {
	return nil, nil
}
func (nilObservabilityPort) SearchLogs(context.Context, obsapp.Actor, obsdomain.LogQuery) (*obsapp.LogSearchResult, error) {
	return nil, nil
}
func (nilObservabilityPort) QueryTraces(context.Context, obsapp.Actor, obsdomain.TraceQuery) (*obsapp.TraceQueryResult, error) {
	return nil, nil
}
func (nilObservabilityPort) QueryTopology(context.Context, obsapp.Actor, obsdomain.TopologyQuery) (*obsapp.TopologyQueryResult, error) {
	return nil, nil
}

func TestEvidenceAnalyzer_CollectEvidenceRejectsNilMetricResult(t *testing.T) {
	analyzer := NewEvidenceAnalyzer(nilObservabilityPort{})
	_, err := analyzer.CollectEvidence(context.Background(), Actor{UserID: "user-1"}, CheckEvidenceInput{
		Check: "metrics.cpu", AccountID: "acc-1",
	})
	if err == nil {
		t.Fatal("expected nil result error")
	}
}

func TestEvidenceAnalyzer_AnalyzeRejectsUnsupportedCheck(t *testing.T) {
	analyzer := NewEvidenceAnalyzer(nilObservabilityPort{})
	_, err := analyzer.Analyze(context.Background(), []string{"metrics.unknown"}, nil)
	if err == nil {
		t.Fatal("expected unsupported check error")
	}
}
