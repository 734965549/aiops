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
	AccountID  string
	Region     string
	ProjectID  string
	Namespace  string
	Metric     string
	Dimensions map[string]string
	From       int64 // Unix 秒
	To         int64 // Unix 秒
	Period     int   // 聚合周期（秒）
	Aggregator string
	// Threshold / Operator 来自告警结构化条件，用于统计破限点与生成阈值参考线；
	// 缺失时仍可取数，只是不计算 breach_points。
	Threshold *float64
	Operator  string
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

// MetricEvidenceResult 取数结果；status 非 ok 时 Metrics 为空且 Reason 说明原因。
type MetricEvidenceResult struct {
	Status  string           `json:"status"`
	Reason  string           `json:"reason,omitempty"`
	Metrics []MetricEvidence `json:"metrics,omitempty"`
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
