package huawei

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/734965549/aiops/internal/observability/domain"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdkerr"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

const (
	alarmRuleCacheTTL         = 24 * time.Hour
	alarmRuleNotFoundCacheTTL = 10 * time.Minute
	alarmRuleLookupBudget     = 50
)

// alarmRuleDetail 是 ListAlarmRules 单条规则的富化快照，仅在 provider 内部使用。
type alarmRuleDetail struct {
	Name          string
	Description   string
	Namespace     string
	MetricName    string
	Type          string
	Level         string
	ResourceLevel string
	Condition     *domain.AlarmCondition
	Dimensions    map[string]string
	ResourceCount int
}

type alarmRuleCacheEntry struct {
	rule      *alarmRuleDetail
	notFound  bool
	expiresAt time.Time
}

type alarmRuleCache struct {
	mu      sync.Mutex
	entries map[string]alarmRuleCacheEntry
}

func (c *alarmRuleCache) get(key string, now time.Time) (alarmRuleCacheEntry, bool) {
	if c == nil {
		return alarmRuleCacheEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return alarmRuleCacheEntry{}, false
	}
	if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
		delete(c.entries, key)
		return alarmRuleCacheEntry{}, false
	}
	return entry, true
}

func (c *alarmRuleCache) put(key string, entry alarmRuleCacheEntry) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]alarmRuleCacheEntry{}
	}
	c.entries[key] = entry
}

// mapAlarmRule 将 ListAlarmRules 规则转换为富化快照。
// 指标阈值优先取第一个带 metric_name 的策略；事件规则不补指标与阈值。
func mapAlarmRule(item cesv2model.ListAlarmRespBodyAlarms) *alarmRuleDetail {
	detail := &alarmRuleDetail{
		Name:        strings.TrimSpace(valueOf(item.Name)),
		Description: strings.TrimSpace(valueOf(item.Description)),
		Namespace:   strings.TrimSpace(valueOf(item.Namespace)),
	}
	if item.Type != nil {
		detail.Type = strings.TrimSpace(item.Type.Value())
	}
	if item.ResourceLevel != nil {
		detail.ResourceLevel = strings.TrimSpace(item.ResourceLevel.Value())
	}
	eventRule := isEventRuleType(detail.Type)
	if item.Policies != nil {
		policies := *item.Policies
		if len(policies) > 0 {
			index := 0
			for i := range policies {
				if strings.TrimSpace(valueOf(policies[i].MetricName)) != "" {
					index = i
					break
				}
			}
			applyAlarmPolicy(detail, policies[index], eventRule)
		}
	}
	if item.Resources != nil {
		detail.ResourceCount = len(*item.Resources)
		// 产品层级规则可能关联大量资源，无法判断当前告警属于哪个实例；
		// 仅在规则恰好关联单个资源时补全维度，避免把错误实例写到告警上。
		if len(*item.Resources) == 1 {
			detail.Dimensions = map[string]string{}
			if dims := (*item.Resources)[0].Dimensions; dims != nil {
				for _, dim := range *dims {
					name := strings.TrimSpace(valueOf(dim.Name))
					value := strings.TrimSpace(valueOf(dim.Value))
					if name == "" || value == "" {
						continue
					}
					detail.Dimensions[name] = value
				}
			}
			if len(detail.Dimensions) == 0 {
				detail.Dimensions = nil
			}
		}
	}
	return detail
}

func applyAlarmPolicy(detail *alarmRuleDetail, policy cesv2model.PolicyResp, eventRule bool) {
	if detail.Namespace == "" {
		detail.Namespace = strings.TrimSpace(valueOf(policy.Namespace))
	}
	if !eventRule {
		if detail.MetricName == "" {
			detail.MetricName = strings.TrimSpace(valueOf(policy.MetricName))
		}
		operator := strings.TrimSpace(valueOf(policy.ComparisonOperator))
		if detail.Condition == nil && (operator != "" || policy.Value != nil) {
			condition := &domain.AlarmCondition{
				Filter:             strings.TrimSpace(valueOf(policy.Filter)),
				ComparisonOperator: operator,
				Unit:               strings.TrimSpace(valueOf(policy.Unit)),
			}
			if policy.Value != nil {
				condition.Threshold = *policy.Value
			}
			if policy.Count != nil {
				condition.TriggerCount = int(*policy.Count)
			}
			if policy.Period != nil {
				condition.PeriodSeconds = int64(*policy.Period)
			}
			if policy.SuppressDuration != nil {
				condition.SuppressDurationS = int64(*policy.SuppressDuration)
			}
			detail.Condition = condition
		}
	}
	if detail.Level == "" && policy.Level != nil {
		detail.Level = strconv.Itoa(int(*policy.Level))
	}
}

func isEventRuleType(ruleType string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(ruleType)), "EVENT")
}

func needsRuleEnrichment(rec domain.AlarmRecord) bool {
	return strings.TrimSpace(rec.MetricName) == "" || rec.Condition == nil
}

func firstAlarmRule(resp *cesv2model.ListAlarmRulesResponse, alarmID string) *cesv2model.ListAlarmRespBodyAlarms {
	if resp == nil || resp.Alarms == nil || len(*resp.Alarms) == 0 {
		return nil
	}
	alarms := *resp.Alarms
	for i := range alarms {
		if strings.TrimSpace(valueOf(alarms[i].AlarmId)) == alarmID {
			return &alarms[i]
		}
	}
	if len(alarms) == 1 {
		return &alarms[0]
	}
	return nil
}

