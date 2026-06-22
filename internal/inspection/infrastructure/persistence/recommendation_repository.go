package persistence

import (
	"context"
	"errors"

	"github.com/734965549/aiops/internal/inspection/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type recommendationModel struct {
	database.BaseModel
	RecommendationID   string  `gorm:"column:recommendation_id;type:varchar(64);uniqueIndex;not null"`
	FindingID          string  `gorm:"column:finding_id;type:varchar(64);not null;index"`
	RunID              string  `gorm:"column:run_id;type:varchar(64);not null;index"`
	Title              string  `gorm:"column:title;type:varchar(256);not null"`
	Reason             string  `gorm:"column:reason;type:text;not null;default:''"`
	SuggestedAction    string  `gorm:"column:suggested_action;type:text;not null;default:''"`
	RiskLevel          string  `gorm:"column:risk_level;type:varchar(16);not null"`
	Status             string  `gorm:"column:status;type:varchar(16);not null;default:'open'"`
	CanCreateExecution bool    `gorm:"column:can_create_execution;not null;default:false"`
	Confidence         float64 `gorm:"column:confidence;not null;default:0"`
	Uncertainty        string  `gorm:"column:uncertainty;type:varchar(512);not null;default:''"`
}

func (recommendationModel) TableName() string { return "inspection_recommendation" }

type RecommendationRepository struct {
	db *gorm.DB
}

func NewRecommendationRepository(db *gorm.DB) *RecommendationRepository {
	return &RecommendationRepository{db: db}
}

func (r *RecommendationRepository) CreateBatch(ctx context.Context, recs []domain.Recommendation) error {
	if len(recs) == 0 {
		return nil
	}
	models := make([]recommendationModel, 0, len(recs))
	for i := range recs {
		models = append(models, toRecommendationModel(&recs[i]))
	}
	return r.db.WithContext(ctx).Create(&models).Error
}

func (r *RecommendationRepository) ListByRunID(ctx context.Context, runID string) ([]domain.Recommendation, error) {
	var rows []recommendationModel
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return fromRecommendationModels(rows)
}

func (r *RecommendationRepository) ListByFindingID(ctx context.Context, findingID string) ([]domain.Recommendation, error) {
	var rows []recommendationModel
	if err := r.db.WithContext(ctx).Where("finding_id = ?", findingID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return fromRecommendationModels(rows)
}

func (r *RecommendationRepository) GetByID(ctx context.Context, recommendationID string) (*domain.Recommendation, error) {
	var m recommendationModel
	err := r.db.WithContext(ctx).Where("recommendation_id = ?", recommendationID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rec := fromRecommendationModel(&m)
	return &rec, nil
}

func (r *RecommendationRepository) Update(ctx context.Context, rec *domain.Recommendation) error {
	m := toRecommendationModel(rec)
	res := r.db.WithContext(ctx).Model(&recommendationModel{}).Where("recommendation_id = ?", rec.RecommendationID).Updates(map[string]any{
		"status": m.Status, "title": m.Title, "reason": m.Reason, "suggested_action": m.SuggestedAction,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func toRecommendationModel(r *domain.Recommendation) recommendationModel {
	return recommendationModel{
		RecommendationID: r.RecommendationID, FindingID: r.FindingID, RunID: r.RunID,
		Title: r.Title, Reason: r.Reason, SuggestedAction: r.SuggestedAction,
		RiskLevel: string(r.RiskLevel), Status: string(r.Status),
		CanCreateExecution: r.CanCreateExecution, Confidence: r.Confidence, Uncertainty: r.Uncertainty,
	}
}

func fromRecommendationModel(m *recommendationModel) domain.Recommendation {
	return domain.Recommendation{
		RecommendationID: m.RecommendationID, FindingID: m.FindingID, RunID: m.RunID,
		Title: m.Title, Reason: m.Reason, SuggestedAction: m.SuggestedAction,
		RiskLevel: domain.RiskLevel(m.RiskLevel), Status: domain.RecommendationStatus(m.Status),
		CanCreateExecution: m.CanCreateExecution, Confidence: m.Confidence, Uncertainty: m.Uncertainty,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func fromRecommendationModels(rows []recommendationModel) ([]domain.Recommendation, error) {
	out := make([]domain.Recommendation, 0, len(rows))
	for i := range rows {
		out = append(out, fromRecommendationModel(&rows[i]))
	}
	return out, nil
}
