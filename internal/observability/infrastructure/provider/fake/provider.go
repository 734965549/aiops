// Package fake 占位只读 Provider 实现，供各厂商 Adapter 委托；不含厂商专属逻辑。
package fake

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/observability/domain"
)

// Provider 返回确定性样本数据，供 DDD 边界与 API 联调。
type Provider struct {
	providerType string
}

func New(providerType string) *Provider {
	return &Provider{providerType: strings.TrimSpace(providerType)}
}

func (p *Provider) ProviderType() string {
	if p == nil {
		return ""
	}
	return p.providerType
}

func (p *Provider) QueryMetrics(_ context.Context, pctx domain.ProviderContext, q domain.MetricQuery) ([]domain.MetricSeries, error) {
	metric := strings.TrimSpace(q.Metric)
	if metric == "" {
		return nil, domain.ErrInvalidArgument
	}
	period := q.Period
	if period <= 0 {
		period = 60
	}
	from := q.From
	to := q.To
	if to <= from {
		to = from + int64(period*3)
	}
	points := make([]domain.MetricPoint, 0, 3)
	for ts := from; ts <= to; ts += int64(period) {
		points = append(points, domain.MetricPoint{TS: ts, Value: fakeMetricValue(metric, ts)})
	}
	labels := cloneLabels(q.Dimensions)
	if labels == nil {
		labels = map[string]string{}
	}
	labels["provider"] = p.providerType
	labels["account_id"] = pctx.Account.AccountID
	if q.Region != "" {
		labels["region"] = q.Region
	}
	return []domain.MetricSeries{{
		Metric: metric,
		Unit:   fakeMetricUnit(metric),
		Labels: labels,
		Points: points,
	}}, nil
}

func (p *Provider) SearchLogs(_ context.Context, pctx domain.ProviderContext, q domain.LogQuery) ([]domain.LogEntry, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	service := strings.TrimSpace(q.Service)
	if service == "" {
		service = "demo-service"
	}
	keyword := strings.TrimSpace(q.Keyword)
	entries := make([]domain.LogEntry, 0, min(limit, 3))
	now := q.To
	if now <= 0 {
		now = time.Now().Unix()
	}
	messages := []string{
		fmt.Sprintf("[%s] request completed", service),
		fmt.Sprintf("[%s] health check ok", service),
	}
	if keyword != "" {
		messages = append(messages, fmt.Sprintf("[%s] matched keyword %q", service, keyword))
	}
	for i, msg := range messages {
		if i >= limit {
			break
		}
		entries = append(entries, domain.LogEntry{
			Timestamp: now - int64(i*30),
			Level:     "INFO",
			Service:   service,
			Message:   msg,
			TraceID:   strings.TrimSpace(q.TraceID),
			Labels: map[string]string{
				"provider":   p.providerType,
				"account_id": pctx.Account.AccountID,
				"fake":       "true",
			},
			Ref: fmt.Sprintf("logref-%s-%d", p.providerType, i),
		})
	}
	return entries, nil
}

func (p *Provider) QueryTraces(_ context.Context, pctx domain.ProviderContext, q domain.TraceQuery) ([]domain.TraceSpan, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	service := strings.TrimSpace(q.Service)
	if service == "" {
		service = "demo-service"
	}
	operation := strings.TrimSpace(q.Operation)
	if operation == "" {
		operation = "GET /health"
	}
	traceID := strings.TrimSpace(q.TraceID)
	if traceID == "" {
		traceID = fmt.Sprintf("trace-fake-%s", p.providerType)
	}
	span := domain.TraceSpan{
		TraceID:    traceID,
		SpanID:     fmt.Sprintf("span-%s-1", p.providerType),
		Service:    service,
		Operation:  operation,
		StartTime:  q.From,
		DurationMS: 120,
		Status:     "ok",
		Error:      false,
	}
	if q.ErrorOnly {
		span.Status = "error"
		span.Error = true
		span.ErrorSummary = "simulated downstream timeout"
		span.DurationMS = max(q.MinLatencyMS, 800)
	}
	_ = pctx
	_ = limit
	return []domain.TraceSpan{span}, nil
}

