package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
)

// fakeMemoryRepo 是内存版 MemoryCaseRepository，按用户过滤以验证跨用户隔离。
type fakeMemoryRepo struct {
	cases       []domain.MemoryCase
	searchErr   error
	listErr     error
	createErr   error
	updateErr   error
	searchCalls int
	lastFilter  domain.MemoryCaseFilter
}

func (f *fakeMemoryRepo) Create(_ context.Context, c domain.MemoryCase) (bool, error) {
	if f.createErr != nil {
		return false, f.createErr
	}
	for _, existing := range f.cases {
		if existing.UserID == c.UserID && existing.SourceMessageID != "" && existing.SourceMessageID == c.SourceMessageID {
			return false, nil
		}
	}
	f.cases = append(f.cases, c)
	return true, nil
}

func (f *fakeMemoryRepo) FindBySourceMessage(_ context.Context, userID, sourceMessageID string) (*domain.MemoryCase, error) {
	for i := range f.cases {
		if f.cases[i].UserID == userID && f.cases[i].SourceMessageID == sourceMessageID {
			copied := f.cases[i]
			return &copied, nil
		}
	}
	return nil, nil
}

func (f *fakeMemoryRepo) UpdateQuality(_ context.Context, c domain.MemoryCase) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	for i := range f.cases {
		if f.cases[i].CaseID == c.CaseID && f.cases[i].UserID == c.UserID {
			f.cases[i].QualityScore = c.QualityScore
			f.cases[i].Status = c.Status
			f.cases[i].UpdatedAt = c.UpdatedAt
			return nil
		}
	}
	return errors.New("not found")
}

// ListForAggregation 返回该用户在某应用下 status=active 的案例（聚合来源）。
func (f *fakeMemoryRepo) ListForAggregation(_ context.Context, userID, applicationID string, limit int) ([]domain.MemoryCase, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	out := make([]domain.MemoryCase, 0, len(f.cases))
	for _, c := range f.cases {
		if c.UserID != userID || c.Status != domain.MemoryCaseStatusActive || c.ApplicationID != applicationID {
			continue
		}
		out = append(out, c)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeMemoryRepo) Search(_ context.Context, filter domain.MemoryCaseFilter) ([]domain.MemoryCase, error) {
	f.searchCalls++
	f.lastFilter = filter
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	out := make([]domain.MemoryCase, 0, len(f.cases))
	for _, c := range f.cases {
		if c.UserID != filter.UserID || c.Status != domain.MemoryCaseStatusActive {
			continue
		}
		if filter.MatchPriority(c) > 0 {
			out = append(out, c)
		}
	}
	if len(out) > filter.Limit && filter.Limit > 0 {
		out = out[:filter.Limit]
	}
	return out, nil
}

// List 是管理端列表：按用户/状态/关键词过滤，返回所有匹配条目。
func (f *fakeMemoryRepo) List(_ context.Context, filter domain.MemoryCaseListFilter) ([]domain.MemoryCase, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]domain.MemoryCase, 0, len(f.cases))
	for _, c := range f.cases {
		if filter.UserID != "" && c.UserID != filter.UserID {
			continue
		}
		if filter.ApplicationID != "" && c.ApplicationID != filter.ApplicationID {
			continue
		}
		if filter.ResourceID != "" && c.ResourceID != filter.ResourceID {
			continue
		}
		if filter.Status != nil && c.Status != *filter.Status {
			continue
		}
		if filter.Keyword != "" && !strings.Contains(strings.ToLower(c.Summary+" "+c.Keywords), strings.ToLower(filter.Keyword)) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeMemoryRepo) Get(_ context.Context, caseID string) (*domain.MemoryCase, error) {
	for i := range f.cases {
		if f.cases[i].CaseID == caseID {
			copied := f.cases[i]
			return &copied, nil
		}
	}
	return nil, domain.ErrMemoryCaseNotFound
}

func (f *fakeMemoryRepo) UpdateStatus(_ context.Context, caseID string, status domain.MemoryCaseStatus, updatedAt time.Time) error {
	for i := range f.cases {
		if f.cases[i].CaseID == caseID {
			f.cases[i].Status = status
			f.cases[i].UpdatedAt = updatedAt
			return nil
		}
	}
	return domain.ErrMemoryCaseNotFound
}

