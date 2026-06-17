package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/runbook/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type templateModel struct {
	database.BaseModel
	TemplateID        string `gorm:"column:template_id;type:varchar(36);uniqueIndex;not null"`
	Name              string `gorm:"column:name;type:varchar(255);not null"`
	Description       string `gorm:"column:description;type:text;not null;default:''"`
	Enabled           bool   `gorm:"column:enabled;not null;default:true;index"`
	OperationType     string `gorm:"column:operation_type;type:varchar(32);not null"`
	RiskLevel         string `gorm:"column:risk_level;type:varchar(16);not null"`
	MatchAlertName    string `gorm:"column:match_alert_name;type:varchar(255);not null;default:''"`
	MatchResourceType string `gorm:"column:match_resource_type;type:varchar(64);not null;default:''"`
	MatchEnvironment  string `gorm:"column:match_environment;type:varchar(64);not null;default:''"`
	ParameterSchema   []byte `gorm:"column:parameter_schema;type:jsonb;not null;default:'{}'::jsonb"`
	RollbackPlan      []byte `gorm:"column:rollback_plan;type:jsonb;not null;default:'{}'::jsonb"`
	CreatedBy         string `gorm:"column:created_by;type:varchar(36);not null;default:''"`
}

func (templateModel) TableName() string { return "runbook_template" }

type TemplateRepository struct {
	db *gorm.DB
}

func NewTemplateRepository(db *gorm.DB) *TemplateRepository {
	return &TemplateRepository{db: db}
}

func (r *TemplateRepository) Create(ctx context.Context, tpl *domain.Template) error {
	if r == nil || r.db == nil {
		return errors.New("runbook template repository is not configured")
	}
	return r.create(ctx, r.db.WithContext(ctx), tpl)
}

