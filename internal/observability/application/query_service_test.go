package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type stubAccountPort struct {
	acc *domain.AccountSnapshot
	err error
}

func (s stubAccountPort) ResolveAccount(context.Context, string) (*domain.AccountSnapshot, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.acc, nil
}

type memEvidenceRepo struct {
	refs []*domain.EvidenceRef
}

func (m *memEvidenceRepo) Create(_ context.Context, ref *domain.EvidenceRef) error {
	cp := *ref
	m.refs = append(m.refs, &cp)
	return nil
}

func (m *memEvidenceRepo) GetByID(context.Context, string) (*domain.EvidenceRef, error) {
	return nil, domain.ErrNotFound
}

type captureAudit struct {
	last AuditRecord
}

func (c *captureAudit) Record(_ context.Context, rec AuditRecord) error {
	c.last = rec
	return nil
}

type stubProviderEntry struct{}

func (stubProviderEntry) ProviderType() string { return string(integdomain.ProviderHuaweiCloud) }
func (stubProviderEntry) QueryMetrics(context.Context, domain.ProviderContext, domain.MetricQuery) ([]domain.MetricSeries, error) {
	return []domain.MetricSeries{{Metric: "cpu_util", Unit: "Percent", Points: []domain.MetricPoint{{TS: 1, Value: 1}}}}, nil
}
func (stubProviderEntry) SearchLogs(context.Context, domain.ProviderContext, domain.LogQuery) ([]domain.LogEntry, error) {
	return []domain.LogEntry{{Timestamp: 1, Level: "INFO", Service: "demo", Message: "matched"}}, nil
}
func (stubProviderEntry) QueryTraces(_ context.Context, _ domain.ProviderContext, q domain.TraceQuery) ([]domain.TraceSpan, error) {
	span := domain.TraceSpan{TraceID: "t1", SpanID: "s1", Service: "demo", Status: "ok"}
	if q.ErrorOnly {
		span.Error = true
		span.Status = "error"
	}
	return []domain.TraceSpan{span}, nil
}
func (stubProviderEntry) QueryTopology(context.Context, domain.ProviderContext, domain.TopologyQuery) (*domain.TopologySnapshot, error) {
	return &domain.TopologySnapshot{}, nil
}
func (stubProviderEntry) ListResources(context.Context, domain.ProviderContext, domain.AssetDiscoveryQuery) ([]domain.CloudResource, error) {
	return []domain.CloudResource{{ResourceID: "res-1", Name: "demo", Type: "ecs", Region: "cn-north-4", Status: "running"}}, nil
}
func (stubProviderEntry) ListAlertRules(context.Context, domain.ProviderContext, domain.AlertRuleQuery) ([]domain.AlertRule, error) {
	return []domain.AlertRule{{RuleID: "rule-1", Name: "cpu", Enabled: true}}, nil
}

type stubRegistry struct {
	p ProviderEntry
}

func (r stubRegistry) Get(string) (ProviderEntry, error) { return r.p, nil }

type captureQueryProvider struct {
	metricPeriod int
	logLimit     int
	traceLimit   int
}

func (p *captureQueryProvider) ProviderType() string { return string(integdomain.ProviderHuaweiCloud) }
func (p *captureQueryProvider) QueryMetrics(_ context.Context, _ domain.ProviderContext, q domain.MetricQuery) ([]domain.MetricSeries, error) {
	p.metricPeriod = q.Period
	return []domain.MetricSeries{{Metric: q.Metric, Points: []domain.MetricPoint{{TS: q.From, Value: 1}}}}, nil
}
func (p *captureQueryProvider) SearchLogs(_ context.Context, _ domain.ProviderContext, q domain.LogQuery) ([]domain.LogEntry, error) {
	p.logLimit = q.Limit
	return []domain.LogEntry{{Timestamp: q.From, Level: "INFO", Service: q.Service, Message: "matched"}}, nil
}
func (p *captureQueryProvider) QueryTraces(_ context.Context, _ domain.ProviderContext, q domain.TraceQuery) ([]domain.TraceSpan, error) {
	p.traceLimit = q.Limit
	return []domain.TraceSpan{{TraceID: "trace-1", SpanID: "span-1", Service: q.Service}}, nil
}

func TestQueryService_SearchLogsAndTraces(t *testing.T) {
	audit := &captureAudit{}
	evidence := &memEvidenceRepo{}
	svc := NewQueryService(stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Capabilities: []string{
			string(integdomain.CapabilityLogs),
			string(integdomain.CapabilityTraces),
		},
	}}, stubRegistry{p: stubProviderEntry{}}, evidence, audit)

	logs, err := svc.SearchLogs(context.Background(), Actor{UserID: "u1"}, domain.LogQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Service: "payment-service", Keyword: "timeout",
		From: 1710000000, To: 1710003600, Limit: 10,
	})
	if err != nil || len(logs.Entries) == 0 || logs.EvidenceID == "" {
		t.Fatalf("SearchLogs: err=%v out=%+v", err, logs)
	}
	if audit.last.Action != AuditLogsSearch {
		t.Fatalf("expected logs_search audit, got %q", audit.last.Action)
	}

	traces, err := svc.QueryTraces(context.Background(), Actor{UserID: "u1"}, domain.TraceQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Service: "payment-service", ErrorOnly: true, MinLatencyMS: 500,
		From: 1710000000, To: 1710003600, Limit: 10,
	})
	if err != nil || len(traces.Spans) == 0 || traces.EvidenceID == "" {
		t.Fatalf("QueryTraces: err=%v out=%+v", err, traces)
	}
	if !traces.Spans[0].Error {
		t.Fatal("expected error span when error_only=true")
	}
}

