package application

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/734965549/aiops/internal/ai/toolgateway"
)

func toolParamsFromJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	return out
}

func TestBuildMetricEvidenceQueryUsesModelParams(t *testing.T) {
	alert := evidenceTestAlert()
	params := toolParamsFromJSON(t, `{
		"namespace":"SYS.RDS",
		"metric":"mem_util",
		"dimensions":{"instance_id":"i-9"},
		"from":1700000000,
		"to":1700003600,
		"period":60
	}`)
	query, status, reason := buildMetricEvidenceQuery(alert, nil, params, 1700005000)
	if status != "" || reason != "" {
		t.Fatalf("unexpected status=%q reason=%q", status, reason)
	}
	if query.AccountID != "acc-1" || query.Region != "cn-north-4" {
		t.Fatalf("alert identity fallback missing: %+v", query)
	}
	if query.Namespace != "SYS.RDS" || query.Metric != "mem_util" {
		t.Fatalf("model params not applied: %+v", query)
	}
	if len(query.Dimensions) != 1 || query.Dimensions["instance_id"] != "i-9" {
		t.Fatalf("dimensions = %+v", query.Dimensions)
	}
	if query.From != 1700000000 || query.To != 1700003600 || query.Period != 60 {
		t.Fatalf("time params = %+v", query)
	}
	// 指标被模型覆盖后不再继承告警阈值，避免破限点误判。
	if query.Threshold != nil || query.Operator != "" {
		t.Fatalf("threshold must be dropped when metric changes: %+v", query)
	}
}

func TestBuildMetricEvidenceQueryFallsBackToAlert(t *testing.T) {
	query, status, reason := buildMetricEvidenceQuery(evidenceTestAlert(), nil, nil, 1700005000)
	if status != "" || reason != "" {
		t.Fatalf("unexpected status=%q reason=%q", status, reason)
	}
	if query.Namespace != "SYS.ECS" || query.Metric != "cpu_util" || query.Period != defaultEvidencePeriod {
		t.Fatalf("derived query = %+v", query)
	}
	if query.Threshold == nil || *query.Threshold != 90 || query.Operator != ">" {
		t.Fatalf("threshold fallback = %+v", query)
	}
	if len(query.Dimensions) != 1 || query.Dimensions["instance_id"] != "i-1" {
		t.Fatalf("dimensions = %+v", query.Dimensions)
	}
}

func TestBuildMetricEvidenceQueryWindowLimits(t *testing.T) {
	alert := evidenceTestAlert()
	from := int64(1700000000)
	cases := []struct {
		name   string
		params map[string]any
		status string
	}{
		{
			name: "8 days with minute period is rejected",
			params: map[string]any{
				"from": from, "to": from + 8*24*3600, "period": 300,
			},
			status: chatObservationStatusInvalidArgument,
		},
		{
			name: "8 days with daily period is allowed",
			params: map[string]any{
				"from": from, "to": from + 8*24*3600, "period": 86400,
			},
			status: "",
		},
		{
			name: "30 days with daily period is allowed",
			params: map[string]any{
				"from": from, "to": from + 30*24*3600, "period": 86400,
			},
			status: "",
		},
		{
			name: "31 days with daily period is rejected",
			params: map[string]any{
				"from": from, "to": from + 31*24*3600, "period": 86400,
			},
			status: chatObservationStatusInvalidArgument,
		},
		{
			name: "too many points is rejected",
			params: map[string]any{
				"from": from, "to": from + 7*24*3600, "period": 10,
			},
			status: chatObservationStatusInvalidArgument,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, status, _ := buildMetricEvidenceQuery(alert, nil, tc.params, from)
			if status != tc.status {
				t.Fatalf("status = %q, want %q", status, tc.status)
			}
		})
	}
}

func TestBuildMetricEvidenceQueryValidatesShape(t *testing.T) {
	alert := evidenceTestAlert()
	badNamespace := map[string]any{"namespace": "SYS RDS", "metric": "cpu_util"}
	if _, status, _ := buildMetricEvidenceQuery(alert, nil, badNamespace, 1700005000); status != chatObservationStatusInvalidArgument {
		t.Fatalf("invalid namespace accepted: %q", status)
	}
	tooManyDims := map[string]any{"dimensions": map[string]string{}}
	for i := 0; i < chatMetricMaxDimensions+1; i++ {
		tooManyDims["dimensions"].(map[string]string)["k"+strings.Repeat("x", i)] = "v"
	}
	if _, status, _ := buildMetricEvidenceQuery(alert, nil, tooManyDims, 1700005000); status != chatObservationStatusInvalidArgument {
		t.Fatalf("too many dimensions accepted: %q", status)
	}
	halfRange := map[string]any{"from": int64(1700000000)}
	if _, status, _ := buildMetricEvidenceQuery(alert, nil, halfRange, 1700005000); status != chatObservationStatusInvalidArgument {
		t.Fatalf("half time range accepted: %q", status)
	}
	negativePeriod := map[string]any{"period": -5}
	if _, status, _ := buildMetricEvidenceQuery(alert, nil, negativePeriod, 1700005000); status != chatObservationStatusInvalidArgument {
		t.Fatalf("negative period accepted: %q", status)
	}
}

