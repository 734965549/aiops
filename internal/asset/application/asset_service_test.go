package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/asset/domain"
)

type fakeAppRepo struct {
	apps map[string]domain.Application
}

func (r *fakeAppRepo) Create(_ context.Context, app *domain.Application) error {
	r.apps[app.Name] = *app
	return nil
}

func (r *fakeAppRepo) List(_ context.Context) ([]domain.Application, error) { return nil, nil }

func (r *fakeAppRepo) Count(_ context.Context) (int64, error) { return int64(len(r.apps)), nil }

func (r *fakeAppRepo) FindByNameEnv(_ context.Context, name, environment string) (*domain.Application, error) {
	app, ok := r.apps[name]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if environment != "" && app.Environment != "" && app.Environment != environment {
		return nil, domain.ErrNotFound
	}
	cp := app
	return &cp, nil
}

func (r *fakeAppRepo) ExistsByID(_ context.Context, id string) (bool, error) {
	for _, app := range r.apps {
		if app.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeAppRepo) GetByID(_ context.Context, id string) (*domain.Application, error) {
	for _, app := range r.apps {
		if app.ID == id {
			cp := app
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeAppRepo) Update(_ context.Context, app *domain.Application) error {
	for name, row := range r.apps {
		if row.ID == app.ID {
			r.apps[name] = *app
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeAppRepo) Delete(_ context.Context, id string) error {
	for name, app := range r.apps {
		if app.ID == id {
			delete(r.apps, name)
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeResRepo struct {
	rows []domain.Resource
}

func (r *fakeResRepo) Create(_ context.Context, res *domain.Resource) error {
	r.rows = append(r.rows, *res)
	return nil
}

func (r *fakeResRepo) Count(_ context.Context) (int64, error) { return int64(len(r.rows)), nil }

func (r *fakeResRepo) ListByApplicationID(_ context.Context, applicationID string) ([]domain.Resource, error) {
	out := make([]domain.Resource, 0)
	for _, row := range r.rows {
		if row.ApplicationID == applicationID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeResRepo) FindBestMatch(_ context.Context, q domain.ResourceMatchQuery) (*domain.Resource, error) {
	for i := range r.rows {
		row := r.rows[i]
		if row.ApplicationID != q.ApplicationID {
			continue
		}
		if q.Pod != "" && row.Pod == q.Pod {
			cp := row
			return &cp, nil
		}
		if q.Instance != "" && row.Instance == q.Instance {
			cp := row
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeResRepo) GetByID(_ context.Context, id string) (*domain.Resource, error) {
	for i := range r.rows {
		if r.rows[i].ID == id {
			cp := r.rows[i]
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeResRepo) Update(_ context.Context, res *domain.Resource) error {
	for i := range r.rows {
		if r.rows[i].ID == res.ID {
			r.rows[i] = *res
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeResRepo) Delete(_ context.Context, id string) error {
	for i := range r.rows {
		if r.rows[i].ID == id {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeResRepo) CountByApplicationID(_ context.Context, applicationID string) (int64, error) {
	var n int64
	for _, row := range r.rows {
		if row.ApplicationID == applicationID {
			n++
		}
	}
	return n, nil
}

type fakeRuleRepo struct {
	rules []domain.MatchRule
}

func (r *fakeRuleRepo) Create(_ context.Context, rule *domain.MatchRule) error {
	r.rules = append(r.rules, *rule)
	return nil
}

func (r *fakeRuleRepo) List(_ context.Context) ([]domain.MatchRule, error) { return r.rules, nil }

func (r *fakeRuleRepo) ListEnabledByPriority(_ context.Context) ([]domain.MatchRule, error) {
	out := make([]domain.MatchRule, 0)
	for _, rule := range r.rules {
		if rule.Enabled {
			out = append(out, rule)
		}
	}
	return out, nil
}

func (r *fakeRuleRepo) GetByID(_ context.Context, id string) (*domain.MatchRule, error) {
	for i := range r.rules {
		if r.rules[i].ID == id {
			cp := r.rules[i]
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeRuleRepo) Update(_ context.Context, rule *domain.MatchRule) error {
	for i := range r.rules {
		if r.rules[i].ID == rule.ID {
			r.rules[i] = *rule
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeRuleRepo) Delete(_ context.Context, id string) error {
	for i := range r.rules {
		if r.rules[i].ID == id {
			r.rules = append(r.rules[:i], r.rules[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeRuleRepo) CountByApplicationID(_ context.Context, applicationID string) (int64, error) {
	var n int64
	for _, rule := range r.rules {
		if rule.ApplicationID == applicationID {
			n++
		}
	}
	return n, nil
}

func (r *fakeRuleRepo) CountByResourceID(_ context.Context, resourceID string) (int64, error) {
	var n int64
	for _, rule := range r.rules {
		if rule.ResourceID == resourceID {
			n++
		}
	}
	return n, nil
}

func TestMatcherService_MatchApplicationAndPod(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service", Environment: "prod"},
	}}
	resources := &fakeResRepo{rows: []domain.Resource{
		{ID: "res-1", ApplicationID: "app-1", Pod: "payment-xxx-1", Namespace: "payment"},
	}}
	svc := NewMatcherService(apps, resources, nil)
	out, err := svc.Match(context.Background(), MatchInput{
		ApplicationName: "payment-service",
		Environment:     "prod",
		Labels:          map[string]string{"pod": "payment-xxx-1", "namespace": "payment"},
	})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "app-1" || out.ResourceID != "res-1" {
		t.Fatalf("unexpected match result: %+v", out)
	}
}

func TestMatcherService_MatchByServiceLabel(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"checkout-service": {ID: "app-2", Name: "checkout-service", Environment: "prod"},
	}}
	svc := NewMatcherService(apps, &fakeResRepo{}, nil)
	out, err := svc.Match(context.Background(), MatchInput{
		Environment: "prod",
		Labels:      map[string]string{"service": "checkout-service"},
	})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "app-2" || out.ResourceID != "" {
		t.Fatalf("unexpected match result: %+v", out)
	}
}

func TestMatcherService_MatchInstanceWhenNoPod(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"metrics-app": {ID: "app-3", Name: "metrics-app", Environment: "prod"},
	}}
	resources := &fakeResRepo{rows: []domain.Resource{
		{ID: "res-2", ApplicationID: "app-3", Instance: "node-1"},
	}}
	svc := NewMatcherService(apps, resources, nil)
	out, err := svc.Match(context.Background(), MatchInput{
		ApplicationName: "metrics-app",
		Environment:     "prod",
		Labels:          map[string]string{"instance": "node-1"},
	})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "app-3" || out.ResourceID != "res-2" {
		t.Fatalf("unexpected match result: %+v", out)
	}
}

func TestMatcherService_NoMatchReturnsEmpty(t *testing.T) {
	svc := NewMatcherService(&fakeAppRepo{apps: map[string]domain.Application{}}, &fakeResRepo{}, nil)
	out, err := svc.Match(context.Background(), MatchInput{ApplicationName: "missing"})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "" || out.ResourceID != "" {
		t.Fatalf("expected empty match, got %+v", out)
	}
}

func TestAssetService_CreateResourceApplicationNotFound(t *testing.T) {
	svc := NewAssetService(&fakeAppRepo{apps: map[string]domain.Application{}}, &fakeResRepo{}, nil, NoopAuditRecorder{})
	_, err := svc.CreateResource(context.Background(), Actor{}, CreateResourceInput{
		ApplicationID: "missing-app-id",
		Name:          "pod-1",
		ResourceType:  "pod",
	})
	if err == nil {
		t.Fatal("expected error for missing application")
	}
}
