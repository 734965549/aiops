package prometheus

import (
	"context"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	"github.com/734965549/aiops/internal/observability/infrastructure/provider/fake"
)

// Adapter Prometheus 只读观测 Adapter（partial provider：metrics + alerts）。
type Adapter struct {
	inner *fake.Provider
}

func NewAdapter() *Adapter {
	return &Adapter{inner: fake.New(string(integdomain.ProviderPrometheus))}
}

func (a *Adapter) ProviderType() string { return string(integdomain.ProviderPrometheus) }

func (a *Adapter) QueryMetrics(ctx context.Context, pctx domain.ProviderContext, q domain.MetricQuery) ([]domain.MetricSeries, error) {
	return a.inner.QueryMetrics(ctx, pctx, q)
}

func (a *Adapter) ListAlertRules(ctx context.Context, pctx domain.ProviderContext, q domain.AlertRuleQuery) ([]domain.AlertRule, error) {
	return a.inner.ListAlertRules(ctx, pctx, q)
}

var (
	_ obsapp.ProviderEntry      = (*Adapter)(nil)
	_ obsapp.MetricQueryPort    = (*Adapter)(nil)
	_ obsapp.AlertRuleQueryPort = (*Adapter)(nil)
)
