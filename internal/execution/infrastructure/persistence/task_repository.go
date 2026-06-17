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

type taskModel struct {
	database.BaseModel
	TaskID            string     `gorm:"column:task_id;type:varchar(36);uniqueIndex;not null"`
	Name              string     `gorm:"column:name;type:varchar(255);not null"`
	SourceType        string     `gorm:"column:source_type;type:varchar(32);not null;index:idx_exec_task_source,priority:1"`
	SourceID          string     `gorm:"column:source_id;type:varchar(128);not null;default:'';index:idx_exec_task_source,priority:2"`
	OperationType     string     `gorm:"column:operation_type;type:varchar(32);not null"`
	TargetType        string     `gorm:"column:target_type;type:varchar(64);not null;default:''"`
	TargetID          string     `gorm:"column:target_id;type:varchar(128);not null;default:''"`
	TargetName        string     `gorm:"column:target_name;type:varchar(255);not null;default:''"`
	Environment       string     `gorm:"column:environment;type:varchar(64);not null;default:''"`
	RiskLevel         string     `gorm:"column:risk_level;type:varchar(16);not null"`
	Status            string     `gorm:"column:status;type:varchar(32);not null;index:idx_exec_task_status_created,priority:1"`
	Parameters        []byte     `gorm:"column:parameters;type:jsonb;not null;default:'{}'::jsonb"`
	RollbackPlan      []byte     `gorm:"column:rollback_plan;type:jsonb;not null;default:'{}'::jsonb"`
	RunbookTemplateID string     `gorm:"column:runbook_template_id;type:varchar(36);not null;default:''"`
	RunbookSnapshot   []byte     `gorm:"column:runbook_snapshot;type:jsonb;not null;default:'{}'::jsonb"`
	DryRun            bool       `gorm:"column:dry_run;not null;default:false"`
	ResultSummary     string     `gorm:"column:result_summary;type:varchar(512);not null;default:''"`
	ErrorMessage      string     `gorm:"column:error_message;type:text;not null;default:''"`
	CreatedBy         string     `gorm:"column:created_by;type:varchar(36);not null;default:''"`
	ConfirmedBy       string     `gorm:"column:confirmed_by;type:varchar(36);not null;default:''"`
	ExecutedBy        string     `gorm:"column:executed_by;type:varchar(36);not null;default:''"`
	ConfirmedAt       *time.Time `gorm:"column:confirmed_at"`
	StartedAt         *time.Time `gorm:"column:started_at"`
	FinishedAt        *time.Time `gorm:"column:finished_at"`
}

func (taskModel) TableName() string { return "exec_task" }

type TaskRepository struct {
	db *gorm.DB
}

func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) CreateWithSteps(ctx context.Context, task *domain.Task, steps []domain.Step) error {
	if r == nil || r.db == nil {
		return errors.New("execution task repository is not configured")
	}
	if task == nil {
		return errors.New("task is nil")
	}
	if err := domain.ValidateStepsForTask(task, steps); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		taskRepo := &TaskRepository{db: tx}
		stepRepo := &StepRepository{db: tx}
		if err := taskRepo.Create(ctx, task); err != nil {
			return err
		}
		for i := range steps {
			if err := stepRepo.Create(ctx, &steps[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *TaskRepository) Create(ctx context.Context, task *domain.Task) error {
	if r == nil || r.db == nil {
		return errors.New("execution task repository is not configured")
	}
	if task == nil {
		return errors.New("task is nil")
	}
	m, err := toTaskModel(task)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	fillTaskFromModel(task, m)
	return nil
}

func (r *TaskRepository) Update(ctx context.Context, task *domain.Task) error {
	if r == nil || r.db == nil {
		return errors.New("execution task repository is not configured")
	}
	if task == nil {
		return errors.New("task is nil")
	}
	m, err := toTaskModel(task)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&taskModel{}).Where("task_id = ?", task.ID).Updates(m)
	if res.Error != nil {
		return database.MapUniqueViolation(res.Error, domain.ErrAlreadyExists)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *TaskRepository) UpdateStatusIf(
	ctx context.Context,
	taskID string,
	fromStatus, toStatus domain.TaskStatus,
	mutator func(*domain.Task),
) (*domain.Task, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("execution task repository is not configured")
	}
	taskID = strings.TrimSpace(taskID)
	task, err := r.GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != fromStatus {
		return nil, domain.ErrInvalidTransition
	}

	next := *task
	if mutator != nil {
		mutator(&next)
	}
	next.Status = toStatus
	if next.UpdatedAt.IsZero() {
		next.UpdatedAt = time.Now()
	}

	updates := map[string]any{
		"status":     string(toStatus),
		"updated_at": next.UpdatedAt,
	}
	if next.ExecutedBy != task.ExecutedBy {
		updates["executed_by"] = next.ExecutedBy
	}
	if next.ConfirmedBy != task.ConfirmedBy {
		updates["confirmed_by"] = next.ConfirmedBy
	}
	if next.StartedAt != task.StartedAt {
		updates["started_at"] = next.StartedAt
	}
	if next.ConfirmedAt != task.ConfirmedAt {
		updates["confirmed_at"] = next.ConfirmedAt
	}

	res := r.db.WithContext(ctx).Model(&taskModel{}).
		Where("task_id = ? AND status = ?", taskID, string(fromStatus)).
		Updates(updates)
	if res.Error != nil {
		return nil, database.MapUniqueViolation(res.Error, domain.ErrAlreadyExists)
	}
	if res.RowsAffected == 0 {
		return nil, domain.ErrInvalidTransition
	}
	return r.GetByID(ctx, taskID)
}

func (r *TaskRepository) GetByID(ctx context.Context, taskID string) (*domain.Task, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("execution task repository is not configured")
	}
	var m taskModel
	if err := r.db.WithContext(ctx).Where("task_id = ?", strings.TrimSpace(taskID)).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	t := toTaskDomain(&m)
	return &t, nil
}

func (r *TaskRepository) List(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("execution task repository is not configured")
	}
	q := r.applyFilter(r.db.WithContext(ctx).Model(&taskModel{}), filter)
	var rows []taskModel
	if err := q.Order("created_at DESC, id DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Task, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTaskDomain(&row))
	}
	return out, nil
}

