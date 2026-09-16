package application

import (
	"context"
	"encoding/json"
	"errors"
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
func (stubProviderEntry) ListResources(context.Context, domain.ProviderContext, domain.AssetDiscoveryQuery) ([]domain.CloudResource, bool, error) {
	return []domain.CloudResource{{ResourceID: "res-1", Name: "demo", Type: "ecs", Region: "cn-north-4", Status: "running"}}, false, nil
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

// countingAccountPort 包装 stubAccountPort 并计数 ResolveAccount 调用，用于断言冻结快照路径是否跳过 DB 重读。
type countingAccountPort struct {
	inner stubAccountPort
	calls int
}

func (c *countingAccountPort) ResolveAccount(ctx context.Context, accountID string) (*domain.AccountSnapshot, error) {
	c.calls++
	return c.inner.ResolveAccount(ctx, accountID)
}

// fullSyncProvider 实现 CloudFullSyncPort，捕获收到的 ProviderContext.Account 用于断言冻结快照被透传。
type fullSyncProvider struct {
	capturedAccount domain.AccountSnapshot
}

func (fullSyncProvider) ProviderType() string { return string(integdomain.ProviderHuaweiCloud) }
func (f *fullSyncProvider) ListAllResources(_ context.Context, pctx domain.ProviderContext, _ AssetFullSyncQuery) ([]domain.CloudResource, *CloudSyncSummary, error) {
	f.capturedAccount = pctx.Account
	return []domain.CloudResource{{ResourceID: "res-1", Name: "demo", Type: "ecs", Region: "cn-north-4", Status: "running"}},
		&CloudSyncSummary{Region: "cn-north-4", Discovered: 1}, nil
}

// TestQueryService_ListAllResourcesUsesFrozenSnapshot 验证 q.Account 非 nil 时跳过 ResolveAccount（DB 重读），
// 直接用冻结快照构造 ProviderContext 透传给 provider，见 ops/huawei-ces-sync-contract.md §13.2。
func TestQueryService_ListAllResourcesUsesFrozenSnapshot(t *testing.T) {
	accounts := &countingAccountPort{inner: stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Capabilities: []string{string(integdomain.CapabilityAssets)},
	}}}
	provider := &fullSyncProvider{}
	svc := NewQueryService(accounts, stubRegistry{p: provider}, nil, nil)

	frozen := &domain.AccountSnapshot{
		AccountID:       "acc-1",
		Provider:        string(integdomain.ProviderHuaweiCloud),
		AuthType:        string(integdomain.AuthAKSK),
		ProjectID:       "pid-frozen",
		CredentialRefID: "cref-frozen",
		Capabilities:    []string{string(integdomain.CapabilityAssets)},
		ExtraConfig:     []byte(`{"sync_mode":"hybrid"}`),
	}
	out, err := svc.ListAllResources(context.Background(), Actor{UserID: "u1"}, AssetFullSyncQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud), Region: "cn-north-4",
		Account: frozen,
	})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if accounts.calls != 0 {
		t.Fatalf("expected ResolveAccount to be skipped when frozen snapshot provided, got %d calls", accounts.calls)
	}
	if provider.capturedAccount.ProjectID != "pid-frozen" {
		t.Fatalf("expected provider to receive frozen project_id=pid-frozen, got %q", provider.capturedAccount.ProjectID)
	}
	if provider.capturedAccount.CredentialRefID != "cref-frozen" {
		t.Fatalf("expected provider to receive frozen credential_ref_id, got %q", provider.capturedAccount.CredentialRefID)
	}
	if string(provider.capturedAccount.ExtraConfig) != `{"sync_mode":"hybrid"}` {
		t.Fatalf("expected provider to receive frozen extra_config, got %s", provider.capturedAccount.ExtraConfig)
	}
	if len(out.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(out.Resources))
	}
}

// TestQueryService_ListAllResourcesFallsBackToResolveAccount 验证 q.Account 为 nil 时回退 resolveEntry，
// 保持交互式/旧调用方行为不变。
func TestQueryService_ListAllResourcesFallsBackToResolveAccount(t *testing.T) {
	accounts := &countingAccountPort{inner: stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud),
		Capabilities: []string{string(integdomain.CapabilityAssets)},
	}}}
	provider := &fullSyncProvider{}
	svc := NewQueryService(accounts, stubRegistry{p: provider}, nil, nil)

	_, err := svc.ListAllResources(context.Background(), Actor{UserID: "u1"}, AssetFullSyncQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud), Region: "cn-north-4",
	})
	if err != nil {
		t.Fatalf("ListAllResources: %v", err)
	}
	if accounts.calls != 1 {
		t.Fatalf("expected ResolveAccount to be called once when no frozen snapshot, got %d calls", accounts.calls)
	}
}

// TestQueryService_ListAllResourcesUnsupportedProviderReturnsCapabilityError 验证 provider 未实现
// CloudFullSyncPort 时，ListAllResourcesDiscovery 返回包住 domain.ErrCapabilityUnsupported 的业务错误，
// 使 asset 层 isFullSyncUnsupported 能识别并回退 syncGeneric，见 ops/huawei-ces-sync-contract.md §13.2。
// 此用例直接覆盖真实 QueryService 路径：fakeDiscoveryPort 在 port.ListAllResources 内包 sentinel，
// 绕过了类型断言失败分支，无法覆盖该路径，导致该 bug 此前未被测试发现。
func TestQueryService_ListAllResourcesUnsupportedProviderReturnsCapabilityError(t *testing.T) {
	accounts := stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID:    "acc-1",
		Provider:     string(integdomain.ProviderHuaweiCloud),
		Capabilities: []string{string(integdomain.CapabilityAssets)},
	}}
	// stubProviderEntry 只实现 AssetDiscoveryPort，不实现 CloudFullSyncPort，
	// 触发 ListAllResourcesDiscovery 的类型断言失败分支。
	svc := NewQueryService(accounts, stubRegistry{p: stubProviderEntry{}}, nil, nil)

	_, err := svc.ListAllResources(context.Background(), Actor{UserID: "u1"}, AssetFullSyncQuery{
		AccountID: "acc-1", Provider: string(integdomain.ProviderHuaweiCloud), Region: "cn-north-4",
	})
	if err == nil {
		t.Fatal("expected error when provider does not implement CloudFullSyncPort, got nil")
	}
	if !errors.Is(err, domain.ErrCapabilityUnsupported) {
		t.Fatalf("expected error to wrap domain.ErrCapabilityUnsupported so asset sync can fall back to generic, got %v", err)
	}
	if apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("expected CodeFailedPrecondition, got %s", apperr.CodeOf(err))
	}
}
