package application

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/734965549/aiops/internal/inspection/domain"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// EvidenceAnalyzer 基于 Observability 证据链做规则分析。
//
// fake 与真实 provider 数据都要经过同一条脱敏证据链；后续可替换成 AI Agent，但不能绕过凭据与执行边界。
type EvidenceAnalyzer struct {
	obs ObservabilityQueryPort
}

func NewEvidenceAnalyzer(obs ObservabilityQueryPort) *EvidenceAnalyzer {
	return &EvidenceAnalyzer{obs: obs}
}

func (a *EvidenceAnalyzer) CollectEvidence(ctx context.Context, actor Actor, input CheckEvidenceInput) (*EvidenceSummary, error) {
	if a == nil || a.obs == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "observability port is not configured")
	}
	if !domain.IsSupportedCheck(input.Check) {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "%s: %s", domain.ErrUnsupportedCheck, input.Check)
	}
	from, to := input.From, input.To
	if to <= from {
		from, to = defaultTimeWindow()
	}
	obsActor := obsapp.Actor{UserID: actor.UserID, DisplayName: actor.DisplayName}
	provider := strings.TrimSpace(input.Provider)
	accountID := strings.TrimSpace(input.AccountID)
	region := strings.TrimSpace(input.Region)
	if region == "" {
		region = "cn-north-4"
	}
	service := strings.TrimSpace(input.Service)
	if service == "" {
		service = "payment-service"
	}

	switch {
	case strings.HasPrefix(input.Check, "metrics."):
		metric := metricForCheck(input.Check)
		res, err := a.obs.QueryMetrics(ctx, obsActor, obsdomain.MetricQuery{
			AccountID: accountID, Provider: provider, Region: region,
			Namespace: "SYS.ECS", Metric: metric, From: from, To: to, Period: 60,
		})
		if err != nil {
			return nil, err
		}
		if res == nil {
			return nil, apperr.New(apperr.CodeInternal, "metrics query returned empty result")
		}
		maxVal := maxMetricValue(res.Series)
		return &EvidenceSummary{Check: input.Check, Type: "metrics", EvidenceID: res.EvidenceID, Metric: metric, MaxValue: maxVal}, nil
	case input.Check == "traces.latency" || input.Check == "traces.error_rate":
		res, err := a.obs.QueryTraces(ctx, obsActor, obsdomain.TraceQuery{
			AccountID: accountID, Provider: provider, Service: service,
			ErrorOnly: input.Check == "traces.error_rate", MinLatencyMS: 500,
			From: from, To: to, Limit: 50,
		})
		if err != nil {
			return nil, err
		}
		if res == nil {
			return nil, apperr.New(apperr.CodeInternal, "trace query returned empty result")
		}
		errCount := 0
		for _, sp := range res.Spans {
			if sp.Error {
				errCount++
			}
		}
		return &EvidenceSummary{
			Check: input.Check, Type: "traces", EvidenceID: res.EvidenceID, SpanCount: len(res.Spans), ErrorSpans: errCount,
		}, nil
	case input.Check == "logs.error_burst":
		res, err := a.obs.SearchLogs(ctx, obsActor, obsdomain.LogQuery{
			AccountID: accountID, Provider: provider, Service: service,
			Keyword: "error", From: from, To: to, Limit: 100,
		})
		if err != nil {
			return nil, err
		}
		if res == nil {
			return nil, apperr.New(apperr.CodeInternal, "log search returned empty result")
		}
		return &EvidenceSummary{Check: input.Check, Type: "logs", EvidenceID: res.EvidenceID, EntryCount: len(res.Entries)}, nil
	default:
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "unsupported check: %s", input.Check)
	}
}

func (a *EvidenceAnalyzer) Analyze(_ context.Context, checks []string, evidence []EvidenceSummary) ([]AnalysisResult, error) {
	for _, check := range checks {
		if !domain.IsSupportedCheck(check) {
			return nil, apperr.Newf(apperr.CodeInvalidArgument, "%s: %s", domain.ErrUnsupportedCheck, check)
		}
	}
	byCheck := map[string]EvidenceSummary{}
	for _, ev := range evidence {
		key := ev.Check
		if key == "" {
			key = ev.Type
		}
		byCheck[key] = ev
	}

	var out []AnalysisResult
	for _, check := range checks {
		ev, ok := byCheck[check]
		if !ok {
			continue
		}
		if ar := analyzeCheck(check, ev); ar != nil {
			out = append(out, *ar)
		}
	}
	return out, nil
}

