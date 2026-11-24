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
	"github.com/734965549/aiops/pkg/logger"
)

const (
	ruleHealthTotalTimeout = 75 * time.Second
	ruleHealthRulesCap     = 20
	// ruleHealthMaxResourceSamples 限制单条规则取数的实例样本数，避免多实例规则放大调用量。
	ruleHealthMaxResourceSamples = 3
	// ruleHealthMaxMetricQueries 限制单次体检的总指标查询数；
	// 覆盖策略为广度优先：先保证每条规则至少 1 个实例，预算有余再补第 2/3 个实例。
	ruleHealthMaxMetricQueries = 60
	// ruleHealthDiscoveryLimit 是资源发现兜底的单次拉取条数，足够覆盖 3 个样本并容忍无效行。
	ruleHealthDiscoveryLimit = 12
	// ruleHealthMaxProbes 限制单次体检对零数据规则做近端探测的次数。
	ruleHealthMaxProbes       = 6
	ruleHealthProbeWindowSecs = int64(3600)
	ruleHealthProbePeriod     = 60
	// ruleHealthLongWindowSec 与 AI 工具侧的 7 天长窗口阈值一致，只影响 AllowLongWindow 标记。
	ruleHealthLongWindowSec = int64(7 * 24 * 3600)
)

// RuleHealthCollector 实现 aiapp.RuleHealthPort：
// 先按 namespace 拉取规则，再对每条规则按关联实例抽样取历史曲线并计算统计量；
// 规则没有任何资源绑定时，按 namespace 对应的资源类型发现实例做兜底抽样。
type RuleHealthCollector struct {
	rules         AlertRulesQuerier
	metrics       MetricsQuerier
	accounts      AccountResolver
	accountAccess aiapp.AccountAccessAuthorizer
	// resources 为可选的资源发现能力：云产品级规则没有实例维度时用它兜底。
	resources    aiapp.ResourceDiscoveryPort
	timeout      time.Duration
	totalTimeout time.Duration
	// queryBudget 为单次体检的指标查询上限，0 表示使用默认值（测试可注入更小预算）。
	queryBudget int
}

func NewRuleHealthCollector(
	rules AlertRulesQuerier,
	metrics MetricsQuerier,
	accounts AccountResolver,
	accountAccess aiapp.AccountAccessAuthorizer,
	resources aiapp.ResourceDiscoveryPort,
	timeout time.Duration,
) *RuleHealthCollector {
	if timeout <= 0 {
		timeout = defaultEvidenceTimeout
	}
	return &RuleHealthCollector{
		rules: rules, metrics: metrics, accounts: accounts, accountAccess: accountAccess,
		resources: resources, timeout: timeout, totalTimeout: ruleHealthTotalTimeout,
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
	queries := 0
	probes := 0
	budget := c.effectiveQueryBudget()
	fallbackScopes := c.collectFallbackScopes(checkCtx, userID, q, rules)
	for i, rule := range rules {
		perRule := samplesPerRule(budget-queries, len(rules)-i)
		item := c.checkRule(checkCtx, userID, q, rule, fallbackScopes[fallbackScopeKey(rule)], perRule, &queries, &probes)
		if item.Status == aiapp.EvidenceStatusOK {
			out.CheckedCount++
		}
		out.Rules = append(out.Rules, item)
	}
	return out, nil
}

// fallbackScopeKey 是资源发现兜底的缓存键：同 namespace 但维度/引擎不同的规则不能共用样本。
func fallbackScopeKey(rule obsdomain.AlertRule) string {
	return strings.TrimSpace(rule.Namespace) + "|" + strings.TrimSpace(rule.DimensionName) + "|" + ruleEngineHint(rule)
}

// ruleEngineHint 从规则名/描述推断目标引擎（CES 规则资源不携带引擎）。
func ruleEngineHint(rule obsdomain.AlertRule) string {
	text := strings.ToLower(strings.TrimSpace(rule.Name) + " " + strings.TrimSpace(rule.Description))
	switch {
	case strings.Contains(text, "postgres"):
		return "postgresql"
	case strings.Contains(text, "mysql"):
		return "mysql"
	case strings.Contains(text, "sqlserver"):
		return "sqlserver"
	}
	return ""
}

// samplesPerRule 按剩余预算与剩余规则数分配每条规则的实例样本数（广度优先，1..3）。
func samplesPerRule(remainingBudget, remainingRules int) int {
	if remainingRules <= 0 || remainingBudget <= 0 {
		return 1
	}
	share := remainingBudget / remainingRules
	if share < 1 {
		share = 1
	}
	if share > ruleHealthMaxResourceSamples {
		share = ruleHealthMaxResourceSamples
	}
	return share
}

func (c *RuleHealthCollector) effectiveQueryBudget() int {
	if c != nil && c.queryBudget > 0 {
		return c.queryBudget
	}
	return ruleHealthMaxMetricQueries
}

func (c *RuleHealthCollector) effectiveTotalTimeout() time.Duration {
	if c.totalTimeout > 0 {
		return c.totalTimeout
	}
	return ruleHealthTotalTimeout
}

// collectFallbackScopes 为没有任何资源维度的规则按 namespace 发现实例样本（每个 namespace 只查一次）。
// 云产品级规则在 CES 规则详情里没有实例维度，只能从已同步资源推导查询维度。
func (c *RuleHealthCollector) collectFallbackScopes(ctx context.Context, userID string, q aiapp.RuleHealthQuery, rules []obsdomain.AlertRule) map[string][]map[string]string {
	out := make(map[string][]map[string]string)
	if c.resources == nil {
		return out
	}
	attempted := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		if len(rule.Resources) > 0 {
			continue
		}
		namespace := strings.TrimSpace(rule.Namespace)
		if namespace == "" {
			continue
		}
		key := fallbackScopeKey(rule)
		if _, ok := attempted[key]; ok {
			continue
		}
		attempted[key] = struct{}{}
		out[key] = c.discoverScopeSamples(ctx, userID, q, namespace, strings.TrimSpace(rule.DimensionName), ruleEngineHint(rule))
	}
	return out
}

