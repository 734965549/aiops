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

type logStreamModel struct {
	database.BaseModel
	LogID      string    `gorm:"column:log_id;type:varchar(64);uniqueIndex;not null"`
	LeaseID    string    `gorm:"column:lease_id;type:varchar(64);not null"`
	TaskID     string    `gorm:"column:task_id;type:varchar(36);not null;index"`
	StepID     string    `gorm:"column:step_id;type:varchar(36);not null"`
	AgentID    string    `gorm:"column:agent_id;type:varchar(64);not null"`
	Stream     string    `gorm:"column:stream;type:varchar(16);not null"`
	Sequence   int       `gorm:"column:sequence;not null"`
	Content    string    `gorm:"column:content;type:text;not null;default:''"`
	Truncated  bool      `gorm:"column:truncated;not null;default:false"`
	Redacted   bool      `gorm:"column:redacted;not null;default:false"`
	ObservedAt time.Time `gorm:"column:observed_at;not null"`
}

func (logStreamModel) TableName() string { return "exec_log_stream" }

type LogStreamRepository struct{ db *gorm.DB }

func NewLogStreamRepository(db *gorm.DB) *LogStreamRepository {
	return &LogStreamRepository{db: db}
}

func (r *LogStreamRepository) Create(ctx context.Context, entry *domain.LogStreamEntry) error {
	if r == nil || r.db == nil || entry == nil {
		return errors.New("log stream repository is not configured")
	}
	m := toLogStreamModel(entry)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	fillLogStreamFromModel(entry, m)
	return nil
}

func (r *LogStreamRepository) ListByTaskStep(ctx context.Context, taskID, stepID string) ([]domain.LogStreamEntry, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("log stream repository is not configured")
	}
	var rows []logStreamModel
	if err := r.db.WithContext(ctx).
		Where("task_id = ? AND step_id = ?", strings.TrimSpace(taskID), strings.TrimSpace(stepID)).
		Order("sequence ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.LogStreamEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, toLogStreamDomain(&row))
	}
	return out, nil
}

func toLogStreamModel(entry *domain.LogStreamEntry) *logStreamModel {
	return &logStreamModel{
		LogID: entry.LogID, LeaseID: entry.LeaseID, TaskID: entry.TaskID, StepID: entry.StepID,
		AgentID: entry.AgentID, Stream: entry.Stream, Sequence: entry.Sequence,
		Content: entry.Content, Truncated: entry.Truncated, Redacted: entry.Redacted,
		ObservedAt: entry.ObservedAt,
	}
}

func toLogStreamDomain(m *logStreamModel) domain.LogStreamEntry {
	return domain.LogStreamEntry{
		LogID: m.LogID, LeaseID: m.LeaseID, TaskID: m.TaskID, StepID: m.StepID,
		AgentID: m.AgentID, Stream: m.Stream, Sequence: m.Sequence,
		Content: m.Content, Truncated: m.Truncated, Redacted: m.Redacted,
		ObservedAt: m.ObservedAt, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func fillLogStreamFromModel(entry *domain.LogStreamEntry, m *logStreamModel) {
	entry.CreatedAt = m.CreatedAt
	entry.UpdatedAt = m.UpdatedAt
}
