package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	rbdomain "github.com/734965549/aiops/internal/runbook/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeRBTemplateRepo struct {
	mu   sync.Mutex
	byID map[string]*rbdomain.Template
}

func newFakeRBTemplateRepo() *fakeRBTemplateRepo {
	return &fakeRBTemplateRepo{byID: map[string]*rbdomain.Template{}}
}

func (r *fakeRBTemplateRepo) Create(_ context.Context, tpl *rbdomain.Template) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *tpl
	r.byID[tpl.ID] = &cp
	return nil
}

func (r *fakeRBTemplateRepo) CreateWithSteps(ctx context.Context, tpl *rbdomain.Template, steps []rbdomain.Step) error {
	if err := r.Create(ctx, tpl); err != nil {
		return err
	}
	return r.steps().storeSteps(tpl.ID, steps)
}

func (r *fakeRBTemplateRepo) Update(_ context.Context, tpl *rbdomain.Template) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[tpl.ID]; !ok {
		return rbdomain.ErrNotFound
	}
	cp := *tpl
	r.byID[tpl.ID] = &cp
	return nil
}

func (r *fakeRBTemplateRepo) ReplaceWithSteps(ctx context.Context, tpl *rbdomain.Template, steps []rbdomain.Step) error {
	if err := r.Update(ctx, tpl); err != nil {
		return err
	}
	return r.steps().replaceSteps(tpl.ID, steps)
}

func (r *fakeRBTemplateRepo) Delete(_ context.Context, templateID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[templateID]; !ok {
		return rbdomain.ErrNotFound
	}
	delete(r.byID, templateID)
	return nil
}

func (r *fakeRBTemplateRepo) GetByID(_ context.Context, templateID string) (*rbdomain.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[templateID]
	if !ok {
		return nil, rbdomain.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (r *fakeRBTemplateRepo) List(_ context.Context, filter rbdomain.TemplateFilter) ([]rbdomain.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]rbdomain.Template, 0)
	for _, t := range r.byID {
		if filter.Enabled != nil && t.Enabled != *filter.Enabled {
			continue
		}
		out = append(out, *t)
	}
	return out, nil
}

func (r *fakeRBTemplateRepo) Count(_ context.Context, filter rbdomain.TemplateFilter) (int64, error) {
	items, err := r.List(context.Background(), filter)
	return int64(len(items)), err
}

func (r *fakeRBTemplateRepo) ListEnabled(_ context.Context) ([]rbdomain.Template, error) {
	enabled := true
	return r.List(context.Background(), rbdomain.TemplateFilter{Enabled: &enabled})
}

func (r *fakeRBTemplateRepo) steps() *fakeRBStepRepo {
	return fakeRBStepRepoForTemplates[r]
}

var fakeRBStepRepoForTemplates = map[*fakeRBTemplateRepo]*fakeRBStepRepo{}

type fakeRBStepRepo struct {
	mu    sync.Mutex
	byTpl map[string][]rbdomain.Step
}

func newFakeRBStepRepo(templates *fakeRBTemplateRepo) *fakeRBStepRepo {
	r := &fakeRBStepRepo{byTpl: map[string][]rbdomain.Step{}}
	fakeRBStepRepoForTemplates[templates] = r
	return r
}

func (r *fakeRBStepRepo) storeSteps(templateID string, steps []rbdomain.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]rbdomain.Step, len(steps))
	copy(cp, steps)
	r.byTpl[templateID] = cp
	return nil
}

func (r *fakeRBStepRepo) replaceSteps(templateID string, steps []rbdomain.Step) error {
	return r.storeSteps(templateID, steps)
}

func (r *fakeRBStepRepo) Create(_ context.Context, step *rbdomain.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byTpl[step.TemplateID] = append(r.byTpl[step.TemplateID], *step)
	return nil
}

func (r *fakeRBStepRepo) Update(_ context.Context, step *rbdomain.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := r.byTpl[step.TemplateID]
	for i := range rows {
		if rows[i].ID == step.ID {
			rows[i] = *step
			r.byTpl[step.TemplateID] = rows
			return nil
		}
	}
	return rbdomain.ErrNotFound
}

