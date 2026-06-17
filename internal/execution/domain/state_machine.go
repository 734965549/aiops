package domain

// CanConfirm 是否允许确认：pending_confirm → pending_execute。
func CanConfirm(status TaskStatus) bool {
	return status == StatusPendingConfirm
}

// CanExecute 是否允许触发执行。
func CanExecute(status TaskStatus) bool {
	return status == StatusPendingExecute
}

// TransitionConfirm 确认后目标状态。
func TransitionConfirm(status TaskStatus) (TaskStatus, error) {
	if !CanConfirm(status) {
		return "", ErrInvalidTransition
	}
	return StatusPendingExecute, nil
}

// TransitionExecute 触发执行后目标状态。
func TransitionExecute(status TaskStatus) (TaskStatus, error) {
	if !CanExecute(status) {
		return "", ErrInvalidTransition
	}
	return StatusRunning, nil
}
