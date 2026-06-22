package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type credentialModel struct {
	database.BaseModel
	CredentialRefID string `gorm:"column:credential_ref_id;type:varchar(64);uniqueIndex;not null"`
	AccountID       string `gorm:"column:account_id;type:varchar(64);not null;uniqueIndex"`
	StoreType       string `gorm:"column:store_type;type:varchar(32);not null;default:'local_encrypted'"`
	Ciphertext      []byte `gorm:"column:ciphertext;type:bytea;not null"`
	ExternalRef     string `gorm:"column:external_ref;type:varchar(512);not null;default:''"`
	Fingerprint     string `gorm:"column:fingerprint;type:varchar(64);not null;default:''"`
}

func (credentialModel) TableName() string { return "integration_credential_ref" }

type CredentialRepository struct {
	db *gorm.DB
}

func NewCredentialRepository(db *gorm.DB) *CredentialRepository {
	return &CredentialRepository{db: db}
}

func (r *CredentialRepository) Create(ctx context.Context, ref *domain.CredentialRef) error {
	if r == nil || r.db == nil {
		return errors.New("integration credential repository is not configured")
	}
	if ref == nil {
		return errors.New("credential ref is nil")
	}
	m := credentialModel{
		CredentialRefID: ref.CredentialRefID, AccountID: ref.AccountID,
		StoreType: string(ref.StoreType), Ciphertext: ref.Ciphertext,
		ExternalRef: ref.ExternalRef, Fingerprint: ref.Fingerprint,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	ref.CreatedAt = m.CreatedAt
	ref.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *CredentialRepository) Update(ctx context.Context, ref *domain.CredentialRef) error {
	if r == nil || r.db == nil {
		return errors.New("integration credential repository is not configured")
	}
	if ref == nil || strings.TrimSpace(ref.CredentialRefID) == "" {
		return errors.New("credential ref is nil")
	}
	res := r.db.WithContext(ctx).Model(&credentialModel{}).Where("credential_ref_id = ?", ref.CredentialRefID).Updates(map[string]any{
		"ciphertext": ref.Ciphertext, "fingerprint": ref.Fingerprint, "external_ref": ref.ExternalRef,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *CredentialRepository) GetByAccountID(ctx context.Context, accountID string) (*domain.CredentialRef, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("integration credential repository is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, domain.ErrNotFound
	}
	var row credentialModel
	if err := r.db.WithContext(ctx).Where("account_id = ?", accountID).Order("id DESC").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toCredentialDomain(&row)
	return &out, nil
}

func (r *CredentialRepository) DeleteByAccountID(ctx context.Context, accountID string) error {
	if r == nil || r.db == nil {
		return errors.New("integration credential repository is not configured")
	}
	return r.db.WithContext(ctx).Where("account_id = ?", strings.TrimSpace(accountID)).Delete(&credentialModel{}).Error
}

func toCredentialDomain(row *credentialModel) domain.CredentialRef {
	return domain.CredentialRef{
		CredentialRefID: row.CredentialRefID, AccountID: row.AccountID,
		StoreType: domain.CredentialStoreType(row.StoreType), Ciphertext: row.Ciphertext,
		ExternalRef: row.ExternalRef, Fingerprint: row.Fingerprint,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