func (r *fakeRBStepRepo) DeleteByTemplateID(_ context.Context, templateID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byTpl, templateID)
	return nil
}

func (r *fakeRBStepRepo) ListByTemplateID(_ context.Context, templateID string) ([]rbdomain.Step, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := r.byTpl[templateID]
	out := make([]rbdomain.Step, len(rows))
	copy(out, rows)
	return out, nil
}

type fakeRBAlertReader struct {
	ctx *AlertContext
	err error
}

func (f *fakeRBAlertReader) GetForExecution(_ context.Context, _ string) (*AlertContext, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ctx, nil
}

func sampleCreateStepInput(order int, name string) CreateStepInput {
	return CreateStepInput{
		StepOrder: order, Name: name, ActionType: "restart", RiskLevel: "medium",
		DryRunSupported: true, TimeoutSeconds: 120,
	}
}

func newTestTemplateService(t *testing.T) (*TemplateService, *fakeRBTemplateRepo, *fakeRBStepRepo) {
	t.Helper()
	templates := newFakeRBTemplateRepo()
	steps := newFakeRBStepRepo(templates)
	svc := NewTemplateService(templates, steps, &fakeRBAlertReader{
		ctx: &AlertContext{
			ID: "a1", Name: "HighCPU", Status: "processing",
			Environment: "prod", ResourceType: "pod",
		},
	}, NoopAuditRecorder{})
	fixed := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }
	return svc, templates, steps
}

