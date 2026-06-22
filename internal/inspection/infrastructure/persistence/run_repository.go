package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/734965549/aiops/internal/inspection/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type runModel struct {
	database.BaseModel
	RunID       string     `gorm:"column:run_id;type:varchar(64);uniqueIndex;not null"`
	PolicyID    string     `gorm:"column:policy_id;type:varchar(64);not null;index"`
	Status      string     `gorm:"column:status;type:varchar(16);not null;default:'pending'"`
	TriggerType string     `gorm:"column:trigger_type;type:varchar(16);not null;default:'manual'"`
	Summary     string     `gorm:"column:summary;type:varchar(512);not null;default:''"`
	Timeline    []byte     `gorm:"column:timeline;type:jsonb;not null"`
	StartedAt   *time.Time `gorm:"column:started_at"`
	FinishedAt  *time.Time `gorm:"column:finished_at"`
}

func (runModel) TableName() string { return "inspection_run" }

type RunRepository struct {
	db *gorm.DB
}

func NewRunRepository(db *gorm.DB) *RunRepository {
	return &RunRepository{db: db}
}

func (r *RunRepository) Create(ctx context.Context, run *domain.InspectionRun) error {
	m, err := toRunModel(run)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	run.CreatedAt = m.CreatedAt
	run.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *RunRepository) Update(ctx context.Context, run *domain.InspectionRun) error {
	m, err := toRunModel(run)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&runModel{}).Where("run_id = ?", run.RunID).Updates(map[string]any{
		"status": m.Status, "summary": m.Summary, "timeline": m.Timeline,
		"started_at": m.StartedAt, "finished_at": m.FinishedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *RunRepository) GetByID(ctx context.Context, runID string) (*domain.InspectionRun, error) {
	var m runModel
	err := r.db.WithContext(ctx).Where("run_id = ?", runID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return fromRunModel(&m)
}

func (r *RunRepository) List(ctx context.Context, filter domain.RunFilter) ([]domain.InspectionRun, error) {
	q := r.db.WithContext(ctx).Model(&runModel{})
	if filter.PolicyID != "" {
		q = q.Where("policy_id = ?", filter.PolicyID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	var rows []runModel
	if err := q.Order("created_at DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.InspectionRun, 0, len(rows))
	for i := range rows {
		run, err := fromRunModel(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, nil
}

func (r *RunRepository) Count(ctx context.Context, filter domain.RunFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&runModel{})
	if filter.PolicyID != "" {
		q = q.Where("policy_id = ?", filter.PolicyID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	var total int64
	return total, q.Count(&total).Error
}

func toRunModel(run *domain.InspectionRun) (runModel, error) {
	timeline, err := marshalJSON(run.Timeline)
	if err != nil {
		return runModel{}, err
	}
	return runModel{
		RunID: run.RunID, PolicyID: run.PolicyID, Status: string(run.Status),
		TriggerType: string(run.TriggerType), Summary: run.Summary, Timeline: timeline,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
	}, nil
}

func fromRunModel(m *runModel) (*domain.InspectionRun, error) {
	var timeline []domain.TimelineEvent
	if len(m.Timeline) > 0 {
		_ = json.Unmarshal(m.Timeline, &timeline)
	}
	return &domain.InspectionRun{
		RunID: m.RunID, PolicyID: m.PolicyID, Status: domain.RunStatus(m.Status),
		TriggerType: domain.TriggerType(m.TriggerType), Summary: m.Summary, Timeline: timeline,
		StartedAt: m.StartedAt, FinishedAt: m.FinishedAt,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}, nil
}
