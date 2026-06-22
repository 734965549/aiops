package persistence

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/734965549/aiops/internal/observability/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type evidenceModel struct {
	database.BaseModel
	EvidenceID string `gorm:"column:evidence_id;type:varchar(64);uniqueIndex;not null"`
	AccountID  string `gorm:"column:account_id;type:varchar(64);not null;index"`
	QueryType  string `gorm:"column:query_type;type:varchar(32);not null"`
	QueryHash  string `gorm:"column:query_hash;type:varchar(64);not null;default:''"`
	Summary    []byte `gorm:"column:summary;type:jsonb;not null"`
}

func (evidenceModel) TableName() string { return "obs_evidence_ref" }

type EvidenceRepository struct {
	db *gorm.DB
}

func NewEvidenceRepository(db *gorm.DB) *EvidenceRepository {
	return &EvidenceRepository{db: db}
}

func (r *EvidenceRepository) Create(ctx context.Context, ref *domain.EvidenceRef) error {
	if r == nil || r.db == nil {
		return errors.New("observability evidence repository is not configured")
	}
	if ref == nil {
		return errors.New("evidence ref is nil")
	}
	summary, err := json.Marshal(ref.Summary)
	if err != nil {
		return err
	}
	m := evidenceModel{
		EvidenceID: ref.EvidenceID,
		AccountID:  ref.AccountID,
		QueryType:  ref.QueryType,
		QueryHash:  ref.QueryHash,
		Summary:    summary,
	}
	return r.db.WithContext(ctx).Create(&m).Error
}

func (r *EvidenceRepository) GetByID(ctx context.Context, evidenceID string) (*domain.EvidenceRef, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("observability evidence repository is not configured")
	}
	var m evidenceModel
	if err := r.db.WithContext(ctx).Where("evidence_id = ?", evidenceID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	summary := map[string]any{}
	if len(m.Summary) > 0 {
		_ = json.Unmarshal(m.Summary, &summary)
	}
	return &domain.EvidenceRef{
		EvidenceID: m.EvidenceID,
		AccountID:  m.AccountID,
		QueryType:  m.QueryType,
		QueryHash:  m.QueryHash,
		Summary:    summary,
	}, nil
}
