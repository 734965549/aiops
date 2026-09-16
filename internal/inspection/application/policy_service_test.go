package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/inspection/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeApplicationCatalog struct {
	existing map[string]struct{}
}

func (f *fakeApplicationCatalog) ExistsByID(_ context.Context, applicationID string) (bool, error) {
	if f == nil || f.existing == nil {
		return false, nil
	}
	_, ok := f.existing[applicationID]
	return ok, nil
}

func TestPolicyService_CreateRejectsUnknownApplicationID(t *testing.T) {
	svc := NewPolicyService(&memPolicyRepo{items: map[string]*domain.InspectionPolicy{}}, &fakeApplicationCatalog{
		existing: map[string]struct{}{"app-known": {}},
	}, NoopAuditRecorder{})
	_, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreatePolicyInput{
		Name:   "policy-a",
		Checks: []string{domain.CheckMetricsCPU},
		Scope: PolicyScopeDTO{
			AccountID:      "acc-1",
			ApplicationIDs: []string{"app-known", "app-missing"},
		},
	})
	if err == nil || apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestPolicyService_CreateAcceptsKnownApplicationIDs(t *testing.T) {
	repo := &memPolicyRepo{items: map[string]*domain.InspectionPolicy{}}
	svc := NewPolicyService(repo, &fakeApplicationCatalog{
		existing: map[string]struct{}{"app-known": {}},
	}, NoopAuditRecorder{})
	out, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreatePolicyInput{
		Name:   "policy-b",
		Checks: []string{domain.CheckMetricsCPU},
		Scope: PolicyScopeDTO{
			AccountID:      "acc-1",
			ApplicationIDs: []string{"app-known"},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out == nil || out.PolicyID == "" {
		t.Fatal("expected policy dto")
	}
}

type memPolicyRepo struct {
	items map[string]*domain.InspectionPolicy
}

func (m *memPolicyRepo) Create(_ context.Context, p *domain.InspectionPolicy) error {
	m.items[p.PolicyID] = p
	return nil
}
func (m *memPolicyRepo) Update(_ context.Context, p *domain.InspectionPolicy) error {
	m.items[p.PolicyID] = p
	return nil
}
func (m *memPolicyRepo) GetByID(_ context.Context, policyID string) (*domain.InspectionPolicy, error) {
	p, ok := m.items[policyID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return p, nil
}
func (m *memPolicyRepo) List(_ context.Context, _ domain.PolicyFilter) ([]domain.InspectionPolicy, error) {
	return nil, nil
}
func (m *memPolicyRepo) Count(_ context.Context, _ domain.PolicyFilter) (int64, error) { return 0, nil }
func (m *memPolicyRepo) SoftDelete(_ context.Context, policyID string) error {
	delete(m.items, policyID)
	return nil
}
