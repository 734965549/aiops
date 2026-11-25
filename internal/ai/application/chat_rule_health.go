package application

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/pkg/logger"
)

const chatRuleHealthMaxReferences = 10

type chatRuleHealthParams struct {
	AccountID  string `json:"account_id"`
	Region     string `json:"region"`
	Namespace  string `json:"namespace"`
	Keyword    string `json:"keyword"`
	Limit      int    `json:"limit"`
	From       *int64 `json:"from"`
	To         *int64 `json:"to"`
	Period     int    `json:"period"`
	Aggregator string `json:"aggregator"`
}

// buildRuleHealthQuery 复用 alert.rules.list 的账号/区域/namespace 回退与校验，
// 再叠加时间窗口、聚合粒度与规则条数上限。
func buildRuleHealthQuery(alert *AlertContext, resource *AssetContext, raw map[string]any, now int64, timezone string) (RuleHealthQuery, string, string) {
	params := chatRuleHealthParams{}
	if _, err := decodeToolParams(raw, &params); err != nil {
		return RuleHealthQuery{}, chatObservationStatusInvalidArgument, "invalid alert.rules.health params"
	}
	base, status, reason := buildAlertRuleQuery(alert, resource, raw)
	if status != "" {
		return RuleHealthQuery{}, status, reason
	}
	query := RuleHealthQuery{
		AccountID: base.AccountID, Region: base.Region, Namespace: base.Namespace,
		Keyword: base.Keyword, Limit: params.Limit,
		Period: params.Period, Aggregator: strings.TrimSpace(params.Aggregator),
		Timezone: strings.TrimSpace(timezone),
	}
	if query.Limit <= 0 {
		query.Limit = ruleHealthDefaultLimit
	}
	if query.Limit > ruleHealthMaxLimit {
		return RuleHealthQuery{}, chatObservationStatusInvalidArgument, "limit must be at most 20"
	}
	if utf8.RuneCountInString(query.Keyword) > chatRulesMaxKeywordRune {
		return RuleHealthQuery{}, chatObservationStatusInvalidArgument, "keyword exceeds 100 characters"
	}
	if (params.From == nil) != (params.To == nil) {
		return RuleHealthQuery{}, chatObservationStatusInvalidArgument, "from and to must be provided together"
	}
	if params.From != nil {
		query.From, query.To = *params.From, *params.To
	} else {
		query.To = now
		query.From = now - ruleHealthDefaultWindowSeconds
	}
	if query.From <= 0 || query.To <= 0 || query.From >= query.To {
		return RuleHealthQuery{}, chatObservationStatusInvalidArgument, "invalid time range"
	}
	if query.Period <= 0 {
		query.Period = ruleHealthDefaultPeriod
	}
	if query.Period < chatMetricMinPeriod {
		return RuleHealthQuery{}, chatObservationStatusInvalidArgument, "period below minimum 10 seconds"
	}
	aggregator, ok := normalizeRuleHealthAggregator(query.Aggregator)
	if !ok {
		return RuleHealthQuery{}, chatObservationStatusInvalidArgument, "invalid aggregator"
	}
	query.Aggregator = aggregator
	window := query.To - query.From
	if window > chatMetricMaxLongWindowSec {
		return RuleHealthQuery{}, chatObservationStatusInvalidArgument, "time range exceeds maximum 30 days"
	}
	if window > chatMetricMaxWindowSec && query.Period < chatMetricLongWindowMinPeriodSec {
		return RuleHealthQuery{}, chatObservationStatusInvalidArgument, "time range exceeds 7 days; period must be at least 3600"
	}
	if window/int64(query.Period)+1 > ruleHealthMaxPoints {
		return RuleHealthQuery{}, chatObservationStatusInvalidArgument, "too many data points"
	}
	return query, "", ""
}

func normalizeRuleHealthAggregator(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "max", "maximum":
		return "max", true
	case "avg", "average", "mean":
		return "average", true
	case "min", "minimum":
		return "min", true
	case "sum", "total":
		return "sum", true
	case "variance", "var":
		return "variance", true
	default:
		return "", false
	}
}

