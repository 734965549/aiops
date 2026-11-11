package application

import (
	"context"
	"time"
)

// AlertSyncLeaseStore 是跨 API 实例的接入源拉取租约端口（数据库租约，含过期时间）。
// 手动同步与后台 worker 都必须先获取租约；租约丢失后不得再写入同步状态，
// 见 ops/huawei-ces-alert-sync-contract.md §9。
type AlertSyncLeaseStore interface {
	// Acquire 尝试获取或续租；已有未过期租约且 owner 不同则返回 false。
	Acquire(ctx context.Context, sourceID, ownerID string, ttl time.Duration) (bool, error)
	// Renew 仅当 owner 仍持有未过期租约时延长。
	Renew(ctx context.Context, sourceID, ownerID string, ttl time.Duration) (bool, error)
	// Release 释放本人持有的租约；非本人持有不报错。
	Release(ctx context.Context, sourceID, ownerID string) error
	// CommitSyncState 在租约仍有效时原子写入 last_sync_at/last_sync_status；
	// 租约已过期或易主时返回 false，调用方必须丢弃本轮收尾结果（过期写入保护）。
	CommitSyncState(ctx context.Context, sourceID, ownerID string, at time.Time, status string) (bool, error)
}