func (p *Provider) QueryTopology(_ context.Context, _ domain.ProviderContext, q domain.TopologyQuery) (*domain.TopologySnapshot, error) {
	appID := strings.TrimSpace(q.ApplicationID)
	if appID == "" {
		appID = "app-demo"
	}
	gateway := fmt.Sprintf("svc-gateway-%s", appID)
	backend := fmt.Sprintf("svc-backend-%s", appID)
	return &domain.TopologySnapshot{
		Nodes: []domain.TopologyNode{
			{NodeID: gateway, Name: gateway, Type: "service", ErrorRate: 0.01, P95MS: 45},
			{NodeID: backend, Name: backend, Type: "service", ErrorRate: 0.03, P95MS: 180},
		},
		Edges: []domain.TopologyEdge{
			{From: gateway, To: backend, CallCount: 1280, ErrorRate: 0.02},
		},
	}, nil
}

func (p *Provider) ListResources(_ context.Context, pctx domain.ProviderContext, q domain.AssetDiscoveryQuery) ([]domain.CloudResource, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	region := strings.TrimSpace(q.Region)
	if region == "" && len(pctx.Account.Regions) > 0 {
		region = pctx.Account.Regions[0]
	}
	if region == "" {
		region = "cn-north-4"
	}
	resType := strings.TrimSpace(q.ResourceType)
	if resType == "" {
		resType = "ecs"
	}
	resources := []domain.CloudResource{
		{
			ResourceID:  fmt.Sprintf("res-fake-%s-1", p.providerType),
			Name:        fmt.Sprintf("%s-demo-1", resType),
			Type:        resType,
			Region:      region,
			Status:      "running",
			ProviderRef: fmt.Sprintf("%s/demo-1", p.providerType),
			Labels:      map[string]string{"provider": p.providerType, "fake": "true"},
		},
	}
	if limit >= 2 {
		resources = append(resources, domain.CloudResource{
			ResourceID:  fmt.Sprintf("res-fake-%s-2", p.providerType),
			Name:        fmt.Sprintf("%s-demo-2", resType),
			Type:        resType,
			Region:      region,
			Status:      "running",
			ProviderRef: fmt.Sprintf("%s/demo-2", p.providerType),
			Labels:      map[string]string{"provider": p.providerType, "fake": "true"},
		})
	}
	return resources[:min(len(resources), limit)], nil
}

func (p *Provider) ListAlertRules(_ context.Context, pctx domain.ProviderContext, q domain.AlertRuleQuery) ([]domain.AlertRule, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	ns := strings.TrimSpace(q.Namespace)
	if ns == "" {
		ns = "SYS.ECS"
	}
	rules := []domain.AlertRule{
		{
			RuleID:      fmt.Sprintf("rule-fake-%s-cpu", p.providerType),
			Name:        "High CPU Utilization",
			Namespace:   ns,
			Severity:    "major",
			Enabled:     true,
			Description: "fake provider placeholder rule",
			Labels:      map[string]string{"provider": p.providerType, "account_id": pctx.Account.AccountID},
		},
	}
	return rules[:min(len(rules), limit)], nil
}

func fakeMetricValue(metric string, ts int64) float64 {
	base := 40.0
	switch strings.ToLower(metric) {
	case "cpu_util", "cpu_usage":
		base = 42.1
	case "mem_util", "memory_util":
		base = 68.5
	case "disk_util":
		base = 55.0
	}
	return base + float64(ts%10)
}

func fakeMetricUnit(metric string) string {
	switch strings.ToLower(metric) {
	case "cpu_util", "cpu_usage", "mem_util", "memory_util", "disk_util":
		return "Percent"
	default:
		return "Count"
	}
}

func cloneLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
