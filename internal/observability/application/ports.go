package application

import (
	"context"

	"github.com/734965549/aiops/internal/observability/domain"
)

// IntegrationAccountPort 从 Integration 上下文解析启用账号与凭据（infrastructure 适配，application 不依赖 GORM）。
type IntegrationAccountPort interface {
	ResolveAccount(ctx context.Context, accountID string) (*domain.AccountSnapshot, error)
}

// MetricQueryPort 指标只读查询端口。
type MetricQueryPort interface {
	QueryMetrics(ctx context.Context, pctx domain.ProviderContext, q domain.MetricQuery) ([]domain.MetricSeries, error)
}

// LogSearchPort 日志只读搜索端口。
type LogSearchPort interface {
	SearchLogs(ctx context.Context, pctx domain.ProviderContext, q domain.LogQuery) ([]domain.LogEntry, error)
}

// TraceQueryPort 链路只读查询端口。
type TraceQueryPort interface {
	QueryTraces(ctx context.Context, pctx domain.ProviderContext, q domain.TraceQuery) ([]domain.TraceSpan, error)
}

// TopologyQueryPort 拓扑只读查询端口。
type TopologyQueryPort interface {
	QueryTopology(ctx context.Context, pctx domain.ProviderContext, q domain.TopologyQuery) (*domain.TopologySnapshot, error)
}

// AssetDiscoveryPort 云资源只读发现端口（交互查询口，供 Asset Sync / cloud.resources.list 复用）。
// ListResources 返回 (resources, hasMore, err)：hasMore=true 表示因达到查询上限而截断，
// 云端仍有更多资源。它只负责交互式发现，不承担全量同步或增强职责。
type AssetDiscoveryPort interface {
	ListResources(ctx context.Context, pctx domain.ProviderContext, q domain.AssetDiscoveryQuery) ([]domain.CloudResource, bool, error)
}

// CloudFullSyncPort 云资源全量同步端口（专供 Asset Sync 全量分页发现，不受交互查询 limit<=500 限制），
// 见 ops/huawei-ces-sync-contract.md §7.2。返回资源列表与同步摘要，摘要用于回写 batch message；
// 它是全量发现口，不做增强。
type CloudFullSyncPort interface {
	ListAllResources(ctx context.Context, pctx domain.ProviderContext, q AssetFullSyncQuery) ([]domain.CloudResource, *CloudSyncSummary, error)
}

// CloudEnrichmentPort 云资源增强端口（hybrid 第二阶段独立增强入口），
// 由 Asset 层在基础资源落库后调用，见 ops/huawei-ces-sync-contract.md §8.2；只负责补详情与回写前增强，不负责发现。
type CloudEnrichmentPort interface {
	EnrichAllResources(ctx context.Context, actor Actor, q AssetFullSyncQuery, resources []domain.CloudResource) (*AssetFullSyncEnrichmentResult, error)
}

// AlertRuleQueryPort 告警规则只读查询端口（cloud.alerts.list 复用）。
type AlertRuleQueryPort interface {
	ListAlertRules(ctx context.Context, pctx domain.ProviderContext, q domain.AlertRuleQuery) ([]domain.AlertRule, error)
}

// ProviderEntry 标识已注册的观测 Provider；具体能力通过小 Port 接口按需类型断言，避免强制实现全部方法。
type ProviderEntry interface {
	ProviderType() string
}

// ProviderRegistry 按 provider 类型解析 ProviderEntry。
type ProviderRegistry interface {
	Get(provider string) (ProviderEntry, error)
}
