package huawei

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

type fakeRuleListAPI struct {
	requests []*cesv2model.ListAlarmRulesRequest
	ruleFn   func(req *cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error)
}

func (f *fakeRuleListAPI) ListAlarmRules(req *cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error) {
	f.requests = append(f.requests, req)
	if f.ruleFn == nil {
		return &cesv2model.ListAlarmRulesResponse{}, nil
	}
	return f.ruleFn(req)
}

type fakeAlarmRuleListClient struct {
	rules     []domain.AlertRule
	err       error
	calls     int
	cred      AKSKCredential
	projectID string
	region    string
	namespace string
	maxRules  int
}

func (f *fakeAlarmRuleListClient) ListRules(_ context.Context, cred AKSKCredential, projectID, region, namespace string, maxRules int) ([]domain.AlertRule, error) {
	f.calls++
	f.cred = cred
	f.projectID = projectID
	f.region = region
	f.namespace = namespace
	f.maxRules = maxRules
	return f.rules, f.err
}

func TestCESAlarmRuleListClientMapsPolicies(t *testing.T) {
	enabled := false
	metricName := "cpu_util"
	operator := ">"
	threshold := 80.0
	period := int32(300)
	level := int32(2)
	unit := "ms"
	resourceDims := []cesv2model.MetricDimensionResp{{
		Name: strPtr("rds_instance_id"), Value: strPtr("inst-1"),
	}}
	resources := []cesv2model.ResourcesInListResp{{Dimensions: &resourceDims}}
	api := &fakeRuleListAPI{ruleFn: func(req *cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error) {
		if valueOf(req.Namespace) != "SYS.RDS" {
			t.Fatalf("namespace = %q", valueOf(req.Namespace))
		}
		return &cesv2model.ListAlarmRulesResponse{Alarms: &[]cesv2model.ListAlarmRespBodyAlarms{
			{
				AlarmId: strPtr("al-1"), Name: strPtr("CPU high"), Namespace: strPtr("SYS.RDS"),
				Description: strPtr("cpu utilization"), Enabled: &enabled,
				Policies: &[]cesv2model.PolicyResp{{
					MetricName: &metricName, ComparisonOperator: &operator,
					Value: &threshold, Period: &period, Level: &level, Unit: &unit,
				}},
				Resources: &resources,
			},
			{
				AlarmId: strPtr("al-2"), Name: strPtr("Event rule"), Namespace: strPtr("SYS.RDS"),
				Enabled: strPtrBool(true), Policies: &[]cesv2model.PolicyResp{},
			},
		}}, nil
	}}
	var gotProject, gotRegion string
	client := &CESAlarmRuleListClient{newAPI: func(_ AKSKCredential, projectID, region string) (ruleListAPI, error) {
		gotProject, gotRegion = projectID, region
		return api, nil
	}}
	rules, err := client.ListRules(context.Background(), AKSKCredential{AccessKey: "ak"}, "p-1", "cn-south-1", "SYS.RDS", 10)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if gotProject != "p-1" || gotRegion != "cn-south-1" {
		t.Fatalf("client args project=%q region=%q", gotProject, gotRegion)
	}
	if len(rules) != 2 {
		t.Fatalf("rules = %+v", rules)
	}
	rule := rules[0]
	if rule.RuleID != "al-1" || rule.Enabled || rule.Metric != "cpu_util" || rule.Operator != ">" ||
		rule.Threshold == nil || *rule.Threshold != 80 || rule.WindowSeconds != 300 || rule.Severity != "2" {
		t.Fatalf("unexpected mapped rule: %+v", rule)
	}
	if rule.Unit != "ms" || rule.ResourceCount != 1 || len(rule.Resources) != 1 ||
		rule.Resources[0]["rds_instance_id"] != "inst-1" {
		t.Fatalf("resource/unit not mapped: %+v", rule)
	}
	event := rules[1]
	if !event.Enabled || event.Metric != "" || event.Threshold != nil || event.WindowSeconds != 0 {
		t.Fatalf("event rule must keep skeleton only: %+v", event)
	}
}

func TestCESAlarmRuleListClientPaginatesAndTruncates(t *testing.T) {
	api := &fakeRuleListAPI{ruleFn: func(req *cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error) {
		switch valueOfInt32(req.Offset) {
		case 0:
			items := make([]cesv2model.ListAlarmRespBodyAlarms, ruleListPageLimit)
			for i := range items {
				items[i] = cesv2model.ListAlarmRespBodyAlarms{AlarmId: strPtr(fmt.Sprintf("al-%d", i))}
			}
			return &cesv2model.ListAlarmRulesResponse{Alarms: &items}, nil
		case ruleListPageLimit:
			return &cesv2model.ListAlarmRulesResponse{Alarms: &[]cesv2model.ListAlarmRespBodyAlarms{{AlarmId: strPtr("al-last")}}}, nil
		default:
			t.Fatalf("unexpected offset %d", valueOfInt32(req.Offset))
			return nil, nil
		}
	}}
	client := &CESAlarmRuleListClient{newAPI: func(AKSKCredential, string, string) (ruleListAPI, error) { return api, nil }}
	rules, err := client.ListRules(context.Background(), AKSKCredential{}, "p-1", "cn-south-1", "SYS.RDS", 150)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != ruleListPageLimit+1 || len(api.requests) != 2 {
		t.Fatalf("rules=%d calls=%d", len(rules), len(api.requests))
	}
	if got := valueOfInt32(api.requests[1].Offset); got != ruleListPageLimit {
		t.Fatalf("second offset = %d", got)
	}

	api.requests = nil
	truncated, err := client.ListRules(context.Background(), AKSKCredential{}, "p-1", "cn-south-1", "SYS.RDS", 50)
	if err != nil {
		t.Fatalf("list rules truncated: %v", err)
	}
	if len(truncated) != 50 || len(api.requests) != 1 {
		t.Fatalf("truncated rules=%d calls=%d", len(truncated), len(api.requests))
	}
}

