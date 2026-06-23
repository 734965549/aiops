package domain

// MetricQuery 指标查询条件，见 ops/cloud-observability-contract.md §5.1。
type MetricQuery struct {
	AccountID   string
	Provider    string
	Region      string
	Namespace   string
	Metric      string
	Dimensions  map[string]string
	From        int64
	To          int64
	Period      int
	Aggregator  string
}

// LogQuery 日志搜索条件，见 ops/cloud-observability-contract.md §5.2。
type LogQuery struct {
	AccountID  string
	Provider   string
	Service    string
	ResourceID string
	Keyword    string
	TraceID    string
	From       int64
	To         int64
	Limit      int
}

// TraceQuery 链路查询条件，见 ops/cloud-observability-contract.md §5.3。
type TraceQuery struct {
	AccountID     string
	Provider      string
	Service       string
	Operation     string
	TraceID       string
	ErrorOnly     bool
	MinLatencyMS  int
	From          int64
	To            int64
	Limit         int
}

// TopologyQuery 拓扑查询条件，见 ops/cloud-observability-contract.md §5.4。
type TopologyQuery struct {
	AccountID     string
	Provider      string
	ApplicationID string
	From          int64
	To            int64
}

// AssetDiscoveryQuery 云资源发现查询（Provider Port，供 Asset Sync 与 Agent 工具复用）。
type AssetDiscoveryQuery struct {
	AccountID    string
	Provider     string
	Region       string
	ResourceType string
	Keyword      string
	Limit        int
}

// AlertRuleQuery 告警规则查询（Provider Port，供巡检与 Agent 工具复用）。
type AlertRuleQuery struct {
	AccountID string
	Provider  string
	Region    string
	Namespace string
	Keyword   string
	Limit     int
}