func TestBuildMetricEvidenceQuerySkipsMissingContext(t *testing.T) {
	cases := []struct {
		name   string
		alert  *AlertContext
		params map[string]any
		reason string
	}{
		{name: "no alert no params", alert: nil, params: nil, reason: "no_alert"},
		{
			name:  "no account",
			alert: nil,
			params: map[string]any{
				"namespace": "SYS.RDS", "metric": "cpu_util",
				"dimensions": map[string]string{"instance_id": "i-1"},
			},
			reason: "no_account",
		},
		{
			name:  "no dimensions",
			alert: nil,
			params: map[string]any{
				"account_id": "acc-1", "region": "cn-north-4", "namespace": "SYS.RDS", "metric": "cpu_util",
			},
			reason: "no_dimensions",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, status, reason := buildMetricEvidenceQuery(tc.alert, nil, tc.params, 1700005000)
			if status != "skipped" || reason != tc.reason {
				t.Fatalf("status=%q reason=%q, want skipped/%q", status, reason, tc.reason)
			}
		})
	}
}

func TestBuildAlertRuleQueryUsesParamsAndFallback(t *testing.T) {
	query, status, reason := buildAlertRuleQuery(evidenceTestAlert(), nil, map[string]any{
		"namespace": "SYS.RDS", "keyword": "cpu", "limit": 10,
	})
	if status != "" || reason != "" {
		t.Fatalf("unexpected status=%q reason=%q", status, reason)
	}
	if query.AccountID != "acc-1" || query.Region != "cn-north-4" {
		t.Fatalf("account fallback = %+v", query)
	}
	if query.Namespace != "SYS.RDS" || query.Keyword != "cpu" || query.Limit != 10 {
		t.Fatalf("params not applied: %+v", query)
	}

	fallback, status, _ := buildAlertRuleQuery(evidenceTestAlert(), nil, nil)
	if status != "" || fallback.Namespace != "SYS.ECS" || fallback.Limit != chatRulesDefaultLimit {
		t.Fatalf("alert fallback = %+v status=%q", fallback, status)
	}
}

func TestBuildAlertRuleQueryValidates(t *testing.T) {
	alert := evidenceTestAlert()
	cases := []struct {
		name   string
		params map[string]any
		status string
		reason string
	}{
		{name: "no namespace", params: nil, status: "skipped", reason: "no_namespace"},
		{name: "limit too large", params: map[string]any{"namespace": "SYS.RDS", "limit": 51}, status: chatObservationStatusInvalidArgument},
		{name: "bad namespace", params: map[string]any{"namespace": "SYS RDS"}, status: chatObservationStatusInvalidArgument},
		{name: "long keyword", params: map[string]any{"namespace": "SYS.RDS", "keyword": strings.Repeat("x", chatRulesMaxKeywordRune+1)}, status: chatObservationStatusInvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := alert
			if tc.name == "no namespace" {
				a = &AlertContext{Labels: map[string]string{"account_id": "acc-1"}}
			}
			_, status, reason := buildAlertRuleQuery(a, nil, tc.params)
			if status != tc.status || (tc.reason != "" && reason != tc.reason) {
				t.Fatalf("status=%q reason=%q, want %q/%q", status, reason, tc.status, tc.reason)
			}
		})
	}
}

func TestCapAlertRulesForObservation(t *testing.T) {
	labels := map[string]string{}
	for i := 0; i < chatRuleMaxLabels+5; i++ {
		labels["k"+strings.Repeat("x", i+1)] = "v"
	}
	rules := capAlertRulesForObservation([]AlertRuleSummary{{
		RuleID: "rule-1", Metric: "rds002_mem_util",
		Description: strings.Repeat("描", chatRuleDescriptionRune+10), Labels: labels,
	}})
	if len(rules) != 1 {
		t.Fatalf("rules = %+v", rules)
	}
	if got := len([]rune(rules[0].Description)); got != chatRuleDescriptionRune {
		t.Fatalf("description runes = %d", got)
	}
	if len(rules[0].Labels) != chatRuleMaxLabels {
		t.Fatalf("labels = %d", len(rules[0].Labels))
	}
	if rules[0].MetricLabel != "内存使用率" {
		t.Fatalf("metric_label = %q", rules[0].MetricLabel)
	}
}

