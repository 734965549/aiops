package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// AlertSyncLeaseRepository 基于 alert_source_sync_lease 实现跨实例数据库租约。
type AlertSyncLeaseRepository struct {
	db *gorm.DB
}

func NewAlertSyncLeaseRepository(db *gorm.DB) *AlertSyncLeaseRepository {
	return &AlertSyncLeaseRepository{db: db}
}

// Acquire 使用 INSERT ... ON CONFLICT DO UPDATE 原子抢占或续租：
// 仅在原租约已过期或 owner 相同时成功，避免跨 API 实例重复拉取。
func (r *AlertSyncLeaseRepository) Acquire(ctx context.Context, sourceID, ownerID string, ttl time.Duration) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("alert sync lease repository is not configured")
	}
	now := time.Now().UTC()
	until := now.Add(ttl)
	// 租约表是协调数据，不承载业务字段；updated_at 由本语句显式维护。
	res := r.db.WithContext(ctx).Exec(`
INSERT INTO alert_source_sync_lease (source_id, owner_id, lease_until, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (source_id) DO UPDATE
SET owner_id = EXCLUDED.owner_id,
    lease_until = EXCLUDED.lease_until,
    updated_at = EXCLUDED.updated_at
WHERE alert_source_sync_lease.lease_until <= ? OR alert_source_sync_lease.owner_id = ?`,
		sourceID, ownerID, until, now, now, now, ownerID)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *AlertSyncLeaseRepository) Renew(ctx context.Context, sourceID, ownerID string, ttl time.Duration) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("alert sync lease repository is not configured")
	}
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Exec(`
UPDATE alert_source_sync_lease
SET lease_until = ?, updated_at = ?
WHERE source_id = ? AND owner_id = ? AND lease_until > ?`,
		now.Add(ttl), now, sourceID, ownerID, now)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *AlertSyncLeaseRepository) Release(ctx context.Context, sourceID, ownerID string) error {
	if r == nil || r.db == nil {
		return errors.New("alert sync lease repository is not configured")
	}
	return r.db.WithContext(ctx).Exec(
		`DELETE FROM alert_source_sync_lease WHERE source_id = ? AND owner_id = ?`,
		sourceID, ownerID).Error
}

// CommitSyncState 在同一条件更新中校验租约，防止旧 worker 在租约过期后覆盖新一轮状态。
func (r *AlertSyncLeaseRepository) CommitSyncState(ctx context.Context, sourceID, ownerID string, at time.Time, status string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("alert sync lease repository is not configured")
	}
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&alertSourceModel{}).
		Where(`source_id = ? AND EXISTS (
			SELECT 1 FROM alert_source_sync_lease
			WHERE source_id = ? AND owner_id = ? AND lease_until > ?
		)`, sourceID, sourceID, ownerID, now).
		Updates(map[string]any{
			"last_sync_at":     at.UTC(),
			"last_sync_status": status,
			"updated_at":       now,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
