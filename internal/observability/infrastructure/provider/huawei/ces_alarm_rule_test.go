package huawei

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/observability/domain"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

type fakeAlarmHistoryAPI struct {
	historyFn func(req *cesv2model.ListAlarmHistoriesRequest) (*cesv2model.ListAlarmHistoriesResponse, error)
	ruleFn    func(req *cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error)
	ruleCalls int
}

func (f *fakeAlarmHistoryAPI) ListAlarmHistories(req *cesv2model.ListAlarmHistoriesRequest) (*cesv2model.ListAlarmHistoriesResponse, error) {
	if f.historyFn == nil {
		return &cesv2model.ListAlarmHistoriesResponse{}, nil
	}
	return f.historyFn(req)
}

func (f *fakeAlarmHistoryAPI) ListAlarmRules(req *cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error) {
	f.ruleCalls++
	if f.ruleFn == nil {
		return &cesv2model.ListAlarmRulesResponse{}, nil
	}
	return f.ruleFn(req)
}

func newRuleTestClient(api alarmHistoryAPI) *CESAlarmHistoryClient {
	return &CESAlarmHistoryClient{
		newAPI: func(AKSKCredential, string, string) (alarmHistoryAPI, error) { return api, nil },
		rules:  &alarmRuleCache{},
	}
}

func alarmHistoryWithoutMetric(namespace string, dims map[string]string) cesv2model.AlarmHistoryItemV2 {
	status := cesv2model.GetAlarmHistoryItemV2StatusEnum().ALARM
	level := cesv2model.GetAlarmHistoryItemV2LevelEnum().E_2
	metric := &cesv2model.AlarmHistoryItemV2Metric{Namespace: strPtr(namespace)}
	if len(dims) > 0 {
		items := make([]cesv2model.AlarmHistoryItemV2MetricDimensions, 0, len(dims))
		for name, value := range dims {
			items = append(items, cesv2model.AlarmHistoryItemV2MetricDimensions{Name: strPtr(name), Value: strPtr(value)})
		}
		metric.Dimensions = &items
	}
	return cesv2model.AlarmHistoryItemV2{
		RecordId: strPtr("ah-rule-1"),
		AlarmId:  strPtr("al-rule-1"),
		Name:     strPtr("High CPU Utilization"),
		Status:   &status,
		Level:    &level,
		Metric:   metric,
	}
}

func ruleListResponse() *cesv2model.ListAlarmRulesResponse {
	metricName := "cpu_util"
	period := int32(300)
	filter := "average"
	operator := ">"
	value := 90.0
	unit := "%"
	count := int32(3)
	level := int32(2)
	suppress := int32(600)
	return &cesv2model.ListAlarmRulesResponse{
		Alarms: &[]cesv2model.ListAlarmRespBodyAlarms{{
			AlarmId:     strPtr("al-rule-1"),
			Name:        strPtr("High CPU Utilization"),
			Description: strPtr("ECS CPU utilization exceeds 90%"),
			Namespace:   strPtr("SYS.ECS"),
			Policies: &[]cesv2model.PolicyResp{{
				MetricName:         &metricName,
				Period:             &period,
				Filter:             &filter,
				ComparisonOperator: &operator,
				Value:              &value,
				Unit:               &unit,
				Count:              &count,
				Level:              &level,
				SuppressDuration:   &suppress,
			}},
		}},
	}
}

func TestPullScopeEnrichesMissingMetricFromAlarmRule(t *testing.T) {
	history := alarmHistoryWithoutMetric("SYS.ECS", map[string]string{"instance_id": "i-1"})
	api := &fakeAlarmHistoryAPI{
		historyFn: func(*cesv2model.ListAlarmHistoriesRequest) (*cesv2model.ListAlarmHistoriesResponse, error) {
			return &cesv2model.ListAlarmHistoriesResponse{
				AlarmHistories: &[]cesv2model.AlarmHistoryItemV2{history},
				Count:          int32Ptr(1),
			}, nil
		},
		ruleFn: func(req *cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error) {
			if req.AlarmId == nil || valueOf(req.AlarmId) != "al-rule-1" {
				t.Fatalf("unexpected alarm_id filter: %+v", req.AlarmId)
			}
			return ruleListResponse(), nil
		},
	}
	client := newRuleTestClient(api)
	page, err := client.PullScope(context.Background(), AKSKCredential{}, domain.AlarmPullScope{
		Region: "cn-north-4", ProjectID: "proj-1",
	}, 0, 1000, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("pull scope: %v", err)
	}
	if !page.Complete || len(page.Records) != 1 {
		t.Fatalf("unexpected page: %+v", page)
	}
	rec := page.Records[0]
	if rec.MetricName != "cpu_util" {
		t.Fatalf("metric name = %q, want cpu_util", rec.MetricName)
	}
	if rec.Condition == nil || rec.Condition.Describe() != "> 90% (average, 300s x3)" {
		t.Fatalf("unexpected condition: %+v", rec.Condition)
	}
	if rec.RuleName != "High CPU Utilization" || rec.RuleDescription != "ECS CPU utilization exceeds 90%" {
		t.Fatalf("unexpected rule metadata: %+v", rec)
	}
	if rec.ResourceID != "i-1" || rec.ResourceName != "i-1" {
		t.Fatalf("unexpected resource fallback: %q/%q", rec.ResourceID, rec.ResourceName)
	}
	if api.ruleCalls != 1 {
		t.Fatalf("expected one rule lookup, got %d", api.ruleCalls)
	}

	if _, err := client.PullScope(context.Background(), AKSKCredential{}, domain.AlarmPullScope{
		Region: "cn-north-4", ProjectID: "proj-1",
	}, 1000, 2000, time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("second pull scope: %v", err)
	}
	if api.ruleCalls != 1 {
		t.Fatalf("expected cached rule lookup, got %d calls", api.ruleCalls)
	}
}