// refreshRuleHealthObservation 查询多条规则的历史曲线统计（规则体检）。
func (s *ChatService) refreshRuleHealthObservation(ctx context.Context, userID string, session *domain.Session, data *chatContextData, raw map[string]any) bool {
	obs := s.checkTool(ctx, userID, chatToolAlertRulesHealth)
	if !obs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	if s.ruleHealth == nil {
		obs.Status = EvidenceStatusSkipped
		obs.Reason = "rule health collector is not configured"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	query, status, reason := buildRuleHealthQuery(data.Alert, data.Resource, raw, s.now().Unix(), s.timezone)
	if status != "" {
		obs.Status = status
		obs.Reason = reason
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	result, err := s.ruleHealth.Check(ctx, userID, query)
	if err != nil {
		logger.From(ctx).Warn("ai chat rule health check failed", logger.Error(err))
		obs.Status = EvidenceStatusError
		obs.Reason = "rule health check failed"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	if result == nil {
		obs.Status = EvidenceStatusError
		obs.Reason = "empty rule health result"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	obs.Status = result.Status
	if obs.Status == "" {
		obs.Status = EvidenceStatusOK
	}
	obs.Reason = result.Reason
	rules := capRuleHealthForObservation(result.Rules)
	obs.EvidenceID = firstRuleHealthEvidenceID(result.Rules)
	sampled := 0
	for _, rule := range result.Rules {
		sampled += rule.CheckedResources
	}
	obs.Summary = fmt.Sprintf("规则体检 %s（规则 %d 条，成功 %d 条，实例曲线 %d 条）", obs.Status, result.RuleCount, result.CheckedCount, sampled)
	obs.Data = map[string]any{
		"status": obs.Status, "reason": obs.Reason,
		"window_start": result.WindowStart, "window_end": result.WindowEnd,
		"period": result.Period, "aggregator": result.Aggregator,
		"timezone":   result.Timezone,
		"rule_count": result.RuleCount, "checked_count": result.CheckedCount,
		"checked_resource_count": sampled,
		"rules":                  rules,
		"granularity_note": "破限比例与越限时长按请求的 period 聚合口径统计（越限时长为 breach_points × period 的上限估计）；" +
			"breach_by_hour 是按 timezone 统计的 24 小时越限点分布（index 0-23 对应本地 0-23 时，基于全量序列计算），breach_top_hours 为越限最多的小时摘要，breach_days 为出现越限的本地自然日数，breach_first_at_local / peak_at_local 为本地时间文本；判断周期性时先看越限是否集中在固定时段并与窗口天数对比。" +
			"规则采样窗口小于 period 时，平台已自动补充 recent_*（近 24 小时/60s）复核；60s 是 CES 可用的最细历史粒度，1s/15s 精确频率不可还原、也不需要再要求原始粒度复查；频率结论按 recent_* 的 60s 口径给出近似值并注明。recent_breach_duration_seconds 是累计越限时长（breach_points × 60s），recent_longest_breach_seconds 是最长连续越限时长，二者不得混用为同一口径。" +
			"规则多实例时按预算尽量覆盖全部实例（先每条规则 1 个，再轮转补全，单规则上限 200）；覆盖不全时 sampled=true，实际覆盖见 checked_resources。统计取越限最严重的样本；scope_source=resource_discovery 时 resource_count 为空表示规则资源总数未知，scope_note 给出未匹配原因（引擎/维度/发现条数）。point_count=0 的规则附带 probe_point_count（近 1 小时/60s 探针）：大于 0 说明主窗口无点但近期有数据，等于 0 说明近期也无数据（事件型指标无事件/未上报/维度不匹配）",
	}
	appendRuleHealthReferences(data, result.Rules)
	s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
	return obs.Status == EvidenceStatusOK
}

func capRuleHealthForObservation(rules []RuleHealthRuleResult) []RuleHealthRuleResult {
	out := make([]RuleHealthRuleResult, 0, min(len(rules), ruleHealthMaxRules))
	for _, rule := range rules {
		rule.Name = truncateRunes(rule.Name, 200)
		rule.MetricLabel = chatMetricLabel(rule.Metric)
		rule.Unit = truncateRunes(strings.TrimSpace(rule.Unit), 32)
		rule.RuleUnit = truncateRunes(strings.TrimSpace(rule.RuleUnit), 32)
		if len(rule.Dimensions) > 0 {
			if cleaned := sanitizeRuleResources([]map[string]string{rule.Dimensions}); len(cleaned) == 1 {
				rule.Dimensions = cleaned[0]
			} else {
				rule.Dimensions = nil
			}
		}
		out = append(out, rule)
		if len(out) >= ruleHealthMaxRules {
			break
		}
	}
	return out
}

func appendRuleHealthReferences(data *chatContextData, rules []RuleHealthRuleResult) {
	appended := 0
	for _, rule := range rules {
		if appended >= chatRuleHealthMaxReferences {
			return
		}
		evidenceID := strings.TrimSpace(rule.EvidenceID)
		if evidenceID == "" {
			continue
		}
		appendObservabilityEvidenceReference(data, evidenceID, "规则体检证据 "+evidenceID)
		appended++
	}
}

// firstRuleHealthEvidenceID 用于 tool_invoke 审计的单值 evidence_id（多规则时取首条成功证据）。
func firstRuleHealthEvidenceID(rules []RuleHealthRuleResult) string {
	for _, rule := range rules {
		if evidenceID := strings.TrimSpace(rule.EvidenceID); evidenceID != "" {
			return evidenceID
		}
	}
	return ""
}
