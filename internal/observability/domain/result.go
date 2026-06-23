package domain

// MetricPoint 指标采样点。
type MetricPoint struct {
	TS    int64   `json:"ts"`
	Value float64 `json:"value"`
}

// MetricSeries 指标时间序列。
type MetricSeries struct {
	Metric string            `json:"metric"`
	Unit   string            `json:"unit"`
	Labels map[string]string `json:"labels"`
	Points []MetricPoint     `json:"points"`
}

// LogEntry 脱敏日志摘要。
type LogEntry struct {
	Timestamp int64             `json:"timestamp"`
	Level     string            `json:"level"`
	Service   string            `json:"service"`
	Message   string            `json:"message"`
	TraceID   string            `json:"trace_id,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Ref       string            `json:"ref,omitempty"`
}

// TraceSpan 链路 Span 摘要。
type TraceSpan struct {
	TraceID     string `json:"trace_id"`
	SpanID      string `json:"span_id"`
	Service     string `json:"service"`
	Operation   string `json:"operation"`
	StartTime   int64  `json:"start_time"`
	DurationMS  int    `json:"duration_ms"`
	Status      string `json:"status"`
	Error       bool   `json:"error"`
	ErrorSummary string `json:"error_summary,omitempty"`
}

// TopologyNode 拓扑节点。
type TopologyNode struct {
	NodeID    string            `json:"node_id"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Labels    map[string]string `json:"labels,omitempty"`
	ErrorRate float64           `json:"error_rate,omitempty"`
	P95MS     float64           `json:"p95_ms,omitempty"`
}

// TopologyEdge 拓扑边。
type TopologyEdge struct {
	From      string  `json:"from"`
	To        string  `json:"to"`
	CallCount int64   `json:"call_count"`
	ErrorRate float64 `json:"error_rate,omitempty"`
}

// TopologySnapshot 服务/资源拓扑快照。
type TopologySnapshot struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// CloudResource 云资源摘要（AssetDiscoveryPort 归一化结果）。
type CloudResource struct {
	ResourceID   string            `json:"resource_id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Region       string            `json:"region"`
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels,omitempty"`
	ProviderRef  string            `json:"provider_ref,omitempty"`
}

// AlertRule 告警规则摘要。
type AlertRule struct {
	RuleID      string            `json:"rule_id"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Severity    string            `json:"severity,omitempty"`
	Enabled     bool              `json:"enabled"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// EvidenceRef 查询证据引用，供巡检与 AI 分析复用。
type EvidenceRef struct {
	EvidenceID string
	AccountID  string
	QueryType  string
	QueryHash  string
	Summary    map[string]any
}
