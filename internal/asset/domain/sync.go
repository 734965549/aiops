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
	// LeaseExpiresAt 仅 running 批次写入；终态批次为 nil。
	// 超时后由下一次同步 reap 为 failed，实现崩溃批次的自动释放，
	// 避免 TriggerSync 因残留 running 批次永久 409。见 docs/huawei-ces-asset-sync-plan.md §P1。
	LeaseExpiresAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CloudResourceKey 云资源唯一键（账号 + 区域 + 类型 + 云 ID）。
// region 必填，避免多区域同类型同云 ID 资源互相覆盖。
type CloudResourceKey struct {
	IntegrationAccountID string
	CloudResourceType    string
	CloudResourceID      string
	Region               string
}