func TestQueryService_NormalizesQueriesAndRedactsLogKeywordEvidence(t *testing.T) {
	evidence := &memEvidenceRepo{}
	provider := &captureQueryProvider{}
	svc := NewQueryService(stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Capabilities: []string{
			string(integdomain.CapabilityMetrics),
			string(integdomain.CapabilityLogs),
			string(integdomain.CapabilityTraces),
		},
	}}, stubRegistry{p: provider}, evidence, nil)

	_, err := svc.QueryMetrics(context.Background(), Actor{}, domain.MetricQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Metric: "cpu_util", From: 1710000000, To: 1710000600,
	})
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if provider.metricPeriod != defaultMetricPeriod {
		t.Fatalf("expected normalized metric period %d, got %d", defaultMetricPeriod, provider.metricPeriod)
	}

	sensitiveKeyword := "token=secret-123456"
	_, err = svc.SearchLogs(context.Background(), Actor{}, domain.LogQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Service: "payment-service", Keyword: sensitiveKeyword,
		From: 1710000000, To: 1710000600,
	})
	if err != nil {
		t.Fatalf("SearchLogs: %v", err)
	}
	if provider.logLimit != defaultLogLimit {
		t.Fatalf("expected normalized log limit %d, got %d", defaultLogLimit, provider.logLimit)
	}
	if len(evidence.refs) < 2 {
		t.Fatalf("expected evidence records, got %d", len(evidence.refs))
	}
	rawSummary, err := json.Marshal(evidence.refs[len(evidence.refs)-1].Summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if strings.Contains(string(rawSummary), sensitiveKeyword) {
		t.Fatalf("evidence summary leaked raw keyword: %s", rawSummary)
	}
	keywordSummary, ok := evidence.refs[len(evidence.refs)-1].Summary["keyword"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured keyword summary, got %+v", evidence.refs[len(evidence.refs)-1].Summary["keyword"])
	}
	if keywordSummary["has_keyword"] != true || keywordSummary["keyword_hash"] == "" {
		t.Fatalf("expected keyword presence and hash, got %+v", keywordSummary)
	}

	_, err = svc.QueryTraces(context.Background(), Actor{}, domain.TraceQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Service: "payment-service", From: 1710000000, To: 1710000600,
	})
	if err != nil {
		t.Fatalf("QueryTraces: %v", err)
	}
	if provider.traceLimit != defaultTraceLimit {
		t.Fatalf("expected normalized trace limit %d, got %d", defaultTraceLimit, provider.traceLimit)
	}
}

func TestQueryService_RejectsExcessiveLimits(t *testing.T) {
	svc := NewQueryService(stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Capabilities: []string{
			string(integdomain.CapabilityLogs),
			string(integdomain.CapabilityTraces),
			string(integdomain.CapabilityAssets),
			string(integdomain.CapabilityAlerts),
		},
	}}, stubRegistry{p: stubProviderEntry{}}, nil, nil)

	cases := []struct {
		name string
		run  func() error
	}{
		{
			name: "logs",
			run: func() error {
				_, err := svc.SearchLogs(context.Background(), Actor{}, domain.LogQuery{
					AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
					From: 1710000000, To: 1710000600, Limit: maxLogLimit + 1,
				})
				return err
			},
		},
		{
			name: "traces",
			run: func() error {
				_, err := svc.QueryTraces(context.Background(), Actor{}, domain.TraceQuery{
					AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
					From: 1710000000, To: 1710000600, Limit: maxTraceLimit + 1,
				})
				return err
			},
		},
		{
			name: "assets",
			run: func() error {
				_, err := svc.ListResources(context.Background(), Actor{}, domain.AssetDiscoveryQuery{
					AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud), Limit: maxAssetLimit + 1,
				})
				return err
			},
		},
		{
			name: "alerts",
			run: func() error {
				_, err := svc.ListAlertRules(context.Background(), Actor{}, domain.AlertRuleQuery{
					AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud), Limit: maxAlertLimit + 1,
				})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("expected limit error")
			}
			if ae := apperr.FromError(err); ae.Code != apperr.CodeInvalidArgument {
				t.Fatalf("expected INVALID_ARGUMENT, got %s", ae.Code)
			}
		})
	}
}

