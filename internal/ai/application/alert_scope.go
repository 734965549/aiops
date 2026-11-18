package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/ai/domain"
)

// 告警标签到数据范围维度的映射键（ops/ai-contract.md §10.6.4）。
//
// 平台告警没有独立的部门/团队/区域/tag 字段，只有归属负责人与标准化标签，
// 因此对象上下文按固定键取值；缺失即视为“无该维度上下文”，由授权服务按
// 数据范围配置决定拒绝（fail-closed），而不是放行。
var (
	alertDeptLabelKeys   = []string{"department", "dept", "department_id"}
	alertTeamLabelKeys   = []string{"team", "owner_team", "team_id"}
	alertRegionLabelKeys = []string{"region", "region_id"}
	// alertTagLabelKeys 是参与 tag 数据范围匹配的标签键；只取稳定语义标签，
	// 避免任意标签把 tag 范围放大。
	alertTagLabelKeys = []string{"env", "environment", "business_line", "tags"}
	// alertScopeMaxTags 限制送入授权服务的标签数量，防止标签膨胀。
	alertScopeMaxTags = 16
)

// AlertObjectContext 是告警对象的数据范围上下文。
type AlertObjectContext struct {
	Owner  string
	Dept   string
	Team   string
	Region string
	Tags   []string
}

// AlertScopeAuthorizer 校验当前用户是否可读取指定告警对象（对象级数据范围）。
// 由 infrastructure/identity 适配统一授权服务实现；未装配时调用方必须拒绝（fail-closed）。
type AlertScopeAuthorizer interface {
	AuthorizeAlert(ctx context.Context, userID string, alert *AlertContext) error
}

// AlertObjectContextOf 从告警快照解析数据范围上下文。
//
// 映射规则：
//   - Owner ← owner_user_id，用于 custom 范围的 owner_ids 匹配；
//   - Dept ← labels.department / dept / department_id；
//   - Team ← labels.team / owner_team / team_id；
//   - Region ← labels.region / region_id（CES 拉取源会写入）；
//   - Tags ← labels.env/environment/business_line/tags 与 environment/business_line 字段的取值。
func AlertObjectContextOf(alert *AlertContext) AlertObjectContext {
	if alert == nil {
		return AlertObjectContext{}
	}
	labels := alert.Labels
	obj := AlertObjectContext{
		Owner:  strings.TrimSpace(alert.OwnerUserID),
		Dept:   firstLabelValue(labels, alertDeptLabelKeys...),
		Team:   firstLabelValue(labels, alertTeamLabelKeys...),
		Region: firstLabelValue(labels, alertRegionLabelKeys...),
	}
	tags := make([]string, 0, len(alertTagLabelKeys)+2)
	seen := make(map[string]struct{}, len(alertTagLabelKeys)+2)
	for _, key := range alertTagLabelKeys {
		for _, value := range splitScopeValues(labels[key]) {
			tags = appendTag(tags, seen, value)
		}
	}
	// Environment / BusinessLine 是归一化后的字段（CES 拉取源会写入默认值），
	// 告警标签未必保留同名键，因此一并作为 tag 维度候选。
	tags = appendTag(tags, seen, alert.Environment)
	tags = appendTag(tags, seen, alert.BusinessLine)
	obj.Tags = tags
	return obj
}

// domainContextOf 把告警快照折算为页面上下文，用于记忆检索的匹配键。
func domainContextOf(alert *AlertContext) domain.SessionContext {
	if alert == nil {
		return domain.SessionContext{}
	}
	return domain.SessionContext{
		AlertID:       alert.ID,
		ApplicationID: alert.ApplicationID,
		ResourceID:    alert.ResourceID,
		ResourceType:  alert.ResourceType,
	}
}

func firstLabelValue(labels map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(labels[key]); value != "" {
			return value
		}
	}
	return ""
}

// splitScopeValues 兼容标签值使用逗号或分号分隔多个标签。
func splitScopeValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '|'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if value := strings.TrimSpace(field); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func appendTag(tags []string, seen map[string]struct{}, value string) []string {
	if len(tags) >= alertScopeMaxTags {
		return tags
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return tags
	}
	key := strings.ToLower(value)
	if _, ok := seen[key]; ok {
		return tags
	}
	seen[key] = struct{}{}
	return append(tags, value)
}
