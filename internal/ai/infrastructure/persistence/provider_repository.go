// Package persistence 实现 AI 上下文 provider 配置的 PostgreSQL 仓储。
package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type providerModel struct {
	database.BaseModel
	ProviderID      string `gorm:"column:provider_id;type:varchar(64);uniqueIndex;not null"`
	Name            string `gorm:"column:name;type:varchar(128);not null"`
	Type            string `gorm:"column:type;type:varchar(16);not null"`
	BaseURL         string `gorm:"column:base_url;type:varchar(512);not null"`
	Model           string `gorm:"column:model;type:varchar(128);not null;default:''"`
	APIKeyEncrypted []byte `gorm:"column:api_key_encrypted;type:bytea"`
	Headers         []byte `gorm:"column:headers;type:jsonb;not null"`
	TimeoutMS       int64  `gorm:"column:timeout_ms;not null;default:30000"`
	Enabled         bool   `gorm:"column:enabled;not null;default:true"`
	Description     string `gorm:"column:description;type:varchar(512);not null;default:''"`
}

func (providerModel) TableName() string { return "ai_provider" }

// ProviderRepository 是 ai_provider 表的 GORM 实现。
type ProviderRepository struct {
	db *gorm.DB
}

func NewProviderRepository(db *gorm.DB) *ProviderRepository {
	return &ProviderRepository{db: db}
}

func (r *ProviderRepository) List(ctx context.Context) ([]domain.Provider, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai provider repository is not configured")
	}
	var rows []providerModel
	if err := r.db.WithContext(ctx).Order("provider_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Provider, 0, len(rows))
	for i := range rows {
		p, err := toProviderDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *ProviderRepository) Get(ctx context.Context, providerID string) (*domain.Provider, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai provider repository is not configured")
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, domain.ErrProviderNotFound
	}
	var row providerModel
	if err := r.db.WithContext(ctx).Where("provider_id = ?", providerID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrProviderNotFound
		}
		return nil, err
	}
	p, err := toProviderDomain(&row)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *ProviderRepository) Upsert(ctx context.Context, provider domain.Provider) error {
	if r == nil || r.db == nil {
		return errors.New("ai provider repository is not configured")
	}
	providerID := strings.TrimSpace(provider.ProviderID)
	if providerID == "" {
		return errors.New("ai provider provider_id is required")
	}
	headers, err := marshalHeaders(provider.Headers)
	if err != nil {
		return err
	}
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&providerModel{}).Where("provider_id = ?", providerID).Updates(map[string]any{
		"name":              provider.Name,
		"type":              provider.Type,
		"base_url":          provider.BaseURL,
		"model":             provider.Model,
		"api_key_encrypted": provider.APIKeyCiphertext,
		"headers":           headers,
		"timeout_ms":        provider.TimeoutMS,
		"enabled":           provider.Enabled,
		"description":       provider.Description,
		"updated_at":        now,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	row := providerModel{
		ProviderID: providerID, Name: provider.Name, Type: provider.Type,
		BaseURL: provider.BaseURL, Model: provider.Model, APIKeyEncrypted: provider.APIKeyCiphertext,
		Headers: headers, TimeoutMS: provider.TimeoutMS, Enabled: provider.Enabled,
		Description: provider.Description,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if database.IsUniqueViolation(err) {
			return r.db.WithContext(ctx).Model(&providerModel{}).Where("provider_id = ?", providerID).Updates(map[string]any{
				"name": provider.Name, "type": provider.Type, "base_url": provider.BaseURL,
				"model": provider.Model, "api_key_encrypted": provider.APIKeyCiphertext,
				"headers": headers, "timeout_ms": provider.TimeoutMS, "enabled": provider.Enabled,
				"description": provider.Description, "updated_at": time.Now(),
			}).Error
		}
		return err
	}
	return nil
}

func (r *ProviderRepository) Delete(ctx context.Context, providerID string) error {
	if r == nil || r.db == nil {
		return errors.New("ai provider repository is not configured")
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil
	}
	return r.db.WithContext(ctx).Where("provider_id = ?", providerID).Delete(&providerModel{}).Error
}

func toProviderDomain(row *providerModel) (domain.Provider, error) {
	headers, err := unmarshalHeaders(row.Headers)
	if err != nil {
		return domain.Provider{}, err
	}
	return domain.Provider{
		ProviderID:       row.ProviderID,
		Name:             row.Name,
		Type:             row.Type,
		BaseURL:          row.BaseURL,
		Model:            row.Model,
		Headers:          headers,
		TimeoutMS:        row.TimeoutMS,
		Enabled:          row.Enabled,
		Description:      row.Description,
		APIKeyCiphertext: row.APIKeyEncrypted,
		HasAPIKey:        len(row.APIKeyEncrypted) > 0,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}, nil
}

func marshalHeaders(headers map[string]string) ([]byte, error) {
	if len(headers) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(headers)
}

func unmarshalHeaders(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]string{}
	}
	return out, nil
}
