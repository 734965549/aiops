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
	Summary              []byte
	StartedAt            time.Time
	FinishedAt           *time.Time
	// FencingToken 是后台同步任务持有的租约所有权令牌；续租和写前校验必须匹配。
	FencingToken string
	// TriggeredBy 触发本批次的用户 user_id。TriggerSync 创建 running 批次时写入，
	// 用于进程崩溃后还原原批次操作者；reap 崩溃批次时 sync_reaped 审计 actor 取该字段。
	TriggeredBy string
	// LeaseExpiresAt 仅 running 批次写入；终态批次为 nil。
	// 超时后由下一次同步 reap 为 failed，实现崩溃批次的自动释放，
	// 避免 TriggerSync 因残留 running 批次永久 409。见 ops/huawei-ces-sync-contract.md §13。
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

// CloudSyncLabelPatch 描述一次 hybrid 第二阶段增强 label 回写：按 CloudResourceKey 定位资源，
// 整体替换 labels。仅更新本轮已 upsert 的 active 资源，见 ops/huawei-ces-sync-contract.md §8.2。
type CloudSyncLabelPatch struct {
	CloudResourceKey
	Labels map[string]string
}

// ReapedSyncBatch 描述被 reap 的崩溃批次摘要，供应用层写 sync_reaped 审计。
type ReapedSyncBatch struct {
	BatchID              string
	IntegrationAccountID string
	// TriggeredBy 原批次触发用户；作为 sync_reaped 审计的 actor，避免归因到当次请求用户。
	TriggeredBy string
}