func memoryTestSession() *domain.Session {
	return &domain.Session{
		SessionID: "s-1", UserID: "u-1", Status: domain.SessionStatusActive,
		Context: domain.SessionContext{
			Page: "alerts", AlertID: "a1", ApplicationID: "app-1",
			ResourceID: "i-1", ResourceType: "ecs",
		},
	}
}

func memoryTestAssistantMessage(id, summary string) *domain.Message {
	return &domain.Message{
		MessageID: id, SessionID: "s-1", Role: domain.RoleAssistant,
		Content: chatAnswerJSON(summary),
	}
}

func TestMemoryServiceRecordsValidFeedback(t *testing.T) {
	repo := &fakeMemoryRepo{}
	svc := NewMemoryService(repo, &fakeAnalyzeAlertReader{alert: &AlertContext{
		ID: "a1", Name: "HighCPU", Labels: map[string]string{"metric_name": "cpu_util", "instance_id": "i-1"},
	}})
	svc.now = func() time.Time { return time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC) }

	if err := svc.RecordValidFeedback(context.Background(), memoryTestSession(), memoryTestAssistantMessage("m-1", "CPU 持续升高")); err != nil {
		t.Fatalf("record valid feedback: %v", err)
	}
	if len(repo.cases) != 1 {
		t.Fatalf("expected one case, got %+v", repo.cases)
	}
	c := repo.cases[0]
	if c.UserID != "u-1" || c.SourceMessageID != "m-1" || c.SessionID != "s-1" {
		t.Fatalf("unexpected case identity: %+v", c)
	}
	if c.ApplicationID != "app-1" || c.ResourceID != "i-1" || c.ResourceType != "ecs" {
		t.Fatalf("context not propagated: %+v", c)
	}
	if c.QualityScore != domain.MemoryCaseValidDelta || c.Status != domain.MemoryCaseStatusActive {
		t.Fatalf("unexpected initial quality: %+v", c)
	}
	if c.AlertSignature == "" || c.Keywords == "" {
		t.Fatalf("signature/keywords missing: %+v", c)
	}
	if len(c.Recommendations) == 0 || len(c.Recommendations) > domain.MemoryCaseMaxRecommendations {
		t.Fatalf("recommendations = %+v", c.Recommendations)
	}
	if c.Summary != "CPU 持续升高" {
		t.Fatalf("summary = %q", c.Summary)
	}

	// 幂等：同一条消息重复反馈不重复沉淀。
	if err := svc.RecordValidFeedback(context.Background(), memoryTestSession(), memoryTestAssistantMessage("m-1", "CPU 持续升高")); err != nil {
		t.Fatalf("repeat record: %v", err)
	}
	if len(repo.cases) != 1 {
		t.Fatalf("duplicate case created: %d", len(repo.cases))
	}
}

func TestMemoryServiceSkipsDegradedAnswer(t *testing.T) {
	repo := &fakeMemoryRepo{}
	svc := NewMemoryService(repo, nil)
	message := &domain.Message{
		MessageID: "m-1", Role: domain.RoleAssistant,
		Content: `{"summary":"降级","degraded":true,"degraded_reason":"model_failed","risk_level":"info","recommendations":[]}`,
	}
	if err := svc.RecordValidFeedback(context.Background(), memoryTestSession(), message); err != nil {
		t.Fatalf("record: %v", err)
	}
	if len(repo.cases) != 0 {
		t.Fatalf("degraded answer must not be remembered: %+v", repo.cases)
	}
}

