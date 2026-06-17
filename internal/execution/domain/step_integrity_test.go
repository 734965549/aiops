package domain

import "testing"

func TestValidateStepsForTask_RejectsEmptySteps(t *testing.T) {
	task := &Task{ID: "t1"}
	if err := ValidateStepsForTask(task, nil); err == nil {
		t.Fatal("expected error for empty steps")
	}
}

func TestValidateStepsForTask_RejectsTaskIDMismatch(t *testing.T) {
	task := &Task{ID: "t1"}
	steps := []Step{{TaskID: "t2", StepOrder: 1}}
	if err := ValidateStepsForTask(task, steps); err == nil {
		t.Fatal("expected error for task_id mismatch")
	}
}

func TestValidateStepsForTask_AcceptsValidSteps(t *testing.T) {
	task := &Task{ID: "t1"}
	steps := []Step{{TaskID: "t1", StepOrder: 1}}
	if err := ValidateStepsForTask(task, steps); err != nil {
		t.Fatal(err)
	}
}
