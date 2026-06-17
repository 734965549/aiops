package domain

// TaskStatus 执行任务状态（ops/execution-contract.md §4.1）。
type TaskStatus string

const (
	StatusPendingConfirm TaskStatus = "pending_confirm"
	StatusPendingExecute TaskStatus = "pending_execute"
	StatusRunning        TaskStatus = "running"
	StatusSuccess        TaskStatus = "success"
	StatusFailed         TaskStatus = "failed"
	StatusCancelled      TaskStatus = "cancelled"
)

var allTaskStatuses = []TaskStatus{
	StatusPendingConfirm, StatusPendingExecute, StatusRunning,
	StatusSuccess, StatusFailed, StatusCancelled,
}

func (s TaskStatus) IsValid() bool {
	for _, v := range allTaskStatuses {
		if s == v {
			return true
		}
	}
	return false
}

func (s TaskStatus) IsTerminal() bool {
	return s == StatusSuccess || s == StatusFailed || s == StatusCancelled
}

// StepStatus 步骤状态。
type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepRunning StepStatus = "running"
	StepSuccess StepStatus = "success"
	StepFailed  StepStatus = "failed"
)

func (s StepStatus) IsValid() bool {
	switch s {
	case StepPending, StepRunning, StepSuccess, StepFailed:
		return true
	default:
		return false
	}
}
