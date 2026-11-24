package huawei

import (
	"context"
	"sort"
	"strings"

	"github.com/734965549/aiops/internal/observability/domain"
	"github.com/734965549/aiops/pkg/logger"
)

// ruleGroupExpandBudget 限制单次规则列表请求的分组展开次数，避免分组型规则很多时
// 逐个调用 CES 接口放大延迟；超出预算的规则仍按 no_resource_scope 呈现。
const ruleGroupExpandBudget = 10

// expandRuleResources 把资源分组型规则展开为实例维度样本（最多 3 条）。
// 展开失败只降级该规则，不影响规则列表返回。
func (a *Adapter) expandRuleResources(ctx context.Context, cred AKSKCredential, projectID, region string, rules []domain.AlertRule) {
	if a == nil || a.ruleResources == nil {
		return
	}
	expanded := 0
	for i := range rules {
		rule := &rules[i]
		if len(rule.Resources) > 0 || len(rule.ResourceGroupIDs) == 0 {
			continue
		}
		service := strings.TrimSpace(rule.Namespace)
		if service == "" {
			continue
		}
		if expanded >= ruleGroupExpandBudget {
			return
		}
		expanded++
		rule.Resources = a.expandRuleResourceGroups(ctx, cred, projectID, region, rule, service)
	}
}

func (a *Adapter) expandRuleResourceGroups(ctx context.Context, cred AKSKCredential, projectID, region string, rule *domain.AlertRule, service string) []map[string]string {
	samples := rule.Resources
	for _, groupID := range rule.ResourceGroupIDs {
		if len(samples) >= ruleResourceSampleMax {
			break
		}
		dims, err := a.ruleResources.ExpandGroupDimensions(
			ctx, cred, projectID, region, groupID, service, ruleResourceSampleMax-len(samples))
		if err != nil {
			logger.From(ctx).Warn("huawei ces rule resource group expand failed",
				logger.String("rule_id", rule.RuleID),
				logger.String("group_id", groupID),
				logger.Error(err))
			continue
		}
		samples = appendUniqueRuleResources(samples, dims)
	}
	return samples
}

func appendUniqueRuleResources(existing []map[string]string, samples []map[string]string) []map[string]string {
	seen := make(map[string]struct{}, len(existing))
	for _, dims := range existing {
		seen[resourceSampleKey(dims)] = struct{}{}
	}
	for _, dims := range samples {
		if len(dims) == 0 {
			continue
		}
		key := resourceSampleKey(dims)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		existing = append(existing, dims)
		if len(existing) >= ruleResourceSampleMax {
			break
		}
	}
	return existing
}

func resourceSampleKey(dims map[string]string) string {
	keys := make([]string, 0, len(dims))
	for key := range dims {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(dims[key])
		b.WriteString(";")
	}
	return b.String()
}
