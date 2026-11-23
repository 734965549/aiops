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
	chatMetricLongWindowMinPeriodSec = 3600
	chatMetricMaxPoints              = 1440
	chatMetricMaxDimensions          = 20
	chatMetricMaxDimensionKeyRunes   = 64
	chatMetricMaxDimensionValueRunes = 128
	chatMetricMaxIdentityRunes       = 128

	chatRulesDefaultLimit      = 50
	chatRulesMaxLimit          = 50
	chatRulesMaxKeywordRune    = 100
	chatRuleDescriptionRune    = 300
	chatRuleMaxLabels          = 20
	chatRuleMaxResourceSamples = 3

	chatResourceListDefaultLimit   = 20
	chatResourceListMaxLimit       = 50
	chatResourceListMaxKeywordRune = 100
)

var (
	chatNamespacePattern    = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
	chatResourceTypePattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
)

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

type chatResourceListParams struct {
	AccountID    string `json:"account_id"`
	ResourceType string `json:"resource_type"`
	Region       string `json:"region"`
	Keyword      string `json:"keyword"`
	Limit        int    `json:"limit"`
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

// buildMetricEvidenceQuery 合并模型参数、告警推导与资源上下文推导，并执行 §10.12.1 的边界校验。
// 优先级：模型参数 > 告警推导 > 资源上下文推导。
// 返回 status 非空表示不应发起取数：skipped（上下文缺失）或 invalid_argument（参数非法）。
func buildMetricEvidenceQuery(alert *AlertContext, resource *AssetContext, raw map[string]any, now int64) (MetricEvidenceQuery, string, string) {
	alertQuery, alertSkip := MetricEvidenceQuery{}, "no_alert"
	if alert != nil {
		alertQuery, alertSkip = deriveMetricEvidenceQuery(alert)
	}
	resourceQuery, resourceSkip := deriveResourceMetricQuery(resource)

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
	if !hasParams && alertSkip != "" && resourceSkip != "" {
		reason := "no_alert"
		if alert != nil {
			reason = alertSkip
		} else if resource != nil {
			reason = resourceSkip
		}
		return MetricEvidenceQuery{}, "skipped", reason
	}

	query := MetricEvidenceQuery{}
	mergeMetricQueryFields(&query, resourceQuery)
	mergeMetricQueryFields(&query, alertQuery)
	if len(alertQuery.Dimensions) > 0 {
		query.ResourceID = alertQuery.ResourceID
		query.CloudResourceID = alertQuery.CloudResourceID
	} else {
		query.ResourceID = resourceQuery.ResourceID
		query.CloudResourceID = resourceQuery.CloudResourceID
	}
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
	if params.AccountID != "" || params.Region != "" || params.ProjectID != "" || len(params.Dimensions) > 0 {
		query.ResourceID = ""
		query.CloudResourceID = ""
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
	query.Metric = normalizeRDSMetricAlias(query.Namespace, query.Metric)

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
	if alertQuery.Metric != "" && strings.EqualFold(query.Metric, alertQuery.Metric) {
		query.Threshold = alertQuery.Threshold
		query.Operator = alertQuery.Operator
	} else {
		query.Threshold = nil
		query.Operator = ""
	}
	return query, "", ""
}

// mergeMetricQueryFields 把 src 的非空推导字段覆盖到 dst；用于资源上下文（低）与告警（中）的逐层回退。
func mergeMetricQueryFields(dst *MetricEvidenceQuery, src MetricEvidenceQuery) {
	if dst == nil {
		return
	}
	if src.AccountID != "" {
		dst.AccountID = src.AccountID
	}
	if src.Region != "" {
		dst.Region = src.Region
	}
	if src.ProjectID != "" {
		dst.ProjectID = src.ProjectID
	}
	if src.Namespace != "" {
		dst.Namespace = src.Namespace
	}
	if src.Metric != "" {
		dst.Metric = src.Metric
	}
	if len(src.Dimensions) > 0 {
		dst.Dimensions = src.Dimensions
	}
	if src.From > 0 {
		dst.From = src.From
	}
	if src.To > 0 {
		dst.To = src.To
	}
	if src.Period > 0 {
		dst.Period = src.Period
	}
	if src.Aggregator != "" {
		dst.Aggregator = src.Aggregator
	}
	if src.Threshold != nil {
		dst.Threshold = src.Threshold
	}
	if src.Operator != "" {
		dst.Operator = src.Operator
	}
	if src.ResourceID != "" {
		dst.ResourceID = src.ResourceID
	}
	if src.CloudResourceID != "" {
		dst.CloudResourceID = src.CloudResourceID
	}
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
		if period < chatMetricLongWindowMinPeriodSec {
			return "time range exceeds 7 days; period must be at least 3600"
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

// buildAlertRuleQuery 合并模型参数、告警标签与资源上下文；namespace 必须显式提供或可推导。
// 优先级：模型参数 > 告警标签 > 资源上下文。
func buildAlertRuleQuery(alert *AlertContext, resource *AssetContext, raw map[string]any) (AlertRuleQuery, string, string) {
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
	fallback := AlertRuleQuery{}
	if resource != nil {
		selector := deriveResourceCloudSelector(resource)
		fallback.AccountID = strings.TrimSpace(resource.AccountID)
		fallback.Region = strings.TrimSpace(resource.Region)
		fallback.Namespace = selector.Namespace
	}
	if alert != nil {
		labels := alert.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		if v := strings.TrimSpace(labels["account_id"]); v != "" {
			fallback.AccountID = v
		}
		if v := strings.TrimSpace(labels["region"]); v != "" {
			fallback.Region = v
		}
		namespace := strings.TrimSpace(labels["ces_namespace"])
		if namespace == "" {
			namespace = evidenceAnnotationString(alert.Annotations, "ces_namespace")
		}
		if namespace != "" {
			fallback.Namespace = namespace
		}
	}
	if query.AccountID == "" {
		query.AccountID = fallback.AccountID
	}
	if query.Region == "" {
		query.Region = fallback.Region
	}
	if query.Namespace == "" {
		query.Namespace = fallback.Namespace
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

// buildResourceListQuery 校验 resources.list 参数；account_id 必填，可回退当前资源上下文。
func buildResourceListQuery(resource *AssetContext, raw map[string]any) (ResourceQuery, string, string) {
	params := chatResourceListParams{}
	if _, err := decodeToolParams(raw, &params); err != nil {
		return ResourceQuery{}, chatObservationStatusInvalidArgument, "invalid resources.list params"
	}
	query := ResourceQuery{
		AccountID:    strings.TrimSpace(params.AccountID),
		ResourceType: strings.ToLower(strings.TrimSpace(params.ResourceType)),
		Region:       strings.TrimSpace(params.Region),
		Keyword:      strings.TrimSpace(params.Keyword),
		Limit:        params.Limit,
	}
	if resource != nil {
		if query.AccountID == "" {
			query.AccountID = strings.TrimSpace(resource.AccountID)
		}
		if query.Region == "" {
			query.Region = strings.TrimSpace(resource.Region)
		}
	}
	if query.AccountID == "" {
		return ResourceQuery{}, "skipped", "no_account"
	}
	if utf8.RuneCountInString(query.AccountID) > chatMetricMaxIdentityRunes ||
		utf8.RuneCountInString(query.Region) > chatMetricMaxIdentityRunes {
		return ResourceQuery{}, chatObservationStatusInvalidArgument, "account_id or region is too long"
	}
	if query.ResourceType != "" && !chatResourceTypePattern.MatchString(query.ResourceType) {
		return ResourceQuery{}, chatObservationStatusInvalidArgument, "invalid resource_type"
	}
	if utf8.RuneCountInString(query.Keyword) > chatResourceListMaxKeywordRune {
		return ResourceQuery{}, chatObservationStatusInvalidArgument, "keyword exceeds 100 characters"
	}
	if query.Limit <= 0 {
		query.Limit = chatResourceListDefaultLimit
	}
	if query.Limit > chatResourceListMaxLimit {
		return ResourceQuery{}, chatObservationStatusInvalidArgument, "limit must be at most 50"
	}
	return query, "", ""
}

// capAlertRulesForObservation 控制单次规则观测体积：描述/标签/资源样本截断。
func capAlertRulesForObservation(rules []AlertRuleSummary) []AlertRuleSummary {
	out := make([]AlertRuleSummary, 0, len(rules))
	for _, rule := range rules {
		rule.Description = truncateRunes(rule.Description, chatRuleDescriptionRune)
		rule.Labels = sanitizeRuleLabels(rule.Labels)
		rule.MetricLabel = chatMetricLabel(rule.Metric)
		rule.Unit = truncateRunes(strings.TrimSpace(rule.Unit), 32)
		rule.Resources = sanitizeRuleResources(rule.Resources)
		out = append(out, rule)
	}
	return out
}

// sanitizeRuleResources 限制资源维度样本数量与长度；空资源返回 nil。
func sanitizeRuleResources(resources []map[string]string) []map[string]string {
	if len(resources) == 0 {
		return nil
	}
	out := make([]map[string]string, 0, min(len(resources), chatRuleMaxResourceSamples))
	for _, dims := range resources {
		clean := make(map[string]string, len(dims))
		for key, value := range dims {
			key = truncateRunes(strings.TrimSpace(key), chatMetricMaxDimensionKeyRunes)
			value = truncateRunes(strings.TrimSpace(value), chatMetricMaxDimensionValueRunes)
			if key == "" || value == "" {
				continue
			}
			clean[key] = value
			if len(clean) >= chatMetricMaxDimensions {
				break
			}
		}
		if len(clean) > 0 {
			out = append(out, clean)
		}
		if len(out) >= chatRuleMaxResourceSamples {
			break
		}
	}
	if len(out) == 0 {
		return nil
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