func TestMemoryServiceInvalidFeedbackRetiresCase(t *testing.T) {
	repo := &fakeMemoryRepo{}
	svc := NewMemoryService(repo, nil)
	session := memoryTestSession()
	message := memoryTestAssistantMessage("m-1", "CPU 持续升高")
	if err := svc.RecordValidFeedback(context.Background(), session, message); err != nil {
		t.Fatalf("record valid: %v", err)
	}
	if err := svc.RecordInvalidFeedback(context.Background(), session, message); err != nil {
		t.Fatalf("record invalid: %v", err)
	}
	if repo.cases[0].QualityScore != domain.MemoryCaseValidDelta+domain.MemoryCaseInvalidDelta {
		t.Fatalf("quality after one invalid = %d", repo.cases[0].QualityScore)
	}
	if repo.cases[0].Status != domain.MemoryCaseStatusActive {
		t.Fatalf("status after one invalid = %s", repo.cases[0].Status)
	}
	if err := svc.RecordInvalidFeedback(context.Background(), session, message); err != nil {
		t.Fatalf("record invalid again: %v", err)
	}
	if repo.cases[0].Status != domain.MemoryCaseStatusRetired {
		t.Fatalf("expected retired after second invalid, got %s", repo.cases[0].Status)
	}
	// retired 之后不再被检索到。
	result := svc.SearchSimilar(context.Background(), MemorySearchInput{UserID: "u-1", Context: session.Context})
	if len(result.Cases) != 0 || result.Status != MemoryStatusEmpty {
		t.Fatalf("retired case must not match: %+v", result)
	}
}

func TestMemoryServiceSearchIsolatesUsers(t *testing.T) {
	repo := &fakeMemoryRepo{}
	svc := NewMemoryService(repo, nil)
	if err := svc.RecordValidFeedback(context.Background(), memoryTestSession(), memoryTestAssistantMessage("m-1", "A 用户的案例")); err != nil {
		t.Fatalf("record: %v", err)
	}
	// B 用户即使上下文完全相同也检索不到 A 的案例。
	result := svc.SearchSimilar(context.Background(), MemorySearchInput{
		UserID: "u-2",
		Context: domain.SessionContext{
			ApplicationID: "app-1", ResourceID: "i-1",
		},
	})
	if len(result.Cases) != 0 || result.Status != MemoryStatusEmpty {
		t.Fatalf("cross-user memory leak: %+v", result)
	}
}

func TestMemoryServiceSearchBuildsFilter(t *testing.T) {
	repo := &fakeMemoryRepo{}
	svc := NewMemoryService(repo, nil)
	alert := &AlertContext{
		ID: "a1", Name: "HighCPU", ApplicationID: "app-9", ResourceID: "i-9",
		Labels: map[string]string{"metric_name": "cpu_util", "instance_id": "i-9"},
	}
	evidence := &MetricEvidenceResult{
		Status:  EvidenceStatusOK,
		Metrics: []MetricEvidence{{Metric: "cpu_util", Labels: map[string]string{"instance_id": "i-9"}}},
	}
	_ = svc.SearchSimilar(context.Background(), MemorySearchInput{
		UserID: "u-1", Alert: alert, Evidence: evidence,
		Context:  domain.SessionContext{Page: "alerts"},
		Keywords: "为什么 CPU 高",
	})
	filter := repo.lastFilter
	if filter.UserID != "u-1" {
		t.Fatalf("user filter = %q", filter.UserID)
	}
	if filter.AlertSignature == "" || filter.ApplicationID != "app-9" || filter.ResourceID != "i-9" {
		t.Fatalf("unexpected filter: %+v", filter)
	}
	if filter.Keywords == "" || filter.Limit != domain.MemoryCaseMaxMatches {
		t.Fatalf("unexpected keywords/limit: %+v", filter)
	}
}

func TestMemoryServiceSearchFailureDegrades(t *testing.T) {
	repo := &fakeMemoryRepo{searchErr: errors.New("db down")}
	svc := NewMemoryService(repo, nil)
	result := svc.SearchSimilar(context.Background(), MemorySearchInput{
		UserID: "u-1", Context: domain.SessionContext{ApplicationID: "app-1"},
	})
	if result.Status != MemoryStatusError || len(result.Cases) != 0 {
		t.Fatalf("expected error status with empty cases, got %+v", result)
	}
}

func TestMemoryServiceDisabled(t *testing.T) {
	svc := NewMemoryService(nil, nil)
	if result := svc.SearchSimilar(context.Background(), MemorySearchInput{UserID: "u-1"}); result.Status != MemoryStatusSkipped {
		t.Fatalf("status = %q", result.Status)
	}
	if err := svc.RecordValidFeedback(context.Background(), memoryTestSession(), memoryTestAssistantMessage("m-1", "x")); err != nil {
		t.Fatalf("disabled service must be a no-op: %v", err)
	}
}
