package domain

import "time"

// MetricQuery 指标查询条件，见 ops/cloud-observability-contract.md §5.1。
type MetricQuery struct {
	AccountID string
	Provider  string
	Region    string
	// ProjectID 可选；多项目账号显式指定，缺省时由 provider 按账号/region 配置解析。
	ProjectID       string
	Namespace       string
	Metric          string
	Dimensions      map[string]string
	From            int64
	To              int64
	Period          int
	Aggregator      string
	ResourceID      string `json:"-"`
	CloudResourceID string `json:"-"`
	// SnapshotRequired 标记 AI 路径必须在定稿后补存 payload；初始记录保持 collecting，不能支撑结论。
	SnapshotRequired bool      `json:"-"`
	CollectedAt      time.Time `json:"-"`
	// AllowLongWindow 仅供 AI 只读工具路径使用：period>=86400 的日聚合允许 30 天窗口。
	// 不参与 HTTP 绑定与 evidence 哈希，普通查询仍保持 7 天上限。
	AllowLongWindow bool `json:"-"`
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
	AccountID    string
	Provider     string
	Service      string
	Operation    string
	TraceID      string
	ErrorOnly    bool
	MinLatencyMS int
	From         int64
	To           int64
	Limit        int
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
