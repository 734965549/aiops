package domain

import "strings"

// ValidateStepsForTask 校验步骤与任务归属（应用层替代 DB 外键）。
func ValidateStepsForTask(task *Task, steps []Step) error {
	if task == nil {
		return ErrInvalidArgument
	}
	taskID := strings.TrimSpace(task.ID)
	if taskID == "" {
		return ErrInvalidArgument
	}
	if len(steps) == 0 {
		return ErrInvalidArgument
	}
	for i := range steps {
		if strings.TrimSpace(steps[i].TaskID) != taskID {
			return ErrInvalidArgument
		}
	}
	return nil
}