func (r *TemplateRepository) CreateWithSteps(ctx context.Context, tpl *domain.Template, steps []domain.Step) error {
	if r == nil || r.db == nil {
		return errors.New("runbook template repository is not configured")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.create(ctx, tx, tpl); err != nil {
			return err
		}
		for i := range steps {
			if err := createStep(ctx, tx, &steps[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *TemplateRepository) create(_ context.Context, db *gorm.DB, tpl *domain.Template) error {
	if tpl == nil {
		return errors.New("template is nil")
	}
	m, err := toTemplateModel(tpl)
	if err != nil {
		return err
	}
	if err := db.Create(m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	fillTemplateFromModel(tpl, m)
	return nil
}

func (r *TemplateRepository) Update(ctx context.Context, tpl *domain.Template) error {
	if r == nil || r.db == nil {
		return errors.New("runbook template repository is not configured")
	}
	if tpl == nil {
		return errors.New("template is nil")
	}
	m, err := toTemplateModel(tpl)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&templateModel{}).Where("template_id = ?", tpl.ID).Updates(templateUpdateValues(m))
	if res.Error != nil {
		return database.MapUniqueViolation(res.Error, domain.ErrAlreadyExists)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *TemplateRepository) ReplaceWithSteps(ctx context.Context, tpl *domain.Template, steps []domain.Step) error {
	if r == nil || r.db == nil {
		return errors.New("runbook template repository is not configured")
	}
	if tpl == nil {
		return errors.New("template is nil")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m, err := toTemplateModel(tpl)
		if err != nil {
			return err
		}
		res := tx.Model(&templateModel{}).Where("template_id = ?", tpl.ID).Updates(templateUpdateValues(m))
		if res.Error != nil {
			return database.MapUniqueViolation(res.Error, domain.ErrAlreadyExists)
		}
		if res.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		if err := tx.Where("template_id = ?", strings.TrimSpace(tpl.ID)).Delete(&stepModel{}).Error; err != nil {
			return err
		}
		for i := range steps {
			if err := createStep(ctx, tx, &steps[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *TemplateRepository) Delete(ctx context.Context, templateID string) error {
	if r == nil || r.db == nil {
		return errors.New("runbook template repository is not configured")
	}
	res := r.db.WithContext(ctx).Where("template_id = ?", strings.TrimSpace(templateID)).Delete(&templateModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *TemplateRepository) GetByID(ctx context.Context, templateID string) (*domain.Template, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("runbook template repository is not configured")
	}
	var m templateModel
	if err := r.db.WithContext(ctx).Where("template_id = ?", strings.TrimSpace(templateID)).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	t := toTemplateDomain(&m)
	return &t, nil
}

func (r *TemplateRepository) List(ctx context.Context, filter domain.TemplateFilter) ([]domain.Template, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("runbook template repository is not configured")
	}
	q := r.applyFilter(r.db.WithContext(ctx).Model(&templateModel{}), filter)
	var rows []templateModel
	if err := q.Order("updated_at DESC, id DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Template, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTemplateDomain(&row))
	}
	return out, nil
}

func (r *TemplateRepository) Count(ctx context.Context, filter domain.TemplateFilter) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("runbook template repository is not configured")
	}
	var total int64
	q := r.applyFilter(r.db.WithContext(ctx).Model(&templateModel{}), filter)
	if err := q.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *TemplateRepository) ListEnabled(ctx context.Context) ([]domain.Template, error) {
	enabled := true
	return r.List(ctx, domain.TemplateFilter{Enabled: &enabled, Limit: 1000})
}

func (r *TemplateRepository) applyFilter(q *gorm.DB, filter domain.TemplateFilter) *gorm.DB {
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	if filter.Keyword != "" {
		kw := "%" + filter.Keyword + "%"
		q = q.Where("name ILIKE ? OR description ILIKE ?", kw, kw)
	}
	return q
}

func toTemplateModel(tpl *domain.Template) (*templateModel, error) {
	schema, err := marshalAnyMap(tpl.ParameterSchema)
	if err != nil {
		return nil, err
	}
	rollback, err := marshalAnyMap(tpl.RollbackPlan)
	if err != nil {
		return nil, err
	}
	return &templateModel{
		TemplateID:        tpl.ID,
		Name:              tpl.Name,
		Description:       tpl.Description,
		Enabled:           tpl.Enabled,
		OperationType:     string(tpl.OperationType),
		RiskLevel:         string(tpl.RiskLevel),
		MatchAlertName:    tpl.MatchAlertName,
		MatchResourceType: tpl.MatchResourceType,
		MatchEnvironment:  tpl.MatchEnvironment,
		ParameterSchema:   schema,
		RollbackPlan:      rollback,
		CreatedBy:         tpl.CreatedBy,
	}, nil
}

func templateUpdateValues(m *templateModel) map[string]any {
	return map[string]any{
		"name":                m.Name,
		"description":         m.Description,
		"enabled":             m.Enabled,
		"operation_type":      m.OperationType,
		"risk_level":          m.RiskLevel,
		"match_alert_name":    m.MatchAlertName,
		"match_resource_type": m.MatchResourceType,
		"match_environment":   m.MatchEnvironment,
		"parameter_schema":    m.ParameterSchema,
		"rollback_plan":       m.RollbackPlan,
	}
}

func toTemplateDomain(m *templateModel) domain.Template {
	return domain.Template{
		ID:                m.TemplateID,
		Name:              m.Name,
		Description:       m.Description,
		Enabled:           m.Enabled,
		OperationType:     domain.OperationType(m.OperationType),
		RiskLevel:         domain.RiskLevel(m.RiskLevel),
		MatchAlertName:    m.MatchAlertName,
		MatchResourceType: m.MatchResourceType,
		MatchEnvironment:  m.MatchEnvironment,
		ParameterSchema:   unmarshalAnyMap(m.ParameterSchema),
		RollbackPlan:      unmarshalAnyMap(m.RollbackPlan),
		CreatedBy:         m.CreatedBy,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

func fillTemplateFromModel(tpl *domain.Template, m *templateModel) {
	tpl.CreatedAt = m.CreatedAt
	tpl.UpdatedAt = m.UpdatedAt
	if tpl.UpdatedAt.IsZero() {
		tpl.UpdatedAt = time.Now()
	}
}
