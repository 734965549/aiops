package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

const (
	maxMetricWindowSec  = 7 * 24 * 3600 // 7d
	minMetricPeriodSec  = 10
	defaultMetricPeriod = 60
	maxMetricPoints     = 1440
	defaultLogLimit     = 100
	maxLogLimit         = 500
	defaultTraceLimit   = 50
	maxTraceLimit       = 1000
	defaultAssetLimit   = 100
	maxAssetLimit       = 500
	defaultAlertLimit   = 100
	maxAlertLimit       = 500
)

// QueryService 统一观测查询编排：账号解析 -> 能力校验 -> Provider Port -> 证据引用 -> 审计。
type QueryService struct {
	accounts  IntegrationAccountPort
	providers ProviderRegistry
	evidence  domain.EvidenceRepository
	audit     AuditRecorder
}

func NewQueryService(
	accounts IntegrationAccountPort,
	providers ProviderRegistry,
	evidence domain.EvidenceRepository,
	audit AuditRecorder,
) *QueryService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &QueryService{accounts: accounts, providers: providers, evidence: evidence, audit: audit}
}

func (s *QueryService) QueryMetrics(ctx context.Context, actor Actor, q domain.MetricQuery) (*MetricQueryResult, error) {
	normalized, err := normalizeMetricQuery(q)
	if err != nil {
		return nil, err
	}
	q = normalized
	pctx, entry, err := s.resolveEntry(ctx, q.AccountID, q.Provider, integdomain.CapabilityMetrics)
	if err != nil {
		return nil, err
	}
	port, ok := entry.(MetricQueryPort)
	if !ok {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider does not support metrics")
	}
	series, err := port.QueryMetrics(ctx, pctx, q)
	if err != nil {
		return nil, wrapProviderError(err, "query metrics failed")
	}
	evidenceID, err := s.persistEvidence(ctx, q.AccountID, "metrics", q, map[string]any{
		"metric": q.Metric, "namespace": q.Namespace, "series_count": len(series),
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "observability_query",
		ResourceID:   evidenceID,
		Action:       AuditMetricsQuery,
		UserID:       actor.UserID,
		Payload: map[string]any{
			"account_id": q.AccountID, "provider": q.Provider, "metric": q.Metric,
			"namespace": q.Namespace, "series_count": len(series), "evidence_id": evidenceID,
		},
	})
	return &MetricQueryResult{Series: series, EvidenceID: evidenceID}, nil
}

func (s *QueryService) SearchLogs(ctx context.Context, actor Actor, q domain.LogQuery) (*LogSearchResult, error) {
	normalized, err := normalizeLogQuery(q)
	if err != nil {
		return nil, err
	}
	q = normalized
	pctx, entry, err := s.resolveEntry(ctx, q.AccountID, q.Provider, integdomain.CapabilityLogs)
	if err != nil {
		return nil, err
	}
	port, ok := entry.(LogSearchPort)
	if !ok {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider does not support logs")
	}
	entries, err := port.SearchLogs(ctx, pctx, q)
	if err != nil {
		return nil, wrapProviderError(err, "search logs failed")
	}
	evidenceID, err := s.persistEvidence(ctx, q.AccountID, "logs", q, map[string]any{
		"service": q.Service, "entry_count": len(entries),
		"keyword": keywordEvidenceSummary(q.Keyword),
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "observability_query",
		ResourceID:   evidenceID,
		Action:       AuditLogsSearch,
		UserID:       actor.UserID,
		Payload: map[string]any{
			"account_id": q.AccountID, "provider": q.Provider, "service": q.Service,
			"entry_count": len(entries), "evidence_id": evidenceID,
		},
	})
	return &LogSearchResult{Entries: entries, EvidenceID: evidenceID}, nil
}

