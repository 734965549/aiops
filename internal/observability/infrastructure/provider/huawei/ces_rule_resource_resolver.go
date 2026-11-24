package huawei

import (
	"context"
	"strings"

	apperr "github.com/734965549/aiops/pkg/errors"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

// RuleResourceResolver 把规则关联的资源分组展开为实例维度样本。
// 资源分组型规则在 CES ListAlarmRules 中只返回 resource_group_id，没有实例维度。
type RuleResourceResolver interface {
	ExpandGroupDimensions(ctx context.Context, cred AKSKCredential, projectID, region, groupID, service string, limit int) ([]map[string]string, error)
}

// CESRuleResourceResolver 基于 CES ListResourceGroupsServicesResources 的生产实现。
type CESRuleResourceResolver struct {
	newAPI func(cred AKSKCredential, projectID, region string) (cesResourceGroupAPI, error)
}

func NewCESRuleResourceResolver() *CESRuleResourceResolver {
	return &CESRuleResourceResolver{
		newAPI: func(cred AKSKCredential, projectID, region string) (cesResourceGroupAPI, error) {
			return newCESv2Client(cred, projectID, region)
		},
	}
}

var _ RuleResourceResolver = (*CESRuleResourceResolver)(nil)

func (r *CESRuleResourceResolver) ExpandGroupDimensions(
	ctx context.Context,
	cred AKSKCredential,
	projectID, region, groupID, service string,
	limit int,
) ([]map[string]string, error) {
	if r == nil || r.newAPI == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "huawei rule resource resolver is not configured")
	}
	groupID = strings.TrimSpace(groupID)
	service = strings.TrimSpace(service)
	if groupID == "" || service == "" {
		return nil, nil
	}
	if limit <= 0 || limit > ruleResourceSampleMax {
		limit = ruleResourceSampleMax
	}
	api, err := r.newAPI(cred, projectID, region)
	if err != nil {
		return nil, err
	}
	offset := int32(0)
	req := &cesv2model.ListResourceGroupsServicesResourcesRequest{
		GroupId: groupID,
		Service: service,
		Offset:  &offset,
		Limit:   ptrCESLimit(int32(limit)),
	}
	resp, err := callCESWithRetry(ctx, region, "list rule resource group resources",
		func() (*cesv2model.ListResourceGroupsServicesResourcesResponse, error) {
			return api.ListResourceGroupsServicesResources(req)
		})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Resources == nil {
		return nil, nil
	}
	out := make([]map[string]string, 0, limit)
	for _, res := range *resp.Resources {
		dims := make(map[string]string, len(res.Dimensions))
		for _, dim := range res.Dimensions {
			name := strings.TrimSpace(dim.Name)
			value := strings.TrimSpace(dim.Value)
			if name == "" || value == "" {
				continue
			}
			dims[name] = value
		}
		if len(dims) == 0 {
			continue
		}
		out = append(out, dims)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
