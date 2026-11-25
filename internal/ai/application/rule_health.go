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
// Limit 控制规则条数；实现方按预算尽量覆盖每条规则的全部关联实例，
// 统计取越限最严重的样本，并附带近端分钟级复核与零数据探测。
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
	// Timezone 是本地时间文本与 breach_by_hour 的统计时区；空值回退 UTC。
	Timezone string
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
	// ResourceLevel / DimensionName 是规则在 CES 里的资源层级与维度名。
	ResourceLevel string `json:"resource_level,omitempty"`
	DimensionName string `json:"dimension_name,omitempty"`
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
	PeakAt *int64 `json:"peak_at,omitempty"`
	// PeakAtLocal 是峰值时刻的证据时区文本。
	PeakAtLocal  string   `json:"peak_at_local,omitempty"`
	Last         *float64 `json:"last,omitempty"`
	BreachPoints int      `json:"breach_points,omitempty"`
	BreachRatio  *float64 `json:"breach_ratio,omitempty"`
	// BreachEpisodes / BreachDurationSeconds / LongestBreachSeconds 是 period 聚合口径下的
	// 连续越限片段数、累计越限时长与最长片段时长（breach_points × period 推算）。
	BreachEpisodes        int   `json:"breach_episodes,omitempty"`
	BreachDurationSeconds int64 `json:"breach_duration_seconds,omitempty"`
	LongestBreachSeconds  int64 `json:"longest_breach_seconds,omitempty"`
	// BreachByHour 是证据时区下的 24 小时越限点分布，index 0-23 对应本地 0-23 时。
	// BreachTopHours 是越限点最多的小时摘要（最多 3 个小时），用于识别周期性时段。
	BreachByHour   []int  `json:"breach_by_hour,omitempty"`
	BreachTopHours string `json:"breach_top_hours,omitempty"`
	// BreachDays 是出现越限的本地自然日数（同一自然日多次越限只计一次）。
	BreachDays int `json:"breach_days,omitempty"`
	// BreachFirstAt / BreachFirstAtLocal 是首个越限时刻（Unix 秒与证据时区文本）。
	BreachFirstAt      *int64 `json:"breach_first_at,omitempty"`
	BreachFirstAtLocal string `json:"breach_first_at_local,omitempty"`
	// ProbePointCount 是曲线为 0 点时对近 1 小时/60s 窗口探测得到的点数：
	// 大于 0 说明近期有数据（主窗口无点可能是保留期/窗口问题），为 0 说明近期也无数据
	// （事件型指标无事件、指标未上报或维度不匹配）。
	ProbePointCount    int   `json:"probe_point_count,omitempty"`
	ProbePeriod        int   `json:"probe_period,omitempty"`
	ProbeWindowSeconds int64 `json:"probe_window_seconds,omitempty"`
	// Recent* 是近端分钟级复核（默认近 24 小时、period=60）的越限统计，
	// 用于评估秒级规则窗口的触发频率；30 天小时级统计仍用于趋势与阈值分布。
	RecentWindowSeconds         int64    `json:"recent_window_seconds,omitempty"`
	RecentPeriod                int      `json:"recent_period,omitempty"`
	RecentPointCount            int      `json:"recent_point_count,omitempty"`
	RecentBreachPoints          int      `json:"recent_breach_points,omitempty"`
	RecentBreachRatio           *float64 `json:"recent_breach_ratio,omitempty"`
	RecentBreachEpisodes        int      `json:"recent_breach_episodes,omitempty"`
	RecentBreachDurationSeconds int64    `json:"recent_breach_duration_seconds,omitempty"`
	RecentLongestBreachSeconds  int64    `json:"recent_longest_breach_seconds,omitempty"`
	// ScopeNote 说明兜底抽样失败的具体原因（引擎/维度/发现条数），供模型和用户核对。
	ScopeNote string `json:"scope_note,omitempty"`
}

// RuleHealthResult 是批量规则体检结果。
type RuleHealthResult struct {
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
	WindowStart int64  `json:"window_start,omitempty"`
	WindowEnd   int64  `json:"window_end,omitempty"`
	Period      int    `json:"period,omitempty"`
	Aggregator  string `json:"aggregator,omitempty"`
	// Timezone 是本地时间文本与 breach_by_hour 的统计时区。
	Timezone     string                 `json:"timezone,omitempty"`
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
