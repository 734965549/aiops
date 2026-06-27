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
	StartedAt            time.Time  `gorm:"column:started_at;not null"`
	FinishedAt           *time.Time `gorm:"column:finished_at"`
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
	// 终态批次清空租约，释放账号级 running 槽位（部分唯一索引 WHERE status='running'）。
	leaseVal := (*time.Time)(nil)
	if batch.Status == domain.SyncBatchStatusRunning {
		leaseVal = batch.LeaseExpiresAt
	}
	result := r.db.WithContext(ctx).Model(&syncBatchModel{}).Where("batch_id = ?", batch.BatchID).Updates(map[string]any{
		"status":           batch.Status,
		"created_count":    batch.CreatedCount,
		"updated_count":    batch.UpdatedCount,
		"stale_count":      batch.StaleCount,
		"failed_count":     batch.FailedCount,
		"message":          batch.Message,
		"finished_at":      batch.FinishedAt,
		"lease_expires_at": leaseVal,
		"updated_at":       now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	batch.UpdatedAt = now
	return nil
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
func (r *SyncBatchRepository) ReapExpiredRunning(ctx context.Context, accountID string, now time.Time) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset sync batch repository is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Model(&syncBatchModel{}).
		Where("integration_account_id = ? AND status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at < ?",
			accountID, domain.SyncBatchStatusRunning, now).
		Updates(map[string]any{
			"status":           domain.SyncBatchStatusFailed,
			"finished_at":      now,
			"lease_expires_at": nil,
			"message":          "lease expired; previous sync batch interrupted",
			"updated_at":       now,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// RenewLease 续租 running 批次：把 lease_expires_at 置为 now+ttl，updated_at=now。
// 仅当 status='running' 时续租；批次已终态（RowsAffected=0）返回 ErrNotFound，
// 调用方据此停止心跳。
func (r *SyncBatchRepository) RenewLease(ctx context.Context, batchID string, now time.Time, ttl time.Duration) error {
	if r == nil || r.db == nil {
		return errors.New("asset sync batch repository is not configured")
	}
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return domain.ErrNotFound
	}
	expires := now.Add(ttl)
	result := r.db.WithContext(ctx).Model(&syncBatchModel{}).
		Where("batch_id = ? AND status = ?", batchID, domain.SyncBatchStatusRunning).
		Updates(map[string]any{
			"lease_expires_at": expires,
			"updated_at":       now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
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
		StartedAt:            batch.StartedAt,
		FinishedAt:           batch.FinishedAt,
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
		StartedAt:            m.StartedAt,
		FinishedAt:           m.FinishedAt,
		LeaseExpiresAt:       m.LeaseExpiresAt,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}
