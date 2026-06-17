package domain

import "context"

// TaskFilter 列表过滤。
type TaskFilter struct {
	Statuses   []TaskStatus
	SourceType string
	SourceID   string
	Keyword    string
	Limit      int
	Offset     int
}

// TaskRepository 任务仓储。
type TaskRepository interface {
	Create(ctx context.Context, task *Task) error
	Update(ctx context.Context, task *Task) error
	UpdateStatusIf(ctx context.Context, taskID string, fromStatus, toStatus TaskStatus, mutator func(*Task)) (*Task, error)
	GetByID(ctx context.Context, taskID string) (*Task, error)
	List(ctx context.Context, filter TaskFilter) ([]Task, error)
	Count(ctx context.Context, filter TaskFilter) (int64, error)
}

// TaskCreator 原子创建任务及其步骤。
type TaskCreator interface {
	CreateWithSteps(ctx context.Context, task *Task, steps []Step) error
}

// StepRepository 步骤仓储。
type StepRepository interface {
	Create(ctx context.Context, step *Step) error
	Update(ctx context.Context, step *Step) error
	ListByTaskID(ctx context.Context, taskID string) ([]Step, error)
}
