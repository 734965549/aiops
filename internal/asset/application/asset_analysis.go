package application

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/asset/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
)

// ListCloudResourcesQuery 是按账号发现云同步资源的入参（AI resources.list）。
type ListCloudResourcesQuery struct {
	AccountID    string
	ResourceType string
	Region       string
	Keyword      string
	Limit        int
}

// ResourceAnalysisSnapshot 是 AI 只读分析所需的资源摘要（脱敏，不含凭据）。
type ResourceAnalysisSnapshot struct {
	ResourceID           string
	Name                 string
	ApplicationID        string
	ResourceType         string
	CloudResourceType    string
	CloudResourceID      string
	Region               string
	IntegrationAccountID string
	Source               string
	Labels               map[string]string
}

// SetDataScope 注入资源对象级数据范围校验能力；未注入时 AI 资源读取/发现 fail-closed。
func (s *AssetService) SetDataScope(authz AuthorizationPort, accountScopes AccountScopePort) {
	if s == nil {
		return
	}
	s.authz = authz
	s.accountScopes = accountScopes
}

// GetResourceForAnalysis 读取 AI 分析所需的资源快照，并在读取端口统一收口数据范围。
// ID 兼容：优先按资产业务 ID 查询，未命中时按 cloud_resource_id 查询云同步资源（观测页上下文来源）。
func (s *AssetService) GetResourceForAnalysis(ctx context.Context, userID, resourceID string) (*ResourceAnalysisSnapshot, error) {
	if s == nil || s.resources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
	}
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "resource_id is required")
	}
	resource, err := s.resources.GetByID(ctx, resourceID)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, wrapAssetError(err, "load resource failed")
		}
		candidates, listErr := s.resources.ListByCloudResourceID(ctx, resourceID)
		if listErr != nil {
			return nil, wrapAssetError(listErr, "load resource by cloud id failed")
		}
		resource, err = s.pickAccessibleResource(ctx, userID, candidates)
		if err != nil {
			return nil, err
		}
	} else if err := s.authorizeResourceAccess(ctx, userID, resource); err != nil {
		return nil, err
	}
	snapshot := toResourceAnalysisSnapshot(*resource)
	return &snapshot, nil
}

// pickAccessibleResource 在多条云维度命中中挑选当前用户可见的第一条，其余只记 warn。
func (s *AssetService) pickAccessibleResource(ctx context.Context, userID string, candidates []domain.Resource) (*domain.Resource, error) {
	if len(candidates) == 0 {
		return nil, apperr.New(apperr.CodeNotFound, "resource not found")
	}
	var deniedErr error
	for i := range candidates {
		if err := s.authorizeResourceAccess(ctx, userID, &candidates[i]); err != nil {
			if apperr.CodeOf(err) == apperr.CodePermissionDenied {
				deniedErr = err
				continue
			}
			return nil, err
		}
		if len(candidates) > 1 {
			logger.From(ctx).Warn("ai resource context matched multiple cloud resources",
				logger.String("cloud_resource_id", candidates[i].CloudResourceID),
				logger.Int("match_count", len(candidates)),
				logger.String("resource_id", candidates[i].ID))
		}
		return &candidates[i], nil
	}
	if deniedErr != nil {
		return nil, deniedErr
	}
	return nil, apperr.New(apperr.CodeNotFound, "resource not found")
}

