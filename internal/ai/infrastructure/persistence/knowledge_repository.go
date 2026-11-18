package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

// knowledgeItemModel 对应 ai_knowledge_item（migration 0052）。
type knowledgeItemModel struct {
	database.BaseModel
	ItemID      string     `gorm:"column:item_id;type:varchar(64);uniqueIndex;not null"`
	Scope       string     `gorm:"column:scope;type:varchar(32);not null"`
	ScopeRef    string     `gorm:"column:scope_ref;type:varchar(128);not null;default:''"`
	Title       string     `gorm:"column:title;type:varchar(255);not null"`
	Content     string     `gorm:"column:content;type:text;not null;default:''"`
	Origin      string     `gorm:"column:origin;type:varchar(32);not null"`
	Status      string     `gorm:"column:status;type:varchar(16);not null;default:'draft'"`
	HitCount    int        `gorm:"column:hit_count;not null;default:0"`
	UsefulCount int        `gorm:"column:useful_count;not null;default:0"`
	ReviewedBy  string     `gorm:"column:reviewed_by;type:varchar(64);not null;default:''"`
	ReviewedAt  *time.Time `gorm:"column:reviewed_at"`
	CreatedBy   string     `gorm:"column:created_by;type:varchar(64);not null;default:''"`
}

func (knowledgeItemModel) TableName() string { return "ai_knowledge_item" }

// KnowledgeItemRepository 是 ai_knowledge_item 的 GORM 实现。
type KnowledgeItemRepository struct {
	db *gorm.DB
}

func NewKnowledgeItemRepository(db *gorm.DB) *KnowledgeItemRepository {
	return &KnowledgeItemRepository{db: db}
}

func (r *KnowledgeItemRepository) Create(ctx context.Context, item domain.KnowledgeItem) error {
	if r == nil || r.db == nil {
		return errors.New("ai knowledge repository is not configured")
	}
	row := toKnowledgeItemModel(item)
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *KnowledgeItemRepository) Get(ctx context.Context, itemID string) (*domain.KnowledgeItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai knowledge repository is not configured")
	}
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, domain.ErrKnowledgeItemNotFound
	}
	var row knowledgeItemModel
	if err := r.db.WithContext(ctx).Where("item_id = ?", itemID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrKnowledgeItemNotFound
		}
		return nil, err
	}
	item := toKnowledgeItemDomain(&row)
	return &item, nil
}

func (r *KnowledgeItemRepository) Update(ctx context.Context, item domain.KnowledgeItem) error {
	if r == nil || r.db == nil {
		return errors.New("ai knowledge repository is not configured")
	}
	res := r.db.WithContext(ctx).Model(&knowledgeItemModel{}).
		Where("item_id = ?", strings.TrimSpace(item.ItemID)).
		Updates(map[string]any{
			"scope":       string(item.Scope),
			"scope_ref":   strings.TrimSpace(item.ScopeRef),
			"title":       item.Title,
			"content":     item.Content,
			"status":      string(item.Status),
			"reviewed_by": strings.TrimSpace(item.ReviewedBy),
			"reviewed_at": item.ReviewedAt,
			"updated_at":  item.UpdatedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrKnowledgeItemNotFound
	}
	return nil
}

func (r *KnowledgeItemRepository) Delete(ctx context.Context, itemID string) error {
	if r == nil || r.db == nil {
		return errors.New("ai knowledge repository is not configured")
	}
	res := r.db.WithContext(ctx).Where("item_id = ?", strings.TrimSpace(itemID)).Delete(&knowledgeItemModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrKnowledgeItemNotFound
	}
	return nil
}

func (r *KnowledgeItemRepository) List(ctx context.Context, filter domain.KnowledgeItemFilter) ([]domain.KnowledgeItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai knowledge repository is not configured")
	}
	query := r.db.WithContext(ctx)
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}
	if filter.Scope != nil {
		query = query.Where("scope = ?", string(*filter.Scope))
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []knowledgeItemModel
	if err := query.Order("updated_at DESC, id DESC").Limit(limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.KnowledgeItem, 0, len(rows))
	for i := range rows {
		out = append(out, toKnowledgeItemDomain(&rows[i]))
	}
	return out, nil
}

