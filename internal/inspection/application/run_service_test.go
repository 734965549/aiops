package application

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/inspection/domain"
)

type testPolicyRepo struct {
	policy *domain.InspectionPolicy
}

func (r *testPolicyRepo) Create(context.Context, *domain.InspectionPolicy) error { return nil }
func (r *testPolicyRepo) Update(context.Context, *domain.InspectionPolicy) error { return nil }
func (r *testPolicyRepo) GetByID(context.Context, string) (*domain.InspectionPolicy, error) {
	if r.policy == nil {
		return nil, domain.ErrNotFound
	}
	return r.policy, nil
}
func (r *testPolicyRepo) List(context.Context, domain.PolicyFilter) ([]domain.InspectionPolicy, error) {
	return nil, nil
}
func (r *testPolicyRepo) Count(context.Context, domain.PolicyFilter) (int64, error) { return 0, nil }
func (r *testPolicyRepo) SoftDelete(context.Context, string) error                  { return nil }

type testRunRepo struct {
	items     map[string]*domain.InspectionRun
	updateErr error
	updates   int
}

func (r *testRunRepo) Create(_ context.Context, run *domain.InspectionRun) error {
	if r.items == nil {
		r.items = map[string]*domain.InspectionRun{}
	}
	r.items[run.RunID] = cloneTestRun(run)
	return nil
}
func (r *testRunRepo) Update(_ context.Context, run *domain.InspectionRun) error {
	r.updates++
	if r.updateErr != nil {
		return r.updateErr
	}
	if r.items == nil {
		r.items = map[string]*domain.InspectionRun{}
	}
	r.items[run.RunID] = cloneTestRun(run)
	return nil
}
func (r *testRunRepo) GetByID(_ context.Context, runID string) (*domain.InspectionRun, error) {
	if run, ok := r.items[runID]; ok {
		return cloneTestRun(run), nil
	}
	return nil, domain.ErrNotFound
}
func (r *testRunRepo) List(context.Context, domain.RunFilter) ([]domain.InspectionRun, error) {
	return nil, nil
}
func (r *testRunRepo) Count(context.Context, domain.RunFilter) (int64, error) { return 0, nil }

type testFindingRepo struct {
	created []domain.InspectionFinding
	err     error
}

func (r *testFindingRepo) CreateBatch(_ context.Context, findings []domain.InspectionFinding) error {
	if r.err != nil {
		return r.err
	}
	r.created = append(r.created, findings...)
	return nil
}
func (r *testFindingRepo) List(context.Context, domain.FindingFilter) ([]domain.InspectionFinding, error) {
	return nil, nil
}
func (r *testFindingRepo) Count(context.Context, domain.FindingFilter) (int64, error) {
	return 0, nil
}
func (r *testFindingRepo) GetByID(context.Context, string) (*domain.InspectionFinding, error) {
	return nil, domain.ErrNotFound
}

type testRecRepo struct {
	created []domain.Recommendation
	err     error
}

func (r *testRecRepo) CreateBatch(_ context.Context, recs []domain.Recommendation) error {
	if r.err != nil {
		return r.err
	}
	r.created = append(r.created, recs...)
	return nil
}
func (r *testRecRepo) ListByRunID(context.Context, string) ([]domain.Recommendation, error) {
	return nil, nil
}
func (r *testRecRepo) ListByFindingID(context.Context, string) ([]domain.Recommendation, error) {
	return nil, nil
}
func (r *testRecRepo) GetByID(context.Context, string) (*domain.Recommendation, error) {
	return nil, domain.ErrNotFound
}
func (r *testRecRepo) Update(context.Context, *domain.Recommendation) error { return nil }

type testAnalyzer struct{}

func (testAnalyzer) CollectEvidence(context.Context, Actor, CheckEvidenceInput) (*EvidenceSummary, error) {
	return &EvidenceSummary{Check: "metrics.cpu", Type: "metrics", EvidenceID: "ev-1", Metric: "cpu_util", MaxValue: 90}, nil
}
func (testAnalyzer) Analyze(context.Context, []string, []EvidenceSummary) ([]AnalysisResult, error) {
	return []AnalysisResult{{
		Category: "metrics.cpu", RiskLevel: "high", Summary: "cpu high",
		EvidenceRefs: []string{"ev-1"}, Confidence: 0.9,
		Recommendations: []RecommendationDraft{{Title: "collect process snapshot", RiskLevel: "medium"}},
	}}, nil
}

type testArtifactUOW struct {
	called bool
	repos  domain.ArtifactRepositories
}

func (u *testArtifactUOW) WithinArtifactsTransaction(ctx context.Context, fn func(context.Context, domain.ArtifactRepositories) error) error {
	u.called = true
	return fn(ctx, u.repos)
}

func TestRunService_TriggerRunReturnsErrorWhenStartUpdateFails(t *testing.T) {
	policyRepo := &testPolicyRepo{policy: enabledTestPolicy()}
	runRepo := &testRunRepo{updateErr: errors.New("db unavailable")}
	svc := NewRunService(policyRepo, runRepo, &testFindingRepo{}, &testRecRepo{}, testAnalyzer{}, nil, nil)

	_, err := svc.TriggerRun(context.Background(), Actor{UserID: "user-1"}, "pol-1", domain.TriggerManual)
	if err == nil {
		t.Fatal("expected execution error")
	}
}

func TestRunService_PersistsArtifactsAndFinalRunInUnitOfWork(t *testing.T) {
	policyRepo := &testPolicyRepo{policy: enabledTestPolicy()}
	runRepo := &testRunRepo{}
	findingRepo := &testFindingRepo{}
	recRepo := &testRecRepo{}
	uow := &testArtifactUOW{repos: domain.ArtifactRepositories{
		Runs: runRepo, Findings: findingRepo, Recommendations: recRepo,
	}}
	svc := NewRunService(policyRepo, runRepo, findingRepo, recRepo, testAnalyzer{}, nil, uow)

	dto, err := svc.TriggerRun(context.Background(), Actor{UserID: "user-1"}, "pol-1", domain.TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if !uow.called {
		t.Fatal("expected artifact unit of work to be used")
	}
	if dto.Status != string(domain.RunStatusSuccess) {
		t.Fatalf("expected success run, got %s", dto.Status)
	}
	if len(findingRepo.created) != 1 || len(recRepo.created) != 1 {
		t.Fatalf("expected finding and recommendation, got findings=%d recs=%d", len(findingRepo.created), len(recRepo.created))
	}
}

func enabledTestPolicy() *domain.InspectionPolicy {
	return &domain.InspectionPolicy{
		PolicyID: "pol-1",
		Name:     "policy",
		Enabled:  true,
		Scope:    domain.PolicyScope{AccountID: "acc-1", Provider: "fake"},
		Checks:   []string{"metrics.cpu"},
	}
}

func cloneTestRun(run *domain.InspectionRun) *domain.InspectionRun {
	cp := *run
	cp.Timeline = append([]domain.TimelineEvent(nil), run.Timeline...)
	return &cp
}
