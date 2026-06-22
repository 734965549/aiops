package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/execution/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeTaskRepo struct {
	mu   sync.Mutex
	byID map[string]*domain.Task
}

func newFakeTaskRepo() *fakeTaskRepo {
	return &fakeTaskRepo{byID: map[string]*domain.Task{}}
}

func (r *fakeTaskRepo) Create(_ context.Context, task *domain.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *task
	r.byID[task.ID] = &cp
	return nil
}

func (r *fakeTaskRepo) Update(_ context.Context, task *domain.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[task.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *task
	r.byID[task.ID] = &cp
	return nil
}

func (r *fakeTaskRepo) UpdateStatusIf(_ context.Context, taskID string, fromStatus, toStatus domain.TaskStatus, mutator func(*domain.Task)) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[taskID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if t.Status != fromStatus {
		return nil, domain.ErrInvalidTransition
	}
	cp := *t
	if mutator != nil {
		mutator(&cp)
	}
	cp.Status = toStatus
	r.byID[taskID] = &cp
	out := cp
	return &out, nil
}

func (r *fakeTaskRepo) GetByID(_ context.Context, taskID string) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[taskID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (r *fakeTaskRepo) List(_ context.Context, _ domain.TaskFilter) ([]domain.Task, error) {
	return nil, nil
}

func (r *fakeTaskRepo) Count(_ context.Context, _ domain.TaskFilter) (int64, error) {
	return 0, nil
}

type fakeStepRepo struct {
	mu      sync.Mutex
	rows    []domain.Step
	listErr error
}

func newFakeStepRepo() *fakeStepRepo {
	return &fakeStepRepo{rows: []domain.Step{}}
}

func (r *fakeStepRepo) Create(_ context.Context, step *domain.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *step
	r.rows = append(r.rows, cp)
	return nil
}

func (r *fakeStepRepo) Update(_ context.Context, step *domain.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rows {
		if r.rows[i].ID == step.ID {
			r.rows[i] = *step
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeStepRepo) ListByTaskID(_ context.Context, taskID string) ([]domain.Step, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := make([]domain.Step, 0)
	for _, s := range r.rows {
		if s.TaskID == taskID {
			out = append(out, s)
		}
	}
	return out, nil
}

type fakeTaskCreator struct {
	tasks *fakeTaskRepo
	steps *fakeStepRepo
}

func (f *fakeTaskCreator) CreateWithSteps(ctx context.Context, task *domain.Task, steps []domain.Step) error {
	if err := domain.ValidateStepsForTask(task, steps); err != nil {
		return err
	}
	if err := f.tasks.Create(ctx, task); err != nil {
		return err
	}
	for i := range steps {
		if err := f.steps.Create(ctx, &steps[i]); err != nil {
			return err
		}
	}
	return nil
}

type fakeAlertReader struct {
	ctx *AlertContext
}

func (f *fakeAlertReader) GetForExecution(_ context.Context, _ string) (*AlertContext, error) {
	return f.ctx, nil
}

type fakeTimeline struct {
	events []string
}

func (f *fakeTimeline) RecordExecutionCreated(_ context.Context, _ string, _ Actor, _ string, _ map[string]any) error {
	f.events = append(f.events, "created")
	return nil
}

func (f *fakeTimeline) RecordExecutionStarted(_ context.Context, _ string, _ Actor, _ string, _ map[string]any) error {
	f.events = append(f.events, "started")
	return nil
}

func (f *fakeTimeline) RecordExecutionFinished(_ context.Context, _ string, _ Actor, _ string, _ map[string]any) error {
	f.events = append(f.events, "finished")
	return nil
}

func TestTaskService_CreateFromAlertAndExecute(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	timeline := &fakeTimeline{}
	taskRepo := newFakeTaskRepo()
	stepRepo := newFakeStepRepo()
	svc := NewTaskService(
		taskRepo,
		stepRepo,
		&fakeTaskCreator{tasks: taskRepo, steps: stepRepo},
		&fakeAlertReader{ctx: &AlertContext{
			ID: "a1", Name: "HighCPU", Status: "processing",
			Environment: "prod", ResourceName: "node-1", ResourceType: "host",
		}},
		timeline,
		NoopAuditRecorder{},
		nil,
		nil, nil,
	)
	svc.now = func() time.Time { return now }

	created, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateTaskInput{
		SourceType: "alert", SourceID: "a1", OperationType: "restart",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != string(domain.StatusPendingConfirm) {
		t.Fatalf("expected pending_confirm, got %s", created.Status)
	}
	if len(timeline.events) != 1 || timeline.events[0] != "created" {
		t.Fatalf("timeline events: %v", timeline.events)
	}

	_, err = svc.Confirm(context.Background(), created.TaskID, Actor{UserID: "u1"}, ConfirmTaskInput{
		Confirm: true, ConfirmText: "CONFIRM",
	})
	if err != nil {
		t.Fatal(err)
	}

	detail, err := svc.Execute(context.Background(), created.TaskID, Actor{UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.Status != string(domain.StatusSuccess) {
		t.Fatalf("expected success, got %s", detail.Task.Status)
	}
	if len(timeline.events) != 3 {
		t.Fatalf("timeline events: %v", timeline.events)
	}
}

func TestTaskService_ExecuteMarksFailedWhenListStepsFails(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	taskRepo := newFakeTaskRepo()
	stepRepo := newFakeStepRepo()
	stepRepo.listErr = errors.New("db unavailable")
	timeline := &fakeTimeline{}
	svc := NewTaskService(
		taskRepo,
		stepRepo,
		&fakeTaskCreator{tasks: taskRepo, steps: stepRepo},
		&fakeAlertReader{ctx: &AlertContext{ID: "a1", Status: "processing"}},
		timeline,
		NoopAuditRecorder{},
		nil,
		nil, nil,
	)
	svc.now = func() time.Time { return now }

	created, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateTaskInput{
		SourceType: "alert", SourceID: "a1", OperationType: "restart", Name: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Confirm(context.Background(), created.TaskID, Actor{UserID: "u1"}, ConfirmTaskInput{
		Confirm: true, ConfirmText: "CONFIRM",
	})
	if err != nil {
		t.Fatal(err)
	}

	detail, err := svc.Execute(context.Background(), created.TaskID, Actor{UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.Status != string(domain.StatusFailed) {
		t.Fatalf("expected failed, got %s", detail.Task.Status)
	}
	if detail.Task.ErrorMessage == "" {
		t.Fatal("expected error message")
	}
	if len(timeline.events) != 3 || timeline.events[2] != "finished" {
		t.Fatalf("expected finished timeline event, got %v", timeline.events)
	}
	stored, _ := taskRepo.GetByID(context.Background(), created.TaskID)
	if stored.Status != domain.StatusFailed {
		t.Fatalf("task stuck in %s", stored.Status)
	}
}

func TestTaskService_ConfirmRejectsDuplicateConfirm(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	taskRepo := newFakeTaskRepo()
	stepRepo := newFakeStepRepo()
	svc := NewTaskService(
		taskRepo,
		stepRepo,
		&fakeTaskCreator{tasks: taskRepo, steps: stepRepo},
		&fakeAlertReader{ctx: &AlertContext{ID: "a1", Status: "processing"}},
		nil,
		NoopAuditRecorder{},
		nil,
		nil, nil,
	)
	svc.now = func() time.Time { return now }

	created, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateTaskInput{
		SourceType: "alert", SourceID: "a1", OperationType: "restart",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Confirm(context.Background(), created.TaskID, Actor{UserID: "u1"}, ConfirmTaskInput{
		Confirm: true, ConfirmText: "CONFIRM",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Confirm(context.Background(), created.TaskID, Actor{UserID: "u1"}, ConfirmTaskInput{
		Confirm: true, ConfirmText: "CONFIRM",
	})
	if err == nil || apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument on duplicate confirm, got %v", err)
	}
}

func TestTaskService_ExecuteFailsWithoutSteps(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	taskRepo := newFakeTaskRepo()
	stepRepo := newFakeStepRepo()
	svc := NewTaskService(taskRepo, stepRepo, &fakeTaskCreator{tasks: taskRepo, steps: stepRepo}, nil, nil, NoopAuditRecorder{}, nil, nil, nil)
	svc.now = func() time.Time { return now }

	taskID := "task-no-steps"
	taskRepo.byID[taskID] = &domain.Task{
		ID: taskID, Name: "orphan", Status: domain.StatusPendingExecute,
		OperationType: domain.OpRestart, RiskLevel: domain.RiskMedium,
	}

	detail, err := svc.Execute(context.Background(), taskID, Actor{UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.Status != string(domain.StatusFailed) {
		t.Fatalf("expected failed, got %s", detail.Task.Status)
	}
	if detail.Task.ErrorMessage == "" {
		t.Fatal("expected error message for missing steps")
	}
}

func TestTaskService_CreateRejectsNonProcessingAlert(t *testing.T) {
	cases := []struct {
		name   string
		status string
	}{
		{name: "closed", status: "closed"},
		{name: "acknowledged", status: "acknowledged"},
		{name: "recovered", status: "recovered"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			taskRepo := newFakeTaskRepo()
			stepRepo := newFakeStepRepo()
			svc := NewTaskService(taskRepo, stepRepo, &fakeTaskCreator{tasks: taskRepo, steps: stepRepo},
				&fakeAlertReader{ctx: &AlertContext{ID: "a1", Status: tc.status}},
				nil, NoopAuditRecorder{}, nil, nil, nil)
			_, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateTaskInput{
				SourceType: "alert", SourceID: "a1", OperationType: "restart", Name: "x",
			})
			if err == nil || apperr.FromError(err).Code != apperr.CodeInvalidArgument {
				t.Fatalf("expected invalid argument, got %v", err)
			}
		})
	}
}

func TestTaskService_ExecuteRejectsDuplicateStart(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	taskRepo := newFakeTaskRepo()
	stepRepo := newFakeStepRepo()
	svc := NewTaskService(
		taskRepo,
		stepRepo,
		&fakeTaskCreator{tasks: taskRepo, steps: stepRepo},
		&fakeAlertReader{ctx: &AlertContext{
			ID: "a1", Name: "HighCPU", Status: "processing",
			Environment: "prod", ResourceName: "node-1", ResourceType: "host",
		}},
		&fakeTimeline{},
		NoopAuditRecorder{},
		nil,
		nil, nil,
	)
	svc.now = func() time.Time { return now }

	created, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateTaskInput{
		SourceType: "alert", SourceID: "a1", OperationType: "restart",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Confirm(context.Background(), created.TaskID, Actor{UserID: "u1"}, ConfirmTaskInput{
		Confirm: true, ConfirmText: "CONFIRM",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Execute(context.Background(), created.TaskID, Actor{UserID: "u1"}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Execute(context.Background(), created.TaskID, Actor{UserID: "u1"})
	if err == nil || apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument on duplicate execute, got %v", err)
	}
}

type fakeRunbookLoader struct {
	tpl *ExecutableRunbook
}

func (f *fakeRunbookLoader) GetForExecution(_ context.Context, _ string) (*ExecutableRunbook, error) {
	return f.tpl, nil
}

func TestTaskService_CreateFromRunbookGeneratesMultipleSteps(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	taskRepo := newFakeTaskRepo()
	stepRepo := newFakeStepRepo()
	timeline := &fakeTimeline{}
	loader := &fakeRunbookLoader{tpl: &ExecutableRunbook{
		TemplateID:    "tpl-1",
		Name:          "重启并验证",
		OperationType: "runbook",
		RiskLevel:     "medium",
		Steps: []ExecutableRunbookStep{
			{StepID: "s1", StepOrder: 1, Name: "预检", ActionType: "command", RiskLevel: "low", DryRunSupported: true},
			{StepID: "s2", StepOrder: 2, Name: "重启", ActionType: "restart", RiskLevel: "medium", DryRunSupported: true},
		},
	}}
	svc := NewTaskService(
		taskRepo, stepRepo, &fakeTaskCreator{tasks: taskRepo, steps: stepRepo},
		&fakeAlertReader{ctx: &AlertContext{ID: "a1", Name: "HighCPU", Status: "processing", Environment: "prod", ResourceType: "pod"}},
		timeline, NoopAuditRecorder{}, loader, nil, nil,
	)
	svc.now = func() time.Time { return now }

	created, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateTaskInput{
		SourceType: "alert", SourceID: "a1", RunbookTemplateID: "tpl-1", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != string(domain.StatusPendingConfirm) {
		t.Fatalf("expected pending_confirm, got %s", created.Status)
	}
	steps, _ := stepRepo.ListByTaskID(context.Background(), created.TaskID)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	task, _ := taskRepo.GetByID(context.Background(), created.TaskID)
	if task.RunbookTemplateID != "tpl-1" || task.RunbookSnapshot == nil || !task.DryRun {
		t.Fatalf("runbook metadata missing: %+v", task)
	}
}
