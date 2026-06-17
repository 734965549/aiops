package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type externalIdentityModel struct {
	database.BaseModel
	ExternalIdentityID string     `gorm:"column:external_identity_id;type:varchar(36);uniqueIndex;not null"`
	UserID             string     `gorm:"column:user_id;type:varchar(36);not null;index"`
	ProviderID         string     `gorm:"column:provider_id;type:varchar(64);not null;index"`
	ExternalSubject    string     `gorm:"column:external_subject;type:varchar(512);not null"`
	ExternalUsername   string     `gorm:"column:external_username;type:varchar(128);not null;default:''"`
	ExternalEmail      string     `gorm:"column:external_email;type:varchar(128);not null;default:''"`
	ExternalGroups     []byte     `gorm:"column:external_groups;type:jsonb;not null;default:'[]'"`
	LastLoginAt        *time.Time `gorm:"column:last_login_at"`
}

func (externalIdentityModel) TableName() string { return "iam_external_identity" }

// ExternalIdentityRepository 基于 GORM 的外部身份绑定仓储。
type ExternalIdentityRepository struct {
	db *gorm.DB
}

// NewExternalIdentityRepository 构造仓储实例。
func NewExternalIdentityRepository(db *gorm.DB) *ExternalIdentityRepository {
	return &ExternalIdentityRepository{db: db}
}

// FindByProviderSubject 按身份源与外部主体查询绑定。
func (r *ExternalIdentityRepository) FindByProviderSubject(ctx context.Context, providerID, externalSubject string) (*domain.ExternalIdentity, error) {
	providerID = strings.TrimSpace(providerID)
	externalSubject = strings.TrimSpace(externalSubject)
	if providerID == "" || externalSubject == "" {
		return nil, nil
	}
	var m externalIdentityModel
	err := r.db.WithContext(ctx).
		Where("provider_id = ? AND external_subject = ?", providerID, externalSubject).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toExternalIdentityDomain(&m)
}

// FindByUserAndProvider 按平台用户与身份源查询绑定。
func (r *ExternalIdentityRepository) FindByUserAndProvider(ctx context.Context, userID, providerID string) (*domain.ExternalIdentity, error) {
	userID = strings.TrimSpace(userID)
	providerID = strings.TrimSpace(providerID)
	if userID == "" || providerID == "" {
		return nil, nil
	}
	var m externalIdentityModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND provider_id = ?", userID, providerID).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toExternalIdentityDomain(&m)
}

// Create 插入外部身份绑定。
func (r *ExternalIdentityRepository) Create(ctx context.Context, ext *domain.ExternalIdentity) error {
	if ext == nil {
		return errors.New("external identity is nil")
	}
	groupsJSON, err := json.Marshal(ext.ExternalGroups)
	if err != nil {
		return err
	}
	m := externalIdentityModel{
		ExternalIdentityID: ext.ID,
		UserID:             ext.UserID,
		ProviderID:         ext.ProviderID,
		ExternalSubject:    ext.ExternalSubject,
		ExternalUsername:   ext.ExternalUsername,
		ExternalEmail:      ext.ExternalEmail,
		ExternalGroups:     groupsJSON,
		LastLoginAt:        ext.LastLoginAt,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	ext.CreatedAt = m.CreatedAt
	ext.UpdatedAt = m.UpdatedAt
	return nil
}

// Update 更新外部身份同步属性。
func (r *ExternalIdentityRepository) Update(ctx context.Context, ext *domain.ExternalIdentity) error {
	if ext == nil || strings.TrimSpace(ext.ID) == "" {
		return errors.New("external identity is nil or missing id")
	}
	groupsJSON, err := json.Marshal(ext.ExternalGroups)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&externalIdentityModel{}).
		Where("external_identity_id = ?", ext.ID).
		Updates(map[string]any{
			"external_username": ext.ExternalUsername,
			"external_email":    ext.ExternalEmail,
			"external_groups":   groupsJSON,
			"last_login_at":     ext.LastLoginAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteByProviderSubject 删除外部身份绑定（用于导入失败时的补偿回滚）。
func (r *ExternalIdentityRepository) DeleteByProviderSubject(ctx context.Context, providerID, externalSubject string) error {
	providerID = strings.TrimSpace(providerID)
	externalSubject = strings.TrimSpace(externalSubject)
	if providerID == "" || externalSubject == "" {
		return errors.New("provider_id and external_subject are required")
	}
	res := r.db.WithContext(ctx).
		Where("provider_id = ? AND external_subject = ?", providerID, externalSubject).
		Delete(&externalIdentityModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func toExternalIdentityDomain(m *externalIdentityModel) (*domain.ExternalIdentity, error) {
	if m == nil {
		return nil, nil
	}
	groups := []string{}
	if len(m.ExternalGroups) > 0 {
		if err := json.Unmarshal(m.ExternalGroups, &groups); err != nil {
			return nil, err
		}
	}
	return &domain.ExternalIdentity{
		ID:               m.ExternalIdentityID,
		UserID:           m.UserID,
		ProviderID:       m.ProviderID,
		ExternalSubject:  m.ExternalSubject,
		ExternalUsername: m.ExternalUsername,
		ExternalEmail:    m.ExternalEmail,
		ExternalGroups:   groups,
		LastLoginAt:      m.LastLoginAt,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}, nil
}
