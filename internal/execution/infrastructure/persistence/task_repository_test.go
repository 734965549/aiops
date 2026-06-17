package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/execution/domain"
	"github.com/google/uuid"
)

func TestTaskRepository_IntegrationCreateWithStepsAndSnapshot(t *testing.T) {
	db := openTestPostgres(t)
	taskRepo := NewTaskRepository(db)
	stepRepo := NewStepRepository(db)
	ctx := context.Background()

	taskID := uuid.NewString()
	stepID := uuid.NewString()
	t.Cleanup(func() { deleteExecTasks(t, db, taskID) })

	now := time.Now().UTC().Truncate(time.Microsecond)
	snapshot := map[string]any{
		"template_id": "tpl-1",
		"name":        "重启并验证",
		"steps": []any{
			map[string]any{"step_id": stepID, "name": "预检"},
		},
	}
	task := &domain.Task{
		ID: taskID, Name: "runbook task", SourceType: domain.SourceAlert, SourceID: "a1",
		OperationType: domain.OpRunbook, RiskLevel: domain.RiskMedium, Status: domain.StatusPendingConfirm,
		Environment: "prod", RunbookTemplateID: "tpl-1", RunbookSnapshot: snapshot, DryRun: true,
		CreatedBy: "u1", CreatedAt: now, UpdatedAt: now,
	}
	steps := []domain.Step{{
		ID: stepID, TaskID: taskID, StepOrder: 1, Name: "预检", ActionType: "command",
		Status: domain.StepPending, RiskLevel: domain.RiskLow, DryRun: true,
		RunbookStepID: "rb-step-1", Parameters: map[string]any{"cmd": "echo ok"},
	}}

	if err := taskRepo.CreateWithSteps(ctx, task, steps); err != nil {
		t.Fatalf("create with steps: %v", err)
	}

	got, err := taskRepo.GetByID(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RunbookTemplateID != "tpl-1" || !got.DryRun {
		t.Fatalf("runbook metadata missing: %+v", got)
	}
	if got.RunbookSnapshot["name"] != "重启并验证" {
		t.Fatalf("snapshot not persisted: %+v", got.RunbookSnapshot)
	}

	gotSteps, err := stepRepo.ListByTaskID(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotSteps) != 1 || gotSteps[0].RunbookStepID != "rb-step-1" {
		t.Fatalf("unexpected steps: %+v", gotSteps)
	}
}

func TestTaskRepository_IntegrationUpdateStatusIf(t *testing.T) {
	db := openTestPostgres(t)
	repo := NewTaskRepository(db)
	ctx := context.Background()

	taskID := uuid.NewString()
	t.Cleanup(func() { deleteExecTasks(t, db, taskID) })

	now := time.Now().UTC().Truncate(time.Microsecond)
	task := &domain.Task{
		ID: taskID, Name: "status flow", SourceType: domain.SourceManual,
		OperationType: domain.OpRestart, RiskLevel: domain.RiskMedium,
		Status: domain.StatusPendingConfirm, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, task); err != nil {
		t.Fatalf("create: %v", err)
	}

	confirmedAt := now.Add(time.Minute)
	updated, err := repo.UpdateStatusIf(ctx, taskID, domain.StatusPendingConfirm, domain.StatusPendingExecute, func(t *domain.Task) {
		t.ConfirmedBy = "u1"
		t.ConfirmedAt = &confirmedAt
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.StatusPendingExecute || updated.ConfirmedBy != "u1" {
		t.Fatalf("unexpected updated task: %+v", updated)
	}

	_, err = repo.UpdateStatusIf(ctx, taskID, domain.StatusPendingConfirm, domain.StatusPendingExecute, nil)
	if err == nil || !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}

func TestTaskRepository_IntegrationStepStatusUpdate(t *testing.T) {
	db := openTestPostgres(t)
	taskRepo := NewTaskRepository(db)
	stepRepo := NewStepRepository(db)
	ctx := context.Background()

	taskID := uuid.NewString()
	stepID := uuid.NewString()
	t.Cleanup(func() { deleteExecTasks(t, db, taskID) })

	now := time.Now().UTC().Truncate(time.Microsecond)
	task := &domain.Task{
		ID: taskID, Name: "step update", SourceType: domain.SourceManual,
		OperationType: domain.OpRestart, RiskLevel: domain.RiskMedium,
		Status: domain.StatusRunning, CreatedAt: now, UpdatedAt: now,
	}
	step := &domain.Step{
		ID: stepID, TaskID: taskID, StepOrder: 1, Name: "restart", ActionType: "restart",
		Status: domain.StepRunning, RiskLevel: domain.RiskMedium,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := stepRepo.Create(ctx, step); err != nil {
		t.Fatal(err)
	}

	finished := now.Add(2 * time.Second)
	step.Status = domain.StepSuccess
	step.Output = map[string]any{"dry_run": true, "message": "simulated"}
	step.FinishedAt = &finished
	if err := stepRepo.Update(ctx, step); err != nil {
		t.Fatal(err)
	}

	gotSteps, err := stepRepo.ListByTaskID(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotSteps) != 1 || gotSteps[0].Status != domain.StepSuccess {
		t.Fatalf("expected success step, got %+v", gotSteps)
	}
	if gotSteps[0].Output["dry_run"] != true {
		t.Fatalf("step output not persisted: %+v", gotSteps[0].Output)
	}
}

func TestStepRepository_RejectsOrphanStep(t *testing.T) {
	db := openTestPostgres(t)
	stepRepo := NewStepRepository(db)
	ctx := context.Background()

	err := stepRepo.Create(ctx, &domain.Step{
		ID: uuid.NewString(), TaskID: uuid.NewString(), StepOrder: 1,
		Name: "orphan", ActionType: "restart", Status: domain.StepPending,
	})
	if err == nil || !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for orphan step, got %v", err)
	}
}
