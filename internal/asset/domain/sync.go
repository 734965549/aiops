package domain

import "time"

const (
	SyncBatchStatusRunning = "running"
	SyncBatchStatusSuccess = "success"
	SyncBatchStatusPartial = "partial"
	SyncBatchStatusFailed  = "failed"
)

// SyncBatch 云资源同步批次记录。
type SyncBatch struct {
	BatchID              string
	IntegrationAccountID string
	Provider             string
	Status               string
	CreatedCount         int
	UpdatedCount         int
	StaleCount           int
	FailedCount          int
	Message              string
	StartedAt            time.Time
	FinishedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// CloudResourceKey 云资源唯一键（账号 + 类型 + 云 ID）。
type CloudResourceKey struct {
	IntegrationAccountID string
	CloudResourceType    string
	CloudResourceID      string
}
