// Package http 提供平台 HTTP 响应封装与通用中间件。

package http

// HealthStatus 表示健康检查项的状态。
type HealthStatus string

const (
	HealthStatusOK       HealthStatus = "ok"
	HealthStatusDown     HealthStatus = "down"
	HealthStatusDegraded HealthStatus = "degraded"
	HealthStatusReady    HealthStatus = "ready"
	HealthStatusNotReady HealthStatus = "not_ready"
)

// HealthMigrationDetails 描述迁移检查的结构化附加信息。
type HealthMigrationDetails struct {
	Dir            string                `json:"dir,omitempty"`
	LatestVersion  string                `json:"latest_version,omitempty"`
	AppliedVersion string                `json:"applied_version,omitempty"`
	PendingCount   int                   `json:"pending_count,omitempty"`
	UpToDate       bool                  `json:"up_to_date,omitempty"`
	ChecksumDrifts []HealthChecksumDrift `json:"checksum_drifts,omitempty"`
}

// HealthChecksumDrift 描述单个迁移版本的 checksum 漂移信息。
type HealthChecksumDrift struct {
	Version string `json:"version"`
	Name    string `json:"name,omitempty"`
	Stored  string `json:"stored,omitempty"`
	Current string `json:"current,omitempty"`
}

// HealthDBDetails 描述数据库检查的结构化附加信息。
type HealthDBDetails struct {
	MaxOpenConns      int   `json:"max_open_conns,omitempty"`
	MaxIdleConns      int   `json:"max_idle_conns,omitempty"`
	OpenConnections   int   `json:"open_connections,omitempty"`
	InUseConnections  int   `json:"in_use_connections,omitempty"`
	IdleConnections   int   `json:"idle_connections,omitempty"`
	WaitCount         int64 `json:"wait_count,omitempty"`
	WaitDurationMS    int64 `json:"wait_duration_ms,omitempty"`
	MaxIdleClosed     int64 `json:"max_idle_closed,omitempty"`
	MaxLifetimeClosed int64 `json:"max_lifetime_closed,omitempty"`
	PingTimeoutMS     int64 `json:"ping_timeout_ms,omitempty"`
}

// HealthRedisDetails 描述 Redis 检查的结构化附加信息。
type HealthRedisDetails struct {
	Addr          string `json:"addr,omitempty"`
	DB            int    `json:"db,omitempty"`
	PoolSize      int    `json:"pool_size,omitempty"`
	PingLatencyMS int64  `json:"ping_latency_ms,omitempty"`
	Endpoint      string `json:"endpoint,omitempty"`
}

// HealthCheck 表示单个健康检查项。
type HealthCheck struct {
	Name    string       `json:"name"`
	Status  HealthStatus `json:"status"`
	Error   string       `json:"error,omitempty"`
	Details any          `json:"details,omitempty"`
}

// HealthResponse 是 /healthz /readyz 的统一响应载体。
type HealthResponse struct {
	Status   HealthStatus  `json:"status"`
	Checks   []HealthCheck `json:"checks"`
	UptimeMS int64         `json:"uptime_ms"`
}
