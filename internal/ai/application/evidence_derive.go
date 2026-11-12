package application

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	maxEvidencePayloadBytes = 32 * 1024
	defaultEvidencePeriod   = 300
	evidenceWindowBefore    = 30 * 60
	evidenceWindowAfter     = 10 * 60
	evidenceMinWindow       = 60 * 60
	// evidenceMaxWindow 略小于 QueryService 的 7 天上限；长告警优先保留最近证据，避免必然降级。
	evidenceMaxWindow = 6*24*60*60 + 12*60*60
)

// metricEvidenceMetaLabels 是告警定位元数据，不是 CES 指标维度，取数时必须剔除。
var metricEvidenceMetaLabels = map[string]struct{}{
	"account_id":        {},
	"region":            {},
	"project_id":        {},
	"record_id":         {},
	"alarm_id":          {},
	"rule_name":         {},
	"metric_name":       {},
	"resource_type":     {},
	"ces_namespace":     {},
	"cloud_resource_id": {},
}

// deriveMetricEvidenceQuery 从告警快照推导只读指标取数请求。
// 返回非空 skip 原因表示证据不可取，调用方应注入 skipped 状态而不是发起查询。
func deriveMetricEvidenceQuery(alert *AlertContext) (MetricEvidenceQuery, string) {
	if alert == nil {
		return MetricEvidenceQuery{}, "no_alert"
	}
	labels := alert.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	accountID := strings.TrimSpace(labels["account_id"])
	if accountID == "" {
		return MetricEvidenceQuery{}, "no_account"
	}
	region := strings.TrimSpace(labels["region"])
	if region == "" {
		return MetricEvidenceQuery{}, "no_region"
	}
	projectID := strings.TrimSpace(labels["project_id"])
	namespace := strings.TrimSpace(labels["ces_namespace"])
	if namespace == "" {
		namespace = evidenceAnnotationString(alert.Annotations, "ces_namespace")
	}
	if namespace == "" {
		return MetricEvidenceQuery{}, "no_namespace"
	}
	metric := strings.TrimSpace(labels["metric_name"])
	if metric == "" {
		metric = evidenceAnnotationString(alert.Annotations, "ces_metric_name")
	}
	if metric == "" {
		return MetricEvidenceQuery{}, "no_metric"
	}
	dimensions := evidenceDimensions(labels)
	if len(dimensions) == 0 {
		return MetricEvidenceQuery{}, "no_dimensions"
	}
	from, to := evidenceTimeWindow(alert)
	return MetricEvidenceQuery{
		AccountID:  accountID,
		Region:     region,
		ProjectID:  projectID,
		Namespace:  namespace,
		Metric:     metric,
		Dimensions: dimensions,
		From:       from,
		To:         to,
		Period:     defaultEvidencePeriod,
		Aggregator: evidenceAggregator(evidenceAnnotationString(alert.Annotations, "ces_filter")),
		Threshold:  evidenceThreshold(alert.Annotations),
		Operator:   evidenceAnnotationString(alert.Annotations, "ces_operator"),
	}, ""
}

func evidenceDimensions(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		name := strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if name == "" || value == "" {
			continue
		}
		if _, meta := metricEvidenceMetaLabels[name]; meta {
			continue
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func evidenceTimeWindow(alert *AlertContext) (int64, int64) {
	now := time.Now().Unix()
	from := now - evidenceMinWindow
	if alert.FirstSeenAt > 0 {
		from = alert.FirstSeenAt - evidenceWindowBefore
	}
	to := now
	if alert.LastSeenAt > 0 {
		to = alert.LastSeenAt + evidenceWindowAfter
	}
	if to <= from {
		to = from + evidenceMinWindow
	}
	if to > now {
		to = now
	}
	if from >= to {
		from = to - evidenceMinWindow
	}
	if to-from > evidenceMaxWindow {
		from = to - evidenceMaxWindow
	}
	if from < 0 {
		from = 0
	}
	return from, to
}

func evidenceAggregator(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "max":
		return "max"
	case "min":
		return "min"
	case "sum":
		return "sum"
	case "variance", "var":
		return "variance"
	default:
		return "average"
	}
}

func evidenceThreshold(annotations map[string]any) *float64 {
	raw := evidenceAnnotationString(annotations, "ces_threshold")
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &value
}

func evidenceAnnotationString(annotations map[string]any, key string) string {
	if annotations == nil {
		return ""
	}
	value, ok := annotations[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

// capEvidencePayload 保证 evidence 不超过 32KB：先丢弃数据点仅保留统计摘要，
// 仍然超限时丢弃整个 metrics，只保留状态与截断原因。
func capEvidencePayload(evidence *MetricEvidenceResult) *MetricEvidenceResult {
	if evidence == nil {
		return nil
	}
	if evidencePayloadSize(evidence) <= maxEvidencePayloadBytes {
		return evidence
	}
	trimmed := *evidence
	trimmed.Metrics = make([]MetricEvidence, len(evidence.Metrics))
	for i, metric := range evidence.Metrics {
		metric.Points = nil
		trimmed.Metrics[i] = metric
	}
	trimmed.Reason = "truncated"
	if evidencePayloadSize(&trimmed) <= maxEvidencePayloadBytes {
		return &trimmed
	}
	return &MetricEvidenceResult{Status: evidence.Status, Reason: "truncated"}
}

func evidencePayloadSize(evidence *MetricEvidenceResult) int {
	raw, err := json.Marshal(evidence)
	if err != nil {
		return 0
	}
	return len(raw)
}

func evidenceIDs(evidence *MetricEvidenceResult) []string {
	if evidence == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(evidence.Metrics))
	ids := make([]string, 0, len(evidence.Metrics))
	for _, metric := range evidence.Metrics {
		id := strings.TrimSpace(metric.EvidenceID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func evidenceReferences(evidence *MetricEvidenceResult) []string {
	ids := evidenceIDs(evidence)
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, "observability evidence "+id)
	}
	return out
}