func (s *QueryService) QueryTraces(ctx context.Context, actor Actor, q domain.TraceQuery) (*TraceQueryResult, error) {
	normalized, err := normalizeTraceQuery(q)
	if err != nil {
		return nil, err
	}
	q = normalized
	pctx, entry, err := s.resolveEntry(ctx, q.AccountID, q.Provider, integdomain.CapabilityTraces)
	if err != nil {
		return nil, err
	}
	port, ok := entry.(TraceQueryPort)
	if !ok {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider does not support traces")
	}
	spans, err := port.QueryTraces(ctx, pctx, q)
	if err != nil {
		return nil, wrapProviderError(err, "query traces failed")
	}
	evidenceID, err := s.persistEvidence(ctx, q.AccountID, "traces", q, map[string]any{
		"service": q.Service, "operation": q.Operation, "span_count": len(spans),
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "observability_query",
		ResourceID:   evidenceID,
		Action:       AuditTracesQuery,
		UserID:       actor.UserID,
		Payload: map[string]any{
			"account_id": q.AccountID, "provider": q.Provider, "service": q.Service,
			"span_count": len(spans), "evidence_id": evidenceID,
		},
	})
	return &TraceQueryResult{Spans: spans, EvidenceID: evidenceID}, nil
}

func (s *QueryService) QueryTopology(ctx context.Context, actor Actor, q domain.TopologyQuery) (*TopologyQueryResult, error) {
	if err := validateTopologyQuery(q); err != nil {
		return nil, err
	}
	pctx, entry, err := s.resolveEntry(ctx, q.AccountID, q.Provider, integdomain.CapabilityTopology)
	if err != nil {
		return nil, err
	}
	port, ok := entry.(TopologyQueryPort)
	if !ok {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider does not support topology")
	}
	topology, err := port.QueryTopology(ctx, pctx, q)
	if err != nil {
		return nil, wrapProviderError(err, "query topology failed")
	}
	if topology == nil {
		topology = &domain.TopologySnapshot{}
	}
	evidenceID, err := s.persistEvidence(ctx, q.AccountID, "topology", q, map[string]any{
		"application_id": q.ApplicationID, "node_count": len(topology.Nodes), "edge_count": len(topology.Edges),
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "observability_query",
		ResourceID:   evidenceID,
		Action:       AuditTopologyGet,
		UserID:       actor.UserID,
		Payload: map[string]any{
			"account_id": q.AccountID, "provider": q.Provider, "application_id": q.ApplicationID,
			"node_count": len(topology.Nodes), "edge_count": len(topology.Edges), "evidence_id": evidenceID,
		},
	})
	return &TopologyQueryResult{Topology: *topology, EvidenceID: evidenceID}, nil
}

func (s *QueryService) ListResources(ctx context.Context, actor Actor, q domain.AssetDiscoveryQuery) (*AssetDiscoveryResult, error) {
	normalized, err := normalizeAssetQuery(q)
	if err != nil {
		return nil, err
	}
	q = normalized
	pctx, entry, err := s.resolveEntry(ctx, q.AccountID, q.Provider, integdomain.CapabilityAssets)
	if err != nil {
		return nil, err
	}
	port, ok := entry.(AssetDiscoveryPort)
	if !ok {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider does not support asset discovery")
	}
	resources, err := port.ListResources(ctx, pctx, q)
	if err != nil {
		return nil, wrapProviderError(err, "list resources failed")
	}
	evidenceID, err := s.persistEvidence(ctx, q.AccountID, "assets", q, map[string]any{
		"resource_type": q.ResourceType, "resource_count": len(resources),
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "observability_query",
		ResourceID:   evidenceID,
		Action:       AuditResourcesList,
		UserID:       actor.UserID,
		Payload: map[string]any{
			"account_id": q.AccountID, "provider": q.Provider,
			"resource_count": len(resources), "evidence_id": evidenceID,
		},
	})
	return &AssetDiscoveryResult{Resources: resources, EvidenceID: evidenceID}, nil
}

