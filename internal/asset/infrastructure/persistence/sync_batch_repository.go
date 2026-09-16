package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type syncBatchModel struct {
	database.BaseModel
	BatchID              string     `gorm:"column:batch_id;type:varchar(64);uniqueIndex;not null"`
	IntegrationAccountID string     `gorm:"column:integration_account_id;type:varchar(64);not null;index:idx_asset_sync_batch_account,priority:1"`
	Provider             string     `gorm:"column:provider;type:varchar(64);not null;default:''"`
	Status               string     `gorm:"column:status;type:varchar(32);not null;default:running"`
	CreatedCount         int        `gorm:"column:created_count;not null;default:0"`
	UpdatedCount         int        `gorm:"column:updated_count;not null;default:0"`
	StaleCount           int        `gorm:"column:stale_count;not null;default:0"`
	FailedCount          int        `gorm:"column:failed_count;not null;default:0"`
	Message              string     `gorm:"column:message;type:text;not null;default:''"`
	Summary              []byte     `gorm:"column:summary;type:jsonb;not null;default:'{}'::jsonb"`
	StartedAt            time.Time  `gorm:"column:started_at;not null"`
	FinishedAt           *time.Time `gorm:"column:finished_at"`
	FencingToken         string     `gorm:"column:fencing_token;type:varchar(64);not null;default:''"`
	TriggeredBy          string     `gorm:"column:triggered_by;type:varchar(64);not null;default:''"`
	LeaseExpiresAt       *time.Time `gorm:"column:lease_expires_at"`
}

func (syncBatchModel) TableName() string { return "asset_sync_batch" }

// SyncBatchRepository 同步批次 GORM 仓储。
type SyncBatchRepository struct {
	db *gorm.DB
}

func NewSyncBatchRepository(db *gorm.DB) *SyncBatchRepository {
	return &SyncBatchRepository{db: db}
}

