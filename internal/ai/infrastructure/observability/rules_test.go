package observability

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeAlertRulesQuerier struct {
	result *obsapp.AlertRuleQueryResult
	err    error
	called bool
	actor  obsapp.Actor
	query  obsdomain.AlertRuleQuery
}

func (f *fakeAlertRulesQuerier) ListAlertRules(_ context.Context, actor obsapp.Actor, q obsdomain.AlertRuleQuery) (*obsapp.AlertRuleQueryResult, error) {
	f.called = true
	f.actor = actor
	f.query = q
	return f.result, f.err
}

func newRulesCollector(querier *fakeAlertRulesQuerier, access *fakeAccountAccess) *AlertRulesCollector {
	return NewAlertRulesCollector(querier, &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "none", OwnerTeam: "sre", Regions: []string{"cn-north-4"},
	}}, access, time.Second)
}

func TestAlertRulesCollectorMapsRules(t *testing.T) {
	threshold := 80.0
	querier := &fakeAlertRulesQuerier{result: &obsapp.AlertRuleQueryResult{
		EvidenceID: "rules-ev-1",
		Rules: []obsdomain.AlertRule{{
			RuleID: "rule-1", Name: "High CPU", Namespace: "SYS.RDS", Severity: "major",
			Enabled: true, Metric: "cpu_util", Operator: ">", Threshold: &threshold, Unit: "%",
			WindowSeconds: 300, Labels: map[string]string{"provider": "huawei_cloud"},
			ResourceCount: 2,
			Resources:     []map[string]string{{"instance_id": "i-1"}},
		}},
	}}
	access := &fakeAccountAccess{}
	collector := newRulesCollector(querier, access)

	result, err := collector.List(context.Background(), "u-1", aiapp.AlertRuleQuery{
		AccountID: "acc-1", Region: "cn-north-4", Namespace: "SYS.RDS", Keyword: "cpu", Limit: 10,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusOK || result.EvidenceID != "rules-ev-1" || len(result.Rules) != 1 {
		t.Fatalf("result = %+v", result)
	}
	rule := result.Rules[0]
	if rule.Metric != "cpu_util" || rule.Operator != ">" || rule.Threshold == nil || *rule.Threshold != 80 || rule.WindowSeconds != 300 {
		t.Fatalf("rule fields = %+v", rule)
	}
	if rule.Unit != "%" || rule.ResourceCount != 2 || len(rule.Resources) != 1 ||
		rule.Resources[0]["instance_id"] != "i-1" {
		t.Fatalf("rule unit/resources not mapped: %+v", rule)
	}
	if !access.called || access.ref.AccountID != "acc-1" || access.ref.OwnerTeam != "sre" {
		t.Fatalf("account access not checked: %+v", access)
	}
	if !querier.called || querier.query.Namespace != "SYS.RDS" || querier.query.Limit != 10 || querier.actor.UserID != "u-1" {
		t.Fatalf("query = %+v actor=%+v", querier.query, querier.actor)
	}
}

func TestAlertRulesCollectorPermissionDenied(t *testing.T) {
	querier := &fakeAlertRulesQuerier{result: &obsapp.AlertRuleQueryResult{EvidenceID: "ev"}}
	access := &fakeAccountAccess{err: apperr.New(apperr.CodePermissionDenied, "denied")}
	collector := newRulesCollector(querier, access)

	result, err := collector.List(context.Background(), "u-1", aiapp.AlertRuleQuery{
		AccountID: "acc-1", Namespace: "SYS.RDS",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusPermissionDenied || result.Reason != "no access to integration account for evidence" {
		t.Fatalf("result = %+v", result)
	}
	if querier.called {
		t.Fatal("permission denied must not call the provider")
	}
}

func TestAlertRulesCollectorSkipsMissingNamespace(t *testing.T) {
	collector := newRulesCollector(&fakeAlertRulesQuerier{}, &fakeAccountAccess{})
	result, err := collector.List(context.Background(), "u-1", aiapp.AlertRuleQuery{AccountID: "acc-1"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusSkipped || result.Reason != "no_namespace" {
		t.Fatalf("result = %+v", result)
	}
}

func TestAlertRulesCollectorUnsupported(t *testing.T) {
	querier := &fakeAlertRulesQuerier{err: apperr.Wrap(
		obsdomain.ErrCapabilityUnsupported, apperr.CodeFailedPrecondition, "huawei alerts query is not implemented yet")}
	collector := newRulesCollector(querier, &fakeAccountAccess{})
	result, err := collector.List(context.Background(), "u-1", aiapp.AlertRuleQuery{
		AccountID: "acc-1", Namespace: "SYS.RDS",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusUnsupported || result.Reason != "alert rules are not supported by the provider" {
		t.Fatalf("result = %+v", result)
	}
}

func TestAlertRulesCollectorQueryError(t *testing.T) {
	querier := &fakeAlertRulesQuerier{err: errors.New("boom")}
	collector := newRulesCollector(querier, &fakeAccountAccess{})
	result, err := collector.List(context.Background(), "u-1", aiapp.AlertRuleQuery{
		AccountID: "acc-1", Namespace: "SYS.RDS",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusError || result.Reason != "alert rules query failed" {
		t.Fatalf("result = %+v", result)
	}
}

func TestAlertRulesCollectorProviderUnavailableIsError(t *testing.T) {
	querier := &fakeAlertRulesQuerier{err: fmt.Errorf("%w: huawei ces request timed out", obsdomain.ErrProviderUnavailable)}
	collector := newRulesCollector(querier, &fakeAccountAccess{})
	result, err := collector.List(context.Background(), "u-1", aiapp.AlertRuleQuery{
		AccountID: "acc-1", Namespace: "SYS.RDS",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusError {
		t.Fatalf("provider unavailable must map to error, got %+v", result)
	}
}
