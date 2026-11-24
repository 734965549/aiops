package observability

import (
	"context"
	"strings"
	"testing"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

func ruleHealthRulesQuerier() *fakeAlertRulesQuerier {
	threshold := 80.0
	return &fakeAlertRulesQuerier{result: &obsapp.AlertRuleQueryResult{
		EvidenceID: "rules-ev-1",
		Rules: []obsdomain.AlertRule{
			{
				RuleID: "r-1", Name: "CPU high", Namespace: "SYS.RDS",
				Metric: "rds001_cpu_util", Operator: ">", Threshold: &threshold, Unit: "%",
				ResourceCount: 2, Resources: []map[string]string{{"rds_instance_id": "i-1"}},
			},
			{RuleID: "r-2", Name: "Event rule", Namespace: "SYS.RDS"},
			{
				RuleID: "r-3", Name: "No scope", Namespace: "SYS.RDS",
				Metric: "rds002_mem_util", Operator: ">", Threshold: &threshold,
			},
		},
	}}
}

func newRuleHealthCollectorForTest(rules *fakeAlertRulesQuerier, metrics *fakeMetricsQuerier, access *fakeAccountAccess) *RuleHealthCollector {
	collector := NewRuleHealthCollector(rules, metrics, &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "ak_sk", OwnerTeam: "sre", Regions: []string{"cn-north-4"},
	}}, access, nil, time.Second)
	collector.totalTimeout = 5 * time.Second
	return collector
}

