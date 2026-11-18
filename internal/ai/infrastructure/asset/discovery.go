package asset

import (
	"context"
	"strings"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	assetapp "github.com/734965549/aiops/internal/asset/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

const defaultResourceDiscoveryTimeout = 5 * time.Second

// ResourceLister 是资源发现依赖的最小只读能力（由 assetapp.AssetService 实现）。
type ResourceLister interface {
	ListCloudResourcesForAgent(ctx context.Context, userID string, q assetapp.ListCloudResourcesQuery) ([]assetapp.ResourceAnalysisSnapshot, error)
}

// AccountResolver 解析 integration 账号摘要，用于数据权限预检。
type AccountResolver interface {
	ResolveAccount(ctx context.Context, accountID string) (*obsdomain.AccountSnapshot, error)
}

// DiscoveryAdapter 实现 aiapp.ResourceDiscoveryPort。
// 预检顺序与指标/规则查询一致：解析账号 -> 账号数据范围 -> Asset 查询。
type DiscoveryAdapter struct {
	resources     ResourceLister
	accounts      AccountResolver
	accountAccess aiapp.AccountAccessAuthorizer
	timeout       time.Duration
}

func NewDiscoveryAdapter(
	resources ResourceLister,
	accounts AccountResolver,
	accountAccess aiapp.AccountAccessAuthorizer,
	timeout time.Duration,
) *DiscoveryAdapter {
	if timeout <= 0 {
		timeout = defaultResourceDiscoveryTimeout
	}
	return &DiscoveryAdapter{
		resources: resources, accounts: accounts, accountAccess: accountAccess, timeout: timeout,
	}
}

var _ aiapp.ResourceDiscoveryPort = (*DiscoveryAdapter)(nil)

// List 执行账号/数据范围预检后查询本地云同步资源；业务失败以 Result.Status 降级返回。
func (a *DiscoveryAdapter) List(ctx context.Context, userID string, q aiapp.ResourceQuery) (*aiapp.ResourceListResult, error) {
	if a == nil || a.resources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai resource discovery is not configured")
	}
	q.AccountID = strings.TrimSpace(q.AccountID)
	if q.AccountID == "" {
		return &aiapp.ResourceListResult{Status: aiapp.EvidenceStatusSkipped, Reason: "no_account"}, nil
	}
	if a.accounts == nil {
		return &aiapp.ResourceListResult{Status: aiapp.EvidenceStatusError, Reason: "account_resolver_unavailable"}, nil
	}
	snapshot, err := a.accounts.ResolveAccount(ctx, q.AccountID)
	if err != nil {
		return &aiapp.ResourceListResult{
			Status: aiapp.EvidenceStatusError,
			Reason: resourceErrorReason(err, "resolve integration account failed"),
		}, nil
	}
	if snapshot == nil {
		return &aiapp.ResourceListResult{Status: aiapp.EvidenceStatusError, Reason: "integration account not found"}, nil
	}
	if a.accountAccess != nil && strings.TrimSpace(userID) != "" {
		ref := aiapp.AccountRef{
			AccountID: snapshot.AccountID,
			OwnerTeam: snapshot.OwnerTeam,
			Regions:   append([]string(nil), snapshot.Regions...),
		}
		if err := a.accountAccess.Authorize(ctx, userID, ref); err != nil {
			return &aiapp.ResourceListResult{
				Status: aiapp.EvidenceStatusPermissionDenied,
				Reason: "no access to integration account for evidence",
			}, nil
		}
	}
	queryCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	rows, err := a.resources.ListCloudResourcesForAgent(queryCtx, userID, assetapp.ListCloudResourcesQuery{
		AccountID:    q.AccountID,
		ResourceType: q.ResourceType,
		Region:       q.Region,
		Keyword:      q.Keyword,
		Limit:        q.Limit,
	})
	if err != nil {
		status := aiapp.EvidenceStatusError
		reason := resourceErrorReason(err, "list cloud resources failed")
		if apperr.CodeOf(err) == apperr.CodePermissionDenied {
			status = aiapp.EvidenceStatusPermissionDenied
			reason = "no access to integration account for evidence"
		}
		return &aiapp.ResourceListResult{Status: status, Reason: reason}, nil
	}
	out := &aiapp.ResourceListResult{
		Status:    aiapp.EvidenceStatusOK,
		Resources: make([]aiapp.ResourceSummary, 0, len(rows)),
	}
	for _, row := range rows {
		out.Resources = append(out.Resources, aiapp.ResourceSummary{
			ResourceID:        row.ResourceID,
			Name:              row.Name,
			CloudResourceType: row.CloudResourceType,
			CloudResourceID:   row.CloudResourceID,
			Region:            row.Region,
			AccountID:         row.IntegrationAccountID,
			Labels:            row.Labels,
		})
	}
	return out, nil
}

func resourceErrorReason(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	ae := apperr.FromError(err)
	if ae == nil || ae.Code == apperr.CodeInternal {
		return fallback
	}
	if message := strings.TrimSpace(ae.Message); message != "" {
		return message
	}
	return fallback
}
