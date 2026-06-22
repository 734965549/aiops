package persistence

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/734965549/aiops/internal/inspection/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type findingModel struct {
	database.BaseModel
	FindingID         string  `gorm:"column:finding_id;type:varchar(64);uniqueIndex;not null"`
	RunID             string  `gorm:"column:run_id;type:varchar(64);not null;index"`
	PolicyID          string  `gorm:"column:policy_id;type:varchar(64);not null"`
	RiskLevel         string  `gorm:"column:risk_level;type:varchar(16);not null"`
	Category          string  `gorm:"column:category;type:varchar(64);not null;default:''"`
	Summary           string  `gorm:"column:summary;type:varchar(512);not null"`
	Detail            string  `gorm:"column:detail;type:text;not null;default:''"`
	AffectedResources []byte  `gorm:"column:affected_resources;type:jsonb;not null"`
	EvidenceRefs      []byte  `gorm:"column:evidence_refs;type:jsonb;not null"`
	Confidence        float64 `gorm:"column:confidence;not null;default:0"`
	Uncertainty       string  `gorm:"column:uncertainty;type:varchar(512);not null;default:''"`
}

func (findingModel) TableName() string { return "inspection_finding" }

type FindingRepository struct {
	db *gorm.DB
}

func NewFindingRepository(db *gorm.DB) *FindingRepository {
	return &FindingRepository{db: db}
}

func (r *FindingRepository) CreateBatch(ctx context.Context, findings []domain.InspectionFinding) error {
	if len(findings) == 0 {
		return nil
	}
	models := make([]findingModel, 0, len(findings))
	for i := range findings {
		m, err := toFindingModel(&findings[i])
		if err != nil {
			return err
		}
		models = append(models, m)
	}
	return r.db.WithContext(ctx).Create(&models).Error
}

func (r *FindingRepository) List(ctx context.Context, filter domain.FindingFilter) ([]domain.InspectionFinding, error) {
	q := r.db.WithContext(ctx).Model(&findingModel{})
	if filter.RunID != "" {
		q = q.Where("run_id = ?", filter.RunID)
	}
	if filter.PolicyID != "" {
		q = q.Where("policy_id = ?", filter.PolicyID)
	}
	if filter.RiskLevel != "" {
		q = q.Where("risk_level = ?", filter.RiskLevel)
	}
	var rows []findingModel
	if err := q.Order("created_at DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.InspectionFinding, 0, len(rows))
	for i := range rows {
		f, err := fromFindingModel(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, nil
}

func (r *FindingRepository) Count(ctx context.Context, filter domain.FindingFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&findingModel{})
	if filter.RunID != "" {
		q = q.Where("run_id = ?", filter.RunID)
	}
	if filter.PolicyID != "" {
		q = q.Where("policy_id = ?", filter.PolicyID)
	}
	if filter.RiskLevel != "" {
		q = q.Where("risk_level = ?", filter.RiskLevel)
	}
	var total int64
	return total, q.Count(&total).Error
}

func (r *FindingRepository) GetByID(ctx context.Context, findingID string) (*domain.InspectionFinding, error) {
	var m findingModel
	err := r.db.WithContext(ctx).Where("finding_id = ?", findingID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return fromFindingModel(&m)
}

func toFindingModel(f *domain.InspectionFinding) (findingModel, error) {
	ar, err := marshalJSON(f.AffectedResources)
	if err != nil {
		return findingModel{}, err
	}
	refs, err := marshalStringSlice(f.EvidenceRefs)
	if err != nil {
		return findingModel{}, err
	}
	return findingModel{
		FindingID: f.FindingID, RunID: f.RunID, PolicyID: f.PolicyID,
		RiskLevel: string(f.RiskLevel), Category: f.Category, Summary: f.Summary, Detail: f.Detail,
		AffectedResources: ar, EvidenceRefs: refs, Confidence: f.Confidence, Uncertainty: f.Uncertainty,
	}, nil
}

func fromFindingModel(m *findingModel) (*domain.InspectionFinding, error) {
	var ar []domain.AffectedResource
	var refs []string
	if len(m.AffectedResources) > 0 {
		_ = json.Unmarshal(m.AffectedResources, &ar)
	}
	refs = unmarshalStringSlice(m.EvidenceRefs)
	return &domain.InspectionFinding{
		FindingID: m.FindingID, RunID: m.RunID, PolicyID: m.PolicyID,
		RiskLevel: domain.RiskLevel(m.RiskLevel), Category: m.Category,
		Summary: m.Summary, Detail: m.Detail, AffectedResources: ar,
		EvidenceRefs: refs, Confidence: m.Confidence, Uncertainty: m.Uncertainty,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}, nil
}
