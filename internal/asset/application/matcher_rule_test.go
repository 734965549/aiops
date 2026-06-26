package application

import (
	"context"
	"sort"
	"testing"

	"github.com/734965549/aiops/internal/asset/domain"
)

type fakeMatchRuleRepo struct {
	rows []domain.MatchRule
}

func (f *fakeMatchRuleRepo) Create(_ context.Context, rule *domain.MatchRule) error {
	f.rows = append(f.rows, *rule)
	return nil
}

func (f *fakeMatchRuleRepo) List(_ context.Context) ([]domain.MatchRule, error) {
	return append([]domain.MatchRule(nil), f.rows...), nil
}

func (f *fakeMatchRuleRepo) ListPaged(_ context.Context, filter domain.MatchRuleFilter) ([]domain.MatchRule, int64, error) {
	total := int64(len(f.rows))
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	out := make([]domain.MatchRule, 0)
	for i := offset; i < len(f.rows) && len(out) < limit; i++ {
		out = append(out, f.rows[i])
	}
	return out, total, nil
}

func (f *fakeMatchRuleRepo) ListEnabledByPriority(_ context.Context) ([]domain.MatchRule, error) {
	out := make([]domain.MatchRule, 0)
	for _, row := range f.rows {
		if row.Enabled {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (f *fakeMatchRuleRepo) GetByID(_ context.Context, id string) (*domain.MatchRule, error) {
	for i := range f.rows {
		if f.rows[i].ID == id {
			cp := f.rows[i]
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeMatchRuleRepo) Update(_ context.Context, rule *domain.MatchRule) error {
	for i := range f.rows {
		if f.rows[i].ID == rule.ID {
			f.rows[i] = *rule
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeMatchRuleRepo) Delete(_ context.Context, id string) error {
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeMatchRuleRepo) CountByApplicationID(_ context.Context, applicationID string) (int64, error) {
	var n int64
	for _, row := range f.rows {
		if row.ApplicationID == applicationID {
			n++
		}
	}
	return n, nil
}

func (f *fakeMatchRuleRepo) CountByResourceID(_ context.Context, resourceID string) (int64, error) {
	var n int64
	for _, row := range f.rows {
		if row.ResourceID == resourceID {
			n++
		}
	}
	return n, nil
}

func TestMatcherService_HigherPriorityRuleWins(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service":  {ID: "app-default", Name: "payment-service", Environment: "prod"},
		"checkout-service": {ID: "app-rule", Name: "checkout-service", Environment: "prod"},
	}}
	rules := &fakeMatchRuleRepo{rows: []domain.MatchRule{
		{
			ID: "rule-low", Name: "low", Enabled: true, Priority: 10,
			TargetType: domain.TargetApplication, SourceType: domain.MatchSourceAll,
			LabelKey: "service", LabelValuePattern: "payment-*",
			ApplicationID: "app-low",
		},
		{
			ID: "rule-high", Name: "high", Enabled: true, Priority: 100,
			TargetType: domain.TargetApplication, SourceType: domain.MatchSourceAll,
			LabelKey: "service", LabelValuePattern: "payment-*",
			ApplicationID: "app-high",
		},
	}}
	svc := NewMatcherService(apps, &fakeResRepo{}, rules)
	out, err := svc.Match(context.Background(), MatchInput{
		SourceType: "prometheus_alertmanager",
		Labels:     map[string]string{"service": "payment-api"},
	})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "app-high" {
		t.Fatalf("expected high priority rule app-high, got %s", out.ApplicationID)
	}
}

func TestMatcherService_DisabledRuleSkipped(t *testing.T) {
	rules := &fakeMatchRuleRepo{rows: []domain.MatchRule{
		{
			ID: "rule-off", Name: "off", Enabled: false, Priority: 100,
			TargetType: domain.TargetApplication, SourceType: domain.MatchSourceAll,
			LabelKey: "service", LabelValuePattern: "payment-*",
			ApplicationID: "app-rule",
		},
	}}
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-default", Name: "payment-service", Environment: "prod"},
	}}
	svc := NewMatcherService(apps, &fakeResRepo{}, rules)
	out, err := svc.Match(context.Background(), MatchInput{
		Labels:      map[string]string{"service": "payment-service", "env": "prod"},
		Environment: "prod",
	})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "app-default" {
		t.Fatalf("expected default match app-default, got %s", out.ApplicationID)
	}
}

func TestMatcherService_FallbackDefaultWhenNoRuleHit(t *testing.T) {
	rules := &fakeMatchRuleRepo{rows: []domain.MatchRule{
		{
			ID: "rule-miss", Name: "miss", Enabled: true, Priority: 50,
			TargetType: domain.TargetApplication, SourceType: domain.MatchSourceAll,
			LabelKey: "service", LabelValuePattern: "checkout-*",
			ApplicationID: "app-other",
		},
	}}
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-default", Name: "payment-service", Environment: "prod"},
	}}
	resources := &fakeResRepo{rows: []domain.Resource{
		{ID: "res-1", ApplicationID: "app-default", Pod: "payment-xxx-1"},
	}}
	svc := NewMatcherService(apps, resources, rules)
	out, err := svc.Match(context.Background(), MatchInput{
		ApplicationName: "payment-service",
		Environment:     "prod",
		Labels:          map[string]string{"service": "payment-service", "pod": "payment-xxx-1"},
	})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "app-default" || out.ResourceID != "res-1" {
		t.Fatalf("expected default match, got %+v", out)
	}
}

func TestMatcherService_ResourceTargetRule(t *testing.T) {
	rules := &fakeMatchRuleRepo{rows: []domain.MatchRule{
		{
			ID: "rule-res", Name: "res", Enabled: true, Priority: 100,
			TargetType: domain.TargetResource, SourceType: domain.MatchSourceAll,
			LabelKey: "service", LabelValuePattern: "payment-*",
			ApplicationID: "app-1", ResourceID: "res-99",
		},
	}}
	svc := NewMatcherService(&fakeAppRepo{apps: map[string]domain.Application{}}, &fakeResRepo{}, rules)
	out, err := svc.Match(context.Background(), MatchInput{
		Labels: map[string]string{"service": "payment-api"},
	})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "app-1" || out.ResourceID != "res-99" {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestMatchLabelPattern(t *testing.T) {
	cases := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"payment-*", "payment-api", true},
		{"payment-*", "checkout-api", false},
		{"*", "anything", true},
		{"exact", "exact", true},
	}
	for _, tc := range cases {
		if got := matchLabelPattern(tc.pattern, tc.value); got != tc.want {
			t.Fatalf("matchLabelPattern(%q,%q)=%v want %v", tc.pattern, tc.value, got, tc.want)
		}
	}
}