// authorizeResourceAccess 按资源所属集成账号校验数据范围；手工资源没有账号归属，仅依赖调用方权限。
func (s *AssetService) authorizeResourceAccess(ctx context.Context, userID string, resource *domain.Resource) error {
	if resource == nil {
		return apperr.New(apperr.CodeNotFound, "resource not found")
	}
	accountID := strings.TrimSpace(resource.IntegrationAccountID)
	if accountID == "" {
		return nil
	}
	if s.authz == nil || s.accountScopes == nil {
		return apperr.New(apperr.CodeUnavailable, "resource data scope guard is not configured")
	}
	ownerTeam, _, err := s.accountScopes.ResolveAccountScope(ctx, accountID)
	if err != nil {
		return wrapAssetError(err, "resolve integration account scope failed")
	}
	result, err := s.authz.Authorize(ctx, AuthorizationInput{
		UserID: strings.TrimSpace(userID), Resource: "assets", Action: "read",
		ObjectTeam: strings.TrimSpace(ownerTeam), ObjectRegion: strings.TrimSpace(resource.Region),
	})
	if err != nil {
		return wrapAssetError(err, "authorize resource access failed")
	}
	if result == nil || !result.Allowed {
		return apperr.New(apperr.CodePermissionDenied, "no access to resource")
	}
	return nil
}

// ListCloudResourcesForAgent 按账号发现云同步资源，返回脱敏摘要。
// 数据范围：账号级授权 + owner_team 过滤（用户仅有 team 范围时）双重收口。
func (s *AssetService) ListCloudResourcesForAgent(ctx context.Context, userID string, q ListCloudResourcesQuery) ([]ResourceAnalysisSnapshot, error) {
	if s == nil || s.resources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
	}
	accountID := strings.TrimSpace(q.AccountID)
	if accountID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "account_id is required")
	}
	if s.authz == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "resource data scope guard is not configured")
	}
	ownerTeams, hasAll, err := s.authz.ResolveAccessibleOwnerTeams(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, wrapAssetError(err, "resolve accessible owner teams failed")
	}
	if s.accountScopes != nil {
		ownerTeam, regions, err := s.accountScopes.ResolveAccountScope(ctx, accountID)
		if err != nil {
			return nil, wrapAssetError(err, "resolve integration account scope failed")
		}
		region := strings.TrimSpace(q.Region)
		if region == "" && len(regions) > 0 {
			region = strings.TrimSpace(regions[0])
		}
		result, err := s.authz.Authorize(ctx, AuthorizationInput{
			UserID: strings.TrimSpace(userID), Resource: "assets", Action: "read",
			ObjectTeam: strings.TrimSpace(ownerTeam), ObjectRegion: region,
		})
		if err != nil {
			return nil, wrapAssetError(err, "authorize resource discovery failed")
		}
		if result == nil || !result.Allowed {
			return nil, apperr.New(apperr.CodePermissionDenied, "no access to integration account")
		}
	}
	filter := domain.CloudResourceFilter{
		IntegrationAccountID: accountID,
		CloudResourceType:    strings.ToLower(strings.TrimSpace(q.ResourceType)),
		Region:               strings.TrimSpace(q.Region),
		Keyword:              strings.TrimSpace(q.Keyword),
		Limit:                q.Limit,
	}
	// teams 为空表示用户只有 region 等非 team 范围：账号级 Authorize 已生效，不再按空集合过滤。
	if !hasAll && len(ownerTeams) > 0 {
		filter.OwnerTeams = ownerTeams
	}
	rows, err := s.resources.ListCloudResourcesPaged(ctx, filter)
	if err != nil {
		return nil, wrapAssetError(err, "list cloud resources failed")
	}
	out := make([]ResourceAnalysisSnapshot, 0, len(rows))
	for _, row := range rows {
		out = append(out, toResourceAnalysisSnapshot(row))
	}
	return out, nil
}

func toResourceAnalysisSnapshot(resource domain.Resource) ResourceAnalysisSnapshot {
	return ResourceAnalysisSnapshot{
		ResourceID:           resource.ID,
		Name:                 resource.Name,
		ApplicationID:        resource.ApplicationID,
		ResourceType:         resource.ResourceType,
		CloudResourceType:    resource.CloudResourceType,
		CloudResourceID:      resource.CloudResourceID,
		Region:               resource.Region,
		IntegrationAccountID: resource.IntegrationAccountID,
		Source:               resource.Source,
		Labels:               ensureResourceLabels(resource.Labels),
	}
}
