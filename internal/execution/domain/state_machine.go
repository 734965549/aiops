package domain

// 状态机规则见 ops/execution-contract.md §4.1。

// TaskAction 任务状态流转动作，与契约状态图边标签对齐。
type TaskAction string

const (
	ActionConfirm TaskAction = "confirm" // pending_confirm → pending_execute
	ActionReject  TaskAction = "reject"  // pending_confirm → cancelled
	ActionExecute TaskAction = "execute" // pending_execute → running
)

var allowedTransitions = map[TaskStatus][]TaskStatus{
	StatusPendingConfirm: {StatusPendingExecute, StatusCancelled},
	StatusPendingExecute: {StatusRunning},
	StatusRunning:        {StatusSuccess, StatusFailed},
}

// CanTransitionTo 判断 from → to 是否为合法流转。
func CanTransitionTo(from, to TaskStatus) bool {
	targets, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == to {
			return true
		}
	}
	return false
}

// TransitionForAction 根据动作返回 CAS 所需的 from/to 状态对。
func TransitionForAction(action TaskAction) (from, to TaskStatus, err error) {
	switch action {
	case ActionConfirm:
		return StatusPendingConfirm, StatusPendingExecute, nil
	case ActionReject:
		return StatusPendingConfirm, StatusCancelled, nil
	case ActionExecute:
		return StatusPendingExecute, StatusRunning, nil
	default:
		return "", "", ErrInvalidTransition
	}
}

// CanConfirm 是否允许确认：pending_confirm → pending_execute。
func CanConfirm(status TaskStatus) bool {
	return CanTransitionTo(status, StatusPendingExecute)
}

// CanReject 是否允许拒绝：pending_confirm → cancelled。
func CanReject(status TaskStatus) bool {
	return CanTransitionTo(status, StatusCancelled)
}

// CanExecute 是否允许触发执行：pending_execute → running。
func CanExecute(status TaskStatus) bool {
	return CanTransitionTo(status, StatusRunning)
}

// TransitionConfirm 确认后目标状态。
func TransitionConfirm(status TaskStatus) (TaskStatus, error) {
	to := StatusPendingExecute
	if !CanTransitionTo(status, to) {
		return "", ErrInvalidTransition
	}
	return to, nil
}

// TransitionReject 拒绝后目标状态。
func TransitionReject(status TaskStatus) (TaskStatus, error) {
	to := StatusCancelled
	if !CanTransitionTo(status, to) {
		return "", ErrInvalidTransition
	}
	return to, nil
}

// TransitionExecute 触发执行后目标状态。
func TransitionExecute(status TaskStatus) (TaskStatus, error) {
	to := StatusRunning
	if !CanTransitionTo(status, to) {
		return "", ErrInvalidTransition
	}
	return to, nil
}