func (r *SyncBatchRepository) Create(ctx context.Context, batch *domain.SyncBatch) error {
	if r == nil || r.db == nil {
		return errors.New("asset sync batch repository is not configured")
	}
	if batch == nil {
		return errors.New("sync batch is nil")
	}
	m := toSyncBatchModel(batch)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	batch.CreatedAt = m.CreatedAt
	batch.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *SyncBatchRepository) Update(ctx context.Context, batch *domain.SyncBatch) error {
	if r == nil || r.db == nil {
		return errors.New("asset sync batch repository is not configured")
	}
	if batch == nil || strings.TrimSpace(batch.BatchID) == "" {
		return errors.New("sync batch is nil")
	}
	now := time.Now().UTC()
	// 终态 finalize 必须携带 fencing token 且仍持有未过期 running 租约，
	// 避免旧后台任务覆盖已被 reaper 标记为 failed 的批次。
	if batch.Status != domain.SyncBatchStatusRunning && strings.TrimSpace(batch.FencingToken) == "" {
		return domain.ErrLeaseLost
	}
	// 终态批次清空租约，释放账号级 running 槽位（部分唯一索引 WHERE status='running'）。
	leaseVal := (*time.Time)(nil)
	where := r.db.WithContext(ctx).Model(&syncBatchModel{}).Where("batch_id = ?", batch.BatchID)
	if batch.Status == domain.SyncBatchStatusRunning {
		leaseVal = batch.LeaseExpiresAt
	} else {
		where = where.Where("fencing_token = ? AND status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at >= ?",
			batch.FencingToken, domain.SyncBatchStatusRunning, now)
	}
	result := where.Updates(map[string]any{
		"status":           batch.Status,
		"created_count":    batch.CreatedCount,
		"updated_count":    batch.UpdatedCount,
		"stale_count":      batch.StaleCount,
		"failed_count":     batch.FailedCount,
		"message":          batch.Message,
		"summary":          defaultSummary(batch.Summary),
		"finished_at":      batch.FinishedAt,
		"fencing_token":    batch.FencingToken,
		"lease_expires_at": leaseVal,
		"updated_at":       now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		if batch.Status != domain.SyncBatchStatusRunning {
			return domain.ErrLeaseLost
		}
		return domain.ErrNotFound
	}
	batch.UpdatedAt = now
	return nil
}

// FinalizeSuccess 在同一数据库事务内完成：
// 1) 按 batch_id + fencing_token 校验未过期 running 租约；
// 2) 将批次标记为 success 并清空租约；
// 3) 将本轮已写入的 active 资源 sync_batch_id 提升为本批次。
// 若租约丢失返回 ErrLeaseLost，资源不会被提升，批次也不会被标记为 success。
func (r *SyncBatchRepository) FinalizeSuccess(ctx context.Context, batch *domain.SyncBatch, accountID string, syncedSince time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset sync batch repository is not configured")
	}
	if batch == nil || strings.TrimSpace(batch.BatchID) == "" {
		return 0, errors.New("sync batch is nil")
	}
	if strings.TrimSpace(batch.FencingToken) == "" {
		return 0, domain.ErrLeaseLost
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" || syncedSince.IsZero() {
		return 0, nil
	}
	now := time.Now().UTC()
	var promoted int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row syncBatchModel
		if err := checkSyncLeaseOwnedForUpdate(tx, batch.BatchID, batch.FencingToken, now, &row); err != nil {
			return err
		}
		if err := tx.Model(&syncBatchModel{}).Where("batch_id = ?", batch.BatchID).
			Updates(map[string]any{
				"status":           domain.SyncBatchStatusSuccess,
				"created_count":    batch.CreatedCount,
				"updated_count":    batch.UpdatedCount,
				"stale_count":      batch.StaleCount,
				"failed_count":     batch.FailedCount,
				"message":          batch.Message,
				"summary":          defaultSummary(batch.Summary),
				"finished_at":      batch.FinishedAt,
				"fencing_token":    batch.FencingToken,
				"lease_expires_at": (*time.Time)(nil),
				"updated_at":       now,
			}).Error; err != nil {
			return err
		}
		var err error
		promoted, err = promoteSuccessfulSyncBatch(ctx, tx, accountID, batch.BatchID, syncedSince, now)
		return err
	})
	if err != nil {
		return 0, err
	}
	batch.Status = domain.SyncBatchStatusSuccess
	batch.LeaseExpiresAt = nil
	batch.UpdatedAt = now
	return promoted, nil
}

func (r *SyncBatchRepository) GetByID(ctx context.Context, batchID string) (*domain.SyncBatch, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset sync batch repository is not configured")
	}
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return nil, domain.ErrNotFound
	}
	var row syncBatchModel
	if err := r.db.WithContext(ctx).Where("batch_id = ?", batchID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toSyncBatchDomain(&row)
	return &out, nil
}

