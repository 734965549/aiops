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

// MediumFilter 介体列表过滤。
type MediumFilter struct {
	Enabled     *bool
	Environment string
	MediumType  string
	Keyword     string
	Limit       int
	Offset      int
}

// MediumRepository 执行介体仓储。
type MediumRepository interface {
	Create(ctx context.Context, medium *ExecutionMedium) error
	Update(ctx context.Context, medium *ExecutionMedium) error
	GetByID(ctx context.Context, mediumID string) (*ExecutionMedium, error)
	List(ctx context.Context, filter MediumFilter) ([]ExecutionMedium, error)
	Count(ctx context.Context, filter MediumFilter) (int64, error)
}

// CommandSpecRepository 命令规格仓储。
type CommandSpecRepository interface {
	Create(ctx context.Context, spec *CommandSpec) error
	Update(ctx context.Context, spec *CommandSpec) error
	GetByID(ctx context.Context, commandSpecID string) (*CommandSpec, error)
	List(ctx context.Context, enabled *bool, limit, offset int) ([]CommandSpec, error)
	Count(ctx context.Context, enabled *bool) (int64, error)
}

// AgentRepository 执行代理仓储。
type AgentRepository interface {
	Create(ctx context.Context, agent *ExecutionAgent) error
	Update(ctx context.Context, agent *ExecutionAgent) error
	GetByID(ctx context.Context, agentID string) (*ExecutionAgent, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*ExecutionAgent, error)
	ListByMedium(ctx context.Context, mediumID string) ([]ExecutionAgent, error)
}

// LeaseRepository 租约仓储。
type LeaseRepository interface {
	Create(ctx context.Context, lease *ExecutionLease) error
	Update(ctx context.Context, lease *ExecutionLease) error
	GetByID(ctx context.Context, leaseID string) (*ExecutionLease, error)
	GetActiveByTask(ctx context.Context, taskID string) (*ExecutionLease, error)
}

// LogStreamRepository 日志流仓储。
type LogStreamRepository interface {
	Create(ctx context.Context, entry *LogStreamEntry) error
	ListByTaskStep(ctx context.Context, taskID, stepID string) ([]LogStreamEntry, error)
}

// AgentTaskRepository 代理任务查询扩展。
type AgentTaskRepository interface {
	TaskRepository
	FindDispatchableTask(ctx context.Context, mediumID string) (*Task, error)
}