func TestQueryService_QueryMetricsFakeProvider(t *testing.T) {
	audit := &captureAudit{}
	evidence := &memEvidenceRepo{}
	svc := NewQueryService(stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Capabilities: []string{string(integdomain.CapabilityMetrics)},
	}}, stubRegistry{p: stubProviderEntry{}}, evidence, audit)

	out, err := svc.QueryMetrics(context.Background(), Actor{UserID: "u1"}, domain.MetricQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Metric: "cpu_util", From: 1710000000, To: 1710003600, Period: 60,
	})
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if len(out.Series) == 0 || out.EvidenceID == "" {
		t.Fatalf("unexpected result: %+v", out)
	}
	if len(evidence.refs) != 1 {
		t.Fatalf("expected evidence persisted, got %d", len(evidence.refs))
	}
	if audit.last.Action != AuditMetricsQuery {
		t.Fatalf("expected metrics_query audit, got %q", audit.last.Action)
	}
}

func TestQueryService_AuditUsesResolvedProviderWhenRequestOmitsProvider(t *testing.T) {
	audit := &captureAudit{}
	svc := NewQueryService(stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Capabilities: []string{string(integdomain.CapabilityMetrics)},
	}}, stubRegistry{p: stubProviderEntry{}}, nil, audit)

	_, err := svc.QueryMetrics(context.Background(), Actor{UserID: "u1"}, domain.MetricQuery{
		AccountID: " acc-1 ", Metric: " cpu_util ", From: 1710000000, To: 1710003600, Period: 60,
	})
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if audit.last.Payload["provider"] != string(integdomain.ProviderHuaweiCloud) {
		t.Fatalf("expected resolved provider in audit payload, got %+v", audit.last.Payload)
	}
	if audit.last.Payload["account_id"] != "acc-1" {
		t.Fatalf("expected normalized account_id in audit payload, got %+v", audit.last.Payload)
	}
}

func TestQueryService_CapabilityDenied(t *testing.T) {
	svc := NewQueryService(stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID: "acc-1", Provider: string(integdomain.ProviderPrometheus),
		Capabilities: []string{string(integdomain.CapabilityMetrics)},
	}}, stubRegistry{p: stubProviderEntry{}}, nil, nil)

	_, err := svc.SearchLogs(context.Background(), Actor{}, domain.LogQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderPrometheus),
		From: 1710000000, To: 1710003600,
	})
	if err == nil {
		t.Fatal("expected capability error")
	}
	if ae := apperr.FromError(err); ae.Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected FAILED_PRECONDITION, got %s", ae.Code)
	}
}

func TestQueryService_ListResourcesAndAlerts(t *testing.T) {
	svc := NewQueryService(stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Capabilities: []string{
			string(integdomain.CapabilityAssets),
			string(integdomain.CapabilityAlerts),
		},
	}}, stubRegistry{p: stubProviderEntry{}}, nil, nil)

	resources, err := svc.ListResources(context.Background(), Actor{}, domain.AssetDiscoveryQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
	})
	if err != nil || len(resources.Resources) == 0 {
		t.Fatalf("ListResources: err=%v out=%+v", err, resources)
	}
	rules, err := svc.ListAlertRules(context.Background(), Actor{}, domain.AlertRuleQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
	})
	if err != nil || len(rules.Rules) == 0 {
		t.Fatalf("ListAlertRules: err=%v out=%+v", err, rules)
	}
}

func TestValidateMetricQuery_Limits(t *testing.T) {
	base := domain.MetricQuery{Metric: "cpu_util", From: 1710000000, To: 1710003600, Period: 60}
	if err := validateMetricQuery(base); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}
	tooWide := base
	tooWide.To = tooWide.From + maxMetricWindowSec + 1
	if err := validateMetricQuery(tooWide); err == nil {
		t.Fatal("expected window limit error")
	}
	tooManyPoints := domain.MetricQuery{Metric: "cpu_util", From: 1710000000, To: 1710000000 + 86400, Period: 60}
	if err := validateMetricQuery(tooManyPoints); err == nil {
		t.Fatal("expected point limit error")
	}
	shortPeriod := base
	shortPeriod.Period = 5
	if err := validateMetricQuery(shortPeriod); err == nil {
		t.Fatal("expected period minimum error")
	}
}

func TestQueryService_TopologyRequiresCapability(t *testing.T) {
	svc := NewQueryService(stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID: "acc-1", Provider: string(integdomain.ProviderPrometheus),
		Capabilities: []string{string(integdomain.CapabilityMetrics)},
	}}, stubRegistry{p: stubProviderEntry{}}, nil, nil)

	_, err := svc.QueryTopology(context.Background(), Actor{}, domain.TopologyQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderPrometheus),
		From: 1710000000, To: 1710003600,
	})
	if err == nil {
		t.Fatal("expected topology capability error")
	}
	if ae := apperr.FromError(err); ae.Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected FAILED_PRECONDITION, got %s", ae.Code)
	}
}
