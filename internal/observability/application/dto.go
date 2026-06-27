package application

import (
	"github.com/734965549/aiops/internal/observability/domain"
)

type MetricQueryResult struct {
	Series     []domain.MetricSeries `json:"series"`
	EvidenceID string                `json:"evidence_id"`
}

type LogSearchResult struct {
	Entries    []domain.LogEntry `json:"entries"`
	EvidenceID string            `json:"evidence_id"`
}

type TraceQueryResult struct {
	Spans      []domain.TraceSpan `json:"spans"`
	EvidenceID string             `json:"evidence_id"`
}

type TopologyQueryResult struct {
	Topology   domain.TopologySnapshot `json:"topology"`
	EvidenceID string                  `json:"evidence_id"`
}

type AssetDiscoveryResult struct {
	Resources  []domain.CloudResource `json:"resources"`
	EvidenceID string                 `json:"evidence_id"`
	// HasMore 表示因达到查询上限而截断（云端仍有更多资源）。Asset Sync 通用同步路径据此
	// 跳过该类型 stale 标记，避免未返回资源被误标 stale，见 §13.1。
	HasMore bool `json:"has_more,omitempty"`
}

type AlertRuleQueryResult struct {
	Rules      []domain.AlertRule `json:"rules"`
	EvidenceID string             `json:"evidence_id"`
}

// AssetFullSyncQuery 全量同步查询（专供 Asset Sync，不受交互查询 limit<=500 限制），
// 见 docs/huawei-ces-asset-sync-plan.md §7.2。
type AssetFullSyncQuery struct {
	AccountID    string
	Provider     string
	Region       string
	MaxResources int
}

// CloudSyncSummary 云资源全量同步摘要，用于回写 batch message，见 §7.1。
// EnrichedCount / EnrichmentFailedTypes 仅 hybrid 模式下填充，见 §8.2。
type CloudSyncSummary struct {
	Region                 string   `json:"region"`
	ProjectID              string   `json:"project_id,omitempty"`
	SyncMode               string   `json:"sync_mode,omitempty"`
	ResourceGroupID        string   `json:"resource_group_id,omitempty"`
	ResourceGroupName      string   `json:"resource_group_name,omitempty"`
	ResourceGroupSelection string   `json:"resource_group_selection,omitempty"`
	CESTotal               int      `json:"ces_total,omitempty"`
	Discovered             int      `json:"discovered"`
	FailedScopes           []string `json:"failed_scopes,omitempty"`
	SuccessfulTypes        []string `json:"successful_types,omitempty"`
	// QueryFailedTypes 记录存在 scope 查询失败的类型；该类型已从 SuccessfulTypes 剔除，见 §13.1。
	QueryFailedTypes      []string `json:"query_failed_types,omitempty"`
	UnknownNamespaceCount int      `json:"unknown_namespace_count,omitempty"`
	InvalidResourceCount  int      `json:"invalid_resource_count,omitempty"`
	// ConversionFailedTypes 记录存在资源转换失败的类型，sync_service 据此禁止该类型执行 stale，见 §13。
	ConversionFailedTypes []string `json:"conversion_failed_types,omitempty"`
	ProductNamesEmpty     bool     `json:"product_names_empty,omitempty"`
	// MaxResourcesReached 表示因达到上限而提前终止，调用方必须禁止该 scope 执行 stale，见 §13。
	MaxResourcesReached   bool     `json:"max_resources_reached,omitempty"`
	EnrichedCount         int      `json:"enriched_count,omitempty"`
	EnrichmentFailedTypes []string `json:"enrichment_failed_types,omitempty"`
}

// AssetFullSyncResult 全量同步结果。
type AssetFullSyncResult struct {
	Resources  []domain.CloudResource `json:"resources"`
	Summary    CloudSyncSummary       `json:"summary"`
	EvidenceID string                 `json:"evidence_id"`
}
