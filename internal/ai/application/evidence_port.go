package application

import "context"

// 指标证据状态枚举，随 payload.evidence.status 返回给 provider。
const (
	EvidenceStatusOK               = "ok"
	EvidenceStatusError            = "error"
	EvidenceStatusSkipped          = "skipped"
	EvidenceStatusPermissionDenied = "permission_denied"
	EvidenceStatusUnsupported      = "unsupported"
)

// MetricEvidenceQuery 一次只读指标取数请求（provider 无关）。
type MetricEvidenceQuery struct {
	AccountID       string            `json:"account_id"`
	Region          string            `json:"region"`
	ProjectID       string            `json:"project_id,omitempty"`
	Namespace       string            `json:"namespace"`
	Metric          string            `json:"metric"`
	Dimensions      map[string]string `json:"dimensions"`
	From            int64             `json:"from"` // Unix 秒
	To              int64             `json:"to"`   // Unix 秒
	Period          int               `json:"period"`
	Aggregator      string            `json:"aggregator"`
	ResourceID      string            `json:"-"`
	CloudResourceID string            `json:"-"`
	// Threshold / Operator 来自告警结构化条件，用于统计破限点与生成阈值参考线；
	// 缺失时仍可取数，只是不计算 breach_points。
	Threshold *float64 `json:"threshold,omitempty"`
	Operator  string   `json:"operator,omitempty"`
}

// MetricEvidence 单条指标序列的证据摘要。
type MetricEvidence struct {
	Namespace    string            `json:"namespace"`
	Metric       string            `json:"metric"`
	Unit         string            `json:"unit"`
	Threshold    *float64          `json:"threshold,omitempty"`
	Operator     string            `json:"operator,omitempty"`
	First        *float64          `json:"first,omitempty"`
	Peak         *float64          `json:"peak,omitempty"`
	Last         *float64          `json:"last,omitempty"`
	BreachPoints int               `json:"breach_points,omitempty"`
	Points       [][2]float64      `json:"points,omitempty"` // [ts 秒, 值]
	Labels       map[string]string `json:"labels,omitempty"`
	EvidenceID   string            `json:"evidence_id,omitempty"`
}

// MetricEvidenceSampling 描述定稿前的降采样口径。
type MetricEvidenceSampling struct {
	Strategy           string `json:"strategy"`
	MaxPointsPerSeries int    `json:"max_points_per_series"`
}

// MetricEvidenceResult 取数结果；status 非 ok 时 Metrics 为空且 Reason 说明原因。
type MetricEvidenceResult struct {
	Status          string                  `json:"status"`
	Reason          string                  `json:"reason,omitempty"`
	EvidenceID      string                  `json:"evidence_id,omitempty"`
	SchemaVersion   string                  `json:"schema_version,omitempty"`
	Provider        string                  `json:"provider,omitempty"`
	TraceID         string                  `json:"trace_id,omitempty"`
	ProjectID       string                  `json:"project_id,omitempty"`
	ResourceID      string                  `json:"resource_id,omitempty"`
	CloudResourceID string                  `json:"cloud_resource_id,omitempty"`
	WindowStart     int64                   `json:"window_start,omitempty"`
	WindowEnd       int64                   `json:"window_end,omitempty"`
	CollectedAt     int64                   `json:"collected_at,omitempty"`
	Period          int                     `json:"period,omitempty"`
	Aggregator      string                  `json:"aggregator,omitempty"`
	OriginalCount   int                     `json:"original_count"`
	RetainedCount   int                     `json:"retained_count"`
	Truncated       bool                    `json:"truncated,omitempty"`
	Sampling        *MetricEvidenceSampling `json:"sampling,omitempty"`
	Metrics         []MetricEvidence        `json:"metrics,omitempty"`
}

// MetricEvidencePort 只读观测取数端口，由 infrastructure/observability 适配。
// 实现方负责账号解析、数据权限、超时与降级：业务失败以 Result.Status 表达，
// 只有端口本身不可用或非法入参才返回 error。
type MetricEvidencePort interface {
	Collect(ctx context.Context, userID string, q MetricEvidenceQuery) (*MetricEvidenceResult, error)
}

// AccountRef 权限校验所需账号摘要（不含凭据）。
type AccountRef struct {
	AccountID string
	OwnerTeam string
	Regions   []string
}

// AccountAccessAuthorizer 校验用户对账号的数据范围。
// 不通过时返回 PERMISSION_DENIED。
type AccountAccessAuthorizer interface {
	Authorize(ctx context.Context, userID string, ref AccountRef) error
}
