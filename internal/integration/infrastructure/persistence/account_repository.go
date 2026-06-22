package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type accountModel struct {
	database.BaseModel
	AccountID       string `gorm:"column:account_id;type:varchar(64);uniqueIndex;not null"`
	Name            string `gorm:"column:name;type:varchar(128);not null"`
	Provider        string `gorm:"column:provider;type:varchar(64);not null;index:idx_integration_account_provider_enabled,priority:1"`
	AuthType        string `gorm:"column:auth_type;type:varchar(32);not null"`
	Regions         []byte `gorm:"column:regions;type:jsonb;not null"`
	ProjectID       string `gorm:"column:project_id;type:varchar(128);not null;default:''"`
	CredentialRefID string `gorm:"column:credential_ref_id;type:varchar(64);not null;default:''"`
	Enabled         bool   `gorm:"column:enabled;not null;default:true;index:idx_integration_account_provider_enabled,priority:2"`
	Deleted         bool   `gorm:"column:deleted;not null;default:false"`
	OwnerTeam       string `gorm:"column:owner_team;type:varchar(128);not null;default:''"`
	Description     string `gorm:"column:description;type:varchar(512);not null;default:''"`
}

func (accountModel) TableName() string { return "integration_account" }

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) Create(ctx context.Context, account *domain.IntegrationAccount) error {
	if r == nil || r.db == nil {
		return errors.New("integration account repository is not configured")
	}
	if account == nil {
		return errors.New("integration account is nil")
	}
	regions, err := marshalStringSlice(account.Regions)
	if err != nil {
		return err
	}
	m := accountModel{
		AccountID: account.AccountID, Name: account.Name, Provider: string(account.Provider),
		AuthType: string(account.AuthType), Regions: regions, ProjectID: account.ProjectID,
		CredentialRefID: account.CredentialRefID, Enabled: account.Enabled, Deleted: account.Deleted,
		OwnerTeam: account.OwnerTeam, Description: account.Description,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	account.CreatedAt = m.CreatedAt
	account.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *AccountRepository) Update(ctx context.Context, account *domain.IntegrationAccount) error {
	if r == nil || r.db == nil {
		return errors.New("integration account repository is not configured")
	}
	if account == nil || strings.TrimSpace(account.AccountID) == "" {
		return errors.New("integration account is nil")
	}
	regions, err := marshalStringSlice(account.Regions)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&accountModel{}).Where("account_id = ? AND deleted = FALSE", account.AccountID).Updates(map[string]any{
		"name": account.Name, "provider": string(account.Provider), "auth_type": string(account.AuthType),
		"regions": regions, "project_id": account.ProjectID, "credential_ref_id": account.CredentialRefID,
		"enabled": account.Enabled, "owner_team": account.OwnerTeam, "description": account.Description,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *AccountRepository) GetByID(ctx context.Context, accountID string) (*domain.IntegrationAccount, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("integration account repository is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, domain.ErrNotFound
	}
	var row accountModel
	if err := r.db.WithContext(ctx).Where("account_id = ?", accountID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toAccountDomain(&row)
	return &out, nil
}

func (r *AccountRepository) List(ctx context.Context, filter domain.AccountFilter) ([]domain.IntegrationAccount, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("integration account repository is not configured")
	}
	q := r.db.WithContext(ctx).Model(&accountModel{}).Where("deleted = FALSE")
	q = applyAccountFilter(q, filter)
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	var rows []accountModel
	if err := q.Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.IntegrationAccount, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAccountDomain(&row))
	}
	return out, nil
}

func (r *AccountRepository) Count(ctx context.Context, filter domain.AccountFilter) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("integration account repository is not configured")
	}
	q := r.db.WithContext(ctx).Model(&accountModel{}).Where("deleted = FALSE")
	q = applyAccountFilter(q, filter)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *AccountRepository) SoftDelete(ctx context.Context, accountID string) error {
	if r == nil || r.db == nil {
		return errors.New("integration account repository is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return domain.ErrNotFound
	}
	res := r.db.WithContext(ctx).Model(&accountModel{}).Where("account_id = ? AND deleted = FALSE", accountID).Updates(map[string]any{
		"deleted": true, "enabled": false,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func applyAccountFilter(q *gorm.DB, filter domain.AccountFilter) *gorm.DB {
	if p := strings.TrimSpace(filter.Provider); p != "" {
		q = q.Where("provider = ?", p)
	}
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	if kw := strings.TrimSpace(filter.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name ILIKE ? OR account_id ILIKE ? OR owner_team ILIKE ?", like, like, like)
	}
	return q
}

func toAccountDomain(row *accountModel) domain.IntegrationAccount {
	return domain.IntegrationAccount{
		AccountID: row.AccountID, Name: row.Name, Provider: domain.ProviderType(row.Provider),
		AuthType: domain.AuthType(row.AuthType), Regions: unmarshalStringSlice(row.Regions),
		ProjectID: row.ProjectID, CredentialRefID: row.CredentialRefID, Enabled: row.Enabled,
		Deleted: row.Deleted, OwnerTeam: row.OwnerTeam, Description: row.Description,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
