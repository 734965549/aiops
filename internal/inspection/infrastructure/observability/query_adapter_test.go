package observability

import (
	"context"
	"testing"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

func TestQueryAdapter_ReturnsUnavailableWhenServiceMissing(t *testing.T) {
	adapter := NewQueryAdapter(nil)
	res, err := adapter.QueryMetrics(context.Background(), obsapp.Actor{UserID: "user-1"}, obsdomain.MetricQuery{})
	if err == nil {
		t.Fatal("expected unavailable error")
	}
	if res != nil {
		t.Fatalf("expected nil result, got %+v", res)
	}
	if apperr.CodeOf(err) != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable, got %s", apperr.CodeOf(err))
	}
}
