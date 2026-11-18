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
	query, status, reason := buildMetricEvidenceQuery(alert, params, 1700005000)
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
	query, status, reason := buildMetricEvidenceQuery(evidenceTestAlert(), nil, 1700005000)
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
			_, status, _ := buildMetricEvidenceQuery(alert, tc.params, from)
			if status != tc.status {
				t.Fatalf("status = %q, want %q", status, tc.status)
			}
		})
	}
}

func TestBuildMetricEvidenceQueryValidatesShape(t *testing.T) {
	alert := evidenceTestAlert()
	badNamespace := map[string]any{"namespace": "SYS RDS", "metric": "cpu_util"}
	if _, status, _ := buildMetricEvidenceQuery(alert, badNamespace, 1700005000); status != chatObservationStatusInvalidArgument {
		t.Fatalf("invalid namespace accepted: %q", status)
	}
	tooManyDims := map[string]any{"dimensions": map[string]string{}}
	for i := 0; i < chatMetricMaxDimensions+1; i++ {
		tooManyDims["dimensions"].(map[string]string)["k"+strings.Repeat("x", i)] = "v"
	}
	if _, status, _ := buildMetricEvidenceQuery(alert, tooManyDims, 1700005000); status != chatObservationStatusInvalidArgument {
		t.Fatalf("too many dimensions accepted: %q", status)
	}
	halfRange := map[string]any{"from": int64(1700000000)}
	if _, status, _ := buildMetricEvidenceQuery(alert, halfRange, 1700005000); status != chatObservationStatusInvalidArgument {
		t.Fatalf("half time range accepted: %q", status)
	}
	negativePeriod := map[string]any{"period": -5}
	if _, status, _ := buildMetricEvidenceQuery(alert, negativePeriod, 1700005000); status != chatObservationStatusInvalidArgument {
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
			_, status, reason := buildMetricEvidenceQuery(tc.alert, tc.params, 1700005000)
			if status != "skipped" || reason != tc.reason {
				t.Fatalf("status=%q reason=%q, want skipped/%q", status, reason, tc.reason)
			}
		})
	}
}

func TestBuildAlertRuleQueryUsesParamsAndFallback(t *testing.T) {
	query, status, reason := buildAlertRuleQuery(evidenceTestAlert(), map[string]any{
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

	fallback, status, _ := buildAlertRuleQuery(evidenceTestAlert(), nil)
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
			_, status, reason := buildAlertRuleQuery(a, tc.params)
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
		RuleID: "rule-1", Description: strings.Repeat("描", chatRuleDescriptionRune+10), Labels: labels,
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
}

func TestCanonicalAlertRulesToolCodeRegistered(t *testing.T) {
	if _, ok := toolgateway.LookupBuiltinChatTool(chatToolAlertRulesList); !ok {
		t.Fatalf("%s must be registered as a builtin chat tool", chatToolAlertRulesList)
	}
	if _, ok := toolgateway.LookupBuiltinChatTool(chatToolExecutionPropose); ok {
		t.Fatalf("%s must not be registered as a builtin read-only tool", chatToolExecutionPropose)
	}
}
