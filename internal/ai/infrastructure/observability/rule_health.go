// Package observability 将 Observability 查询能力适配为 AI 批量规则体检端口。
package observability

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

const (
	ruleHealthTotalTimeout = 30 * time.Second
	ruleHealthRulesCap     = 20
)

// RuleHealthCollector 实现 aiapp.RuleHealthPort：
// 先按 namespace 拉取规则，再对每条规则用其第一个关联资源维度取历史曲线并计算统计量。
type RuleHealthCollector struct {
	rules         AlertRulesQuerier
	metrics       MetricsQuerier
	accounts      AccountResolver
	accountAccess aiapp.AccountAccessAuthorizer
	timeout       time.Duration
	totalTimeout  time.Duration
}

func NewRuleHealthCollector(
	rules AlertRulesQuerier,
	metrics MetricsQuerier,
	accounts AccountResolver,
	accountAccess aiapp.AccountAccessAuthorizer,
	timeout time.Duration,
) *RuleHealthCollector {
	if timeout <= 0 {
		timeout = defaultEvidenceTimeout
	}
	return &RuleHealthCollector{
		rules: rules, metrics: metrics, accounts: accounts, accountAccess: accountAccess,
		timeout: timeout, totalTimeout: ruleHealthTotalTimeout,
	}
}

var _ aiapp.RuleHealthPort = (*RuleHealthCollector)(nil)

// Check 执行账号/数据范围预检、规则查询与逐规则取数。
// 单条规则取数失败只降级该规则，不阻断其它规则；业务失败以 Result.Status 表达。
func (c *RuleHealthCollector) Check(ctx context.Context, userID string, q aiapp.RuleHealthQuery) (*aiapp.RuleHealthResult, error) {
	if c == nil || c.rules == nil || c.metrics == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai rule health collector is not configured")
	}
	q.AccountID = strings.TrimSpace(q.AccountID)
	q.Region = strings.TrimSpace(q.Region)
	q.Namespace = strings.TrimSpace(q.Namespace)
	q.Keyword = strings.TrimSpace(q.Keyword)
	switch {
	case q.AccountID == "":
		return ruleHealthDegraded(aiapp.EvidenceStatusSkipped, "no_account"), nil
	case q.Namespace == "":
		return ruleHealthDegraded(aiapp.EvidenceStatusSkipped, "no_namespace"), nil
	}
	if c.accounts == nil {
		return ruleHealthDegraded(aiapp.EvidenceStatusError, "account_resolver_unavailable"), nil
	}
	snapshot, err := c.accounts.ResolveAccount(ctx, q.AccountID)
	if err != nil {
		return ruleHealthDegraded(aiapp.EvidenceStatusError, evidenceErrorReason(err, "resolve integration account failed")), nil
	}
	if snapshot == nil {
		return ruleHealthDegraded(aiapp.EvidenceStatusError, "integration account not found"), nil
	}
	if strings.EqualFold(strings.TrimSpace(snapshot.AuthType), "agency") {
		return ruleHealthDegraded(aiapp.EvidenceStatusUnsupported, "agency auth does not support metrics query"), nil
	}
	if c.accountAccess != nil && strings.TrimSpace(userID) != "" {
		ref := aiapp.AccountRef{
			AccountID: snapshot.AccountID, OwnerTeam: snapshot.OwnerTeam,
			Regions: append([]string(nil), snapshot.Regions...),
		}
		if err := c.accountAccess.Authorize(ctx, userID, ref); err != nil {
			return ruleHealthDegraded(aiapp.EvidenceStatusPermissionDenied, "no access to integration account for evidence"), nil
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, c.effectiveTotalTimeout())
	defer cancel()
	result, err := c.rules.ListAlertRules(checkCtx, obsapp.Actor{UserID: userID}, obsdomain.AlertRuleQuery{
		AccountID: q.AccountID, Region: q.Region, Namespace: q.Namespace, Keyword: q.Keyword, Limit: q.Limit,
	})
	if err != nil {
		return ruleHealthDegraded(ruleErrorStatus(err), ruleErrorReason(err)), nil
	}
	if result == nil {
		return ruleHealthDegraded(aiapp.EvidenceStatusError, "empty alert rule result"), nil
	}
	out := &aiapp.RuleHealthResult{
		Status:      aiapp.EvidenceStatusOK,
		WindowStart: q.From, WindowEnd: q.To, Period: q.Period, Aggregator: q.Aggregator,
		RuleCount: len(result.Rules),
	}
	rules := result.Rules
	if len(rules) > ruleHealthRulesCap {
		rules = rules[:ruleHealthRulesCap]
	}
	for _, rule := range rules {
		item := c.checkRule(checkCtx, userID, q, rule)
		if item.Status == aiapp.EvidenceStatusOK {
			out.CheckedCount++
		}
		out.Rules = append(out.Rules, item)
	}
	return out, nil
}

