package signoz

import (
	"context"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	"github.com/734965549/aiops/internal/observability/infrastructure/provider/fake"
)

// Adapter SigNoz 只读观测 Adapter。
type Adapter struct {
	inner *fake.Provider
}

func NewAdapter() *Adapter {
	return &Adapter{inner: fake.New(string(integdomain.ProviderSigNoz))}
}

func (a *Adapter) ProviderType() string { return string(integdomain.ProviderSigNoz) }

func (a *Adapter) QueryMetrics(ctx context.Context, pctx domain.ProviderContext, q domain.MetricQuery) ([]domain.MetricSeries, error) {
	return a.inner.QueryMetrics(ctx, pctx, q)
}

func (a *Adapter) SearchLogs(ctx context.Context, pctx domain.ProviderContext, q domain.LogQuery) ([]domain.LogEntry, error) {
	return a.inner.SearchLogs(ctx, pctx, q)
}

func (a *Adapter) QueryTraces(ctx context.Context, pctx domain.ProviderContext, q domain.TraceQuery) ([]domain.TraceSpan, error) {
	return a.inner.QueryTraces(ctx, pctx, q)
}

func (a *Adapter) QueryTopology(ctx context.Context, pctx domain.ProviderContext, q domain.TopologyQuery) (*domain.TopologySnapshot, error) {
	return a.inner.QueryTopology(ctx, pctx, q)
}

func (a *Adapter) ListAlertRules(ctx context.Context, pctx domain.ProviderContext, q domain.AlertRuleQuery) ([]domain.AlertRule, error) {
	return a.inner.ListAlertRules(ctx, pctx, q)
}

var (
	_ obsapp.ProviderEntry      = (*Adapter)(nil)
	_ obsapp.MetricQueryPort    = (*Adapter)(nil)
	_ obsapp.LogSearchPort      = (*Adapter)(nil)
	_ obsapp.TraceQueryPort     = (*Adapter)(nil)
	_ obsapp.TopologyQueryPort  = (*Adapter)(nil)
	_ obsapp.AlertRuleQueryPort = (*Adapter)(nil)
)
