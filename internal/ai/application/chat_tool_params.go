package application

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// 工具参数边界（ops/ai-contract.md §10.12.1）。
const (
	chatObservationStatusInvalidArgument = "invalid_argument"

	chatMetricDefaultPeriod          = 300
	chatMetricMinPeriod              = 10
	chatMetricMaxWindowSec           = 7 * 24 * 3600
	chatMetricMaxLongWindowSec       = 30 * 24 * 3600
	chatMetricDailyPeriodSec         = 24 * 3600
	chatMetricMaxPoints              = 1440
	chatMetricMaxDimensions          = 20
	chatMetricMaxDimensionKeyRunes   = 64
	chatMetricMaxDimensionValueRunes = 128
	chatMetricMaxIdentityRunes       = 128

	chatRulesDefaultLimit   = 50
	chatRulesMaxLimit       = 50
	chatRulesMaxKeywordRune = 100
	chatRuleDescriptionRune = 300
	chatRuleMaxLabels       = 20
)

var chatNamespacePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

type chatMetricParams struct {
	AccountID  string            `json:"account_id"`
	Region     string            `json:"region"`
	ProjectID  string            `json:"project_id"`
	Namespace  string            `json:"namespace"`
	Metric     string            `json:"metric"`
	Dimensions map[string]string `json:"dimensions"`
	From       *int64            `json:"from"`
	To         *int64            `json:"to"`
	Period     int               `json:"period"`
	Aggregator string            `json:"aggregator"`
}

type chatAlertRulesParams struct {
	AccountID string `json:"account_id"`
	Region    string `json:"region"`
	Namespace string `json:"namespace"`
	Keyword   string `json:"keyword"`
	Limit     int    `json:"limit"`
}

func decodeToolParams(raw map[string]any, out any) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return true, err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return true, err
	}
	return true, nil
}

// buildMetricEvidenceQuery 合并模型参数与告警推导值，并执行 §10.12.1 的边界校验。
// 返回 status 非空表示不应发起取数：skipped（上下文缺失）或 invalid_argument（参数非法）。
func buildMetricEvidenceQuery(alert *AlertContext, raw map[string]any, now int64) (MetricEvidenceQuery, string, string) {
	var derived MetricEvidenceQuery
	derivedSkip := "no_alert"
	if alert != nil {
		derived, derivedSkip = deriveMetricEvidenceQuery(alert)
	}

	params := chatMetricParams{}
	hasParams, err := decodeToolParams(raw, &params)
	if err != nil {
		return MetricEvidenceQuery{}, chatObservationStatusInvalidArgument, "invalid metrics.query params"
	}
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.Region = strings.TrimSpace(params.Region)
	params.ProjectID = strings.TrimSpace(params.ProjectID)
	params.Namespace = strings.TrimSpace(params.Namespace)
	params.Metric = strings.TrimSpace(params.Metric)
	params.Aggregator = strings.TrimSpace(params.Aggregator)
	if !hasParams && derivedSkip != "" {
		return MetricEvidenceQuery{}, "skipped", derivedSkip
	}

	query := derived
	if params.AccountID != "" {
		query.AccountID = params.AccountID
	}
	if params.Region != "" {
		query.Region = params.Region
	}
	if params.ProjectID != "" {
		query.ProjectID = params.ProjectID
	}
	if params.Namespace != "" {
		query.Namespace = params.Namespace
	}
	if params.Metric != "" {
		query.Metric = params.Metric
	}
	if len(params.Dimensions) > 0 {
		query.Dimensions = params.Dimensions
	}
	if params.Aggregator != "" {
		query.Aggregator = params.Aggregator
	}
	if params.Period != 0 {
		query.Period = params.Period
	}
	if (params.From == nil) != (params.To == nil) {
		return MetricEvidenceQuery{}, chatObservationStatusInvalidArgument, "from and to must be provided together"
	}
	if params.From != nil && params.To != nil {
		query.From, query.To = *params.From, *params.To
	}
	if query.From <= 0 || query.To <= 0 {
		query.From, query.To = now-evidenceMinWindow, now
	}
	if query.Period == 0 {
		query.Period = chatMetricDefaultPeriod
	}

	query.AccountID = strings.TrimSpace(query.AccountID)
	query.Region = strings.TrimSpace(query.Region)
	query.ProjectID = strings.TrimSpace(query.ProjectID)
	query.Namespace = strings.TrimSpace(query.Namespace)
	query.Metric = strings.TrimSpace(query.Metric)
	query.Aggregator = strings.TrimSpace(query.Aggregator)
	switch {
	case query.AccountID == "":
		return MetricEvidenceQuery{}, "skipped", "no_account"
	case query.Region == "":
		return MetricEvidenceQuery{}, "skipped", "no_region"
	case query.Namespace == "":
		return MetricEvidenceQuery{}, "skipped", "no_namespace"
	case query.Metric == "":
		return MetricEvidenceQuery{}, "skipped", "no_metric"
	case len(query.Dimensions) == 0:
		return MetricEvidenceQuery{}, "skipped", "no_dimensions"
	}
	if reason := validateMetricToolQuery(query); reason != "" {
		return MetricEvidenceQuery{}, chatObservationStatusInvalidArgument, reason
	}

	// 阈值只在指标与告警推导指标一致时继承，避免模型换指标后仍套用旧阈值。
	if strings.EqualFold(query.Metric, derived.Metric) {
		query.Threshold = derived.Threshold
		query.Operator = derived.Operator
	} else {
		query.Threshold = nil
		query.Operator = ""
	}
	return query, "", ""
}