func analyzeCheck(check string, ev EvidenceSummary) *AnalysisResult {
	ref := []string{ev.EvidenceID}
	switch check {
	case "metrics.cpu":
		if ev.MaxValue >= 80 {
			return newAnalysis(check, "high", "CPU 利用率偏高",
				fmt.Sprintf("最近窗口 CPU 峰值 %.1f%%，超过 80%% 阈值", ev.MaxValue),
				ref, 0.85, "基于单指标阈值规则；未关联变更事件与多维度交叉验证", true)
		}
		if ev.MaxValue >= 60 {
			return newAnalysis(check, "medium", "CPU 利用率需关注",
				fmt.Sprintf("最近窗口 CPU 峰值 %.1f%%，处于 60-80%% 区间", ev.MaxValue),
				ref, 0.72, "阈值规则检测；建议结合业务峰值与历史基线进一步确认", false)
		}
	case "metrics.memory":
		if ev.MaxValue >= 75 {
			return newAnalysis(check, "high", "内存利用率偏高",
				fmt.Sprintf("最近窗口内存峰值 %.1f%%", ev.MaxValue),
				ref, 0.82, "单指标规则；未排除缓存占用导致的正常高水位", true)
		}
	case "metrics.disk":
		if ev.MaxValue >= 70 {
			return newAnalysis(check, "medium", "磁盘利用率需关注",
				fmt.Sprintf("最近窗口磁盘峰值 %.1f%%", ev.MaxValue),
				ref, 0.7, "需确认是否为日志或临时文件增长", true)
		}
	case "traces.error_rate":
		if ev.ErrorSpans > 0 {
			return newAnalysis(check, "high", "链路错误 Span 检出",
				fmt.Sprintf("窗口内检出 %d 个错误 Span（共 %d 条）", ev.ErrorSpans, ev.SpanCount),
				ref, 0.88, "基于 error_only 采样；完整根因需结合日志与拓扑", true)
		}
	case "traces.latency":
		if ev.SpanCount > 0 {
			return newAnalysis(check, "medium", "高延迟链路样本存在",
				fmt.Sprintf("检出 %d 条高延迟 Span 样本", ev.SpanCount),
				ref, 0.65, "仅采样高延迟 Span；P99 基线未纳入本阶段分析", false)
		}
	case "logs.error_burst":
		if ev.EntryCount >= 2 {
			return newAnalysis(check, "medium", "错误日志条目增多",
				fmt.Sprintf("窗口内匹配 error 关键词的日志摘要 %d 条", ev.EntryCount),
				ref, 0.75, "日志为脱敏摘要；完整堆栈需运维在源系统确认", false)
		}
	}
	return nil
}

func newAnalysis(category, risk, summary, detail string, refs []string, confidence float64, uncertainty string, canExec bool) *AnalysisResult {
	recRisk := risk
	if canExec && (risk == "high" || risk == "critical") {
		recRisk = "medium" // 建议转执行默认不直接高风险自动执行
	}
	return &AnalysisResult{
		Category: category, RiskLevel: risk, Summary: summary, Detail: detail,
		AffectedResources: []AffectedResourceDTO{{Type: "service", ID: "payment-service", Name: "payment-service"}},
		EvidenceRefs:      refs, Confidence: confidence, Uncertainty: uncertainty,
		Recommendations: []RecommendationDraft{{
			Title:              summary,
			Reason:             detail,
			SuggestedAction:    suggestedActionFor(category),
			RiskLevel:          recRisk,
			CanCreateExecution: canExec,
			Confidence:         confidence,
			Uncertainty:        uncertainty,
		}},
	}
}

func suggestedActionFor(category string) string {
	switch category {
	case "metrics.cpu":
		return "排查 CPU 热点进程，必要时创建只读诊断任务采集 top/ps 快照"
	case "metrics.memory":
		return "检查内存泄漏或大对象缓存，必要时创建只读诊断任务采集内存摘要"
	case "metrics.disk":
		return "检查磁盘占用目录，必要时创建只读 df/du 诊断任务"
	case "traces.error_rate":
		return "定位错误 Span 对应服务与依赖，结合日志进一步分析"
	case "traces.latency":
		return "分析慢调用链路与下游依赖延迟"
	case "logs.error_burst":
		return "在源日志系统查看 error 堆栈并关联最近变更"
	default:
		return "进一步人工排查"
	}
}

func metricForCheck(check string) string {
	switch check {
	case "metrics.cpu":
		return "cpu_util"
	case "metrics.memory":
		return "mem_util"
	case "metrics.disk":
		return "disk_util"
	default:
		return "cpu_util"
	}
}

func maxMetricValue(series []obsdomain.MetricSeries) float64 {
	max := 0.0
	for _, s := range series {
		for _, p := range s.Points {
			max = math.Max(max, p.Value)
		}
	}
	return max
}