func TestAdapterListAlertRulesAKSKUsesRealClientAndFiltersKeyword(t *testing.T) {
	credentials, repo, vault := newTestCredentialProvider(t)
	seedAKSKCredential(t, credentials, repo, vault, "acc-1")
	listClient := &fakeAlarmRuleListClient{rules: []domain.AlertRule{
		{RuleID: "r-1", Name: "High CPU", Description: "cpu high", Metric: "cpu_util"},
		{RuleID: "r-2", Name: "Disk full", Description: "disk usage", Metric: "disk_util"},
		{RuleID: "r-3", Name: "CPU again", Description: "another cpu rule", Metric: "cpu_util"},
	}}
	adapter := NewAdapter(credentials, nil, nil).WithAlarmRuleList(listClient)

	rules, err := adapter.ListAlertRules(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: "acc-1", AuthType: string(integdomain.AuthAKSK),
			ProjectID: "p-1", Regions: []string{"cn-north-4"}, CredentialRefID: "cref-1",
		},
	}, domain.AlertRuleQuery{Namespace: "SYS.RDS", Keyword: "cpu", Limit: 1})
	if err != nil {
		t.Fatalf("list alert rules: %v", err)
	}
	if len(rules) != 1 || rules[0].RuleID != "r-1" {
		t.Fatalf("filtered rules = %+v", rules)
	}
	if listClient.projectID != "p-1" || listClient.region != "cn-north-4" || listClient.namespace != "SYS.RDS" {
		t.Fatalf("client args = %+v", listClient)
	}
	if listClient.cred.AccessKey != "AKTEST123456" || listClient.cred.SecretKey != "SKTEST9876543210" {
		t.Fatalf("credential not resolved: %+v", listClient.cred)
	}
	// keyword 查询先取 500 条再本地过滤，避免前 50 条不命中导致漏召回。
	if listClient.maxRules != ruleListCapMax {
		t.Fatalf("maxRules = %d", listClient.maxRules)
	}
}

func TestAdapterListAlertRulesAKSKRequiresProjectID(t *testing.T) {
	adapter := NewAdapter(NewCredentialProvider(nil, nil), nil, nil).WithAlarmRuleList(&fakeAlarmRuleListClient{})
	_, err := adapter.ListAlertRules(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{AccountID: "acc-1", AuthType: string(integdomain.AuthAKSK), Regions: []string{"cn-north-4"}},
	}, domain.AlertRuleQuery{Namespace: "SYS.RDS"})
	if apperr.CodeOf(err) != apperr.CodeInvalidArgument || !strings.Contains(err.Error(), "project_id is required") {
		t.Fatalf("expected project_id invalid argument, got %v", err)
	}
}

func TestAdapterListAlertRulesAgencyUnsupported(t *testing.T) {
	adapter := NewAdapter(nil, nil, nil)
	_, err := adapter.ListAlertRules(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{AccountID: "acc-1", AuthType: string(integdomain.AuthAgency)},
	}, domain.AlertRuleQuery{Namespace: "SYS.RDS"})
	if !errors.Is(err, domain.ErrCapabilityUnsupported) {
		t.Fatalf("expected ErrCapabilityUnsupported, got %v", err)
	}
	if apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("expected FAILED_PRECONDITION, got %s", apperr.CodeOf(err))
	}
}

func TestAdapterListAlertRulesNoneUsesFake(t *testing.T) {
	adapter := NewAdapter(nil, nil, nil)
	rules, err := adapter.ListAlertRules(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{AccountID: "acc-1", AuthType: string(integdomain.AuthNone)},
	}, domain.AlertRuleQuery{Namespace: "SYS.RDS", Limit: 10})
	if err != nil {
		t.Fatalf("list alert rules: %v", err)
	}
	if len(rules) != 1 || rules[0].RuleID == "" || rules[0].Threshold == nil {
		t.Fatalf("fake rule = %+v", rules)
	}
}