func (c *RuleHealthCollector) effectiveTotalTimeout() time.Duration {
	if c.totalTimeout > 0 {
		return c.totalTimeout
	}
	return ruleHealthTotalTimeout
}

func (c *RuleHealthCollector) checkRule(ctx context.Context, userID string, q aiapp.RuleHealthQuery, rule obsdomain.AlertRule) aiapp.RuleHealthRuleResult {
	item := aiapp.RuleHealthRuleResult{
		RuleID: rule.RuleID, Name: rule.Name, Metric: rule.Metric,
		Operator: rule.Operator, Threshold: rule.Threshold, RuleUnit: rule.Unit,
		WindowSeconds: rule.WindowSeconds, ResourceCount: rule.ResourceCount,
		Status: aiapp.EvidenceStatusOK,
	}
	if strings.TrimSpace(rule.Metric) == "" {
		item.Status = aiapp.EvidenceStatusSkipped
		item.Reason = "no_metric"
		return item
	}
	if len(rule.Resources) == 0 {
		item.Status = aiapp.EvidenceStatusSkipped
		item.Reason = "no_resource_scope"
		return item
	}
	item.Dimensions = cloneDimensions(rule.Resources[0])
	item.Sampled = rule.ResourceCount > 1
	if ctx.Err() != nil {
		item.Status = aiapp.EvidenceStatusSkipped
		item.Reason = "rule health query timeout"
		return item
	}
	queryCtx, cancel := context.WithTimeout(ctx, c.timeout)
	res, err := c.metrics.QueryMetrics(queryCtx, obsapp.Actor{UserID: userID}, obsdomain.MetricQuery{
		AccountID: q.AccountID, Region: q.Region, Namespace: q.Namespace, Metric: rule.Metric,
		Dimensions: cloneDimensions(rule.Resources[0]),
		From:       q.From, To: q.To, Period: q.Period, Aggregator: q.Aggregator,
		SnapshotRequired: true, AllowLongWindow: true,
	})
	timedOut := errors.Is(queryCtx.Err(), context.DeadlineExceeded)
	cancel()
	if err != nil {
		item.Status = aiapp.EvidenceStatusError
		if timedOut {
			item.Reason = "metric query timeout"
		} else {
			item.Reason = evidenceErrorReason(err, "metric query failed")
		}
		return item
	}
	if res == nil {
		item.Status = aiapp.EvidenceStatusError
		item.Reason = "empty metric query result"
		return item
	}
	item.EvidenceID = res.EvidenceID
	summarizeRuleHealth(&item, res.Series, rule)
	return item
}

// summarizeRuleHealth 汇总定稿点数、峰值、P95 与破限比例；空序列只保留 point_count=0。
// 只使用第一条有数据点的序列，避免多序列（异常 provider 返回）混算不同实例。
func summarizeRuleHealth(item *aiapp.RuleHealthRuleResult, series []obsdomain.MetricSeries, rule obsdomain.AlertRule) {
	points := make([]obsdomain.MetricPoint, 0)
	unit := ""
	for _, s := range series {
		if unit == "" {
			unit = strings.TrimSpace(s.Unit)
		}
		if len(s.Points) > 0 {
			points = append([]obsdomain.MetricPoint(nil), s.Points...)
			if strings.TrimSpace(s.Unit) != "" {
				unit = strings.TrimSpace(s.Unit)
			}
			break
		}
	}
	if unit == "" {
		unit = strings.TrimSpace(rule.Unit)
	}
	item.Unit = unit
	item.PointCount = len(points)
	if len(points) == 0 {
		return
	}
	sort.Slice(points, func(i, j int) bool { return points[i].TS < points[j].TS })
	values := make([]float64, 0, len(points))
	sum := 0.0
	breach := 0
	for _, p := range points {
		values = append(values, p.Value)
		sum += p.Value
		if isBreachPoint(rule.Operator, p.Value, rule.Threshold) {
			breach++
		}
	}
	first, last := values[0], values[len(values)-1]
	minValue, peak := values[0], values[0]
	for _, v := range values {
		if v < minValue {
			minValue = v
		}
		if v > peak {
			peak = v
		}
	}
	avg := sum / float64(len(values))
	sortedValues := append([]float64(nil), values...)
	sort.Float64s(sortedValues)
	p95 := sortedValues[int(math.Round(0.95*float64(len(sortedValues)-1)))]
	ratio := float64(breach) / float64(len(values))
	item.First, item.Min, item.Avg = &first, &minValue, &avg
	item.P95, item.Peak, item.Last = &p95, &peak, &last
	item.BreachPoints, item.BreachRatio = breach, &ratio
}

func ruleHealthDegraded(status, reason string) *aiapp.RuleHealthResult {
	return &aiapp.RuleHealthResult{Status: status, Reason: reason}
}

func cloneDimensions(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
