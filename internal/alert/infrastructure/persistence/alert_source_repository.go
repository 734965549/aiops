package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

// alertSourceModel 对应 alert_source 表；secret_hash 存 token 哈希。
type alertSourceModel struct {
	database.BaseModel
	SourceID     string `gorm:"column:source_id;type:varchar(64);uniqueIndex;not null"`
	Name         string `gorm:"column:name;type:varchar(128);not null"`
	Type         string `gorm:"column:type;type:varchar(64);not null"`
	Enabled      bool   `gorm:"column:enabled;not null;default:true"`
	SecretHash   string `gorm:"column:secret_hash;type:varchar(128);not null;default:''"`
	Environment  string `gorm:"column:environment;type:varchar(64);not null;default:''"`
	BusinessLine string `gorm:"column:business_line;type:varchar(128);not null;default:''"`
	Description  string `gorm:"column:description;type:varchar(255);not null;default:''"`
	// 拉取式接入扩展字段，见 ops/huawei-ces-alert-sync-contract.md §4.1。
	DeliveryMode   string     `gorm:"column:delivery_mode;type:varchar(16);not null;default:'webhook'"`
	AccountID      string     `gorm:"column:account_id;type:varchar(64);not null;default:''"`
	PullConfig     []byte     `gorm:"column:pull_config;type:jsonb;not null;default:'{}'::jsonb"`
	LastSyncAt     *time.Time `gorm:"column:last_sync_at"`
	LastSyncStatus string     `gorm:"column:last_sync_status;type:varchar(32);not null;default:''"`
	SyncState      []byte     `gorm:"column:sync_state;type:jsonb;not null;default:'{}'::jsonb"`
}

func (alertSourceModel) TableName() string { return "alert_source" }

// AlertSourceRepository 接入源 GORM 仓储。
type AlertSourceRepository struct {
	db *gorm.DB
}

// NewAlertSourceRepository 创建接入源仓储。
func NewAlertSourceRepository(db *gorm.DB) *AlertSourceRepository {
	return &AlertSourceRepository{db: db}
}

func (r *AlertSourceRepository) Create(ctx context.Context, source *domain.AlertSource) error {
	if r == nil || r.db == nil {
		return errors.New("alert source repository is not configured")
	}
	if source == nil {
		return errors.New("alert source is nil")
	}
	pullConfig, err := json.Marshal(source.PullConfig)
	if err != nil {
		return err
	}
	syncState, err := json.Marshal(source.SyncState)
	if err != nil {
		return err
	}
	deliveryMode := string(source.DeliveryMode)
	if deliveryMode == "" {
		deliveryMode = string(domain.DeliveryModeWebhook)
	}
	m := alertSourceModel{
		SourceID:       source.ID,
		Name:           source.Name,
		Type:           string(source.Type),
		Enabled:        source.Enabled,
		SecretHash:     source.SecretHash,
		Environment:    source.Environment,
		BusinessLine:   source.BusinessLine,
		Description:    source.Description,
		DeliveryMode:   deliveryMode,
		AccountID:      source.AccountID,
		PullConfig:     pullConfig,
		LastSyncAt:     source.LastSyncAt,
		LastSyncStatus: source.LastSyncStatus,
		SyncState:      syncState,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	source.CreatedAt = m.CreatedAt
	source.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *AlertSourceRepository) Update(ctx context.Context, source *domain.AlertSource) error {
	if r == nil || r.db == nil {
		return errors.New("alert source repository is not configured")
	}
	if source == nil {
		return errors.New("alert source is nil")
	}
	pullConfig, err := json.Marshal(source.PullConfig)
	if err != nil {
		return err
	}
	syncState, err := json.Marshal(source.SyncState)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&alertSourceModel{}).Where("source_id = ?", source.ID).Updates(map[string]any{
		"name":             source.Name,
		"type":             string(source.Type),
		"enabled":          source.Enabled,
		"secret_hash":      source.SecretHash,
		"environment":      source.Environment,
		"business_line":    source.BusinessLine,
		"description":      source.Description,
		"delivery_mode":    string(source.DeliveryMode),
		"account_id":       source.AccountID,
		"pull_config":      pullConfig,
		"last_sync_at":     source.LastSyncAt,
		"last_sync_status": source.LastSyncStatus,
		"sync_state":       syncState,
	})
	if res.Error != nil {
		return database.MapUniqueViolation(res.Error, domain.ErrAlreadyExists)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	row, err := r.getByID(ctx, source.ID)
	if err != nil {
		return err
	}
	source.CreatedAt = row.CreatedAt
	source.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *AlertSourceRepository) GetByID(ctx context.Context, sourceID string) (*domain.AlertSource, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alert source repository is not configured")
	}
	row, err := r.getByID(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	out := toAlertSourceDomain(row)
	return &out, nil
}

func (r *AlertSourceRepository) List(ctx context.Context) ([]domain.AlertSource, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alert source repository is not configured")
	}
	var rows []alertSourceModel
	if err := r.db.WithContext(ctx).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.AlertSource, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAlertSourceDomain(&row))
	}
	return out, nil
}

func (r *AlertSourceRepository) Delete(ctx context.Context, sourceID string) error {
	if r == nil || r.db == nil {
		return errors.New("alert source repository is not configured")
	}
	res := r.db.WithContext(ctx).Where("source_id = ?", sourceID).Delete(&alertSourceModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *AlertSourceRepository) getByID(ctx context.Context, sourceID string) (*alertSourceModel, error) {
	var row alertSourceModel
	err := r.db.WithContext(ctx).Where("source_id = ?", sourceID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func toAlertSourceDomain(m *alertSourceModel) domain.AlertSource {
	if m == nil {
		return domain.AlertSource{}
	}
	var pullConfig domain.PullConfig
	if len(m.PullConfig) > 0 {
		_ = json.Unmarshal(m.PullConfig, &pullConfig)
	}
	var syncState map[string]domain.ScopeCursor
	if len(m.SyncState) > 0 {
		_ = json.Unmarshal(m.SyncState, &syncState)
	}
	deliveryMode := domain.AlertSourceDeliveryMode(m.DeliveryMode)
	if deliveryMode == "" {
		deliveryMode = domain.DeliveryModeWebhook
	}
	return domain.AlertSource{
		ID:             m.SourceID,
		Name:           m.Name,
		Type:           domain.AlertSourceType(m.Type),
		Enabled:        m.Enabled,
		SecretHash:     m.SecretHash,
		Environment:    m.Environment,
		BusinessLine:   m.BusinessLine,
		Description:    m.Description,
		DeliveryMode:   deliveryMode,
		AccountID:      m.AccountID,
		PullConfig:     pullConfig,
		LastSyncAt:     m.LastSyncAt,
		LastSyncStatus: m.LastSyncStatus,
		SyncState:      syncState,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}
