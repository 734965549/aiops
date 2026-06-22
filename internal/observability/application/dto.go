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
