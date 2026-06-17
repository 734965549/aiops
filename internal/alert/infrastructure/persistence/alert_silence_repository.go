package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

// alertSilenceModel 对应 alert_silence 表。
type alertSilenceModel struct {
	database.BaseModel
	SilenceID string    `gorm:"column:silence_id;type:varchar(36);uniqueIndex;not null"`
	AlertID   string    `gorm:"column:alert_id;type:varchar(36);not null;default:'';index"`
	Matcher   []byte    `gorm:"column:matcher;type:jsonb;not null;default:'{}'::jsonb"`
	Reason    string    `gorm:"column:reason;type:varchar(512);not null"`
	StartsAt  time.Time `gorm:"column:starts_at;not null"`
	EndsAt    time.Time `gorm:"column:ends_at;not null"`
	CreatedBy string    `gorm:"column:created_by;type:varchar(36);not null"`
}

func (alertSilenceModel) TableName() string { return "alert_silence" }

// AlertSilenceRepository 静默记录 GORM 仓储。
type AlertSilenceRepository struct {
	db *gorm.DB
}

// NewAlertSilenceRepository 创建静默仓储。
func NewAlertSilenceRepository(db *gorm.DB) *AlertSilenceRepository {
	return &AlertSilenceRepository{db: db}
}

func (r *AlertSilenceRepository) Create(ctx context.Context, silence *domain.AlertSilence) error {
	if r == nil || r.db == nil {
		return errors.New("alert silence repository is not configured")
	}
	if silence == nil {
		return errors.New("alert silence is nil")
	}
	matcher, err := marshalStringMap(silence.Matcher)
	if err != nil {
		return err
	}
	m := alertSilenceModel{
		SilenceID: silence.ID,
		AlertID:   silence.AlertID,
		Matcher:   matcher,
		Reason:    silence.Reason,
		StartsAt:  silence.StartsAt,
		EndsAt:    silence.EndsAt,
		CreatedBy: silence.CreatedBy,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	silence.CreatedAt = m.CreatedAt
	silence.UpdatedAt = m.UpdatedAt
	return nil
}
