package huawei

import (
	"context"
	"strconv"
	"strings"

	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

const (
	ruleListPageLimit = 100
	ruleListMaxPages  = 10
	ruleListCapMax    = 500
	// ruleResourceSampleMax 限制单条规则返回的资源维度样本，避免观测体积膨胀。
	ruleResourceSampleMax = 3
)

// ruleListAPI 抽象 SDK 调用，便于 fixture 单测（同 alarmHistoryAPI 模式）。
type ruleListAPI interface {
	ListAlarmRules(request *cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error)
}

// AlarmRuleListClient 按 namespace 批量拉取 CES V2 告警规则（ak_sk 真账号路径）。
type AlarmRuleListClient interface {
	ListRules(ctx context.Context, cred AKSKCredential, projectID, region, namespace string, maxRules int) ([]domain.AlertRule, error)
}

// CESAlarmRuleListClient 生产实现。
type CESAlarmRuleListClient struct {
	newAPI func(cred AKSKCredential, projectID, region string) (ruleListAPI, error)
}

func NewCESAlarmRuleListClient() *CESAlarmRuleListClient {
	return &CESAlarmRuleListClient{
		newAPI: func(cred AKSKCredential, projectID, region string) (ruleListAPI, error) {
			return newCESv2Client(cred, projectID, region)
		},
	}
}

var _ AlarmRuleListClient = (*CESAlarmRuleListClient)(nil)

// ListRules 分页拉取 namespace 下全部规则（CES 单页上限 100、最多 10 页），
// 截断上限由调用方 limit 决定（域名查询上限 500，AI 工具侧另有 50 上限）。
func (c *CESAlarmRuleListClient) ListRules(ctx context.Context, cred AKSKCredential, projectID, region, namespace string, maxRules int) ([]domain.AlertRule, error) {
	if c == nil || c.newAPI == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "huawei alarm rule list client is not configured")
	}
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "namespace is required")
	}
	if maxRules <= 0 || maxRules > ruleListCapMax {
		maxRules = ruleListCapMax
	}
	api, err := c.newAPI(cred, projectID, region)
	if err != nil {
		return nil, err
	}
	rules := make([]domain.AlertRule, 0, 16)
	offset := int32(0)
	for page := 0; page < ruleListMaxPages && len(rules) < maxRules; page++ {
		if err := ctx.Err(); err != nil {
			return nil, mapCESError(err)
		}
		limit := int32(ruleListPageLimit)
		req := &cesv2model.ListAlarmRulesRequest{
			Namespace: &ns, Offset: &offset, Limit: &limit,
		}
		resp, err := callWithContext(ctx, func() (*cesv2model.ListAlarmRulesResponse, error) {
			return api.ListAlarmRules(req)
		})
		if err != nil {
			return nil, mapCESError(err)
		}
		if resp == nil || resp.Alarms == nil {
			break
		}
		items := *resp.Alarms
		for i := range items {
			rules = append(rules, mapListAlarmRule(items[i]))
			if len(rules) >= maxRules {
				break
			}
		}
		if len(items) < int(limit) {
			break
		}
		offset += limit
	}
	return rules, nil
}

// mapListAlarmRule 将 CES 规则映射为域模型；指标阈值取第一个带 metric_name 的策略，
// 事件规则（无指标策略）只输出规则骨架，与告警拉取富化口径一致（ces_alarm_rule.go mapAlarmRule）。
func mapListAlarmRule(item cesv2model.ListAlarmRespBodyAlarms) domain.AlertRule {
	rule := domain.AlertRule{
		RuleID:      strings.TrimSpace(valueOf(item.AlarmId)),
		Name:        strings.TrimSpace(valueOf(item.Name)),
		Namespace:   strings.TrimSpace(valueOf(item.Namespace)),
		Enabled:     item.Enabled == nil || *item.Enabled,
		Description: strings.TrimSpace(valueOf(item.Description)),
		Labels:      map[string]string{"provider": "huawei_cloud"},
	}
	if rule.RuleID == "" {
		rule.RuleID = rule.Name
	}
	if item.Resources != nil && len(*item.Resources) > 0 {
		rule.ResourceCount = len(*item.Resources)
		rule.Resources = mapRuleResourceSamples(*item.Resources)
		rule.ResourceGroupIDs = mapRuleResourceGroupIDs(*item.Resources)
	}
	if item.Policies == nil {
		return rule
	}
	for i := range *item.Policies {
		policy := (*item.Policies)[i]
		if strings.TrimSpace(valueOf(policy.MetricName)) == "" {
			continue
		}
		rule.Metric = strings.TrimSpace(valueOf(policy.MetricName))
		rule.Operator = strings.TrimSpace(valueOf(policy.ComparisonOperator))
		rule.Unit = strings.TrimSpace(valueOf(policy.Unit))
		if policy.Value != nil {
			threshold := *policy.Value
			rule.Threshold = &threshold
		}
		if policy.Period != nil {
			rule.WindowSeconds = int(*policy.Period)
		}
		if policy.Level != nil {
			rule.Severity = strconv.Itoa(int(*policy.Level))
		}
		if rule.Namespace == "" {
			rule.Namespace = strings.TrimSpace(valueOf(policy.Namespace))
		}
		break
	}
	return rule
}

// mapRuleResourceGroupIDs 提取规则关联的资源分组 ID；资源分组型规则没有实例维度，
// 需要在 adapter 层调用 CES 资源分组接口展开为实例维度样本。
func mapRuleResourceGroupIDs(resources []cesv2model.ResourcesInListResp) []string {
	out := make([]string, 0, len(resources))
	seen := make(map[string]struct{}, len(resources))
	for i := range resources {
		if resources[i].ResourceGroupId == nil {
			continue
		}
		groupID := strings.TrimSpace(*resources[i].ResourceGroupId)
		if groupID == "" {
			continue
		}
		if _, ok := seen[groupID]; ok {
			continue
		}
		seen[groupID] = struct{}{}
		out = append(out, groupID)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// mapRuleResourceSamples 提取前若干个关联资源的维度样本；资源分组无维度时只计数不采样。
func mapRuleResourceSamples(resources []cesv2model.ResourcesInListResp) []map[string]string {
	out := make([]map[string]string, 0, ruleResourceSampleMax)
	for i := range resources {
		if len(out) >= ruleResourceSampleMax {
			break
		}
		if resources[i].Dimensions == nil || len(*resources[i].Dimensions) == 0 {
			continue
		}
		dims := make(map[string]string, len(*resources[i].Dimensions))
		for _, dim := range *resources[i].Dimensions {
			name := strings.TrimSpace(valueOf(dim.Name))
			value := strings.TrimSpace(valueOf(dim.Value))
			if name == "" || value == "" {
				continue
			}
			dims[name] = value
		}
		if len(dims) > 0 {
			out = append(out, dims)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