func TestCanonicalAlertRulesToolCodeRegistered(t *testing.T) {
	if _, ok := toolgateway.LookupBuiltinChatTool(chatToolAlertRulesList); !ok {
		t.Fatalf("%s must be registered as a builtin chat tool", chatToolAlertRulesList)
	}
	for _, code := range []string{chatToolAccountsList, chatToolResourceAnalyze, chatToolResourcesList} {
		if _, ok := toolgateway.LookupBuiltinChatTool(code); !ok {
			t.Fatalf("%s must be registered as a builtin chat tool", code)
		}
	}
	if _, ok := toolgateway.LookupBuiltinChatTool(chatToolExecutionPropose); ok {
		t.Fatalf("%s must not be registered as a builtin read-only tool", chatToolExecutionPropose)
	}
}

func resourceContextForTest() *AssetContext {
	return &AssetContext{
		ResourceID: "res-1", Name: "orders-rds",
		CloudResourceType: "rds", CloudResourceID: "cluster-1",
		Region: "cn-north-4", AccountID: "acc-1",
		Labels: map[string]string{"namespace": "SYS.RDS", "dim_name": "rds_cluster_id"},
	}
}

func TestBuildMetricEvidenceQueryFallsBackToResourceContext(t *testing.T) {
	query, status, reason := buildMetricEvidenceQuery(nil, resourceContextForTest(), map[string]any{
		"metric": "cpu_util",
	}, 1700005000)
	if status != "" || reason != "" {
		t.Fatalf("unexpected status=%q reason=%q", status, reason)
	}
	if query.AccountID != "acc-1" || query.Region != "cn-north-4" || query.Namespace != "SYS.RDS" {
		t.Fatalf("resource identity fallback = %+v", query)
	}
	if query.Dimensions["rds_cluster_id"] != "cluster-1" {
		t.Fatalf("dimensions = %+v", query.Dimensions)
	}
	if query.Metric != "cpu_util" || query.Period != defaultEvidencePeriod {
		t.Fatalf("query = %+v", query)
	}
	if query.To-query.From != evidenceMinWindow {
		t.Fatalf("resource window = %+v", query)
	}
	if query.Threshold != nil || query.Operator != "" {
		t.Fatalf("resource fallback must not invent thresholds: %+v", query)
	}
}

func TestBuildMetricEvidenceQueryPrefersAlertOverResource(t *testing.T) {
	query, status, reason := buildMetricEvidenceQuery(evidenceTestAlert(), resourceContextForTest(), nil, 1700005000)
	if status != "" || reason != "" {
		t.Fatalf("unexpected status=%q reason=%q", status, reason)
	}
	// 告警上下文优先：namespace/metric/dimensions 均来自告警，资源只作为补充来源。
	if query.Namespace != "SYS.ECS" || query.Metric != "cpu_util" {
		t.Fatalf("alert context must win: %+v", query)
	}
	if query.Dimensions["instance_id"] != "i-1" {
		t.Fatalf("dimensions = %+v", query.Dimensions)
	}
	if query.Threshold == nil || *query.Threshold != 90 {
		t.Fatalf("alert threshold fallback missing: %+v", query)
	}
}

func TestBuildMetricEvidenceQueryResourceSkipReasons(t *testing.T) {
	noAccount := resourceContextForTest()
	noAccount.AccountID = ""
	_, status, reason := buildMetricEvidenceQuery(nil, noAccount, nil, 1700005000)
	if status != "skipped" || reason != "no_account" {
		t.Fatalf("status=%q reason=%q", status, reason)
	}

	noDimensions := resourceContextForTest()
	noDimensions.CloudResourceID = ""
	noDimensions.Labels = map[string]string{"namespace": "SYS.RDS"}
	_, status, reason = buildMetricEvidenceQuery(nil, noDimensions, nil, 1700005000)
	if status != "skipped" || reason != "no_dimensions" {
		t.Fatalf("status=%q reason=%q", status, reason)
	}

	// 资源只提供身份信息时，指标名仍缺失，返回 no_metric 供模型补参数。
	_, status, reason = buildMetricEvidenceQuery(nil, resourceContextForTest(), nil, 1700005000)
	if status != "skipped" || reason != "no_metric" {
		t.Fatalf("status=%q reason=%q", status, reason)
	}
}

func TestBuildAlertRuleQueryFallsBackToResource(t *testing.T) {
	query, status, reason := buildAlertRuleQuery(nil, resourceContextForTest(), nil)
	if status != "" || reason != "" {
		t.Fatalf("unexpected status=%q reason=%q", status, reason)
	}
	if query.AccountID != "acc-1" || query.Region != "cn-north-4" || query.Namespace != "SYS.RDS" {
		t.Fatalf("resource fallback = %+v", query)
	}
	if query.Limit != chatRulesDefaultLimit {
		t.Fatalf("limit = %d", query.Limit)
	}
}

