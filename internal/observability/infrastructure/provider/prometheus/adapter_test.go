package prometheus

import (
	"testing"

	obsapp "github.com/734965549/aiops/internal/observability/application"
)

func TestAdapter_PartialPorts(t *testing.T) {
	a := NewAdapter()
	var _ obsapp.MetricQueryPort = a
	var _ obsapp.AlertRuleQueryPort = a
	if _, ok := any(a).(obsapp.LogSearchPort); ok {
		t.Fatal("prometheus adapter must not implement LogSearchPort")
	}
	if _, ok := any(a).(obsapp.TraceQueryPort); ok {
		t.Fatal("prometheus adapter must not implement TraceQueryPort")
	}
}
