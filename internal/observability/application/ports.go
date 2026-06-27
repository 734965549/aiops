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

// AssetDiscoveryPort 云资源只读发现端口（Asset Sync / cloud.resources.list 复用）。
// ListResources 返回 (resources, hasMore, err)：hasMore=true 表示因达到查询上限而截断，
// 云端仍有更多资源。Asset Sync 通用同步路径据此跳过该类型 stale 标记，见 §13.1。
type AssetDiscoveryPort interface {
	ListResources(ctx context.Context, pctx domain.ProviderContext, q domain.AssetDiscoveryQuery) ([]domain.CloudResource, bool, error)
}

// CloudFullSyncPort 云资源全量同步端口（专供 Asset Sync 全量分页发现，不受交互查询 limit<=500 限制），
// 见 docs/huawei-ces-asset-sync-plan.md §7.2。返回资源列表与同步摘要，摘要用于回写 batch message。
type CloudFullSyncPort interface {
	ListAllResources(ctx context.Context, pctx domain.ProviderContext, q AssetFullSyncQuery) ([]domain.CloudResource, *CloudSyncSummary, error)
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