func applyAlarmRule(rec *domain.AlarmRecord, rule *alarmRuleDetail) {
	if rec == nil || rule == nil {
		return
	}
	if rec.RuleName == "" {
		rec.RuleName = rule.Name
	}
	if rec.RuleDescription == "" {
		rec.RuleDescription = rule.Description
	}
	if rec.RuleType == "" {
		rec.RuleType = rule.Type
	}
	if rec.Namespace == "" {
		rec.Namespace = rule.Namespace
	}
	if rec.MetricName == "" {
		rec.MetricName = rule.MetricName
	}
	if rec.Condition == nil && rule.Condition != nil {
		condition := *rule.Condition
		rec.Condition = &condition
	}
	if len(rec.Dimensions) == 0 && len(rule.Dimensions) > 0 {
		rec.Dimensions = cloneAlarmDimensions(rule.Dimensions)
	}
	if rec.Level == "" {
		rec.Level = rule.Level
	}
	if rec.ResourceID == "" {
		rec.ResourceID = primaryDimensionValue(rec.Dimensions)
	}
	if rec.ResourceName == "" {
		rec.ResourceName = rec.ResourceID
	}
	if rec.ResourceType == "" {
		rec.ResourceType = resourceTypeFromNamespace(rec.Namespace)
	}
}

func cloneAlarmDimensions(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// enrichRecords 按 (project_id, alarm_id) 去重后查询规则并回填缺失字段。
// 富化是增强而非数据本体：预算不足、查询失败或规则不存在都不改变 scope 的 Complete 语义。
func (c *CESAlarmHistoryClient) enrichRecords(ctx context.Context, api alarmHistoryAPI, scope domain.AlarmPullScope, records []domain.AlarmRecord, deadline time.Time) {
	if len(records) == 0 {
		return
	}
	if c.rules == nil {
		c.rules = &alarmRuleCache{}
	}
	seen := make(map[string]struct{}, len(records))
	lookups := 0
	for i := range records {
		alarmID := strings.TrimSpace(records[i].AlarmID)
		if alarmID == "" || !needsRuleEnrichment(records[i]) {
			continue
		}
		if _, ok := seen[alarmID]; ok {
			continue
		}
		seen[alarmID] = struct{}{}
		if lookups >= alarmRuleLookupBudget {
			logger.From(ctx).Warn("huawei ces alarm rule lookup budget exhausted",
				logger.String("region", scope.Region),
				logger.String("project_id", scope.ProjectID),
				logger.Int("budget", alarmRuleLookupBudget),
			)
			return
		}
		if ctx.Err() != nil {
			return
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return
		}
		lookups++
		rule, lookup := c.lookupAlarmRule(ctx, api, scope, alarmID)
		if lookup == "ok" {
			for j := range records {
				if strings.TrimSpace(records[j].AlarmID) == alarmID {
					applyAlarmRule(&records[j], rule)
				}
			}
			continue
		}
		for j := range records {
			if strings.TrimSpace(records[j].AlarmID) == alarmID {
				records[j].RuleLookup = lookup
			}
		}
	}
}

func (c *CESAlarmHistoryClient) lookupAlarmRule(ctx context.Context, api alarmHistoryAPI, scope domain.AlarmPullScope, alarmID string) (*alarmRuleDetail, string) {
	key := scope.ProjectID + "/" + alarmID
	now := time.Now()
	if entry, ok := c.rules.get(key, now); ok {
		if entry.notFound {
			return nil, "not_found"
		}
		return entry.rule, "ok"
	}
	limit := int32(10)
	resp, err := api.ListAlarmRules(&cesv2model.ListAlarmRulesRequest{
		AlarmId: &alarmID,
		Limit:   &limit,
	})
	if err != nil {
		reason := "error"
		var svcErr *sdkerr.ServiceResponseError
		if errors.As(err, &svcErr) && svcErr.StatusCode == 404 {
			reason = "not_found"
			c.rules.put(key, alarmRuleCacheEntry{
				notFound:  true,
				expiresAt: now.Add(alarmRuleNotFoundCacheTTL),
			})
		}
		fields := []logger.Field{
			logger.String("region", scope.Region),
			logger.String("project_id", scope.ProjectID),
			logger.String("alarm_id", alarmID),
			logger.String("reason", reason),
		}
		if errors.As(err, &svcErr) {
			fields = append(fields,
				logger.Int("huawei_status", svcErr.StatusCode),
				logger.String("huawei_error_code", strings.TrimSpace(svcErr.ErrorCode)),
			)
		}
		logger.From(ctx).Warn("huawei ces alarm rule lookup failed", fields...)
		return nil, reason
	}
	item := firstAlarmRule(resp, alarmID)
	if item == nil {
		c.rules.put(key, alarmRuleCacheEntry{
			notFound:  true,
			expiresAt: now.Add(alarmRuleNotFoundCacheTTL),
		})
		return nil, "not_found"
	}
	rule := mapAlarmRule(*item)
	c.rules.put(key, alarmRuleCacheEntry{
		rule:      rule,
		expiresAt: now.Add(alarmRuleCacheTTL),
	})
	return rule, "ok"
}
