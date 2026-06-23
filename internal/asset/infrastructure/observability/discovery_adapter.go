package observability

import (
	"context"

	assetapp "github.com/734965549/aiops/internal/asset/application"
	"github.com/734965549/aiops/internal/asset/domain"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
)

// DiscoveryAdapter 将 Observability QueryService 适配为 Asset CloudDiscoveryPort。
type DiscoveryAdapter struct {
	svc *obsapp.QueryService
}

func NewDiscoveryAdapter(svc *obsapp.QueryService) *DiscoveryAdapter {
	return &DiscoveryAdapter{svc: svc}
}

func (a *DiscoveryAdapter) ListResources(ctx context.Context, actor obsapp.Actor, q obsdomain.AssetDiscoveryQuery) (*obsapp.AssetDiscoveryResult, error) {
	if a == nil || a.svc == nil {
		return nil, domain.ErrDiscoveryUnavailable
	}
	return a.svc.ListResources(ctx, actor, q)
}

var _ assetapp.CloudDiscoveryPort = (*DiscoveryAdapter)(nil)
