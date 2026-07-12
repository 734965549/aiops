package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/execution/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type stepModel struct {
	database.BaseModel
	StepID          string     `gorm:"column:step_id;type:varchar(36);uniqueIndex;not null"`
	TaskID          string     `gorm:"column:task_id;type:varchar(36);not null;index"`
	StepOrder       int        `gorm:"column:step_order;not null"`
	Name            string     `gorm:"column:name;type:varchar(255);not null"`
	ActionType      string     `gorm:"column:action_type;type:varchar(64);not null"`
	Status          string     `gorm:"column:status;type:varchar(32);not null"`
	RunbookStepID   string     `gorm:"column:runbook_step_id;type:varchar(36);not null;default:''"`
	Parameters      []byte     `gorm:"column:parameters;type:jsonb;not null;default:'{}'::jsonb"`
	RiskLevel       string     `gorm:"column:risk_level;type:varchar(16);not null;default:''"`
	DryRun          bool       `gorm:"column:dry_run;not null;default:false"`
	RollbackPlan    []byte     `gorm:"column:rollback_plan;type:jsonb;not null;default:'{}'::jsonb"`
	TimeoutSeconds  int        `gorm:"column:timeout_seconds;not null;default:0"`
	CommandSpecID   string     `gorm:"column:command_spec_id;type:varchar(64);not null;default:''"`
	CommandTemplate string     `gorm:"column:command_template;type:text;not null;default:''"`
	Arguments       []byte     `gorm:"column:arguments;type:jsonb;not null;default:'{}'::jsonb"`
	OutputRedaction []byte     `gorm:"column:output_redaction;type:jsonb;not null;default:'{}'::jsonb"`
	WorkingDir      string     `gorm:"column:working_dir;type:varchar(256);not null;default:''"`
	RequiresTTY     bool       `gorm:"column:requires_tty;not null;default:false"`
	Output          []byte     `gorm:"column:output;type:jsonb;not null;default:'{}'::jsonb"`
	ErrorMessage    string     `gorm:"column:error_message;type:text;not null;default:''"`
	StartedAt       *time.Time `gorm:"column:started_at"`
	FinishedAt      *time.Time `gorm:"column:finished_at"`
}

func (stepModel) TableName() string { return "exec_step" }

type StepRepository struct {
	db *gorm.DB
}

func NewStepRepository(db *gorm.DB) *StepRepository {
	return &StepRepository{db: db}
}

func (r *StepRepository) Create(ctx context.Context, step *domain.Step) error {
	if r == nil || r.db == nil {
		return errors.New("execution step repository is not configured")
	}
	if step == nil {
		return errors.New("step is nil")
	}
	if err := r.ensureParentTaskExists(ctx, step.TaskID); err != nil {
		return err
	}
	out, err := marshalAnyMap(step.Output)
	if err != nil {
		return err
	}
	params, err := marshalAnyMap(step.Parameters)
	if err != nil {
		return err
	}
	rollback, err := marshalAnyMap(step.RollbackPlan)
	if err != nil {
		return err
	}
	args, err := marshalAnyMap(step.Arguments)
	if err != nil {
		return err
	}
	redaction, err := marshalAnyMap(step.OutputRedaction)
	if err != nil {
		return err
	}
	m := stepModel{
		StepID:          step.ID,
		TaskID:          step.TaskID,
		StepOrder:       step.StepOrder,
		Name:            step.Name,
		ActionType:      step.ActionType,
		Status:          string(step.Status),
		RunbookStepID:   step.RunbookStepID,
		CommandSpecID:   step.CommandSpecID,
		CommandTemplate: step.CommandTemplate,
		Arguments:       args,
		OutputRedaction: redaction,
		WorkingDir:      step.WorkingDir,
		RequiresTTY:     step.RequiresTTY,
		Parameters:      params,
		RiskLevel:       string(step.RiskLevel),
		DryRun:          step.DryRun,
		RollbackPlan:    rollback,
		TimeoutSeconds:  step.TimeoutSeconds,
		Output:          out,
		ErrorMessage:    step.ErrorMessage,
		StartedAt:       step.StartedAt,
		FinishedAt:      step.FinishedAt,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	step.CreatedAt = m.CreatedAt
	step.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *StepRepository) Update(ctx context.Context, step *domain.Step) error {
	if r == nil || r.db == nil {
		return errors.New("execution step repository is not configured")
	}
	if step == nil {
		return errors.New("step is nil")
	}
	out, err := marshalAnyMap(step.Output)
	if err != nil {
		return err
	}
	m := stepModel{
		Status:       string(step.Status),
		Output:       out,
		ErrorMessage: step.ErrorMessage,
		StartedAt:    step.StartedAt,
		FinishedAt:   step.FinishedAt,
	}
	res := r.db.WithContext(ctx).Model(&stepModel{}).Where("step_id = ?", step.ID).Updates(m)
	if res.Error != nil {
		return database.MapUniqueViolation(res.Error, domain.ErrAlreadyExists)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *StepRepository) ListByTaskID(ctx context.Context, taskID string) ([]domain.Step, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("execution step repository is not configured")
	}
	var rows []stepModel
	if err := r.db.WithContext(ctx).
		Where("task_id = ?", strings.TrimSpace(taskID)).
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

func (r *StepRepository) ensureParentTaskExists(ctx context.Context, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return domain.ErrInvalidArgument
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&taskModel{}).Where("task_id = ?", taskID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func toStepDomain(m *stepModel) domain.Step {
	return domain.Step{
		ID:              m.StepID,
		TaskID:          m.TaskID,
		StepOrder:       m.StepOrder,
		Name:            m.Name,
		ActionType:      m.ActionType,
		Status:          domain.StepStatus(m.Status),
		RunbookStepID:   m.RunbookStepID,
		CommandSpecID:   m.CommandSpecID,
		CommandTemplate: m.CommandTemplate,
		Arguments:       unmarshalAnyMap(m.Arguments),
		OutputRedaction: unmarshalAnyMap(m.OutputRedaction),
		WorkingDir:      m.WorkingDir,
		RequiresTTY:     m.RequiresTTY,
		Parameters:      unmarshalAnyMap(m.Parameters),
		RiskLevel:       domain.RiskLevel(m.RiskLevel),
		DryRun:          m.DryRun,
		RollbackPlan:    unmarshalAnyMap(m.RollbackPlan),
		TimeoutSeconds:  m.TimeoutSeconds,
		Output:          unmarshalAnyMap(m.Output),
		ErrorMessage:    m.ErrorMessage,
		StartedAt:       m.StartedAt,
		FinishedAt:      m.FinishedAt,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}
