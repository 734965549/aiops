package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/runbook/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type stepModel struct {
	database.BaseModel
	StepID            string `gorm:"column:step_id;type:varchar(36);uniqueIndex;not null"`
	TemplateID        string `gorm:"column:template_id;type:varchar(36);not null;index"`
	StepOrder         int    `gorm:"column:step_order;not null"`
	Name              string `gorm:"column:name;type:varchar(255);not null"`
	ActionType        string `gorm:"column:action_type;type:varchar(64);not null"`
	RiskLevel         string `gorm:"column:risk_level;type:varchar(16);not null"`
	DryRunSupported   bool   `gorm:"column:dry_run_supported;not null;default:false"`
	DefaultDryRun     bool   `gorm:"column:default_dry_run;not null;default:false"`
	ParameterSchema   []byte `gorm:"column:parameter_schema;type:jsonb;not null;default:'{}'::jsonb"`
	DefaultParameters []byte `gorm:"column:default_parameters;type:jsonb;not null;default:'{}'::jsonb"`
	RollbackPlan      []byte `gorm:"column:rollback_plan;type:jsonb;not null;default:'{}'::jsonb"`
	TimeoutSeconds    int    `gorm:"column:timeout_seconds;not null;default:300"`
}

func (stepModel) TableName() string { return "runbook_step" }

type StepRepository struct {
	db *gorm.DB
}

func NewStepRepository(db *gorm.DB) *StepRepository {
	return &StepRepository{db: db}
}

func (r *StepRepository) Create(ctx context.Context, step *domain.Step) error {
	if r == nil || r.db == nil {
		return errors.New("runbook step repository is not configured")
	}
	return createStep(ctx, r.db.WithContext(ctx), step)
}

func createStep(_ context.Context, db *gorm.DB, step *domain.Step) error {
	if step == nil {
		return errors.New("step is nil")
	}
	m, err := toStepModel(step)
	if err != nil {
		return err
	}
	if err := db.Create(m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	step.CreatedAt = m.CreatedAt
	step.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *StepRepository) Update(ctx context.Context, step *domain.Step) error {
	if r == nil || r.db == nil {
		return errors.New("runbook step repository is not configured")
	}
	if step == nil {
		return errors.New("step is nil")
	}
	m, err := toStepModel(step)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&stepModel{}).Where("step_id = ?", step.ID).Updates(m)
	if res.Error != nil {
		return database.MapUniqueViolation(res.Error, domain.ErrAlreadyExists)
	}
	if res.RowsAffected == 0 {
		return domain.ErrStepNotFound
	}
	return nil
}

func (r *StepRepository) DeleteByTemplateID(ctx context.Context, templateID string) error {
	if r == nil || r.db == nil {
		return errors.New("runbook step repository is not configured")
	}
	return r.db.WithContext(ctx).Where("template_id = ?", strings.TrimSpace(templateID)).Delete(&stepModel{}).Error
}

func (r *StepRepository) ListByTemplateID(ctx context.Context, templateID string) ([]domain.Step, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("runbook step repository is not configured")
	}
	var rows []stepModel
	if err := r.db.WithContext(ctx).
		Where("template_id = ?", strings.TrimSpace(templateID)).
		Order("step_order ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Step, 0, len(rows))
	for _, row := range rows {
		out = append(out, toStepDomain(&row))
	}
	return out, nil
}

func toStepModel(step *domain.Step) (*stepModel, error) {
	schema, err := marshalAnyMap(step.ParameterSchema)
	if err != nil {
		return nil, err
	}
	defaults, err := marshalAnyMap(step.DefaultParameters)
	if err != nil {
		return nil, err
	}
	rollback, err := marshalAnyMap(step.RollbackPlan)
	if err != nil {
		return nil, err
	}
	return &stepModel{
		StepID:            step.ID,
		TemplateID:        step.TemplateID,
		StepOrder:         step.StepOrder,
		Name:              step.Name,
		ActionType:        string(step.ActionType),
		RiskLevel:         string(step.RiskLevel),
		DryRunSupported:   step.DryRunSupported,
		DefaultDryRun:     step.DefaultDryRun,
		ParameterSchema:   schema,
		DefaultParameters: defaults,
		RollbackPlan:      rollback,
		TimeoutSeconds:    step.TimeoutSeconds,
	}, nil
}

func toStepDomain(m *stepModel) domain.Step {
	return domain.Step{
		ID:                m.StepID,
		TemplateID:        m.TemplateID,
		StepOrder:         m.StepOrder,
		Name:              m.Name,
		ActionType:        domain.ActionType(m.ActionType),
		RiskLevel:         domain.RiskLevel(m.RiskLevel),
		DryRunSupported:   m.DryRunSupported,
		DefaultDryRun:     m.DefaultDryRun,
		ParameterSchema:   unmarshalAnyMap(m.ParameterSchema),
		DefaultParameters: unmarshalAnyMap(m.DefaultParameters),
		RollbackPlan:      unmarshalAnyMap(m.RollbackPlan),
		TimeoutSeconds:    m.TimeoutSeconds,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}
