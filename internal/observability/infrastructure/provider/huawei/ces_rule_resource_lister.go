package huawei

import (
	"context"
	"strings"

	apperr "github.com/734965549/aiops/pkg/errors"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

// ruleResourcePageLimit 是 ListAlarmRuleResources 的单页上限；规则资源样本只需要前几条。
const ruleResourcePageLimit = 100

// AlarmRuleResourceLister 查询告警规则的完整关联资源。
// CES ListAlarmRules 文档明确说明关联资源需通过 ListAlarmRuleResources 单独获取，
// 云产品级/全部资源规则的列表响应通常不带资源明细。
// 返回值第二个是资源总数（用于校正 ListAlarmRules 的 resource_count）。
type AlarmRuleResourceLister interface {
	ListRuleResources(ctx context.Context, cred AKSKCredential, projectID, region, alarmID string, limit int) ([]map[string]string, int, error)
}

type ruleResourceListAPI interface {
	ListAlarmRuleResources(request *cesv2model.ListAlarmRuleResourcesRequest) (*cesv2model.ListAlarmRuleResourcesResponse, error)
}

// CESAlarmRuleResourceLister 生产实现。
type CESAlarmRuleResourceLister struct {
	newAPI func(cred AKSKCredential, projectID, region string) (ruleResourceListAPI, error)
}

func NewCESAlarmRuleResourceLister() *CESAlarmRuleResourceLister {
	return &CESAlarmRuleResourceLister{
		newAPI: func(cred AKSKCredential, projectID, region string) (ruleResourceListAPI, error) {
			return newCESv2Client(cred, projectID, region)
		},
	}
}

var _ AlarmRuleResourceLister = (*CESAlarmRuleResourceLister)(nil)

// ListRuleResources 分页拉取规则关联资源，最多返回 limit 个维度样本；同时返回资源总数。
func (l *CESAlarmRuleResourceLister) ListRuleResources(
	ctx context.Context,
	cred AKSKCredential,
	projectID, region, alarmID string,
	limit int,
) ([]map[string]string, int, error) {
	if l == nil || l.newAPI == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "huawei alarm rule resource lister is not configured")
	}
	alarmID = strings.TrimSpace(alarmID)
	if alarmID == "" {
		return nil, 0, nil
	}
	if limit <= 0 || limit > ruleResourceSampleMax {
		limit = ruleResourceSampleMax
	}
	api, err := l.newAPI(cred, projectID, region)
	if err != nil {
		return nil, 0, err
	}
	out := make([]map[string]string, 0, limit)
	total := 0
	offset := int32(0)
	for page := 0; page < 3 && len(out) < limit; page++ {
		pageLimit := int32(ruleResourcePageLimit)
		resp, err := callCESWithRetry(ctx, region, "list alarm rule resources",
			func() (*cesv2model.ListAlarmRuleResourcesResponse, error) {
				return api.ListAlarmRuleResources(&cesv2model.ListAlarmRuleResourcesRequest{
					AlarmId: alarmID,
					Offset:  &offset,
					Limit:   &pageLimit,
				})
			})
		if err != nil {
			return nil, 0, err
		}
		if resp == nil || resp.Resources == nil {
			break
		}
		if resp.Count != nil && int(*resp.Count) > total {
			total = int(*resp.Count)
		}
		items := *resp.Resources
		for _, dims := range items {
			sample := make(map[string]string, len(dims))
			for _, dim := range dims {
				name := strings.TrimSpace(valueOf(dim.Name))
				value := strings.TrimSpace(valueOf(dim.Value))
				if name == "" || value == "" {
					continue
				}
				sample[name] = value
			}
			if len(sample) == 0 {
				continue
			}
			out = append(out, sample)
			if len(out) >= limit {
				break
			}
		}
		if len(items) < int(pageLimit) {
			break
		}
		offset += pageLimit
	}
	return out, total, nil
}
