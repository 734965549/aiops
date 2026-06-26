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
type CloudSyncSummary struct {
	Region                string   `json:"region"`
	ResourceGroupID       string   `json:"resource_group_id,omitempty"`
	ResourceGroupName     string   `json:"resource_group_name,omitempty"`
	CESTotal              int      `json:"ces_total,omitempty"`
	Discovered            int      `json:"discovered"`
	FailedScopes          []string `json:"failed_scopes,omitempty"`
	SuccessfulTypes       []string `json:"successful_types,omitempty"`
	UnknownNamespaceCount int      `json:"unknown_namespace_count,omitempty"`
	InvalidResourceCount  int      `json:"invalid_resource_count,omitempty"`
	ProductNamesEmpty     bool     `json:"product_names_empty,omitempty"`
}

// AssetFullSyncResult 全量同步结果。
type AssetFullSyncResult struct {
	Resources  []domain.CloudResource `json:"resources"`
	Summary    CloudSyncSummary       `json:"summary"`
	EvidenceID string                 `json:"evidence_id"`
}