// discoverScopeSamples 按 namespace/维度/引擎发现最多 3 个实例维度样本。
// dimensionName 来自 CES 规则的 product_name；engineHint 用于避免把 PG 规则抽到 MySQL 实例。
func (c *RuleHealthCollector) discoverScopeSamples(ctx context.Context, userID string, q aiapp.RuleHealthQuery, namespace, dimensionName, engineHint string) []map[string]string {
	resourceType := resourceTypeFromCESNamespace(namespace)
	if resourceType == "" {
		return nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	result, err := c.resources.List(queryCtx, userID, aiapp.ResourceQuery{
		AccountID: q.AccountID, Region: q.Region, ResourceType: resourceType, Limit: ruleHealthDiscoveryLimit,
	})
	if err != nil || result == nil || result.Status != aiapp.EvidenceStatusOK {
		logger.From(ctx).Warn("ai rule health resource discovery fallback failed",
			logger.String("namespace", namespace),
			logger.String("resource_type", resourceType),
			logger.Error(err))
		return nil
	}
	type candidate struct {
		dims   map[string]string
		engine string
	}
	candidates := make([]candidate, 0, len(result.Resources))
	for _, item := range result.Resources {
		enriched := aiapp.EnrichResourceSummary(item)
		if enriched.Namespace != "" && !strings.EqualFold(enriched.Namespace, namespace) {
			continue
		}
		if dimensionName != "" && enriched.DimensionName != "" &&
			!strings.EqualFold(enriched.DimensionName, dimensionName) {
			continue
		}
		name := enriched.DimensionName
		if dimensionName != "" {
			name = dimensionName
		}
		if name == "" || enriched.DimensionValue == "" {
			continue
		}
		candidates = append(candidates, candidate{
			dims:   map[string]string{name: enriched.DimensionValue},
			engine: strings.ToLower(strings.TrimSpace(enriched.Engine)),
		})
	}
	if engineHint != "" {
		matched := make([]candidate, 0, len(candidates))
		for _, cand := range candidates {
			if cand.engine != "" && strings.Contains(cand.engine, engineHint) {
				matched = append(matched, cand)
			}
		}
		// 只有确认存在引擎标签时才按引擎过滤，避免资源标签缺失导致完全抽不到样本。
		if len(matched) > 0 {
			candidates = matched
		}
	}
	samples := make([]map[string]string, 0, ruleHealthMaxResourceSamples)
	seen := make(map[string]struct{}, ruleHealthMaxResourceSamples)
	for _, cand := range candidates {
		key := scopeSampleKey(cand.dims)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		samples = append(samples, cand.dims)
		if len(samples) >= ruleHealthMaxResourceSamples {
			break
		}
	}
	return samples
}

func scopeSampleKey(dims map[string]string) string {
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

// resourceTypeFromCESNamespace 由 CES namespace 推导云资源类型（SYS.RDS -> rds）。
func resourceTypeFromCESNamespace(namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return ""
	}
	segments := strings.Split(namespace, ".")
	return strings.ToLower(strings.TrimSpace(segments[len(segments)-1]))
}

// checkRule 对单条规则按关联实例抽样取数，统计字段取越限最严重的样本；maxSamples 是
// 本次预算分配给该规则的实例样本数（广度优先，最多 3 个）。
// fallback 是规则没有资源绑定时由资源发现得到的实例样本；queries 为单次体检共享的查询计数。
func (c *RuleHealthCollector) checkRule(ctx context.Context, userID string, q aiapp.RuleHealthQuery, rule obsdomain.AlertRule, fallback []map[string]string, maxSamples int, queries, probes *int) aiapp.RuleHealthRuleResult {
	item := aiapp.RuleHealthRuleResult{
		RuleID: rule.RuleID, Name: rule.Name, Metric: rule.Metric,
		Operator: rule.Operator, Threshold: rule.Threshold, RuleUnit: rule.Unit,
		WindowSeconds: rule.WindowSeconds, ResourceCount: rule.ResourceCount,
		ResourceGroupCount: len(rule.ResourceGroupIDs),
		ResourceLevel:      rule.ResourceLevel,
		DimensionName:      rule.DimensionName,
		Status:             aiapp.EvidenceStatusOK,
	}
	if strings.TrimSpace(rule.Metric) == "" {
		item.Status = aiapp.EvidenceStatusSkipped
		item.Reason = "no_metric"
		return item
	}
	samples := rule.Resources
	source := "rule"
	if len(rule.ResourceGroupIDs) > 0 {
		source = "resource_group"
	}
	if len(samples) == 0 {
		samples = fallback
		source = "resource_discovery"
	}
	if len(samples) == 0 {
		item.Status = aiapp.EvidenceStatusSkipped
		item.Reason = "no_resource_scope"
		return item
	}
	if maxSamples <= 0 || maxSamples > ruleHealthMaxResourceSamples {
		maxSamples = ruleHealthMaxResourceSamples
	}
	availableSamples := len(samples)
	if len(samples) > maxSamples {
		samples = samples[:maxSamples]
	}
	var best *aiapp.RuleHealthRuleResult
	checked := 0
	lastStatus, lastReason := "", ""
	incomplete := availableSamples > len(samples)
	for _, dims := range samples {
		if ctx.Err() != nil {
			lastStatus, lastReason = aiapp.EvidenceStatusSkipped, "rule health query timeout"
			incomplete = true
			break
		}
		if queries != nil && *queries >= c.effectiveQueryBudget() {
			lastStatus, lastReason = aiapp.EvidenceStatusSkipped, "rule health query budget exhausted"
			incomplete = true
			break
		}
		if queries != nil {
			*queries = *queries + 1
		}
		res, status, reason := c.queryRuleMetrics(ctx, userID, q, rule, dims)
		if status != "" {
			lastStatus, lastReason = status, reason
			incomplete = true
			continue
		}
		candidate := aiapp.RuleHealthRuleResult{
			Dimensions: cloneDimensions(dims),
			EvidenceID: res.EvidenceID,
			Status:     aiapp.EvidenceStatusOK,
		}
		summarizeRuleHealth(&candidate, res.Series, rule, q.Period)
		checked++
		if best == nil || betterRuleHealthSample(&candidate, best) {
			sample := candidate
			best = &sample
		}
	}
	if best != nil {
		merged := *best
		merged.RuleID, merged.Name, merged.Metric = item.RuleID, item.Name, item.Metric
		merged.Operator, merged.Threshold, merged.RuleUnit = item.Operator, item.Threshold, item.RuleUnit
		merged.WindowSeconds, merged.ResourceCount = item.WindowSeconds, item.ResourceCount
		merged.ResourceGroupCount, merged.CheckedResources = item.ResourceGroupCount, checked
		merged.ResourceLevel, merged.DimensionName = item.ResourceLevel, item.DimensionName
		merged.ScopeSource = source
		// 分组型规则的 ResourceCount 是分组数，不能用来判断是否抽样；分组型与实例发现兜底一律标记 Sampled。
		merged.Sampled = source == "resource_discovery" || item.ResourceGroupCount > 0 || item.ResourceCount > checked || incomplete
		if source == "resource_discovery" {
			// 实例发现兜底时规则本身没有资源总数，避免把抽样数误当成规则资源数。
			merged.ResourceCount = 0
		}
		if merged.PointCount == 0 {
			c.probeEmptySample(ctx, userID, q, rule, &merged, probes)
		}
		return merged
	}
	item.CheckedResources = 0
	item.Status = lastStatus
	item.Reason = lastReason
	if item.Status == "" {
		item.Status = aiapp.EvidenceStatusError
		item.Reason = "metric query failed"
	}
	return item
}

// queryRuleMetrics 在单实例上取曲线；返回稳定状态与原因，供规则级降级展示。
func (c *RuleHealthCollector) queryRuleMetrics(ctx context.Context, userID string, q aiapp.RuleHealthQuery, rule obsdomain.AlertRule, dims map[string]string) (*obsapp.MetricQueryResult, string, string) {
	return c.queryRuleMetricsRange(ctx, userID, q, rule, dims, q.From, q.To, q.Period)
}

// queryRuleMetricsRange 按指定窗口/粒度取单实例曲线（零数据探测使用近端短窗口）。
func (c *RuleHealthCollector) queryRuleMetricsRange(ctx context.Context, userID string, q aiapp.RuleHealthQuery, rule obsdomain.AlertRule, dims map[string]string, from, to int64, period int) (*obsapp.MetricQueryResult, string, string) {
	queryCtx, cancel := context.WithTimeout(ctx, c.timeout)
	res, err := c.metrics.QueryMetrics(queryCtx, obsapp.Actor{UserID: userID}, obsdomain.MetricQuery{
		AccountID: q.AccountID, Region: q.Region, Namespace: q.Namespace, Metric: rule.Metric,
		Dimensions: cloneDimensions(dims),
		From:       from, To: to, Period: period, Aggregator: q.Aggregator,
		SnapshotRequired: true,
		AllowLongWindow:  to-from > ruleHealthLongWindowSec,
	})
	timedOut := errors.Is(queryCtx.Err(), context.DeadlineExceeded)
	cancel()
	if err != nil {
		if timedOut {
			return nil, aiapp.EvidenceStatusError, "metric query timeout"
		}
		return nil, aiapp.EvidenceStatusError, evidenceErrorReason(err, "metric query failed")
	}
	if res == nil {
		return nil, aiapp.EvidenceStatusError, "empty metric query result"
	}
	return res, "", ""
}

// probeEmptySample 对零数据样本做近 1 小时/60s 探测，区分“近期有数据但窗口无点”和“近期也无数据”。
func (c *RuleHealthCollector) probeEmptySample(ctx context.Context, userID string, q aiapp.RuleHealthQuery, rule obsdomain.AlertRule, sample *aiapp.RuleHealthRuleResult, probes *int) {
	if sample == nil || len(sample.Dimensions) == 0 || ctx.Err() != nil {
		return
	}
	if probes != nil && *probes >= ruleHealthMaxProbes {
		return
	}
	from := q.To - ruleHealthProbeWindowSecs
	if from < 0 || from >= q.To {
		return
	}
	if probes != nil {
		*probes = *probes + 1
	}
	res, status, _ := c.queryRuleMetricsRange(ctx, userID, q, rule, sample.Dimensions, from, q.To, ruleHealthProbePeriod)
	if status != "" || res == nil {
		return
	}
	count := 0
	for _, series := range res.Series {
		count += len(series.Points)
	}
	sample.ProbePointCount = count
	sample.ProbePeriod = ruleHealthProbePeriod
	sample.ProbeWindowSeconds = ruleHealthProbeWindowSecs
}

// betterRuleHealthSample 选择越限最严重的实例样本：破限比例优先，其次破限点数与峰值。
func betterRuleHealthSample(candidate, current *aiapp.RuleHealthRuleResult) bool {
	ratio := func(item *aiapp.RuleHealthRuleResult) float64 {
		if item.BreachRatio != nil {
			return *item.BreachRatio
		}
		return 0
	}
	if ratio(candidate) != ratio(current) {
		return ratio(candidate) > ratio(current)
	}
	if candidate.BreachPoints != current.BreachPoints {
		return candidate.BreachPoints > current.BreachPoints
	}
	peak := func(item *aiapp.RuleHealthRuleResult) float64 {
		if item.Peak != nil {
			return *item.Peak
		}
		return math.Inf(-1)
	}
	return peak(candidate) > peak(current)
}

// summarizeRuleHealth 汇总定稿点数、峰值、P95、破限比例与破限片段统计；空序列只保留 point_count=0。
// 只使用第一条有数据点的序列，避免多序列（异常 provider 返回）混算不同实例。
func summarizeRuleHealth(item *aiapp.RuleHealthRuleResult, series []obsdomain.MetricSeries, rule obsdomain.AlertRule, period int) {
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
	episodes, currentRun, longestRun, inBreach := 0, 0, 0, false
	peakValue, peakAt := points[0].Value, int64(points[0].TS)
	for _, p := range points {
		values = append(values, p.Value)
		sum += p.Value
		if p.Value > peakValue {
			peakValue = p.Value
			peakAt = int64(p.TS)
		}
		if isBreachPoint(rule.Operator, p.Value, rule.Threshold) {
			breach++
			if !inBreach {
				episodes++
				inBreach = true
				currentRun = 0
			}
			currentRun++
			if currentRun > longestRun {
				longestRun = currentRun
			}
		} else {
			inBreach = false
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
	item.PeakAt = &peakAt
	item.BreachPoints, item.BreachRatio = breach, &ratio
	item.BreachEpisodes = episodes
	if period > 0 {
		item.BreachDurationSeconds = int64(breach) * int64(period)
		item.LongestBreachSeconds = int64(longestRun) * int64(period)
	}
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
