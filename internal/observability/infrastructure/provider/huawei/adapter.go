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
	"github.com/734965549/aiops/pkg/logger"
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
	cfg := ParseSyncModeConfig(pctx.Account.ExtraConfig)
	projectID := cfg.ResolveProjectID(region, strings.TrimSpace(pctx.Account.ProjectID))
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

func (a *Adapter) ListResources(ctx context.Context, pctx domain.ProviderContext, q domain.AssetDiscoveryQuery) ([]domain.CloudResource, bool, error) {
	authType := integdomain.AuthType(strings.TrimSpace(pctx.Account.AuthType))
	switch authType {
	case integdomain.AuthNone:
		resources, _, err := a.inner.ListResources(ctx, pctx, q)
		return resources, false, err
	case integdomain.AuthAKSK:
		return a.listResourcesReal(ctx, pctx, q)
	case integdomain.AuthAgency:
		return nil, false, apperr.Wrap(domain.ErrCapabilityUnsupported, apperr.CodeFailedPrecondition, "huawei assets query with agency auth is not implemented yet")
	default:
		return nil, false, apperr.New(apperr.CodeFailedPrecondition, "unsupported auth type for huawei observability")
	}
}

func (a *Adapter) listResourcesReal(ctx context.Context, pctx domain.ProviderContext, q domain.AssetDiscoveryQuery) ([]domain.CloudResource, bool, error) {
	if a == nil || a.credentials == nil || a.resources == nil {
		return nil, false, apperr.New(apperr.CodeUnavailable, "huawei resource adapter is not configured")
	}
	region := strings.TrimSpace(q.Region)
	if region == "" && len(pctx.Account.Regions) > 0 {
		region = strings.TrimSpace(pctx.Account.Regions[0])
	}
	if region == "" {
		return nil, false, apperr.New(apperr.CodeInvalidArgument, "region is required")
	}
	cfg := ParseSyncModeConfig(pctx.Account.ExtraConfig)
	projectID := cfg.ResolveProjectID(region, strings.TrimSpace(pctx.Account.ProjectID))
	if projectID == "" {
		return nil, false, apperr.New(apperr.CodeInvalidArgument, "project_id is required")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	cred, err := a.credentials.ResolveAKSK(ctx, pctx.Account)
	if err != nil {
		return nil, false, err
	}
	// 请求 limit+1 探测截断：返回超过 limit 条说明云端仍有更多资源，
	// 标记 hasMore 并截断到 limit，避免 Asset Sync 通用路径把未返回资源误标 stale，见 §13.1。
	resources, err := a.resources.ListResources(ctx, cred, projectID, region, q.ResourceType, limit+1)
	if err != nil {
		return nil, false, err
	}
	hasMore := len(resources) > limit
	if hasMore {
		resources = resources[:limit]
	}
	keyword := strings.ToLower(strings.TrimSpace(q.Keyword))
	if keyword == "" {
		return resources, hasMore, nil
	}
	// 关键字过滤后 hasMore 语义不再可靠（已取出的子集被进一步收窄），保守返回 false。
	filtered := make([]domain.CloudResource, 0, len(resources))
	for _, item := range resources {
		if strings.Contains(strings.ToLower(item.Name), keyword) ||
			strings.Contains(strings.ToLower(item.ProviderRef), keyword) {
			filtered = append(filtered, item)
		}
	}
	return filtered, false, nil
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
		resources, _, err := a.inner.ListResources(ctx, pctx, domain.AssetDiscoveryQuery{
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
	cred, err := a.credentials.ResolveAKSK(ctx, pctx.Account)
	if err != nil {
		return nil, nil, err
	}
	cfg := ParseSyncModeConfig(pctx.Account.ExtraConfig)
	projectID := cfg.ResolveProjectID(region, strings.TrimSpace(pctx.Account.ProjectID))
	if projectID == "" {
		return nil, nil, apperr.New(apperr.CodeInvalidArgument, "project_id is required")
	}
	switch cfg.Mode {
	case SyncModeNative:
		return a.listAllResourcesNative(ctx, cred, projectID, region, q, cfg)
	case SyncModeHybrid:
		return a.listAllResourcesHybrid(ctx, cred, projectID, region, q, cfg)
	default:
		// ces / 未知值均走 CES 全量发现。
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
		Region:                 region,
		ProjectID:              projectID,
		SyncMode:               cfg.Mode,
		ResourceGroupID:        result.Summary.ResourceGroupID,
		ResourceGroupName:      result.Summary.ResourceGroupName,
		ResourceGroupSelection: result.Summary.ResourceGroupSelection,
		CESTotal:               result.Summary.CESTotal,
		Discovered:             result.Summary.Discovered,
		FailedScopes:           append([]string(nil), result.Summary.FailedScopes...),
		SuccessfulTypes:        append([]string(nil), result.Summary.SuccessfulTypes...),
		QueryFailedTypes:       append([]string(nil), result.Summary.QueryFailedTypes...),
		UnknownNamespaceCount:  result.Summary.UnknownNamespaceCount,
		InvalidResourceCount:   result.Summary.InvalidResourceCount,
		ConversionFailedTypes:  append([]string(nil), result.Summary.ConversionFailedTypes...),
		ProductNamesEmpty:      result.Summary.ProductNamesEmpty,
		MaxResourcesReached:    result.Summary.MaxResourcesReached,
	}
	return result.Resources, summary, nil
}

// listAllResourcesHybrid 先执行 CES 全量发现，再用原生 API 增强详情，见 §8.2。
// 增强失败不影响基础资源返回，只记录 EnrichmentFailedTypes。
func (a *Adapter) listAllResourcesHybrid(ctx context.Context, cred AKSKCredential, projectID, region string, q obsapp.AssetFullSyncQuery, cfg SyncModeConfig) ([]domain.CloudResource, *obsapp.CloudSyncSummary, error) {
	resources, summary, err := a.listAllResourcesCES(ctx, cred, projectID, region, q, cfg)
	if err != nil {
		return nil, nil, err
	}
	if summary == nil {
		summary = &obsapp.CloudSyncSummary{Region: region}
	}
	a.enrichResources(ctx, cred, projectID, region, resources, summary, effectiveMaxResources(q.MaxResources, cfg.MaxResources))
	return resources, summary, nil
}

// enrichResources 对 CES 发现的资源按类型调用原生 API 补充详情，见 §8.2。
// 按 ProviderRef 匹配原生资源，合并 labels（不覆盖已有 CES label）。
// 增强失败只记录 EnrichmentFailedTypes，不丢弃 CES 资源。
func (a *Adapter) enrichResources(ctx context.Context, cred AKSKCredential, projectID, region string, resources []domain.CloudResource, summary *obsapp.CloudSyncSummary, maxResources int) {
	if a.resources == nil || len(resources) == 0 {
		return
	}

	// 按 cloud_resource_type 分组 CES 资源索引。
	byType := make(map[string][]int)
	for i, r := range resources {
		t := strings.ToLower(strings.TrimSpace(r.Type))
		if t == "" {
			continue
		}
		byType[t] = append(byType[t], i)
	}

	enrichedCount := 0
	var failedTypes []string
	for _, resType := range supportedCloudResourceTypes {
		indices, ok := byType[resType]
		if !ok || len(indices) == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			failedTypes = append(failedTypes, resType)
			break
		}
		native, err := a.resources.ListResources(ctx, cred, projectID, region, resType, maxResources)
		if err != nil {
			failedTypes = append(failedTypes, resType)
			logger.From(ctx).Warn("huawei hybrid enrichment failed",
				logger.String("region", region),
				logger.String("resource_type", resType),
				logger.String("error_code", string(apperr.CodeOf(err))),
			)
			continue
		}
		// 构建 ProviderRef -> 原生 CloudResource 映射。
		nativeMap := make(map[string]domain.CloudResource, len(native))
		for _, n := range native {
			if ref := strings.TrimSpace(n.ProviderRef); ref != "" {
				nativeMap[ref] = n
			}
		}
		// 合并 labels，不覆盖已有 CES label。
		for _, idx := range indices {
			ces := &resources[idx]
			n, ok := nativeMap[strings.TrimSpace(ces.ProviderRef)]
			if !ok {
				continue
			}
			if ces.Labels == nil {
				ces.Labels = map[string]string{}
			}
			changed := false
			for k, v := range n.Labels {
				k = strings.TrimSpace(k)
				v = strings.TrimSpace(v)
				if k == "" || v == "" {
					continue
				}
				if _, exists := ces.Labels[k]; !exists {
					ces.Labels[k] = v
					changed = true
				}
			}
			if changed {
				enrichedCount++
			}
		}
	}

	summary.EnrichedCount = enrichedCount
	if len(failedTypes) > 0 {
		summary.EnrichmentFailedTypes = failedTypes
	}
}

// legacyNativeResourceTypes native 兼容路径仅覆盖旧 ECS/CCE/RDS/ELB 四类，见 §8.3。
// 不沿用 supportedCloudResourceTypes（8 类），避免旧账号在 EVS/VPC/DCS/DMS 调用处鉴权失败
// 导致整批同步失败，违背“兼容旧路径”契约。
var legacyNativeResourceTypes = []string{"ecs", "cce", "rds", "elb"}

// listAllResourcesNative 兼容旧 ECS/CCE/RDS/ELB resource client，见 §8.3。
// 逐类调用原生 API：单类失败只记入 FailedScopes 并跳过，不影响其他类型；全部类型均失败时才返回错误。
// SuccessfulTypes 记录查询成功的类型（即使返回 0 条资源），供 SyncService stale 门控使用，
// 避免某类资源全部消失后旧记录无法标记 stale，见 §13.1。
func (a *Adapter) listAllResourcesNative(ctx context.Context, cred AKSKCredential, projectID, region string, q obsapp.AssetFullSyncQuery, cfg SyncModeConfig) ([]domain.CloudResource, *obsapp.CloudSyncSummary, error) {
	if a.resources == nil {
		return nil, nil, apperr.New(apperr.CodeUnavailable, "huawei resource client is not configured")
	}
	limit := effectiveMaxResources(q.MaxResources, cfg.MaxResources)
	summary := &obsapp.CloudSyncSummary{Region: region, ProjectID: projectID, SyncMode: SyncModeNative}
	out := make([]domain.CloudResource, 0, limit)
	var failedScopes []string
	for _, t := range legacyNativeResourceTypes {
		if len(out) >= limit {
			break
		}
		remaining := limit - len(out)
		// 请求 remaining+1 探测截断：返回超过 remaining 条说明云端仍有更多资源，
		// 该类型被截断，必须禁止 stale（与 CES 路径一致），见 §13.1。
		items, err := a.resources.ListResources(ctx, cred, projectID, region, t, remaining+1)
		if err != nil {
			failedScopes = append(failedScopes, fmt.Sprintf("%s/%s: %s", region, t, apperr.FromError(err).Message))
			logger.From(ctx).Warn("huawei native sync type failed",
				logger.String("region", region),
				logger.String("resource_type", t),
				logger.String("error_code", string(apperr.CodeOf(err))),
			)
			continue
		}
		if len(items) > remaining {
			// 截断：只取 remaining 条，标记上限达到，被截断类型不计入 SuccessfulTypes，
			// SyncService 整 region 跳过 stale，避免云端仍存在的资源被误标 stale。
			summary.MaxResourcesReached = true
			out = append(out, items[:remaining]...)
			break
		}
		summary.SuccessfulTypes = appendUniqueString(summary.SuccessfulTypes, t)
		out = append(out, items...)
	}
	// 全部类型都失败则返回错误，避免空结果被误当作“云端无资源”而触发 stale。
	// 注意：因截断(MaxResourcesReached)提前中断不算“全部失败”，此时已有部分资源成功返回。
	if !summary.MaxResourcesReached && len(summary.SuccessfulTypes) == 0 {
		return nil, nil, apperr.New(apperr.CodeUnavailable, fmt.Sprintf("huawei native sync failed for all types in region %s", region))
	}
	summary.Discovered = len(out)
	summary.FailedScopes = failedScopes
	return out, summary, nil
}

func effectiveMaxResources(requested, configured int) int {
	if configured <= 0 {
		configured = defaultMaxResources
	}
	if configured > maxConfiguredResources {
		configured = maxConfiguredResources
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
