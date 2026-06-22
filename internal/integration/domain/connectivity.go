package domain

import "time"

// ConnectivityStatus 连通性测试结果状态。
type ConnectivityStatus string

const (
	ConnectivityOK      ConnectivityStatus = "ok"
	ConnectivityFailed  ConnectivityStatus = "failed"
	ConnectivityDegraded ConnectivityStatus = "degraded"
)

// ConnectivityCheck 连通性测试结果。
type ConnectivityCheck struct {
	CheckID      string
	AccountID    string
	Status       ConnectivityStatus
	Provider     ProviderType
	Capabilities []Capability
	Message      string
	CheckedAt    time.Time
}