func validateMetricToolQuery(q MetricEvidenceQuery) string {
	if q.From <= 0 || q.To <= 0 || q.From >= q.To {
		return "invalid time range"
	}
	if utf8.RuneCountInString(q.AccountID) > chatMetricMaxIdentityRunes ||
		utf8.RuneCountInString(q.Region) > chatMetricMaxIdentityRunes ||
		utf8.RuneCountInString(q.ProjectID) > chatMetricMaxIdentityRunes ||
		utf8.RuneCountInString(q.Metric) > chatMetricMaxIdentityRunes {
		return "metric identity fields are too long"
	}
	if !chatNamespacePattern.MatchString(strings.TrimSpace(q.Namespace)) {
		return "invalid namespace"
	}
	period := q.Period
	if period < 0 {
		return "period below minimum 10 seconds"
	}
	if period == 0 {
		period = chatMetricDefaultPeriod
	}
	if period < chatMetricMinPeriod {
		return "period below minimum 10 seconds"
	}
	window := q.To - q.From
	if window > chatMetricMaxWindowSec {
		if period < chatMetricDailyPeriodSec {
			return "time range exceeds 7 days; period must be at least 86400"
		}
		if window > chatMetricMaxLongWindowSec {
			return "time range exceeds maximum 30 days"
		}
	}
	if window/int64(period)+1 > chatMetricMaxPoints {
		return "too many data points"
	}
	if len(q.Dimensions) > chatMetricMaxDimensions {
		return "too many dimensions"
	}
	for key, value := range q.Dimensions {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return "dimensions contain empty key or value"
		}
		if utf8.RuneCountInString(key) > chatMetricMaxDimensionKeyRunes ||
			utf8.RuneCountInString(value) > chatMetricMaxDimensionValueRunes {
			return "dimension key or value is too long"
		}
	}
	return ""
}

// buildAlertRuleQuery 合并模型参数与告警标签；namespace 必须显式提供或可从告警推导。
func buildAlertRuleQuery(alert *AlertContext, raw map[string]any) (AlertRuleQuery, string, string) {
	params := chatAlertRulesParams{}
	if _, err := decodeToolParams(raw, &params); err != nil {
		return AlertRuleQuery{}, chatObservationStatusInvalidArgument, "invalid alert.rules.list params"
	}
	query := AlertRuleQuery{
		AccountID: strings.TrimSpace(params.AccountID),
		Region:    strings.TrimSpace(params.Region),
		Namespace: strings.TrimSpace(params.Namespace),
		Keyword:   strings.TrimSpace(params.Keyword),
		Limit:     params.Limit,
	}
	if alert != nil {
		labels := alert.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		if query.AccountID == "" {
			query.AccountID = strings.TrimSpace(labels["account_id"])
		}
		if query.Region == "" {
			query.Region = strings.TrimSpace(labels["region"])
		}
		if query.Namespace == "" {
			query.Namespace = strings.TrimSpace(labels["ces_namespace"])
		}
		if query.Namespace == "" {
			query.Namespace = evidenceAnnotationString(alert.Annotations, "ces_namespace")
		}
	}
	if query.AccountID == "" {
		return AlertRuleQuery{}, "skipped", "no_account"
	}
	if query.Namespace == "" {
		return AlertRuleQuery{}, "skipped", "no_namespace"
	}
	if utf8.RuneCountInString(query.AccountID) > chatMetricMaxIdentityRunes ||
		utf8.RuneCountInString(query.Region) > chatMetricMaxIdentityRunes {
		return AlertRuleQuery{}, chatObservationStatusInvalidArgument, "account_id or region is too long"
	}
	if !chatNamespacePattern.MatchString(query.Namespace) {
		return AlertRuleQuery{}, chatObservationStatusInvalidArgument, "invalid namespace"
	}
	if utf8.RuneCountInString(query.Keyword) > chatRulesMaxKeywordRune {
		return AlertRuleQuery{}, chatObservationStatusInvalidArgument, "keyword exceeds 100 characters"
	}
	if query.Limit <= 0 {
		query.Limit = chatRulesDefaultLimit
	}
	if query.Limit > chatRulesMaxLimit {
		return AlertRuleQuery{}, chatObservationStatusInvalidArgument, "limit must be at most 50"
	}
	return query, "", ""
}

// capAlertRulesForObservation 控制单次规则观测体积：描述截断、标签数限制。
func capAlertRulesForObservation(rules []AlertRuleSummary) []AlertRuleSummary {
	out := make([]AlertRuleSummary, 0, len(rules))
	for _, rule := range rules {
		rule.Description = truncateRunes(rule.Description, chatRuleDescriptionRune)
		rule.Labels = sanitizeRuleLabels(rule.Labels)
		out = append(out, rule)
	}
	return out
}

func sanitizeRuleLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, min(len(labels), chatRuleMaxLabels))
	for key, value := range labels {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
		if len(out) >= chatRuleMaxLabels {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
