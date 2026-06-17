package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type matchRuleModel struct {
	database.BaseModel
	RuleID            string `gorm:"column:rule_id;type:varchar(36);uniqueIndex;not null"`
	Name              string `gorm:"column:name;type:varchar(128);not null"`
	Enabled           bool   `gorm:"column:enabled;not null;default:true"`
	Priority          int    `gorm:"column:priority;not null;default:0"`
	TargetType        string `gorm:"column:target_type;type:varchar(32);not null"`
	SourceType        string `gorm:"column:source_type;type:varchar(64);not null;default:all"`
	LabelKey          string `gorm:"column:label_key;type:varchar(128);not null"`
	LabelValuePattern string `gorm:"column:label_value_pattern;type:varchar(255);not null"`
	ApplicationID     string `gorm:"column:application_id;type:varchar(36);not null"`
	ResourceID        string `gorm:"column:resource_id;type:varchar(36);not null;default:''"`
}

func (matchRuleModel) TableName() string { return "asset_match_rule" }

// MatchRuleRepository 匹配规则 GORM 仓储。
type MatchRuleRepository struct {
	db *gorm.DB
}

// NewMatchRuleRepository 创建匹配规则仓储。
func NewMatchRuleRepository(db *gorm.DB) *MatchRuleRepository {
	return &MatchRuleRepository{db: db}
}

func (r *MatchRuleRepository) Create(ctx context.Context, rule *domain.MatchRule) error {
	if r == nil || r.db == nil {
		return errors.New("asset match rule repository is not configured")
	}
	if rule == nil {
		return errors.New("match rule is nil")
	}
	m := toMatchRuleModel(rule)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	rule.CreatedAt = m.CreatedAt
	rule.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *MatchRuleRepository) List(ctx context.Context) ([]domain.MatchRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset match rule repository is not configured")
	}
	var rows []matchRuleModel
	if err := r.db.WithContext(ctx).Order("priority DESC, created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return toMatchRuleDomains(rows), nil
}

func (r *MatchRuleRepository) ListEnabledByPriority(ctx context.Context) ([]domain.MatchRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset match rule repository is not configured")
	}
	var rows []matchRuleModel
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).
		Order("priority DESC, created_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return toMatchRuleDomains(rows), nil
}

func (r *MatchRuleRepository) GetByID(ctx context.Context, id string) (*domain.MatchRule, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset match rule repository is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, domain.ErrNotFound
	}
	var row matchRuleModel
	if err := r.db.WithContext(ctx).Where("rule_id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toMatchRuleDomain(&row)
	return &out, nil
}

func (r *MatchRuleRepository) Update(ctx context.Context, rule *domain.MatchRule) error {
	if r == nil || r.db == nil {
		return errors.New("asset match rule repository is not configured")
	}
	if rule == nil || strings.TrimSpace(rule.ID) == "" {
		return errors.New("match rule is nil")
	}
	res := r.db.WithContext(ctx).Model(&matchRuleModel{}).Where("rule_id = ?", rule.ID).Updates(map[string]any{
		"name": rule.Name, "enabled": rule.Enabled, "priority": rule.Priority,
		"target_type": string(rule.TargetType), "source_type": string(rule.SourceType),
		"label_key": rule.LabelKey, "label_value_pattern": rule.LabelValuePattern,
		"application_id": rule.ApplicationID, "resource_id": rule.ResourceID,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	var row matchRuleModel
	if err := r.db.WithContext(ctx).Where("rule_id = ?", rule.ID).First(&row).Error; err != nil {
		return err
	}
	rule.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *MatchRuleRepository) Delete(ctx context.Context, id string) error {
	if r == nil || r.db == nil {
		return errors.New("asset match rule repository is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ErrNotFound
	}
	res := r.db.WithContext(ctx).Where("rule_id = ?", id).Delete(&matchRuleModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MatchRuleRepository) CountByApplicationID(ctx context.Context, applicationID string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset match rule repository is not configured")
	}
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return 0, nil
	}
	var n int64
	if err := r.db.WithContext(ctx).Model(&matchRuleModel{}).
		Where("application_id = ?", applicationID).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *MatchRuleRepository) CountByResourceID(ctx context.Context, resourceID string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset match rule repository is not configured")
	}
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return 0, nil
	}
	var n int64
	if err := r.db.WithContext(ctx).Model(&matchRuleModel{}).
		Where("resource_id = ?", resourceID).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func toMatchRuleModel(rule *domain.MatchRule) matchRuleModel {
	return matchRuleModel{
		RuleID:            rule.ID,
		Name:              rule.Name,
		Enabled:           rule.Enabled,
		Priority:          rule.Priority,
		TargetType:        string(rule.TargetType),
		SourceType:        string(rule.SourceType),
		LabelKey:          rule.LabelKey,
		LabelValuePattern: rule.LabelValuePattern,
		ApplicationID:     rule.ApplicationID,
		ResourceID:        rule.ResourceID,
	}
}

func toMatchRuleDomain(m *matchRuleModel) domain.MatchRule {
	if m == nil {
		return domain.MatchRule{}
	}
	return domain.MatchRule{
		ID:                m.RuleID,
		Name:              m.Name,
		Enabled:           m.Enabled,
		Priority:          m.Priority,
		TargetType:        domain.MatchTargetType(m.TargetType),
		SourceType:        domain.MatchSourceType(m.SourceType),
		LabelKey:          m.LabelKey,
		LabelValuePattern: m.LabelValuePattern,
		ApplicationID:     m.ApplicationID,
		ResourceID:        m.ResourceID,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

func toMatchRuleDomains(rows []matchRuleModel) []domain.MatchRule {
	out := make([]domain.MatchRule, 0, len(rows))
	for i := range rows {
		out = append(out, toMatchRuleDomain(&rows[i]))
	}
	return out
}