func TestBuildResourceListQuery(t *testing.T) {
	query, status, reason := buildResourceListQuery(nil, map[string]any{
		"account_id": "acc-1", "resource_type": "RDS", "keyword": "orders",
	})
	if status != "" || reason != "" {
		t.Fatalf("unexpected status=%q reason=%q", status, reason)
	}
	if query.AccountID != "acc-1" || query.ResourceType != "rds" || query.Keyword != "orders" {
		t.Fatalf("query = %+v", query)
	}
	if query.Limit != chatResourceListDefaultLimit {
		t.Fatalf("limit = %d", query.Limit)
	}

	fallback, status, _ := buildResourceListQuery(resourceContextForTest(), map[string]any{
		"resource_type": "rds",
	})
	if status != "" || fallback.AccountID != "acc-1" || fallback.Region != "cn-north-4" {
		t.Fatalf("resource fallback = %+v status=%q", fallback, status)
	}

	if _, status, reason = buildResourceListQuery(nil, map[string]any{"resource_type": "rds"}); status != "skipped" || reason != "no_account" {
		t.Fatalf("status=%q reason=%q", status, reason)
	}
	if _, status, _ = buildResourceListQuery(nil, map[string]any{"account_id": "acc-1", "limit": chatResourceListMaxLimit + 1}); status != chatObservationStatusInvalidArgument {
		t.Fatalf("limit over max accepted: %q", status)
	}
	if _, status, _ = buildResourceListQuery(nil, map[string]any{"account_id": "acc-1", "resource_type": "RDS; DROP"}); status != chatObservationStatusInvalidArgument {
		t.Fatalf("invalid resource_type accepted: %q", status)
	}
	if _, status, _ = buildResourceListQuery(nil, map[string]any{"account_id": "acc-1", "keyword": strings.Repeat("x", chatResourceListMaxKeywordRune+1)}); status != chatObservationStatusInvalidArgument {
		t.Fatalf("long keyword accepted: %q", status)
	}
}

func TestDeriveResourceCloudSelector(t *testing.T) {
	selector := deriveResourceCloudSelector(resourceContextForTest())
	if selector.Namespace != "SYS.RDS" || selector.DimensionName != "rds_cluster_id" || selector.DimensionValue != "cluster-1" {
		t.Fatalf("selector = %+v", selector)
	}

	ecs := &AssetContext{CloudResourceType: "ecs", CloudResourceID: "i-1"}
	selector = deriveResourceCloudSelector(ecs)
	if selector.Namespace != "SYS.ECS" || selector.DimensionName != "instance_id" || selector.DimensionValue != "i-1" {
		t.Fatalf("ecs selector = %+v", selector)
	}

	eip := &AssetContext{
		CloudResourceType: "eip", CloudResourceID: "eip-1",
		Labels: map[string]string{"namespace": "SYS.VPC", "dim_name": "publicip_id"},
	}
	selector = deriveResourceCloudSelector(eip)
	if selector.Namespace != "SYS.VPC" || selector.DimensionName != "publicip_id" || selector.DimensionValue != "eip-1" {
		t.Fatalf("eip selector = %+v", selector)
	}

	// 云同步写入的 instance_id 标签优先于 cloud_resource_id。
	labeled := &AssetContext{
		CloudResourceType: "ecs", CloudResourceID: "provider-ref",
		Labels: map[string]string{"instance_id": "i-9"},
	}
	selector = deriveResourceCloudSelector(labeled)
	if selector.DimensionValue != "i-9" {
		t.Fatalf("labeled selector = %+v", selector)
	}

	// RDS for PostgreSQL 单机版实际维度为 rds_instance_id：以实际维度为准。
	pg := &AssetContext{
		CloudResourceType: "rds", CloudResourceID: "pg-instance-1",
		Labels: map[string]string{"namespace": "SYS.RDS", "dim_name": "rds_instance_id"},
	}
	selector = deriveResourceCloudSelector(pg)
	if selector.DimensionName != "rds_instance_id" || selector.DimensionValue != "pg-instance-1" {
		t.Fatalf("pg selector = %+v", selector)
	}

	// SYS.VPC 实际维度含多个候选维度时，云类型映射（eip -> publicip_id）优先。
	vpc := &AssetContext{
		CloudResourceType: "eip", CloudResourceID: "eip-1",
		Labels: map[string]string{"namespace": "SYS.VPC", "dim_name": "vpc_id,publicip_id"},
	}
	selector = deriveResourceCloudSelector(vpc)
	if selector.DimensionName != "publicip_id" {
		t.Fatalf("vpc selector = %+v", selector)
	}
}
