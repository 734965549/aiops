// 会话分享凭证的 PostgreSQL 仓储（migration 0053）。
package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type sessionShareModel struct {
	database.BaseModel
	ShareID     string    `gorm:"column:share_id;type:varchar(64);uniqueIndex;not null"`
	SessionID   string    `gorm:"column:session_id;type:varchar(64);not null"`
	OwnerUserID string    `gorm:"column:owner_user_id;type:varchar(64);not null"`
	TokenHash   string    `gorm:"column:token_hash;type:varchar(64);uniqueIndex;not null"`
	Status      string    `gorm:"column:status;type:varchar(16);not null;default:'active'"`
	ExpiresAt   time.Time `gorm:"column:expires_at;not null"`
}

func (sessionShareModel) TableName() string { return "ai_session_share" }

// SessionShareRepository 是 ai_session_share 的 GORM 实现。
type SessionShareRepository struct {
	db *gorm.DB
}

func NewSessionShareRepository(db *gorm.DB) *SessionShareRepository {
	return &SessionShareRepository{db: db}
}

func (r *SessionShareRepository) Create(ctx context.Context, share domain.SessionShare) error {
	if r == nil || r.db == nil {
		return errors.New("ai session share repository is not configured")
	}
	row := sessionShareModel{
		BaseModel:   database.BaseModel{CreatedAt: share.CreatedAt, UpdatedAt: share.UpdatedAt},
		ShareID:     strings.TrimSpace(share.ShareID),
		SessionID:   strings.TrimSpace(share.SessionID),
		OwnerUserID: strings.TrimSpace(share.OwnerUserID),
		TokenHash:   strings.TrimSpace(share.TokenHash),
		Status:      share.Status,
		ExpiresAt:   share.ExpiresAt,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *SessionShareRepository) Get(ctx context.Context, shareID string) (*domain.SessionShare, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai session share repository is not configured")
	}
	var row sessionShareModel
	err := r.db.WithContext(ctx).Where("share_id = ?", strings.TrimSpace(shareID)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrShareNotFound
	}
	if err != nil {
		return nil, err
	}
	share := toSessionShare(row)
	return &share, nil
}

func (r *SessionShareRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.SessionShare, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai session share repository is not configured")
	}
	var row sessionShareModel
	err := r.db.WithContext(ctx).Where("token_hash = ?", strings.TrimSpace(tokenHash)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrShareNotFound
	}
	if err != nil {
		return nil, err
	}
	share := toSessionShare(row)
	return &share, nil
}

func (r *SessionShareRepository) ListBySession(ctx context.Context, sessionID string) ([]domain.SessionShare, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai session share repository is not configured")
	}
	var rows []sessionShareModel
	err := r.db.WithContext(ctx).
		Where("session_id = ?", strings.TrimSpace(sessionID)).
		Order("created_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.SessionShare, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSessionShare(row))
	}
	return out, nil
}

// Revoke 只更新状态与 updated_at；ShareID 不存在时返回 ErrShareNotFound。
func (r *SessionShareRepository) Revoke(ctx context.Context, shareID string, updatedAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("ai session share repository is not configured")
	}
	result := r.db.WithContext(ctx).
		Model(&sessionShareModel{}).
		Where("share_id = ?", strings.TrimSpace(shareID)).
		Updates(map[string]any{
			"status":     domain.ShareStatusRevoked,
			"updated_at": updatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrShareNotFound
	}
	return nil
}

func toSessionShare(row sessionShareModel) domain.SessionShare {
	return domain.SessionShare{
		ShareID:     row.ShareID,
		SessionID:   row.SessionID,
		OwnerUserID: row.OwnerUserID,
		TokenHash:   row.TokenHash,
		Status:      row.Status,
		ExpiresAt:   row.ExpiresAt,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