func TestPullScopeRuleNotFoundDoesNotBlockRecords(t *testing.T) {
	history := alarmHistoryWithoutMetric("SYS.ECS", map[string]string{"instance_id": "i-1"})
	api := &fakeAlarmHistoryAPI{
		historyFn: func(*cesv2model.ListAlarmHistoriesRequest) (*cesv2model.ListAlarmHistoriesResponse, error) {
			return &cesv2model.ListAlarmHistoriesResponse{
				AlarmHistories: &[]cesv2model.AlarmHistoryItemV2{history},
				Count:          int32Ptr(1),
			}, nil
		},
		ruleFn: func(*cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error) {
			return &cesv2model.ListAlarmRulesResponse{}, nil
		},
	}
	page, err := newRuleTestClient(api).PullScope(context.Background(), AKSKCredential{}, domain.AlarmPullScope{
		Region: "cn-north-4", ProjectID: "proj-1",
	}, 0, 1000, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("pull scope: %v", err)
	}
	if !page.Complete || len(page.Records) != 1 {
		t.Fatalf("rule miss must not fail the scope: %+v", page)
	}
	if page.Records[0].RuleLookup != "not_found" {
		t.Fatalf("rule lookup = %q, want not_found", page.Records[0].RuleLookup)
	}
	if page.Records[0].Condition != nil {
		t.Fatalf("expected no condition on rule miss, got %+v", page.Records[0].Condition)
	}
}

func TestPullScopeRuleErrorDoesNotBlockRecords(t *testing.T) {
	history := alarmHistoryWithoutMetric("SYS.ECS", map[string]string{"instance_id": "i-1"})
	api := &fakeAlarmHistoryAPI{
		historyFn: func(*cesv2model.ListAlarmHistoriesRequest) (*cesv2model.ListAlarmHistoriesResponse, error) {
			return &cesv2model.ListAlarmHistoriesResponse{
				AlarmHistories: &[]cesv2model.AlarmHistoryItemV2{history},
				Count:          int32Ptr(1),
			}, nil
		},
		ruleFn: func(*cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error) {
			return nil, errors.New("rule upstream unavailable")
		},
	}
	page, err := newRuleTestClient(api).PullScope(context.Background(), AKSKCredential{}, domain.AlarmPullScope{
		Region: "cn-north-4", ProjectID: "proj-1",
	}, 0, 1000, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("pull scope: %v", err)
	}
	if !page.Complete || len(page.Records) != 1 {
		t.Fatalf("rule error must not fail the scope: %+v", page)
	}
	if page.Records[0].RuleLookup != "error" {
		t.Fatalf("rule lookup = %q, want error", page.Records[0].RuleLookup)
	}
}

func TestPullScopeSkipsRuleLookupWhenHistoryHasMetricAndCondition(t *testing.T) {
	status := cesv2model.GetAlarmHistoryItemV2StatusEnum().ALARM
	metricName := "cpu_util"
	operator := ">"
	value := 80.0
	history := cesv2model.AlarmHistoryItemV2{
		RecordId: strPtr("ah-complete-1"),
		AlarmId:  strPtr("al-rule-1"),
		Name:     strPtr("High CPU Utilization"),
		Status:   &status,
		Metric: &cesv2model.AlarmHistoryItemV2Metric{
			Namespace:  strPtr("SYS.ECS"),
			MetricName: &metricName,
		},
		Condition: &cesv2model.AlarmHistoryItemV2Condition{
			ComparisonOperator: &operator,
			Value:              &value,
		},
	}
	api := &fakeAlarmHistoryAPI{
		historyFn: func(*cesv2model.ListAlarmHistoriesRequest) (*cesv2model.ListAlarmHistoriesResponse, error) {
			return &cesv2model.ListAlarmHistoriesResponse{
				AlarmHistories: &[]cesv2model.AlarmHistoryItemV2{history},
				Count:          int32Ptr(1),
			}, nil
		},
	}
	page, err := newRuleTestClient(api).PullScope(context.Background(), AKSKCredential{}, domain.AlarmPullScope{
		Region: "cn-north-4", ProjectID: "proj-1",
	}, 0, 1000, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("pull scope: %v", err)
	}
	if len(page.Records) != 1 || page.Records[0].RuleLookup != "" {
		t.Fatalf("unexpected records: %+v", page.Records)
	}
	if api.ruleCalls != 0 {
		t.Fatalf("expected no rule lookup, got %d", api.ruleCalls)
	}
}

func TestMapAlarmRuleEventRuleHasNoMetricCondition(t *testing.T) {
	eventType := cesv2model.GetAlarmTypeRespEnum().EVENT_SYS
	rule := mapAlarmRule(cesv2model.ListAlarmRespBodyAlarms{
		Name: strPtr("System Event"),
		Type: &eventType,
		Policies: &[]cesv2model.PolicyResp{{
			MetricName:         strPtr("event_metric"),
			ComparisonOperator: strPtr(">"),
		}},
	})
	if rule.MetricName != "" || rule.Condition != nil {
		t.Fatalf("event rule must not enrich metric/condition: %+v", rule)
	}
	if rule.Type != "EVENT.SYS" {
		t.Fatalf("rule type = %q", rule.Type)
	}
}
