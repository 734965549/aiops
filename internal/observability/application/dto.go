package application

import (
	"strings"

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
	// 跳过该类型 stale 标记，避免未返回资源被误标 stale，见 ops/huawei-ces-sync-contract.md §13.1。
	HasMore bool `json:"has_more,omitempty"`
}

type AlertRuleQueryResult struct {
	Rules      []domain.AlertRule `json:"rules"`
	EvidenceID string             `json:"evidence_id"`
}

// AssetFullSyncQuery 全量同步查询（专供 Asset Sync，不受交互查询 limit<=500 限制），
// 见 ops/huawei-ces-sync-contract.md §7.2。
type AssetFullSyncQuery struct {
	AccountID    string
	Provider     string
	Region       string
	MaxResources int
	// Account 是 TriggerSync 时冻结的账号配置快照。非 nil 时 QueryService 跳过 DB 重读，
	// 直接用它构造 ProviderContext，保证整个同步批次使用同一套 sync_mode/resource_group/project_id 配置，
	// 避免同步窗口内修改账号配置导致同批次混用多套配置，见 §13.2。
	// json:\"-\" 防止被 hashQuery 序列化进 evidence。
	Account *domain.AccountSnapshot `json:"-"`
}

// CloudSyncSummary 云资源全量同步摘要，用于回写 batch message，见 ops/huawei-ces-sync-contract.md §7.2。
// 字段分层以契约 §3.3 为准：对账字段用于验收与归档，门控字段驱动 stale/partial/finalize，
// 诊断字段用于排障，展示字段仅用于 UI 说明。
// EnrichedCount / EnrichmentFailedTypes 仅 hybrid 模式下填充，见 §8.2。
type CloudSyncSummary struct {
	Region    string `json:"region"`
	ProjectID string `json:"project_id,omitempty"`
	// SyncMode 标识本 scope 的同步路径；native 仅作为兼容/回退路径，不应被当成与 CES 等价的主路径。
	SyncMode               string   `json:"sync_mode,omitempty"`
	ResourceGroupID        string   `json:"resource_group_id,omitempty"`
	ResourceGroupName      string   `json:"resource_group_name,omitempty"`
	ResourceGroupSelection string   `json:"resource_group_selection,omitempty"`
	CESTotal               int      `json:"ces_total,omitempty"`
	RawFetchedCount        int      `json:"raw_fetched_count,omitempty"`
	MappedCount            int      `json:"mapped_count,omitempty"`
	UniqueDiscoveredCount  int      `json:"unique_discovered_count,omitempty"`
	PersistedCount         int      `json:"persisted_count,omitempty"`
	DuplicateCount         int      `json:"duplicate_count,omitempty"`
	PersistFailedCount     int      `json:"persist_failed_count,omitempty"`
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
	// ResourceLevel 记录 ShowResourceGroup 响应的 resource_level（product|dimension）。
	// 仅 product 级且 product_names 非空时 scope 权威，允许反向 stale。见 §8.5/§13.1。
	ResourceLevel string `json:"resource_level,omitempty"`
	// MaxResourcesReached 表示因达到上限而提前终止，调用方必须禁止该 scope 执行 stale，见 §13。
	MaxResourcesReached bool `json:"max_resources_reached,omitempty"`
	// PartialReason 固化本轮 partial 的显式原因，便于运维无需展开细节也能快速识别风险来源。
	PartialReason         string   `json:"partial_reason,omitempty"`
	ConfigModeFallback    bool     `json:"config_mode_fallback,omitempty"`
	EnrichedCount         int      `json:"enriched_count,omitempty"`
	EnrichmentFailedCount int      `json:"enrichment_failed_count,omitempty"`
	EnrichmentFailedTypes []string `json:"enrichment_failed_types,omitempty"`
	// EnrichmentWarnings 记录 best-effort 增强缺失（如 dms.kafka、dms.rocketmq、vpc.subnet_count），
	// 不影响批次状态，独立于 EnrichmentFailedTypes。见 §8.2。
	EnrichmentWarnings []string `json:"enrichment_warnings,omitempty"`
	// EnrichmentStageError 记录增强阶段整体致命错误（如端口不可用），与 per-type 失败区分。
	// 该字段非空时驱动 partial 判定，但不递增 EnrichmentFailedCount。
	EnrichmentStageError string `json:"enrichment_stage_error,omitempty"`
	// EnrichmentStatus 统一压缩增强结果：success / warning / partial / failed，供前端和排障直接消费。
	EnrichmentStatus string `json:"enrichment_status,omitempty"`
	// WritebackFailedCount 记录 label 回写阶段失败次数，与 enrichment 失败区分。
	WritebackFailedCount int `json:"writeback_failed_count,omitempty"`
}

// AssetFullSyncResult 全量同步结果。
type AssetFullSyncResult struct {
	Resources  []domain.CloudResource `json:"resources"`
	Summary    CloudSyncSummary       `json:"summary"`
	EvidenceID string                 `json:"evidence_id"`
}

// AssetFullSyncEnrichmentResult 表示第二阶段增强输出，资源会在原切片上原地合并 labels。
// Enriched 是实际合并了 label 的资源子集（拷贝），供调用方带租约回写 labels；
// 未被原生 API 命中的资源不在其中，避免无意义回写。
type AssetFullSyncEnrichmentResult struct {
	Summary  CloudSyncSummary       `json:"summary"`
	Enriched []domain.CloudResource `json:"enriched,omitempty"`
}

func deriveEnrichmentStatus(summary *CloudSyncSummary) string {
	if summary == nil {
		return ""
	}
	if strings.TrimSpace(summary.EnrichmentStageError) != "" {
		if strings.Contains(strings.ToLower(summary.EnrichmentStageError), "lease lost") {
			return "failed"
		}
		return "failed"
	}
	if summary.WritebackFailedCount > 0 {
		return "failed"
	}
	if summary.MaxResourcesReached || summary.ProductNamesEmpty {
		return "partial"
	}
	if summary.EnrichmentFailedCount > 0 {
		if summary.EnrichmentFailedCount == len(summary.EnrichmentFailedTypes) {
			return "partial"
		}
		return "failed"
	}
	if len(summary.EnrichmentWarnings) > 0 {
		return "warning"
	}
	return "success"
}
