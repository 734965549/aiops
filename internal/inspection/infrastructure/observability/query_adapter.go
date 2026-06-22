package observability

import (
	"context"

	inspectionapp "github.com/734965549/aiops/internal/inspection/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// QueryAdapter 将 Observability QueryService 适配为 Inspection ObservabilityQueryPort。
type QueryAdapter struct {
	svc *obsapp.QueryService
}

func NewQueryAdapter(svc *obsapp.QueryService) *QueryAdapter {
	return &QueryAdapter{svc: svc}
}

func (a *QueryAdapter) QueryMetrics(ctx context.Context, actor obsapp.Actor, q obsdomain.MetricQuery) (*obsapp.MetricQueryResult, error) {
	if a == nil || a.svc == nil {
		return nil, observabilityUnavailableErr()
	}
	return a.svc.QueryMetrics(ctx, actor, q)
}

func (a *QueryAdapter) SearchLogs(ctx context.Context, actor obsapp.Actor, q obsdomain.LogQuery) (*obsapp.LogSearchResult, error) {
	if a == nil || a.svc == nil {
		return nil, observabilityUnavailableErr()
	}
	return a.svc.SearchLogs(ctx, actor, q)
}

func (a *QueryAdapter) QueryTraces(ctx context.Context, actor obsapp.Actor, q obsdomain.TraceQuery) (*obsapp.TraceQueryResult, error) {
	if a == nil || a.svc == nil {
		return nil, observabilityUnavailableErr()
	}
	return a.svc.QueryTraces(ctx, actor, q)
}

func (a *QueryAdapter) QueryTopology(ctx context.Context, actor obsapp.Actor, q obsdomain.TopologyQuery) (*obsapp.TopologyQueryResult, error) {
	if a == nil || a.svc == nil {
		return nil, observabilityUnavailableErr()
	}
	return a.svc.QueryTopology(ctx, actor, q)
}

func observabilityUnavailableErr() error {
	return apperr.New(apperr.CodeUnavailable, "observability query service is not configured")
}

var _ inspectionapp.ObservabilityQueryPort = (*QueryAdapter)(nil)
