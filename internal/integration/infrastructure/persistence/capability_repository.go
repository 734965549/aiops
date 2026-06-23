package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type capabilityModel struct {
	database.BaseModel
	AccountID  string `gorm:"column:account_id;type:varchar(64);not null;uniqueIndex:uq_integration_capability,priority:1"`
	Capability string `gorm:"column:capability;type:varchar(32);not null;uniqueIndex:uq_integration_capability,priority:2"`
}

func (capabilityModel) TableName() string { return "integration_capability" }

type CapabilityRepository struct {
	db *gorm.DB
}

func NewCapabilityRepository(db *gorm.DB) *CapabilityRepository {
	return &CapabilityRepository{db: db}
}

func (r *CapabilityRepository) ReplaceForAccount(ctx context.Context, accountID string, caps []domain.Capability) error {
	if r == nil || r.db == nil {
		return errors.New("integration capability repository is not configured")
	}
	return replaceCapabilities(ctx, r.db.WithContext(ctx), accountID, caps)
}

func replaceCapabilities(_ context.Context, db *gorm.DB, accountID string, caps []domain.Capability) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.ErrNotFound
	}
	if err := db.Where("account_id = ?", accountID).Delete(&capabilityModel{}).Error; err != nil {
		return err
	}
	for _, cap := range caps {
		if !cap.IsValid() {
			continue
		}
		row := capabilityModel{AccountID: accountID, Capability: string(cap)}
		if err := db.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *CapabilityRepository) ListByAccountID(ctx context.Context, accountID string) ([]domain.Capability, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("integration capability repository is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, domain.ErrNotFound
	}
	var rows []capabilityModel
	if err := r.db.WithContext(ctx).Where("account_id = ?", accountID).Order("capability ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Capability, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Capability(row.Capability))
	}
	return out, nil
}