// FindByScope 按 scope + scope_ref + origin 查找条目（聚合幂等用）。
func (r *KnowledgeItemRepository) FindByScope(ctx context.Context, scope domain.KnowledgeScope, scopeRef string, origin domain.KnowledgeOrigin) (*domain.KnowledgeItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai knowledge repository is not configured")
	}
	var row knowledgeItemModel
	err := r.db.WithContext(ctx).
		Where("scope = ? AND scope_ref = ? AND origin = ?", string(scope), strings.TrimSpace(scopeRef), string(origin)).
		Order("updated_at DESC").
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	item := toKnowledgeItemDomain(&row)
	return &item, nil
}

// ListActiveForTarget 返回对给定应用/资源类型生效的 active 条目（含 global）。
func (r *KnowledgeItemRepository) ListActiveForTarget(ctx context.Context, applicationID, resourceType string) ([]domain.KnowledgeItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai knowledge repository is not configured")
	}
	applicationID = strings.TrimSpace(applicationID)
	resourceType = strings.TrimSpace(resourceType)
	query := r.db.WithContext(ctx).
		Where("status = ?", string(domain.KnowledgeStatusActive)).
		Where("scope = ? OR (scope = ? AND scope_ref = ?) OR (scope = ? AND scope_ref = ?)",
			string(domain.KnowledgeScopeGlobal),
			string(domain.KnowledgeScopeApplication), applicationID,
			string(domain.KnowledgeScopeResourceType), resourceType)
	var rows []knowledgeItemModel
	if err := query.Order("hit_count DESC, updated_at DESC").Limit(domain.KnowledgeMaxInjectedItems * 4).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.KnowledgeItem, 0, len(rows))
	for i := range rows {
		out = append(out, toKnowledgeItemDomain(&rows[i]))
	}
	return out, nil
}

// AddHit 累加注入次数并返回最新计数（用于命中率评估）。
func (r *KnowledgeItemRepository) AddHit(ctx context.Context, itemIDs []string, updatedAt time.Time) error {
	if r == nil || r.db == nil || len(itemIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&knowledgeItemModel{}).
		Where("item_id IN ?", itemIDs).
		Updates(map[string]any{
			"hit_count":  gorm.Expr("hit_count + 1"),
			"updated_at": updatedAt,
		}).Error
}

// AddUseful 累加有效计数。
func (r *KnowledgeItemRepository) AddUseful(ctx context.Context, itemIDs []string, updatedAt time.Time) error {
	if r == nil || r.db == nil || len(itemIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&knowledgeItemModel{}).
		Where("item_id IN ?", itemIDs).
		Updates(map[string]any{
			"useful_count": gorm.Expr("useful_count + 1"),
			"updated_at":   updatedAt,
		}).Error
}

func toKnowledgeItemModel(item domain.KnowledgeItem) knowledgeItemModel {
	return knowledgeItemModel{
		BaseModel:   database.BaseModel{CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt},
		ItemID:      strings.TrimSpace(item.ItemID),
		Scope:       string(item.Scope),
		ScopeRef:    strings.TrimSpace(item.ScopeRef),
		Title:       item.Title,
		Content:     item.Content,
		Origin:      string(item.Origin),
		Status:      string(item.Status),
		HitCount:    item.HitCount,
		UsefulCount: item.UsefulCount,
		ReviewedBy:  strings.TrimSpace(item.ReviewedBy),
		ReviewedAt:  item.ReviewedAt,
		CreatedBy:   strings.TrimSpace(item.CreatedBy),
	}
}

func toKnowledgeItemDomain(row *knowledgeItemModel) domain.KnowledgeItem {
	status := domain.KnowledgeStatus(row.Status)
	if !status.IsValid() {
		status = domain.KnowledgeStatusDraft
	}
	scope := domain.KnowledgeScope(row.Scope)
	if !scope.IsValid() {
		scope = domain.KnowledgeScopeGlobal
	}
	origin := domain.KnowledgeOrigin(row.Origin)
	if !origin.IsValid() {
		origin = domain.KnowledgeOriginManual
	}
	return domain.KnowledgeItem{
		ItemID:      row.ItemID,
		Scope:       scope,
		ScopeRef:    row.ScopeRef,
		Title:       row.Title,
		Content:     row.Content,
		Origin:      origin,
		Status:      status,
		HitCount:    row.HitCount,
		UsefulCount: row.UsefulCount,
		ReviewedBy:  row.ReviewedBy,
		ReviewedAt:  row.ReviewedAt,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
