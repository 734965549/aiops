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
	cesFullSync CESResourceDiscoveryClient
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
		cesFullSync: NewCESResourceClient(),
	}
}

// WithCESResourceDiscovery 注入自定义 CES 资源发现客户端，主要用于测试。
func (a *Adapter) WithCESResourceDiscovery(client CESResourceDiscoveryClient) *Adapter {
	if client != nil {
		a.cesFullSync = client
	}
	return a
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

// ListAllResources 云资源全量同步发现，按 sync_mode 路由，见 docs/huawei-ces-asset-sync-plan.md §7.3。
// auth_type=ak_sk: ces/hybrid 走 CES 资源分组全量发现；native 走旧 ECS/CCE/RDS/ELB resource client。
// auth_type=none: 委托 fake；auth_type=agency: 阶段一返回 unsupported。
func (a *Adapter) ListAllResources(ctx context.Context, pctx domain.ProviderContext, q obsapp.AssetFullSyncQuery) ([]domain.CloudResource, *obsapp.CloudSyncSummary, error) {
	authType := integdomain.AuthType(strings.TrimSpace(pctx.Account.AuthType))
	switch authType {
	case integdomain.AuthNone:
		resources, err := a.inner.ListResources(ctx, pctx, domain.AssetDiscoveryQuery{
			AccountID: q.AccountID, Provider: q.Provider, Region: q.Region, Limit: q.MaxResources,
		})
		if err != nil {
			return nil, nil, err
		}
		return resources, &obsapp.CloudSyncSummary{Region: q.Region, Discovered: len(resources)}, nil
	case integdomain.AuthAKSK:
		return a.listAllResourcesReal(ctx, pctx, q)
	case integdomain.AuthAgency:
		return nil, nil, apperr.Wrap(domain.ErrCapabilityUnsupported, apperr.CodeFailedPrecondition, "huawei full sync with agency auth is not implemented yet")
	default:
		return nil, nil, apperr.New(apperr.CodeFailedPrecondition, "unsupported auth type for huawei full sync")
	}
}

// listAllResourcesReal 真实账号全量发现，按 SyncModeConfig 路由 ces/native。
func (a *Adapter) listAllResourcesReal(ctx context.Context, pctx domain.ProviderContext, q obsapp.AssetFullSyncQuery) ([]domain.CloudResource, *obsapp.CloudSyncSummary, error) {
	if a == nil || a.credentials == nil {
		return nil, nil, apperr.New(apperr.CodeUnavailable, "huawei resource adapter is not configured")
	}
	region := strings.TrimSpace(q.Region)
	if region == "" && len(pctx.Account.Regions) > 0 {
		region = strings.TrimSpace(pctx.Account.Regions[0])
	}
	if region == "" {
		return nil, nil, apperr.New(apperr.CodeInvalidArgument, "region is required")
	}
	projectID := strings.TrimSpace(pctx.Account.ProjectID)
	if projectID == "" {
		return nil, nil, apperr.New(apperr.CodeInvalidArgument, "project_id is required")
	}
	cred, err := a.credentials.ResolveAKSK(ctx, pctx.Account)
	if err != nil {
		return nil, nil, err
	}
	cfg := ParseSyncModeConfig(pctx.Account.ExtraConfig)
	switch cfg.Mode {
	case SyncModeNative:
		return a.listAllResourcesNative(ctx, cred, projectID, region, q, cfg)
	default:
		// ces / hybrid / 未知值均走 CES 全量发现。hybrid 的原生 API 增强留待阶段2。
		return a.listAllResourcesCES(ctx, cred, projectID, region, q, cfg)
	}
}

// listAllResourcesCES 走 CES 资源分组全量发现，见 §8.1。
func (a *Adapter) listAllResourcesCES(ctx context.Context, cred AKSKCredential, projectID, region string, q obsapp.AssetFullSyncQuery, cfg SyncModeConfig) ([]domain.CloudResource, *obsapp.CloudSyncSummary, error) {
	if a.cesFullSync == nil {
		return nil, nil, apperr.New(apperr.CodeUnavailable, "huawei ces resource discovery client is not configured")
	}
	req := CESResourceDiscoveryRequest{
		ProjectID:           projectID,
		Region:              region,
		EnterpriseProjectID: cfg.EnterpriseProjectID,
		ResourceGroupName:   cfg.ResourceGroupName,
		ResourceGroupID:     cfg.ResourceGroupID,
		MaxResources:        effectiveMaxResources(q.MaxResources, cfg.MaxResources),
	}
	result, err := a.cesFullSync.ListCESResources(ctx, cred, req)
	if err != nil {
		return nil, nil, err
	}
	summary := &obsapp.CloudSyncSummary{
		Region:                region,
		ResourceGroupID:       result.Summary.ResourceGroupID,
		ResourceGroupName:     result.Summary.ResourceGroupName,
		CESTotal:              result.Summary.CESTotal,
		Discovered:            result.Summary.Discovered,
		FailedScopes:          append([]string(nil), result.Summary.FailedScopes...),
		SuccessfulTypes:       append([]string(nil), result.Summary.SuccessfulTypes...),
		UnknownNamespaceCount: result.Summary.UnknownNamespaceCount,
		InvalidResourceCount:  result.Summary.InvalidResourceCount,
		ProductNamesEmpty:     result.Summary.ProductNamesEmpty,
	}
	return result.Resources, summary, nil
}

// listAllResourcesNative 兼容旧 ECS/CCE/RDS/ELB resource client，见 §8.3。
func (a *Adapter) listAllResourcesNative(ctx context.Context, cred AKSKCredential, projectID, region string, q obsapp.AssetFullSyncQuery, cfg SyncModeConfig) ([]domain.CloudResource, *obsapp.CloudSyncSummary, error) {
	if a.resources == nil {
		return nil, nil, apperr.New(apperr.CodeUnavailable, "huawei resource client is not configured")
	}
	limit := effectiveMaxResources(q.MaxResources, cfg.MaxResources)
	resources, err := a.resources.ListResources(ctx, cred, projectID, region, "", limit)
	if err != nil {
		return nil, nil, err
	}
	return resources, &obsapp.CloudSyncSummary{Region: region, Discovered: len(resources)}, nil
}

func effectiveMaxResources(requested, configured int) int {
	if configured <= 0 {
		configured = defaultMaxResources
	}
	if requested > 0 && requested < configured {
		return requested
	}
	return configured
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
	_ obsapp.CloudFullSyncPort  = (*Adapter)(nil)
	_ obsapp.AlertRuleQueryPort = (*Adapter)(nil)
)