// ReapExpiredRunning 将指定账号下租约已过期的 running 批次标记为 failed，
// 释放账号级 running 槽位。对应迁移 0028 的部分唯一索引。
// 返回被 reap 的批次摘要（含 triggered_by），供应用层写 sync_reaped 审计。
//
// 实现采用单条原子 UPDATE ... RETURNING（PostgreSQL），WHERE 同时校验
// account_id / status=running / lease_expires_at < now，替代旧的
// "先无锁 SELECT 再按 batch_id UPDATE" 两步模式，消除 TOCTOU：
//   - 若心跳在 reap 期间完成 RenewLease，lease_expires_at 已推进到未来，
//     WHERE 不再命中，批次不会被误杀；
//   - 并发 reaper 的 UPDATE 会被行锁串行化，后到者重新评估 WHERE 时 status
//     已是 failed，返回 0 行，不会重复返回批次，从而不会重复写 sync_reaped 审计。
//
// 注：此处使用原生 SQL 是为了用 UPDATE ... RETURNING 一次性完成"按条件更新并取回行"，
// GORM 的批量 Updates 无法把 RETURNING 行扫描进切片；updated_at 在 SET 中手动维护。
func (r *SyncBatchRepository) ReapExpiredRunning(ctx context.Context, accountID string, now time.Time) ([]domain.ReapedSyncBatch, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset sync batch repository is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, nil
	}
	const reapMsg = "lease expired; previous sync batch interrupted"
	var rows []syncBatchModel
	err := r.db.WithContext(ctx).Raw(`
UPDATE asset_sync_batch
SET status           = ?,
    finished_at      = ?,
    lease_expires_at = NULL,
    message          = ?,
    updated_at       = ?
WHERE integration_account_id = ?
  AND status = ?
  AND lease_expires_at IS NOT NULL
  AND lease_expires_at < ?
RETURNING batch_id, integration_account_id, triggered_by`,
		domain.SyncBatchStatusFailed, now, reapMsg, now,
		accountID, domain.SyncBatchStatusRunning, now,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	reaped := make([]domain.ReapedSyncBatch, 0, len(rows))
	for i := range rows {
		reaped = append(reaped, domain.ReapedSyncBatch{
			BatchID:              rows[i].BatchID,
			IntegrationAccountID: rows[i].IntegrationAccountID,
			TriggeredBy:          rows[i].TriggeredBy,
		})
	}
	return reaped, nil
}

// ReapAllExpiredRunning 将所有账号下租约已过期的 running 批次标记为 failed，
// 供后台定时 reaper 调用，保证无人再触发同步的账号也能自愈。
// 与 ReapExpiredRunning 逻辑相同，只是不按 account_id 过滤，单条原子 UPDATE ... RETURNING
// 一次性回收全部过期批次。并发安全性同理：行锁串行化 + WHERE 重新评估防重复。
func (r *SyncBatchRepository) ReapAllExpiredRunning(ctx context.Context, now time.Time) ([]domain.ReapedSyncBatch, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset sync batch repository is not configured")
	}
	const reapMsg = "lease expired; previous sync batch interrupted"
	var rows []syncBatchModel
	err := r.db.WithContext(ctx).Raw(`
UPDATE asset_sync_batch
SET status           = ?,
    finished_at      = ?,
    lease_expires_at = NULL,
    message          = ?,
    updated_at       = ?
WHERE status = ?
  AND lease_expires_at IS NOT NULL
  AND lease_expires_at < ?
RETURNING batch_id, integration_account_id, triggered_by`,
		domain.SyncBatchStatusFailed, now, reapMsg, now,
		domain.SyncBatchStatusRunning, now,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	reaped := make([]domain.ReapedSyncBatch, 0, len(rows))
	for i := range rows {
		reaped = append(reaped, domain.ReapedSyncBatch{
			BatchID:              rows[i].BatchID,
			IntegrationAccountID: rows[i].IntegrationAccountID,
			TriggeredBy:          rows[i].TriggeredBy,
		})
	}
	return reaped, nil
}