func (r *TaskRepository) Count(ctx context.Context, filter domain.TaskFilter) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("execution task repository is not configured")
	}
	var total int64
	q := r.applyFilter(r.db.WithContext(ctx).Model(&taskModel{}), filter)
	if err := q.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *TaskRepository) applyFilter(q *gorm.DB, filter domain.TaskFilter) *gorm.DB {
	if len(filter.Statuses) > 0 {
		statuses := make([]string, 0, len(filter.Statuses))
		for _, s := range filter.Statuses {
			statuses = append(statuses, string(s))
		}
		q = q.Where("status IN ?", statuses)
	}
	if filter.SourceType != "" {
		q = q.Where("source_type = ?", filter.SourceType)
	}
	if filter.SourceID != "" {
		q = q.Where("source_id = ?", filter.SourceID)
	}
	if filter.Keyword != "" {
		kw := "%" + filter.Keyword + "%"
		q = q.Where("name ILIKE ? OR target_name ILIKE ?", kw, kw)
	}
	return q
}

func toTaskModel(task *domain.Task) (*taskModel, error) {
	params, err := marshalAnyMap(task.Parameters)
	if err != nil {
		return nil, err
	}
	rollback, err := marshalAnyMap(task.RollbackPlan)
	if err != nil {
		return nil, err
	}
	snapshot, err := marshalAnyMap(task.RunbookSnapshot)
	if err != nil {
		return nil, err
	}
	return &taskModel{
		TaskID:            task.ID,
		Name:              task.Name,
		SourceType:        string(task.SourceType),
		SourceID:          task.SourceID,
		OperationType:     string(task.OperationType),
		TargetType:        task.TargetType,
		TargetID:          task.TargetID,
		TargetName:        task.TargetName,
		Environment:       task.Environment,
		RiskLevel:         string(task.RiskLevel),
		Status:            string(task.Status),
		Parameters:        params,
		RollbackPlan:      rollback,
		RunbookTemplateID: task.RunbookTemplateID,
		RunbookSnapshot:   snapshot,
		DryRun:            task.DryRun,
		ResultSummary:     task.ResultSummary,
		ErrorMessage:      task.ErrorMessage,
		CreatedBy:         task.CreatedBy,
		ConfirmedBy:       task.ConfirmedBy,
		ExecutedBy:        task.ExecutedBy,
		ConfirmedAt:       task.ConfirmedAt,
		StartedAt:         task.StartedAt,
		FinishedAt:        task.FinishedAt,
	}, nil
}

func toTaskDomain(m *taskModel) domain.Task {
	return domain.Task{
		ID:                m.TaskID,
		Name:              m.Name,
		SourceType:        domain.SourceType(m.SourceType),
		SourceID:          m.SourceID,
		OperationType:     domain.OperationType(m.OperationType),
		TargetType:        m.TargetType,
		TargetID:          m.TargetID,
		TargetName:        m.TargetName,
		Environment:       m.Environment,
		RiskLevel:         domain.RiskLevel(m.RiskLevel),
		Status:            domain.TaskStatus(m.Status),
		Parameters:        unmarshalAnyMap(m.Parameters),
		RollbackPlan:      unmarshalAnyMap(m.RollbackPlan),
		RunbookTemplateID: m.RunbookTemplateID,
		RunbookSnapshot:   unmarshalAnyMap(m.RunbookSnapshot),
		DryRun:            m.DryRun,
		ResultSummary:     m.ResultSummary,
		ErrorMessage:      m.ErrorMessage,
		CreatedBy:         m.CreatedBy,
		ConfirmedBy:       m.ConfirmedBy,
		ExecutedBy:        m.ExecutedBy,
		ConfirmedAt:       m.ConfirmedAt,
		StartedAt:         m.StartedAt,
		FinishedAt:        m.FinishedAt,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

func fillTaskFromModel(task *domain.Task, m *taskModel) {
	task.CreatedAt = m.CreatedAt
	task.UpdatedAt = m.UpdatedAt
}
