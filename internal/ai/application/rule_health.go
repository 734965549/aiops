package application

import "context"

// 批量规则体检的默认口径与边界（ops/ai-contract.md §10.12.1）。
const (
	chatToolAlertRulesHealth = "alert.rules.health"

	ruleHealthDefaultWindowSeconds = 30 * 24 * 3600
	ruleHealthDefaultPeriod        = 3600
	ruleHealthDefaultAggregator    = "max"
	ruleHealthDefaultLimit         = 10
	ruleHealthMaxLimit             = 20
	ruleHealthMaxRules             = 20
	ruleHealthMaxPoints            = 1440
)

// RuleHealthQuery 是一次按规则批量取历史曲线的请求（provider 无关）。
// Limit 控制规则条数；每条规则使用其第一个关联资源维度取一条样本序列。
type RuleHealthQuery struct {
	AccountID  string
	Region     string
	Namespace  string
	Keyword    string
	Limit      int
	From       int64
	To         int64
	Period     int
	Aggregator string
}

// RuleHealthRuleResult 是单条规则的曲线统计；status 非 ok 时统计字段为空。
// 规则关联多个实例时最多抽样三个实例，统计字段取自越限最严重的样本。
type RuleHealthRuleResult struct {
	RuleID      string   `json:"rule_id"`
	Name        string   `json:"name"`
	Metric      string   `json:"metric"`
	MetricLabel string   `json:"metric_label,omitempty"`
	Operator    string   `json:"operator,omitempty"`
	Threshold   *float64 `json:"threshold,omitempty"`
	// Unit 是本次曲线数据的单位；RuleUnit 是规则阈值配置的单位，二者不一致时需人工核对量纲。
	Unit          string `json:"unit,omitempty"`
	RuleUnit      string `json:"rule_unit,omitempty"`
	WindowSeconds int    `json:"window_seconds,omitempty"`
	ResourceCount int    `json:"resource_count,omitempty"`
	// ResourceGroupCount 大于 0 表示规则按资源分组关联，ResourceCount 是分组数而非实例数。
	ResourceGroupCount int               `json:"resource_group_count,omitempty"`
	Dimensions         map[string]string `json:"dimensions,omitempty"`
	// Sampled 表示规则关联多个资源/资源分组，本次只对其中部分实例取数。
	Sampled bool `json:"sampled,omitempty"`
	// ScopeSource 表示本次取数维度的来源：rule（规则直接绑定）/ resource_group（分组展开）/
	// resource_discovery（规则无绑定，按 namespace 发现实例兜底）。
	ScopeSource string `json:"scope_source,omitempty"`
	// CheckedResources 是本次实际取到曲线的实例数（最多 3 个）。
	CheckedResources int      `json:"checked_resources,omitempty"`
	Status           string   `json:"status"`
	Reason           string   `json:"reason,omitempty"`
	EvidenceID       string   `json:"evidence_id,omitempty"`
	PointCount       int      `json:"point_count"`
	First            *float64 `json:"first,omitempty"`
	Min              *float64 `json:"min,omitempty"`
	Avg              *float64 `json:"avg,omitempty"`
	P95              *float64 `json:"p95,omitempty"`
	Peak             *float64 `json:"peak,omitempty"`
	// PeakAt 是峰值时刻（Unix 秒），基于全量序列计算。
	PeakAt       *int64   `json:"peak_at,omitempty"`
	Last         *float64 `json:"last,omitempty"`
	BreachPoints int      `json:"breach_points,omitempty"`
	BreachRatio  *float64 `json:"breach_ratio,omitempty"`
	// BreachEpisodes / BreachDurationSeconds / LongestBreachSeconds 是 period 聚合口径下的
	// 连续越限片段数、累计越限时长与最长片段时长（breach_points × period 推算）。
	BreachEpisodes        int   `json:"breach_episodes,omitempty"`
	BreachDurationSeconds int64 `json:"breach_duration_seconds,omitempty"`
	LongestBreachSeconds  int64 `json:"longest_breach_seconds,omitempty"`
}

// RuleHealthResult 是批量规则体检结果。
type RuleHealthResult struct {
	Status       string                 `json:"status"`
	Reason       string                 `json:"reason,omitempty"`
	WindowStart  int64                  `json:"window_start,omitempty"`
	WindowEnd    int64                  `json:"window_end,omitempty"`
	Period       int                    `json:"period,omitempty"`
	Aggregator   string                 `json:"aggregator,omitempty"`
	RuleCount    int                    `json:"rule_count"`
	CheckedCount int                    `json:"checked_count"`
	Rules        []RuleHealthRuleResult `json:"rules,omitempty"`
}

// RuleHealthPort 是批量规则体检端口，由 infrastructure/observability 适配。
// 业务失败以 Result.Status 表达，端口不可用才返回 error。
type RuleHealthPort interface {
	Check(ctx context.Context, userID string, q RuleHealthQuery) (*RuleHealthResult, error)
}

// SetRuleHealth 注入批量规则体检能力；nil 表示关闭该工具。
func (s *ChatService) SetRuleHealth(port RuleHealthPort) {
	if s == nil {
		return
	}
	s.ruleHealth = port
}
