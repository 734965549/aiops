package persistence

import (
	"context"
	"errors"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type resourceRelationModel struct {
	database.BaseModel
	RelationID       string `gorm:"column:relation_id;type:varchar(36);not null"`
	SourceResourceID string `gorm:"column:source_resource_id;type:varchar(36);not null"`
	TargetResourceID string `gorm:"column:target_resource_id;type:varchar(36);not null"`
	RelationType     string `gorm:"column:relation_type;type:varchar(32);not null"`
	Origin           string `gorm:"column:origin;type:varchar(32);not null"`
}

func (resourceRelationModel) TableName() string { return "asset_resource_relation" }

type ResourceRelationRepository struct {
	db *gorm.DB
}

func NewResourceRelationRepository(db *gorm.DB) *ResourceRelationRepository {
	return &ResourceRelationRepository{db: db}
}

func (r *ResourceRelationRepository) Create(ctx context.Context, relation *domain.ResourceRelation) error {
	if r == nil || r.db == nil {
		return errors.New("asset resource relation repository is not configured")
	}
	if relation == nil {
		return errors.New("resource relation is nil")
	}
	model := resourceRelationModel{
		RelationID:       relation.ID,
		SourceResourceID: relation.SourceResourceID,
		TargetResourceID: relation.TargetResourceID,
		RelationType:     relation.RelationType,
		Origin:           relation.Origin,
	}
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	relation.CreatedAt = model.CreatedAt
	relation.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *ResourceRelationRepository) GetByID(ctx context.Context, id string) (*domain.ResourceRelation, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset resource relation repository is not configured")
	}
	var model resourceRelationModel
	if err := r.db.WithContext(ctx).Where("relation_id = ?", id).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	relation := toResourceRelationDomain(&model)
	return &relation, nil
}

func (r *ResourceRelationRepository) Delete(ctx context.Context, id string) error {
	if r == nil || r.db == nil {
		return errors.New("asset resource relation repository is not configured")
	}
	result := r.db.WithContext(ctx).Where("relation_id = ?", id).Delete(&resourceRelationModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ResourceRelationRepository) ListByResourceIDs(ctx context.Context, resourceIDs []string) ([]domain.ResourceRelation, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset resource relation repository is not configured")
	}
	if len(resourceIDs) == 0 {
		return []domain.ResourceRelation{}, nil
	}
	var rows []resourceRelationModel
	if err := r.db.WithContext(ctx).
		Where("source_resource_id IN ? OR target_resource_id IN ?", resourceIDs, resourceIDs).
		Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.ResourceRelation, 0, len(rows))
	for i := range rows {
		out = append(out, toResourceRelationDomain(&rows[i]))
	}
	return out, nil
}

func (r *ResourceRelationRepository) CountByResourceID(ctx context.Context, resourceID string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset resource relation repository is not configured")
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&resourceRelationModel{}).
		Where("source_resource_id = ? OR target_resource_id = ?", resourceID, resourceID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func toResourceRelationDomain(model *resourceRelationModel) domain.ResourceRelation {
	return domain.ResourceRelation{
		ID:               model.RelationID,
		SourceResourceID: model.SourceResourceID,
		TargetResourceID: model.TargetResourceID,
		RelationType:     model.RelationType,
		Origin:           model.Origin,
		CreatedAt:        model.CreatedAt,
		UpdatedAt:        model.UpdatedAt,
	}
}