func TestRuleHealthCollectorSummarizesRules(t *testing.T) {
	rules := ruleHealthRulesQuerier()
	metrics := &fakeMetricsQuerier{result: &obsapp.MetricQueryResult{
		EvidenceID: "ev-1",
		Series: []obsdomain.MetricSeries{{
			Metric: "rds001_cpu_util",
			Points: []obsdomain.MetricPoint{
				{TS: 300, Value: 90}, {TS: 100, Value: 70}, {TS: 200, Value: 85},
			},
		}},
	}}
	access := &fakeAccountAccess{}
	collector := newRuleHealthCollectorForTest(rules, metrics, access)

	out, err := collector.Check(context.Background(), "u-1", aiapp.RuleHealthQuery{
		AccountID: "acc-1", Region: "cn-north-4", Namespace: "SYS.RDS", Limit: 10,
		From: 1700000000, To: 1700000000 + 30*24*3600, Period: 3600, Aggregator: "max",
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if out.Status != aiapp.EvidenceStatusOK || out.RuleCount != 3 || out.CheckedCount != 1 {
		t.Fatalf("result = %+v", out)
	}
	first := out.Rules[0]
	if first.Status != aiapp.EvidenceStatusOK || first.EvidenceID != "ev-1" || first.PointCount != 3 {
		t.Fatalf("rule result = %+v", first)
	}
	if first.First == nil || *first.First != 70 || first.Last == nil || *first.Last != 90 ||
		first.Peak == nil || *first.Peak != 90 || first.P95 == nil || *first.P95 != 90 {
		t.Fatalf("stats = %+v", first)
	}
	if first.BreachPoints != 2 || first.BreachRatio == nil || *first.BreachRatio != 2.0/3.0 {
		t.Fatalf("breach = %+v", first)
	}
	if first.Unit != "%" || !first.Sampled || first.Dimensions["rds_instance_id"] != "i-1" {
		t.Fatalf("scope/unit = %+v", first)
	}
	if out.Rules[1].Status != aiapp.EvidenceStatusSkipped || out.Rules[1].Reason != "no_metric" {
		t.Fatalf("event rule = %+v", out.Rules[1])
	}
	if out.Rules[2].Status != aiapp.EvidenceStatusSkipped || out.Rules[2].Reason != "no_resource_scope" {
		t.Fatalf("no scope rule = %+v", out.Rules[2])
	}
	if metrics.query.Namespace != "SYS.RDS" || metrics.query.Metric != "rds001_cpu_util" ||
		metrics.query.Dimensions["rds_instance_id"] != "i-1" ||
		metrics.query.From != 1700000000 || metrics.query.Period != 3600 ||
		metrics.query.Aggregator != "max" || !metrics.query.AllowLongWindow || !metrics.query.SnapshotRequired {
		t.Fatalf("metric query = %+v", metrics.query)
	}
	if !access.called || access.ref.AccountID != "acc-1" {
		t.Fatalf("account access not checked: %+v", access)
	}
}

func TestRuleHealthCollectorPermissionDenied(t *testing.T) {
	rules := ruleHealthRulesQuerier()
	access := &fakeAccountAccess{err: apperr.New(apperr.CodePermissionDenied, "denied")}
	collector := newRuleHealthCollectorForTest(rules, &fakeMetricsQuerier{}, access)

	out, err := collector.Check(context.Background(), "u-1", aiapp.RuleHealthQuery{
		AccountID: "acc-1", Namespace: "SYS.RDS",
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if out.Status != aiapp.EvidenceStatusPermissionDenied || rules.called {
		t.Fatalf("result = %+v, rulesCalled=%v", out, rules.called)
	}
}

func TestRuleHealthCollectorRuleQueryFailure(t *testing.T) {
	rules := &fakeAlertRulesQuerier{err: apperr.New(apperr.CodeInternal, "ces down")}
	collector := newRuleHealthCollectorForTest(rules, &fakeMetricsQuerier{}, &fakeAccountAccess{})

	out, err := collector.Check(context.Background(), "u-1", aiapp.RuleHealthQuery{
		AccountID: "acc-1", Namespace: "SYS.RDS",
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if out.Status != aiapp.EvidenceStatusError || out.Reason != "alert rules query failed" {
		t.Fatalf("result = %+v", out)
	}
}

func TestRuleHealthCollectorAgencyUnsupported(t *testing.T) {
	rules := ruleHealthRulesQuerier()
	collector := NewRuleHealthCollector(rules, &fakeMetricsQuerier{}, &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "agency",
	}}, &fakeAccountAccess{}, nil, time.Second)

	out, err := collector.Check(context.Background(), "u-1", aiapp.RuleHealthQuery{
		AccountID: "acc-1", Namespace: "SYS.RDS",
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if out.Status != aiapp.EvidenceStatusUnsupported || rules.called {
		t.Fatalf("result = %+v, rulesCalled=%v", out, rules.called)
	}
}

type fakeMultiMetricsQuerier struct {
	results map[string]*obsapp.MetricQueryResult
	queries []obsdomain.MetricQuery
}

func (f *fakeMultiMetricsQuerier) QueryMetrics(_ context.Context, _ obsapp.Actor, q obsdomain.MetricQuery) (*obsapp.MetricQueryResult, error) {
	f.queries = append(f.queries, q)
	return f.results[q.Dimensions["rds_instance_id"]], nil
}

func TestRuleHealthCollectorSamplesWorstInstance(t *testing.T) {
	threshold := 80.0
	rules := &fakeAlertRulesQuerier{result: &obsapp.AlertRuleQueryResult{
		Rules: []obsdomain.AlertRule{{
			RuleID: "r-1", Name: "CPU high", Namespace: "SYS.RDS",
			Metric: "rds001_cpu_util", Operator: ">", Threshold: &threshold, Unit: "%",
			ResourceCount: 4,
			Resources: []map[string]string{
				{"rds_instance_id": "i-1"},
				{"rds_instance_id": "i-2"},
				{"rds_instance_id": "i-3"},
				{"rds_instance_id": "i-4"},
			},
		}},
	}}
	metrics := &fakeMultiMetricsQuerier{results: map[string]*obsapp.MetricQueryResult{
		"i-1": {EvidenceID: "ev-1", Series: []obsdomain.MetricSeries{{Points: []obsdomain.MetricPoint{
			{TS: 100, Value: 85}, {TS: 200, Value: 70}, {TS: 300, Value: 75},
		}}}},
		"i-2": {EvidenceID: "ev-2", Series: []obsdomain.MetricSeries{{Points: []obsdomain.MetricPoint{
			{TS: 100, Value: 85}, {TS: 200, Value: 90}, {TS: 300, Value: 75},
		}}}},
		"i-3": {EvidenceID: "ev-3", Series: []obsdomain.MetricSeries{{Points: []obsdomain.MetricPoint{}}}},
	}}
	collector := NewRuleHealthCollector(rules, metrics, &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "ak_sk",
	}}, &fakeAccountAccess{}, nil, time.Second)

	out, err := collector.Check(context.Background(), "u-1", aiapp.RuleHealthQuery{
		AccountID: "acc-1", Region: "cn-north-4", Namespace: "SYS.RDS", Limit: 10,
		From: 1700000000, To: 1700000000 + 30*24*3600, Period: 3600, Aggregator: "max",
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	got := out.Rules[0]
	if got.Status != aiapp.EvidenceStatusOK || got.CheckedResources != 3 || !got.Sampled {
		t.Fatalf("rule result = %+v", got)
	}
	if got.Dimensions["rds_instance_id"] != "i-2" || got.EvidenceID != "ev-2" {
		t.Fatalf("worst sample not selected: %+v", got)
	}
	if got.PeakAt == nil || *got.PeakAt != 200 || got.Peak == nil || *got.Peak != 90 {
		t.Fatalf("peak stats = %+v", got)
	}
	if got.BreachPoints != 2 || got.BreachEpisodes != 1 ||
		got.BreachDurationSeconds != 7200 || got.LongestBreachSeconds != 7200 {
		t.Fatalf("breach stats = %+v", got)
	}
	if len(metrics.queries) != 3 {
		t.Fatalf("queries = %d, want capped 3 samples", len(metrics.queries))
	}
	for i, want := range []string{"i-1", "i-2", "i-3"} {
		if metrics.queries[i].Dimensions["rds_instance_id"] != want {
			t.Fatalf("query[%d] dims = %+v", i, metrics.queries[i].Dimensions)
		}
	}
}

type fakeResourceDiscovery struct {
	result  *aiapp.ResourceListResult
	err     error
	queries []aiapp.ResourceQuery
}

func (f *fakeResourceDiscovery) List(_ context.Context, _ string, q aiapp.ResourceQuery) (*aiapp.ResourceListResult, error) {
	f.queries = append(f.queries, q)
	return f.result, f.err
}

func TestRuleHealthCollectorFallsBackToResourceDiscovery(t *testing.T) {
	threshold := 80.0
	rules := &fakeAlertRulesQuerier{result: &obsapp.AlertRuleQueryResult{
		Rules: []obsdomain.AlertRule{{
			RuleID: "r-product", Name: "Product level CPU", Namespace: "SYS.RDS",
			Metric: "rds001_cpu_util", Operator: ">", Threshold: &threshold, Unit: "%",
		}},
	}}
	metrics := &fakeMultiMetricsQuerier{results: map[string]*obsapp.MetricQueryResult{
		"i-1": {EvidenceID: "ev-1", Series: []obsdomain.MetricSeries{{Points: []obsdomain.MetricPoint{
			{TS: 100, Value: 85}, {TS: 200, Value: 70},
		}}}},
		"i-2": {EvidenceID: "ev-2", Series: []obsdomain.MetricSeries{{Points: []obsdomain.MetricPoint{
			{TS: 100, Value: 85}, {TS: 200, Value: 95},
		}}}},
		"i-3": {EvidenceID: "ev-3", Series: []obsdomain.MetricSeries{{Points: []obsdomain.MetricPoint{
			{TS: 100, Value: 65}, {TS: 200, Value: 70},
		}}}},
	}}
	discovery := &fakeResourceDiscovery{result: &aiapp.ResourceListResult{
		Status: aiapp.EvidenceStatusOK,
		Resources: []aiapp.ResourceSummary{
			{ResourceID: "a-1", CloudResourceType: "rds", CloudResourceID: "i-1",
				Namespace: "SYS.RDS", DimensionName: "rds_instance_id", DimensionValue: "i-1"},
			{ResourceID: "a-2", CloudResourceType: "rds", CloudResourceID: "i-2",
				Namespace: "SYS.RDS", DimensionName: "rds_instance_id", DimensionValue: "i-2"},
			{ResourceID: "a-3", CloudResourceType: "rds", CloudResourceID: "i-3",
				Namespace: "SYS.RDS", DimensionName: "rds_instance_id", DimensionValue: "i-3"},
			// 其他 namespace 与缺失维度的行必须被过滤。
			{ResourceID: "a-9", CloudResourceType: "ecs", CloudResourceID: "e-9",
				Namespace: "SYS.ECS", DimensionName: "instance_id", DimensionValue: "e-9"},
			{ResourceID: "a-x", CloudResourceType: "rds", CloudResourceID: "i-x"},
		},
	}}
	collector := NewRuleHealthCollector(rules, metrics, &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "ak_sk",
	}}, &fakeAccountAccess{}, discovery, time.Second)

	out, err := collector.Check(context.Background(), "u-1", aiapp.RuleHealthQuery{
		AccountID: "acc-1", Region: "cn-north-4", Namespace: "SYS.RDS", Limit: 10,
		From: 1700000000, To: 1700000000 + 30*24*3600, Period: 3600, Aggregator: "max",
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(discovery.queries) != 1 || discovery.queries[0].ResourceType != "rds" ||
		discovery.queries[0].Limit != ruleHealthDiscoveryLimit {
		t.Fatalf("discovery queries = %+v", discovery.queries)
	}
	got := out.Rules[0]
	if got.Status != aiapp.EvidenceStatusOK || got.ScopeSource != "resource_discovery" ||
		!got.Sampled || got.CheckedResources != 3 {
		t.Fatalf("fallback rule result = %+v", got)
	}
	if got.Dimensions["rds_instance_id"] != "i-2" || got.PeakAt == nil || *got.PeakAt != 200 {
		t.Fatalf("worst fallback sample = %+v", got)
	}
}

func TestRuleHealthCollectorBreadthFirstBudget(t *testing.T) {
	threshold := 80.0
	rules := &fakeAlertRulesQuerier{result: &obsapp.AlertRuleQueryResult{
		Rules: []obsdomain.AlertRule{
			{
				RuleID: "r-1", Namespace: "SYS.RDS", Metric: "m-1", Operator: ">", Threshold: &threshold,
				ResourceCount: 3, Resources: []map[string]string{
					{"rds_instance_id": "a1"}, {"rds_instance_id": "a2"}, {"rds_instance_id": "a3"},
				},
			},
			{
				RuleID: "r-2", Namespace: "SYS.RDS", Metric: "m-2", Operator: ">", Threshold: &threshold,
				ResourceCount: 3, Resources: []map[string]string{
					{"rds_instance_id": "b1"}, {"rds_instance_id": "b2"}, {"rds_instance_id": "b3"},
				},
			},
		},
	}}
	series := func(id string) *obsapp.MetricQueryResult {
		return &obsapp.MetricQueryResult{EvidenceID: id, Series: []obsdomain.MetricSeries{{Points: []obsdomain.MetricPoint{
			{TS: 100, Value: 90}, {TS: 200, Value: 70},
		}}}}
	}
	metrics := &fakeMultiMetricsQuerier{results: map[string]*obsapp.MetricQueryResult{
		"a1": series("e-a1"), "a2": series("e-a2"), "a3": series("e-a3"),
		"b1": series("e-b1"), "b2": series("e-b2"), "b3": series("e-b3"),
	}}
	collector := NewRuleHealthCollector(rules, metrics, &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "ak_sk",
	}}, &fakeAccountAccess{}, nil, time.Second)
	collector.queryBudget = 4

	out, err := collector.Check(context.Background(), "u-1", aiapp.RuleHealthQuery{
		AccountID: "acc-1", Region: "cn-north-4", Namespace: "SYS.RDS", Limit: 10,
		From: 1700000000, To: 1700000000 + 30*24*3600, Period: 3600, Aggregator: "max",
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	// 预算 4 / 2 条规则 = 每条 2 个实例，广度优先保证两条规则都有曲线。
	if len(metrics.queries) != 4 {
		t.Fatalf("queries = %d, want 4", len(metrics.queries))
	}
	for i, got := range out.Rules {
		if got.Status != aiapp.EvidenceStatusOK || got.CheckedResources != 2 || !got.Sampled {
			t.Fatalf("rule[%d] = %+v", i, got)
		}
	}
}

func TestRuleHealthCollectorBudgetKeepsBreadth(t *testing.T) {
	threshold := 80.0
	rules := &fakeAlertRulesQuerier{result: &obsapp.AlertRuleQueryResult{
		Rules: []obsdomain.AlertRule{
			{RuleID: "r-1", Namespace: "SYS.RDS", Metric: "m-1", Operator: ">", Threshold: &threshold,
				ResourceCount: 1, Resources: []map[string]string{{"rds_instance_id": "i-1"}}},
			{RuleID: "r-2", Namespace: "SYS.RDS", Metric: "m-2", Operator: ">", Threshold: &threshold,
				ResourceCount: 1, Resources: []map[string]string{{"rds_instance_id": "i-2"}}},
			{RuleID: "r-3", Namespace: "SYS.RDS", Metric: "m-3", Operator: ">", Threshold: &threshold,
				ResourceCount: 1, Resources: []map[string]string{{"rds_instance_id": "i-3"}}},
		},
	}}
	series := func(id string) *obsapp.MetricQueryResult {
		return &obsapp.MetricQueryResult{EvidenceID: id, Series: []obsdomain.MetricSeries{{Points: []obsdomain.MetricPoint{
			{TS: 100, Value: 90},
		}}}}
	}
	metrics := &fakeMultiMetricsQuerier{results: map[string]*obsapp.MetricQueryResult{
		"i-1": series("e-1"), "i-2": series("e-2"), "i-3": series("e-3"),
	}}
	collector := NewRuleHealthCollector(rules, metrics, &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "ak_sk",
	}}, &fakeAccountAccess{}, nil, time.Second)
	collector.queryBudget = 2

	out, err := collector.Check(context.Background(), "u-1", aiapp.RuleHealthQuery{
		AccountID: "acc-1", Region: "cn-north-4", Namespace: "SYS.RDS", Limit: 10,
		From: 1700000000, To: 1700000000 + 30*24*3600, Period: 3600, Aggregator: "max",
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if out.Rules[0].Status != aiapp.EvidenceStatusOK || out.Rules[1].Status != aiapp.EvidenceStatusOK {
		t.Fatalf("breadth coverage lost: %+v", out.Rules)
	}
	if out.Rules[2].Status != aiapp.EvidenceStatusSkipped ||
		out.Rules[2].Reason != "rule health query budget exhausted" {
		t.Fatalf("last rule must be budget-exhausted: %+v", out.Rules[2])
	}
	if len(metrics.queries) != 2 {
		t.Fatalf("queries = %d, want 2", len(metrics.queries))
	}
}

func TestRuleHealthCollectorFallbackFiltersEngineAndDimension(t *testing.T) {
	threshold := 80.0
	rules := &fakeAlertRulesQuerier{result: &obsapp.AlertRuleQueryResult{
		Rules: []obsdomain.AlertRule{{
			RuleID: "al-pg", Name: "RDS-postgresql", Description: "postgresql生产环境资源",
			Namespace: "SYS.RDS", Metric: "rds001_cpu_util", Operator: ">", Threshold: &threshold,
			ResourceLevel: "product", ProductName: "SYS.RDS,rds_instance_id",
			DimensionName: "rds_instance_id", ResourceCount: 1,
		}},
	}}
	series := func(id string) *obsapp.MetricQueryResult {
		return &obsapp.MetricQueryResult{EvidenceID: id, Series: []obsdomain.MetricSeries{{Points: []obsdomain.MetricPoint{
			{TS: 100, Value: 90}, {TS: 200, Value: 70},
		}}}}
	}
	metrics := &fakeMultiMetricsQuerier{results: map[string]*obsapp.MetricQueryResult{
		"pg-1": series("e-pg1"), "pg-2": series("e-pg2"), "pg-3": series("e-pg3"),
	}}
	discovery := &fakeResourceDiscovery{result: &aiapp.ResourceListResult{
		Status: aiapp.EvidenceStatusOK,
		Resources: []aiapp.ResourceSummary{
			{ResourceID: "a-my", CloudResourceType: "rds", CloudResourceID: "my-1",
				Namespace: "SYS.RDS", DimensionName: "rds_instance_id", DimensionValue: "my-1", Engine: "MySQL"},
			{ResourceID: "a-pg1", CloudResourceType: "rds", CloudResourceID: "pg-1",
				Namespace: "SYS.RDS", DimensionName: "rds_instance_id", DimensionValue: "pg-1", Engine: "PostgreSQL"},
			{ResourceID: "a-pg2", CloudResourceType: "rds", CloudResourceID: "pg-2",
				Namespace: "SYS.RDS", DimensionName: "rds_instance_id", DimensionValue: "pg-2", Engine: "PostgreSQL"},
			{ResourceID: "a-pg3", CloudResourceType: "rds", CloudResourceID: "pg-3",
				Namespace: "SYS.RDS", DimensionName: "rds_instance_id", DimensionValue: "pg-3", Engine: "PostgreSQL"},
		},
	}}
	collector := NewRuleHealthCollector(rules, metrics, &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "ak_sk",
	}}, &fakeAccountAccess{}, discovery, time.Second)

	out, err := collector.Check(context.Background(), "u-1", aiapp.RuleHealthQuery{
		AccountID: "acc-1", Region: "cn-north-4", Namespace: "SYS.RDS", Limit: 10,
		From: 1700000000, To: 1700000000 + 30*24*3600, Period: 3600, Aggregator: "max",
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	got := out.Rules[0]
	if got.Status != aiapp.EvidenceStatusOK || got.ScopeSource != "resource_discovery" ||
		got.CheckedResources != 3 || got.DimensionName != "rds_instance_id" {
		t.Fatalf("fallback result = %+v", got)
	}
	if got.ResourceCount != 0 {
		t.Fatalf("discovery fallback must not claim a resource total, got %d", got.ResourceCount)
	}
	for i, q := range metrics.queries {
		if value := q.Dimensions["rds_instance_id"]; strings.HasPrefix(value, "my-") {
			t.Fatalf("query[%d] sampled mysql instance for pg rule: %+v", i, q.Dimensions)
		}
		if value := q.Dimensions["rds_instance_id"]; !strings.HasPrefix(value, "pg-") {
			t.Fatalf("query[%d] unexpected dimension: %+v", i, q.Dimensions)
		}
	}
}

type fakeProbeMetricsQuerier struct {
	primary *obsapp.MetricQueryResult
	probe   *obsapp.MetricQueryResult
	queries []obsdomain.MetricQuery
}

func (f *fakeProbeMetricsQuerier) QueryMetrics(_ context.Context, _ obsapp.Actor, q obsdomain.MetricQuery) (*obsapp.MetricQueryResult, error) {
	f.queries = append(f.queries, q)
	if q.Period == ruleHealthProbePeriod {
		return f.probe, nil
	}
	return f.primary, nil
}

func TestRuleHealthCollectorProbesEmptySeries(t *testing.T) {
	threshold := 80.0
	rules := &fakeAlertRulesQuerier{result: &obsapp.AlertRuleQueryResult{
		Rules: []obsdomain.AlertRule{{
			RuleID: "al-empty", Name: "Event status", Namespace: "SYS.RDS",
			Metric: "abnormalReplicationStatus", Operator: ">", Threshold: &threshold,
			ResourceCount: 1, Resources: []map[string]string{{"rds_instance_id": "i-1"}},
		}},
	}}
	metrics := &fakeProbeMetricsQuerier{
		primary: &obsapp.MetricQueryResult{EvidenceID: "ev-primary"},
		probe: &obsapp.MetricQueryResult{EvidenceID: "ev-probe", Series: []obsdomain.MetricSeries{{Points: []obsdomain.MetricPoint{
			{TS: 1700000000, Value: 0}, {TS: 1700000060, Value: 0}, {TS: 1700000120, Value: 1},
		}}}},
	}
	collector := NewRuleHealthCollector(rules, metrics, &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "ak_sk",
	}}, &fakeAccountAccess{}, nil, time.Second)

	out, err := collector.Check(context.Background(), "u-1", aiapp.RuleHealthQuery{
		AccountID: "acc-1", Region: "cn-north-4", Namespace: "SYS.RDS", Limit: 10,
		From: 1700000000, To: 1700000000 + 30*24*3600, Period: 3600, Aggregator: "max",
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	got := out.Rules[0]
	if got.Status != aiapp.EvidenceStatusOK || got.PointCount != 0 {
		t.Fatalf("rule result = %+v", got)
	}
	if got.ProbePointCount != 3 || got.ProbePeriod != ruleHealthProbePeriod ||
		got.ProbeWindowSeconds != ruleHealthProbeWindowSecs {
		t.Fatalf("probe result = %+v", got)
	}
	if len(metrics.queries) != 2 {
		t.Fatalf("queries = %d, want primary + probe", len(metrics.queries))
	}
	if probe := metrics.queries[1]; probe.Period != ruleHealthProbePeriod ||
		probe.To-probe.From != ruleHealthProbeWindowSecs {
		t.Fatalf("probe query = %+v", probe)
	}
}