func TestCESAlarmRuleListClientMapsSDKError(t *testing.T) {
	client := &CESAlarmRuleListClient{newAPI: func(AKSKCredential, string, string) (ruleListAPI, error) {
		return &fakeRuleListAPI{ruleFn: func(*cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error) {
			return nil, context.DeadlineExceeded
		}}, nil
	}}
	_, err := client.ListRules(context.Background(), AKSKCredential{}, "p-1", "cn-south-1", "SYS.RDS", 10)
	if !errors.Is(err, domain.ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
}

func TestCESAlarmRuleListClientReturnsOnContextDeadline(t *testing.T) {
	release := make(chan struct{})
	api := &fakeRuleListAPI{ruleFn: func(*cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error) {
		<-release
		return &cesv2model.ListAlarmRulesResponse{}, nil
	}}
	client := &CESAlarmRuleListClient{newAPI: func(AKSKCredential, string, string) (ruleListAPI, error) { return api, nil }}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := client.ListRules(ctx, AKSKCredential{}, "p-1", "cn-south-1", "SYS.RDS", 10)
	close(release)
	if !errors.Is(err, domain.ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("ListRules blocked %s on a hanging SDK call", elapsed)
	}
}

func strPtrBool(v bool) *bool { return &v }

func valueOfInt32(v *int32) int {
	if v == nil {
		return 0
	}
	return int(*v)
}

func TestMapListAlarmRuleKeepsResourceGroupIDs(t *testing.T) {
	groupID := "rg-0123456789abcdef01234"
	resources := []cesv2model.ResourcesInListResp{{ResourceGroupId: &groupID}}
	rule := mapListAlarmRule(cesv2model.ListAlarmRespBodyAlarms{
		AlarmId: strPtr("al-group"), Namespace: strPtr("SYS.RDS"), Resources: &resources,
	})
	if len(rule.ResourceGroupIDs) != 1 || rule.ResourceGroupIDs[0] != groupID {
		t.Fatalf("resource group ids = %+v", rule.ResourceGroupIDs)
	}
	if rule.ResourceCount != 1 || len(rule.Resources) != 0 {
		t.Fatalf("group rule must not invent dimensions: %+v", rule)
	}
}

type fakeRuleResourceResolver struct {
	samples map[string][]map[string]string
	err     error
	calls   []string
	limits  []int
}

func (f *fakeRuleResourceResolver) ExpandGroupDimensions(_ context.Context, _ AKSKCredential, _, _, groupID, service string, limit int) ([]map[string]string, error) {
	f.calls = append(f.calls, groupID+"|"+service)
	f.limits = append(f.limits, limit)
	if f.err != nil {
		return nil, f.err
	}
	samples := f.samples[groupID]
	if len(samples) > limit {
		samples = samples[:limit]
	}
	return samples, nil
}

func TestAdapterListAlertRulesExpandsResourceGroupRules(t *testing.T) {
	credentials, repo, vault := newTestCredentialProvider(t)
	seedAKSKCredential(t, credentials, repo, vault, "acc-1")
	listClient := &fakeAlarmRuleListClient{rules: []domain.AlertRule{
		{
			RuleID: "r-group", Name: "Group rule", Namespace: "SYS.RDS", Metric: "rds001_cpu_util",
			ResourceCount: 1, ResourceGroupIDs: []string{"rg-1"},
		},
		{
			RuleID: "r-dim", Name: "Instance rule", Namespace: "SYS.RDS", Metric: "rds002_mem_util",
			ResourceCount: 1, Resources: []map[string]string{{"rds_instance_id": "i-9"}},
		},
	}}
	resolver := &fakeRuleResourceResolver{samples: map[string][]map[string]string{
		"rg-1": {
			{"rds_instance_id": "i-1"},
			{"rds_instance_id": "i-2"},
			{"rds_instance_id": "i-3"},
			{"rds_instance_id": "i-4"},
		},
	}}
	adapter := NewAdapter(credentials, nil, nil).
		WithAlarmRuleList(listClient).
		WithRuleResourceResolver(resolver)

	rules, err := adapter.ListAlertRules(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: "acc-1", AuthType: string(integdomain.AuthAKSK),
			ProjectID: "p-1", Regions: []string{"cn-north-4"}, CredentialRefID: "cref-1",
		},
	}, domain.AlertRuleQuery{Namespace: "SYS.RDS", Limit: 10})
	if err != nil {
		t.Fatalf("list alert rules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("rules = %+v", rules)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "rg-1|SYS.RDS" {
		t.Fatalf("resolver calls = %+v", resolver.calls)
	}
	if resolver.limits[0] != ruleResourceSampleMax {
		t.Fatalf("expand limit = %d, want %d", resolver.limits[0], ruleResourceSampleMax)
	}
	groupRule := rules[0]
	if len(groupRule.Resources) != ruleResourceSampleMax {
		t.Fatalf("group rule samples = %+v", groupRule.Resources)
	}
	for i, want := range []string{"i-1", "i-2", "i-3"} {
		if groupRule.Resources[i]["rds_instance_id"] != want {
			t.Fatalf("sample[%d] = %+v", i, groupRule.Resources[i])
		}
	}
	if dimRule := rules[1]; len(dimRule.Resources) != 1 || dimRule.Resources[0]["rds_instance_id"] != "i-9" {
		t.Fatalf("dimension rule must not be re-expanded: %+v", dimRule)
	}
}