func (s *QueryService) ListAlertRules(ctx context.Context, actor Actor, q domain.AlertRuleQuery) (*AlertRuleQueryResult, error) {
	normalized, err := normalizeAlertQuery(q)
	if err != nil {
		return nil, err
	}
	q = normalized
	pctx, entry, err := s.resolveEntry(ctx, q.AccountID, q.Provider, integdomain.CapabilityAlerts)
	if err != nil {
		return nil, err
	}
	port, ok := entry.(AlertRuleQueryPort)
	if !ok {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider does not support alert rules")
	}
	rules, err := port.ListAlertRules(ctx, pctx, q)
	if err != nil {
		return nil, wrapProviderError(err, "list alert rules failed")
	}
	evidenceID, err := s.persistEvidence(ctx, q.AccountID, "alerts", q, map[string]any{
		"namespace": q.Namespace, "rule_count": len(rules),
	})
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "observability_query",
		ResourceID:   evidenceID,
		Action:       AuditAlertsList,
		UserID:       actor.UserID,
		Payload: map[string]any{
			"account_id": q.AccountID, "provider": q.Provider,
			"rule_count": len(rules), "evidence_id": evidenceID,
		},
	})
	return &AlertRuleQueryResult{Rules: rules, EvidenceID: evidenceID}, nil
}

func (s *QueryService) resolveEntry(ctx context.Context, accountID, provider string, cap integdomain.Capability) (domain.ProviderContext, ProviderEntry, error) {
	if s == nil || s.accounts == nil || s.providers == nil {
		return domain.ProviderContext{}, nil, apperr.New(apperr.CodeUnavailable, "observability query service is not enabled")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.ProviderContext{}, nil, apperr.New(apperr.CodeInvalidArgument, "account_id is required")
	}
	acc, err := s.accounts.ResolveAccount(ctx, accountID)
	if err != nil {
		return domain.ProviderContext{}, nil, wrapAccountError(err)
	}
	if acc == nil {
		return domain.ProviderContext{}, nil, apperr.New(apperr.CodeNotFound, "integration account not found")
	}
	wantProvider := strings.TrimSpace(provider)
	if wantProvider == "" {
		wantProvider = acc.Provider
	}
	if wantProvider != acc.Provider {
		return domain.ProviderContext{}, nil, apperr.New(apperr.CodeInvalidArgument, "provider mismatch with account")
	}
	if !hasCapability(acc.Capabilities, string(cap)) {
		return domain.ProviderContext{}, nil, apperr.New(apperr.CodeFailedPrecondition, "account does not support requested capability")
	}
	entry, err := s.providers.Get(wantProvider)
	if err != nil {
		return domain.ProviderContext{}, nil, err
	}
	return domain.ProviderContext{Account: *acc}, entry, nil
}

func (s *QueryService) persistEvidence(ctx context.Context, accountID, queryType string, query any, summary map[string]any) (string, error) {
	evidenceID := "ev-" + uuid.NewString()
	if s.evidence == nil {
		return evidenceID, nil
	}
	hash, err := hashQuery(query)
	if err != nil {
		return "", apperr.Wrap(err, apperr.CodeInternal, "hash query failed")
	}
	now := time.Now()
	ref := &domain.EvidenceRef{
		EvidenceID: evidenceID,
		AccountID:  accountID,
		QueryType:  queryType,
		QueryHash:  hash,
		Summary:    summary,
	}
	if err := s.evidence.Create(ctx, ref); err != nil {
		return "", apperr.Wrap(err, apperr.CodeInternal, "persist evidence failed")
	}
	_ = now
	return evidenceID, nil
}

func normalizeMetricQuery(q domain.MetricQuery) (domain.MetricQuery, error) {
	if strings.TrimSpace(q.Metric) == "" {
		return domain.MetricQuery{}, apperr.New(apperr.CodeInvalidArgument, "metric is required")
	}
	if q.From <= 0 || q.To <= 0 || q.From >= q.To {
		return domain.MetricQuery{}, apperr.New(apperr.CodeInvalidArgument, "invalid time range")
	}
	window := q.To - q.From
	if window > maxMetricWindowSec {
		return domain.MetricQuery{}, apperr.New(apperr.CodeInvalidArgument, "time range exceeds maximum 7 days")
	}
	period := q.Period
	if period <= 0 {
		period = defaultMetricPeriod
	}
	if period < minMetricPeriodSec {
		return domain.MetricQuery{}, apperr.New(apperr.CodeInvalidArgument, "period below minimum 10 seconds")
	}
	points := window/int64(period) + 1
	if points > maxMetricPoints {
		return domain.MetricQuery{}, apperr.New(apperr.CodeInvalidArgument, "too many data points; narrow time range or increase period")
	}
	q.Period = period
	return q, nil
}