// RenewLease 续租 running 批次：按 batch_id + fencing_token 校验租约所有权，
// 并要求当前租约未过期（lease_expires_at >= now），把 lease_expires_at 置为 now+ttl、updated_at=now。
// 过期后不可复活：批次已终态、被 reap、token 不匹配或租约已过期时返回 ErrLeaseLost。
// 与 CheckLeaseOwned 写入门控保持一致，避免宽松续租架空严格门控，也消除过期批次
// “抢”走 reap 窗口导致新 TriggerSync 伪 409 的竞态。ttl 为续租有效期。
func (r *SyncBatchRepository) RenewLease(ctx context.Context, batchID, fencingToken string, now time.Time, ttl time.Duration) error {
	if r == nil || r.db == nil {
		return errors.New("asset sync batch repository is not configured")
	}
	batchID = strings.TrimSpace(batchID)
	fencingToken = strings.TrimSpace(fencingToken)
	if batchID == "" || fencingToken == "" {
		return domain.ErrLeaseLost
	}
	expires := now.Add(ttl)
	result := r.db.WithContext(ctx).Model(&syncBatchModel{}).
		Where("batch_id = ? AND fencing_token = ? AND status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at >= ?",
			batchID, fencingToken, domain.SyncBatchStatusRunning, now).
		Updates(map[string]any{
			"lease_expires_at": expires,
			"updated_at":       now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrLeaseLost
	}
	return nil
}

// CheckLeaseOwned 校验后台任务仍持有 running 租约，且租约未过期。
func (r *SyncBatchRepository) CheckLeaseOwned(ctx context.Context, batchID, fencingToken string, now time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("asset sync batch repository is not configured")
	}
	batchID = strings.TrimSpace(batchID)
	fencingToken = strings.TrimSpace(fencingToken)
	if batchID == "" || fencingToken == "" {
		return domain.ErrLeaseLost
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&syncBatchModel{}).
		Where("batch_id = ? AND fencing_token = ? AND status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at >= ?",
			batchID, fencingToken, domain.SyncBatchStatusRunning, now).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrLeaseLost
	}
	return nil
}

func (r *SyncBatchRepository) List(ctx context.Context, filter domain.SyncBatchFilter) ([]domain.SyncBatch, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("asset sync batch repository is not configured")
	}
	q := r.db.WithContext(ctx).Model(&syncBatchModel{})
	if accountID := strings.TrimSpace(filter.IntegrationAccountID); accountID != "" {
		q = q.Where("integration_account_id = ?", accountID)
	}
	// 数据范围过滤：仅返回账号 owner_team 在允许集合内的批次。
	// 通过子查询 integration_account 过滤，避免跨上下文 GORM 模型关联。
	if len(filter.OwnerTeams) > 0 {
		q = q.Where("integration_account_id IN (SELECT account_id FROM integration_account WHERE owner_team IN ?)", filter.OwnerTeams)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var rows []syncBatchModel
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.SyncBatch, 0, len(rows))
	for i := range rows {
		out = append(out, toSyncBatchDomain(&rows[i]))
	}
	return out, total, nil
}

func toSyncBatchModel(batch *domain.SyncBatch) syncBatchModel {
	return syncBatchModel{
		BatchID:              batch.BatchID,
		IntegrationAccountID: batch.IntegrationAccountID,
		Provider:             batch.Provider,
		Status:               batch.Status,
		CreatedCount:         batch.CreatedCount,
		UpdatedCount:         batch.UpdatedCount,
		StaleCount:           batch.StaleCount,
		FailedCount:          batch.FailedCount,
		Message:              batch.Message,
		Summary:              defaultSummary(batch.Summary),
		StartedAt:            batch.StartedAt,
		FinishedAt:           batch.FinishedAt,
		FencingToken:         batch.FencingToken,
		TriggeredBy:          batch.TriggeredBy,
		LeaseExpiresAt:       batch.LeaseExpiresAt,
	}
}

func toSyncBatchDomain(m *syncBatchModel) domain.SyncBatch {
	if m == nil {
		return domain.SyncBatch{}
	}
	return domain.SyncBatch{
		BatchID:              m.BatchID,
		IntegrationAccountID: m.IntegrationAccountID,
		Provider:             m.Provider,
		Status:               m.Status,
		CreatedCount:         m.CreatedCount,
		UpdatedCount:         m.UpdatedCount,
		StaleCount:           m.StaleCount,
		FailedCount:          m.FailedCount,
		Message:              m.Message,
		Summary:              defaultSummary(m.Summary),
		StartedAt:            m.StartedAt,
		FinishedAt:           m.FinishedAt,
		FencingToken:         m.FencingToken,
		TriggeredBy:          m.TriggeredBy,
		LeaseExpiresAt:       m.LeaseExpiresAt,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

func defaultSummary(summary []byte) []byte {
	if len(summary) == 0 {
		return []byte("{}")
	}
	return summary
}
