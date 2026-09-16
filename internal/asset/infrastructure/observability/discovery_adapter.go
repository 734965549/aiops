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

// ListAllResourcesDiscovery 桥接 QueryService 全量同步发现，供 Asset Sync 调用。
func (a *DiscoveryAdapter) ListAllResourcesDiscovery(ctx context.Context, actor obsapp.Actor, q obsapp.AssetFullSyncQuery) (*obsapp.AssetFullSyncResult, error) {
	if a == nil || a.svc == nil {
		return nil, domain.ErrDiscoveryUnavailable
	}
	return a.svc.ListAllResources(ctx, actor, q)
}

// ListAllResources 保留兼容入口，等价于第一阶段发现。
func (a *DiscoveryAdapter) ListAllResources(ctx context.Context, actor obsapp.Actor, q obsapp.AssetFullSyncQuery) (*obsapp.AssetFullSyncResult, error) {
	return a.ListAllResourcesDiscovery(ctx, actor, q)
}

// EnrichAllResources 桥接 QueryService 第二阶段独立增强，供 Asset Sync 在基础资源落库后调用。
// 仅 hybrid 模式触发；返回增强摘要与实际合并了 label 的资源子集，供调用方带租约回写 labels。
func (a *DiscoveryAdapter) EnrichAllResources(ctx context.Context, actor obsapp.Actor, q obsapp.AssetFullSyncQuery, resources []obsdomain.CloudResource) (*obsapp.AssetFullSyncEnrichmentResult, error) {
	if a == nil || a.svc == nil {
		return nil, domain.ErrDiscoveryUnavailable
	}
	return a.svc.EnrichAllResources(ctx, actor, q, resources)
}

var _ assetapp.CloudDiscoveryPort = (*DiscoveryAdapter)(nil)