func validateMetricQuery(q domain.MetricQuery) error {
	_, err := normalizeMetricQuery(q)
	return err
}

func normalizeLogQuery(q domain.LogQuery) (domain.LogQuery, error) {
	if q.From <= 0 || q.To <= 0 || q.From >= q.To {
		return domain.LogQuery{}, apperr.New(apperr.CodeInvalidArgument, "invalid time range")
	}
	if q.Limit <= 0 {
		q.Limit = defaultLogLimit
	}
	if q.Limit > maxLogLimit {
		return domain.LogQuery{}, apperr.New(apperr.CodeInvalidArgument, "limit exceeds maximum 500")
	}
	return q, nil
}

func normalizeTraceQuery(q domain.TraceQuery) (domain.TraceQuery, error) {
	if q.From <= 0 || q.To <= 0 || q.From >= q.To {
		return domain.TraceQuery{}, apperr.New(apperr.CodeInvalidArgument, "invalid time range")
	}
	if q.Limit <= 0 {
		q.Limit = defaultTraceLimit
	}
	if q.Limit > maxTraceLimit {
		return domain.TraceQuery{}, apperr.New(apperr.CodeInvalidArgument, "limit exceeds maximum 1000")
	}
	return q, nil
}

func validateTopologyQuery(q domain.TopologyQuery) error {
	if q.From <= 0 || q.To <= 0 || q.From >= q.To {
		return apperr.New(apperr.CodeInvalidArgument, "invalid time range")
	}
	return nil
}

func normalizeAssetQuery(q domain.AssetDiscoveryQuery) (domain.AssetDiscoveryQuery, error) {
	if q.Limit <= 0 {
		q.Limit = defaultAssetLimit
	}
	if q.Limit > maxAssetLimit {
		return domain.AssetDiscoveryQuery{}, apperr.New(apperr.CodeInvalidArgument, "limit exceeds maximum 500")
	}
	return q, nil
}

func normalizeAlertQuery(q domain.AlertRuleQuery) (domain.AlertRuleQuery, error) {
	if q.Limit <= 0 {
		q.Limit = defaultAlertLimit
	}
	if q.Limit > maxAlertLimit {
		return domain.AlertRuleQuery{}, apperr.New(apperr.CodeInvalidArgument, "limit exceeds maximum 500")
	}
	return q, nil
}

func hasCapability(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

func hashQuery(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func keywordEvidenceSummary(keyword string) map[string]any {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return map[string]any{"has_keyword": false, "keyword_len": 0}
	}
	sum := sha256.Sum256([]byte(keyword))
	return map[string]any{
		"has_keyword":  true,
		"keyword_len":  len([]rune(keyword)),
		"keyword_hash": hex.EncodeToString(sum[:]),
	}
}

func wrapAccountError(err error) error {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae
	}
	return apperr.MapSentinels(err, "resolve integration account failed",
		apperr.Sentinel{Err: integdomain.ErrNotFound, Code: apperr.CodeNotFound},
		apperr.Sentinel{Err: integdomain.ErrAccountDisabled, Code: apperr.CodeFailedPrecondition},
	)
}

func wrapProviderError(err error, op string) error {
	if err == nil {
		return nil
	}
	mapped := apperr.MapSentinels(err, op,
		apperr.Sentinel{Err: domain.ErrInvalidArgument, Code: apperr.CodeInvalidArgument},
		apperr.Sentinel{Err: domain.ErrCapabilityUnsupported, Code: apperr.CodeFailedPrecondition},
		apperr.Sentinel{Err: domain.ErrUnsupportedProvider, Code: apperr.CodeFailedPrecondition},
		apperr.Sentinel{Err: domain.ErrProviderUnavailable, Code: apperr.CodeUnavailable},
	)
	if ae := apperr.FromError(mapped); ae.Code != apperr.CodeInternal {
		return mapped
	}
	return apperr.Wrap(err, apperr.CodeInternal, op)
}
