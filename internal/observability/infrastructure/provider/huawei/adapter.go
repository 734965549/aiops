package huawei

import (
	"context"
	"fmt"
	"strings"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	"github.com/734965549/aiops/internal/observability/infrastructure/provider/fake"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// Adapter 华为云只读观测 Adapter；auth_type=ak_sk 时指标与资源发现走真实 API，其余真实凭据账号的非指标能力返回 unsupported。
type Adapter struct {
	inner       *fake.Provider
	credentials *CredentialProvider
	ces         MetricDataClient
	resources   ResourceDiscoveryClient
}

func NewAdapter(credentials *CredentialProvider, ces MetricDataClient, resources ResourceDiscoveryClient) *Adapter {
	if ces == nil {
		ces = NewCESClient()
	}
	if resources == nil {
		resources = NewResourceClient()
	}
	return &Adapter{
		inner:       fake.New(string(integdomain.ProviderHuaweiCloud)),
		credentials: credentials,
		ces:         ces,
		resources:   resources,
	}
}

func (a *Adapter) ProviderType() string { return string(integdomain.ProviderHuaweiCloud) }

func (a *Adapter) QueryMetrics(ctx context.Context, pctx domain.ProviderContext, q domain.MetricQuery) ([]domain.MetricSeries, error) {
	authType := integdomain.AuthType(strings.TrimSpace(pctx.Account.AuthType))
	if authType == integdomain.AuthAKSK {
		return a.queryMetricsCES(ctx, pctx, q)
	}
	return a.inner.QueryMetrics(ctx, pctx, q)
}

func (a *Adapter) queryMetricsCES(ctx context.Context, pctx domain.ProviderContext, q domain.MetricQuery) ([]domain.MetricSeries, error) {
	if a == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "huawei adapter is not configured")
	}
	if err := validateCESMetricQuery(pctx, q); err != nil {
		return nil, err
	}
	if a.credentials == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "huawei credential provider is not configured")
	}
	cesQuery, err := MapMetricQuery(q)
	if err != nil {
		return nil, err
	}
	cred, err := a.credentials.ResolveAKSK(ctx, pctx.Account)
	if err != nil {
		return nil, err
	}
	region := strings.TrimSpace(q.Region)
	projectID := strings.TrimSpace(pctx.Account.ProjectID)
	result, err := a.ces.QueryMetricData(ctx, cred, projectID, region, cesQuery)
	if err != nil {
		return nil, err
	}
	return MapMetricDataResult(q, result), nil
}

func (a *Adapter) SearchLogs(ctx context.Context, pctx domain.ProviderContext, q domain.LogQuery) ([]domain.LogEntry, error) {
	if err := a.requireFakeOnlyCapability(pctx, "logs"); err != nil {
		return nil, err
	}
	return a.inner.SearchLogs(ctx, pctx, q)
}

func (a *Adapter) QueryTraces(ctx context.Context, pctx domain.ProviderContext, q domain.TraceQuery) ([]domain.TraceSpan, error) {
	if err := a.requireFakeOnlyCapability(pctx, "traces"); err != nil {
		return nil, err
	}
	return a.inner.QueryTraces(ctx, pctx, q)
}

func (a *Adapter) QueryTopology(ctx context.Context, pctx domain.ProviderContext, q domain.TopologyQuery) (*domain.TopologySnapshot, error) {
	if err := a.requireFakeOnlyCapability(pctx, "topology"); err != nil {
		return nil, err
	}
	return a.inner.QueryTopology(ctx, pctx, q)
}

func (a *Adapter) ListResources(ctx context.Context, pctx domain.ProviderContext, q domain.AssetDiscoveryQuery) ([]domain.CloudResource, error) {
	authType := integdomain.AuthType(strings.TrimSpace(pctx.Account.AuthType))
	switch authType {
	case integdomain.AuthNone:
		return a.inner.ListResources(ctx, pctx, q)
	case integdomain.AuthAKSK:
		return a.listResourcesReal(ctx, pctx, q)
	case integdomain.AuthAgency:
		return nil, apperr.Wrap(domain.ErrCapabilityUnsupported, apperr.CodeFailedPrecondition, "huawei assets query with agency auth is not implemented yet")
	default:
		return nil, apperr.New(apperr.CodeFailedPrecondition, "unsupported auth type for huawei observability")
	}
}

func (a *Adapter) listResourcesReal(ctx context.Context, pctx domain.ProviderContext, q domain.AssetDiscoveryQuery) ([]domain.CloudResource, error) {
	if a == nil || a.credentials == nil || a.resources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "huawei resource adapter is not configured")
	}
	region := strings.TrimSpace(q.Region)
	if region == "" && len(pctx.Account.Regions) > 0 {
		region = strings.TrimSpace(pctx.Account.Regions[0])
	}
	if region == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "region is required")
	}
	projectID := strings.TrimSpace(pctx.Account.ProjectID)
	if projectID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "project_id is required")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	cred, err := a.credentials.ResolveAKSK(ctx, pctx.Account)
	if err != nil {
		return nil, err
	}
	resources, err := a.resources.ListResources(ctx, cred, projectID, region, q.ResourceType, limit)
	if err != nil {
		return nil, err
	}
	keyword := strings.ToLower(strings.TrimSpace(q.Keyword))
	if keyword == "" {
		return resources, nil
	}
	filtered := make([]domain.CloudResource, 0, len(resources))
	for _, item := range resources {
		if strings.Contains(strings.ToLower(item.Name), keyword) ||
			strings.Contains(strings.ToLower(item.ProviderRef), keyword) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (a *Adapter) ListAlertRules(ctx context.Context, pctx domain.ProviderContext, q domain.AlertRuleQuery) ([]domain.AlertRule, error) {
	if err := a.requireFakeOnlyCapability(pctx, "alerts"); err != nil {
		return nil, err
	}
	return a.inner.ListAlertRules(ctx, pctx, q)
}

// requireFakeOnlyCapability 已配置真实凭据的账号不得返回 fake 样本，避免误当作云端数据。
func (a *Adapter) requireFakeOnlyCapability(pctx domain.ProviderContext, capability string) error {
	authType := integdomain.AuthType(strings.TrimSpace(pctx.Account.AuthType))
	switch authType {
	case integdomain.AuthNone:
		return nil
	case integdomain.AuthAKSK, integdomain.AuthAgency:
		return apperr.Wrap(domain.ErrCapabilityUnsupported, apperr.CodeFailedPrecondition,
			fmt.Sprintf("huawei %s query is not implemented yet", capability))
	default:
		return apperr.New(apperr.CodeFailedPrecondition, "unsupported auth type for huawei observability")
	}
}

var (
	_ obsapp.ProviderEntry      = (*Adapter)(nil)
	_ obsapp.MetricQueryPort    = (*Adapter)(nil)
	_ obsapp.LogSearchPort      = (*Adapter)(nil)
	_ obsapp.TraceQueryPort     = (*Adapter)(nil)
	_ obsapp.TopologyQueryPort  = (*Adapter)(nil)
	_ obsapp.AssetDiscoveryPort = (*Adapter)(nil)
	_ obsapp.AlertRuleQueryPort = (*Adapter)(nil)
)