func TestTemplateService_CreateUpdateDelete(t *testing.T) {
	svc, templates, _ := newTestTemplateService(t)
	ctx := context.Background()
	actor := Actor{UserID: "u1"}

	created, err := svc.Create(ctx, actor, CreateTemplateInput{
		Name: "Pod 重启预案", Enabled: true, OperationType: "runbook", RiskLevel: "medium",
		MatchAlertName: "HighCPU", MatchResourceType: "pod", MatchEnvironment: "prod",
		Steps: []CreateStepInput{
			sampleCreateStepInput(1, "预检"),
			sampleCreateStepInput(2, "重启"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Template.Name != "Pod 重启预案" || len(created.Steps) != 2 {
		t.Fatalf("unexpected create result: %+v", created)
	}

	updated, err := svc.Update(ctx, created.Template.ID, actor, UpdateTemplateInput{
		Name: "Pod 重启预案 v2", Enabled: true, OperationType: "runbook", RiskLevel: "high",
		MatchAlertName: "HighCPU", Steps: []CreateStepInput{sampleCreateStepInput(1, "重启")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Template.RiskLevel != "high" || len(updated.Steps) != 1 {
		t.Fatalf("unexpected update: %+v", updated)
	}

	if err := svc.Delete(ctx, created.Template.ID, actor); err != nil {
		t.Fatal(err)
	}
	stored, _ := templates.GetByID(ctx, created.Template.ID)
	if stored.Enabled {
		t.Fatal("delete should soft-disable template")
	}
}

func TestTemplateService_CreateRejectsEmptySteps(t *testing.T) {
	svc, _, _ := newTestTemplateService(t)
	_, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateTemplateInput{
		Name: "empty", OperationType: "runbook", RiskLevel: "low", Steps: nil,
	})
	if err == nil || apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestTemplateService_RecommendMatchesAlert(t *testing.T) {
	svc, templates, steps := newTestTemplateService(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)

	match := &rbdomain.Template{
		ID: "tpl-match", Name: "HighCPU Pod", Enabled: true,
		OperationType: rbdomain.OpRunbook, RiskLevel: rbdomain.RiskMedium,
		MatchAlertName: "HighCPU", MatchResourceType: "pod", MatchEnvironment: "prod",
		CreatedAt: now, UpdatedAt: now,
	}
	other := &rbdomain.Template{
		ID: "tpl-other", Name: "Disk Full", Enabled: true,
		OperationType: rbdomain.OpRunbook, RiskLevel: rbdomain.RiskLow,
		MatchAlertName: "DiskFull", CreatedAt: now, UpdatedAt: now,
	}
	_ = templates.Create(ctx, match)
	_ = templates.Create(ctx, other)
	_ = steps.storeSteps("tpl-match", []rbdomain.Step{
		{ID: "s1", TemplateID: "tpl-match", StepOrder: 1, Name: "重启", ActionType: rbdomain.ActionRestart, RiskLevel: rbdomain.RiskMedium, DryRunSupported: true},
	})

	items, err := svc.Recommend(ctx, Actor{UserID: "u1"}, "a1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TemplateID != "tpl-match" {
		t.Fatalf("expected single match, got %+v", items)
	}
	if items[0].MatchedReason == "" || items[0].StepsCount != 1 || !items[0].DryRunSupported {
		t.Fatalf("unexpected recommendation metadata: %+v", items[0])
	}
}

func TestTemplateService_GetForExecution_BuildsSnapshotSource(t *testing.T) {
	svc, templates, steps := newTestTemplateService(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)

	tpl := &rbdomain.Template{
		ID: "tpl-1", Name: "重启并验证", Enabled: true,
		OperationType: rbdomain.OpRunbook, RiskLevel: rbdomain.RiskMedium,
		RollbackPlan: map[string]any{"note": "rollback me"},
		CreatedAt:    now, UpdatedAt: now,
	}
	stepRows := []rbdomain.Step{
		{ID: "st-1", TemplateID: "tpl-1", StepOrder: 1, Name: "预检", ActionType: rbdomain.ActionCommand, RiskLevel: rbdomain.RiskLow, DryRunSupported: true},
		{ID: "st-2", TemplateID: "tpl-1", StepOrder: 2, Name: "重启", ActionType: rbdomain.ActionRestart, RiskLevel: rbdomain.RiskMedium, DryRunSupported: true},
	}
	_ = templates.Create(ctx, tpl)
	_ = steps.storeSteps("tpl-1", stepRows)

	got, err := svc.GetForExecution(ctx, "tpl-1")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := rbdomain.BuildSnapshot(got.Template, got.Steps)
	if snapshot["template_id"] != "tpl-1" || snapshot["name"] != "重启并验证" {
		t.Fatalf("unexpected snapshot header: %+v", snapshot)
	}
	stepItems, ok := snapshot["steps"].([]map[string]any)
	if !ok {
		t.Fatalf("expected steps slice in snapshot, got %T", snapshot["steps"])
	}
	if len(stepItems) != 2 || stepItems[0]["step_id"] != "st-1" {
		t.Fatalf("unexpected snapshot steps: %+v", stepItems)
	}
}

func TestTemplateService_GetForExecution_RejectsDisabled(t *testing.T) {
	svc, templates, _ := newTestTemplateService(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	_ = templates.Create(ctx, &rbdomain.Template{
		ID: "tpl-off", Name: "disabled", Enabled: false,
		OperationType: rbdomain.OpRunbook, RiskLevel: rbdomain.RiskLow,
		CreatedAt: now, UpdatedAt: now,
	})
	_, err := svc.GetForExecution(ctx, "tpl-off")
	if err == nil || apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument for disabled template, got %v", err)
	}
}

func TestTemplateService_RecommendRequiresAlertReader(t *testing.T) {
	templates := newFakeRBTemplateRepo()
	steps := newFakeRBStepRepo(templates)
	svc := NewTemplateService(templates, steps, nil, NoopAuditRecorder{})
	_, err := svc.Recommend(context.Background(), Actor{UserID: "u1"}, "a1")
	if err == nil || apperr.FromError(err).Code != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func TestTemplateService_RecommendPropagatesAlertError(t *testing.T) {
	templates := newFakeRBTemplateRepo()
	steps := newFakeRBStepRepo(templates)
	svc := NewTemplateService(templates, steps, &fakeRBAlertReader{
		err: errors.New("alert missing"),
	}, NoopAuditRecorder{})
	_, err := svc.Recommend(context.Background(), Actor{UserID: "u1"}, "missing")
	if err == nil {
		t.Fatal("expected error from alert reader")
	}
}
