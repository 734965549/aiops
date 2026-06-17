package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeAlertRepo struct {
	mu       sync.Mutex
	byID     map[string]*domain.Alert
	active   map[string]*domain.Alert
	listErr  error
	countErr error
}

func newFakeAlertRepo() *fakeAlertRepo {
	return &fakeAlertRepo{
		byID:   map[string]*domain.Alert{},
		active: map[string]*domain.Alert{},
	}
}

func (r *fakeAlertRepo) key(sourceID, dedupKey string) string { return sourceID + "\x00" + dedupKey }

func (r *fakeAlertRepo) Create(_ context.Context, alert *domain.Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.byID {
		if a.DedupKey == alert.DedupKey && a.LifecycleSeq == alert.LifecycleSeq {
			return domain.ErrAlreadyExists
		}
	}
	if alert.Status != domain.StatusClosed {
		if _, ok := r.active[r.key(alert.SourceID, alert.DedupKey)]; ok {
			return domain.ErrAlreadyExists
		}
	}
	cp := *alert
	r.byID[alert.ID] = &cp
	if alert.Status != domain.StatusClosed {
		r.active[r.key(alert.SourceID, alert.DedupKey)] = &cp
	}
	return nil
}

func (r *fakeAlertRepo) Update(_ context.Context, alert *domain.Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[alert.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *alert
	r.byID[alert.ID] = &cp
	if alert.Status == domain.StatusClosed {
		delete(r.active, r.key(alert.SourceID, alert.DedupKey))
	} else {
		r.active[r.key(alert.SourceID, alert.DedupKey)] = &cp
	}
	return nil
}

func (r *fakeAlertRepo) GetByID(_ context.Context, alertID string) (*domain.Alert, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[alertID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (r *fakeAlertRepo) FindActiveByDedupKey(_ context.Context, sourceID, dedupKey string) (*domain.Alert, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.active[r.key(sourceID, dedupKey)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (r *fakeAlertRepo) MaxLifecycleSeq(_ context.Context, dedupKey string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	max := 0
	for _, a := range r.byID {
		if a.DedupKey == dedupKey && a.LifecycleSeq > max {
			max = a.LifecycleSeq
		}
	}
	return max, nil
}

func (r *fakeAlertRepo) List(_ context.Context, _ domain.AlertFilter) ([]domain.Alert, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return nil, nil
}

func (r *fakeAlertRepo) Count(_ context.Context, _ domain.AlertFilter) (int64, error) {
	if r.countErr != nil {
		return 0, r.countErr
	}
	return 0, nil
}

type fakeEventRepo struct {
	mu   sync.Mutex
	rows []domain.AlertEvent
}

func (r *fakeEventRepo) Create(_ context.Context, event *domain.AlertEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *event
	cp.CreatedAt = time.Now()
	r.rows = append(r.rows, cp)
	return nil
}

func (r *fakeEventRepo) ListByAlertID(_ context.Context, alertID string) ([]domain.AlertEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.AlertEvent, 0)
	for _, e := range r.rows {
		if e.AlertID == alertID {
			out = append(out, e)
		}
	}
	return out, nil
}

func TestAlertService_AcknowledgeAndStartProcessing(t *testing.T) {
	alerts := newFakeAlertRepo()
	events := &fakeEventRepo{}
	now := time.Now()
	alert := &domain.Alert{
		ID:              "a1",
		SourceID:        "src1",
		DedupKey:        "dk1",
		Name:            "HighCPU",
		Severity:        domain.SeverityP1,
		Status:          domain.StatusNew,
		Labels:          map[string]string{},
		Annotations:     map[string]string{},
		OccurrenceCount: 1,
		FirstSeenAt:     now,
		LastSeenAt:      now,
	}
	_ = alerts.Create(context.Background(), alert)

	svc := NewAlertService(alerts, events, nil, NoopAuditRecorder{})
	actor := Actor{UserID: "u1", DisplayName: "alice"}

	out, err := svc.Acknowledge(context.Background(), "a1", actor, "")
	if err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if out.Status != string(domain.StatusAcknowledged) {
		t.Fatalf("expected acknowledged, got %s", out.Status)
	}

	out, err = svc.StartProcessing(context.Background(), "a1", actor)
	if err != nil {
		t.Fatalf("start processing: %v", err)
	}
	if out.Status != string(domain.StatusProcessing) {
		t.Fatalf("expected processing, got %s", out.Status)
	}
	if len(events.rows) < 2 {
		t.Fatalf("expected events recorded")
	}
}

func TestAlertService_InvalidTransition(t *testing.T) {
	alerts := newFakeAlertRepo()
	now := time.Now()
	alert := &domain.Alert{
		ID: "a1", SourceID: "src1", DedupKey: "dk1", Name: "x",
		Severity: domain.SeverityP1, Status: domain.StatusClosed,
		Labels: map[string]string{}, Annotations: map[string]string{},
		OccurrenceCount: 1, FirstSeenAt: now, LastSeenAt: now,
	}
	_ = alerts.Create(context.Background(), alert)
	svc := NewAlertService(alerts, &fakeEventRepo{}, nil, NoopAuditRecorder{})
	_, err := svc.Acknowledge(context.Background(), "a1", Actor{UserID: "u1"}, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", apperr.FromError(err).Code)
	}
}

func TestAlertService_GetDetailWithNilEventRepoReturnsEmptyTimeline(t *testing.T) {
	alerts := newFakeAlertRepo()
	now := time.Now()
	alert := &domain.Alert{
		ID: "a1", SourceID: "src1", DedupKey: "dk1", Name: "HighCPU",
		Severity: domain.SeverityP1, Status: domain.StatusNew,
		Labels: map[string]string{}, Annotations: map[string]string{},
		OccurrenceCount: 1, FirstSeenAt: now, LastSeenAt: now,
	}
	_ = alerts.Create(context.Background(), alert)

	svc := NewAlertService(alerts, nil, nil, NoopAuditRecorder{})
	detail, err := svc.GetDetail(context.Background(), "a1")
	if err != nil {
		t.Fatalf("get detail: %v", err)
	}
	if detail.Alert.ID != "a1" {
		t.Fatalf("expected alert a1, got %s", detail.Alert.ID)
	}
	if len(detail.Events) != 0 {
		t.Fatalf("expected empty timeline, got %d events", len(detail.Events))
	}
}

func TestAlertService_CommentWithNilEventRepoReturnsUnavailable(t *testing.T) {
	alerts := newFakeAlertRepo()
	now := time.Now()
	alert := &domain.Alert{
		ID: "a1", SourceID: "src1", DedupKey: "dk1", Name: "HighCPU",
		Severity: domain.SeverityP1, Status: domain.StatusNew,
		Labels: map[string]string{}, Annotations: map[string]string{},
		OccurrenceCount: 1, FirstSeenAt: now, LastSeenAt: now,
	}
	_ = alerts.Create(context.Background(), alert)

	svc := NewAlertService(alerts, nil, nil, NoopAuditRecorder{})
	_, err := svc.Comment(context.Background(), "a1", Actor{UserID: "u1", DisplayName: "alice"}, "note")
	if err == nil {
		t.Fatal("expected error")
	}
	if apperr.FromError(err).Code != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable, got %v", apperr.FromError(err).Code)
	}
}

func TestAlertService_ListInvalidStatusFilter(t *testing.T) {
	svc := NewAlertService(newFakeAlertRepo(), &fakeEventRepo{}, nil, NoopAuditRecorder{})
	_, _, err := svc.List(context.Background(), AlertListQuery{Status: "bogus"})
	if err == nil {
		t.Fatal("expected error")
	}
	if apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", apperr.FromError(err).Code)
	}
}

func TestAlertService_ListSeverityFilterCaseInsensitive(t *testing.T) {
	alerts := newFakeAlertRepo()
	svc := NewAlertService(alerts, &fakeEventRepo{}, nil, NoopAuditRecorder{})
	_, _, err := svc.List(context.Background(), AlertListQuery{Severity: "P1,p2"})
	if err != nil {
		t.Fatalf("expected valid severity filter, got %v", err)
	}
	_, _, err = svc.List(context.Background(), AlertListQuery{Severity: "critical"})
	if err == nil {
		t.Fatal("expected error for non-platform severity filter")
	}
	if apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", apperr.FromError(err).Code)
	}
}

func TestAlertService_ListWrapsRepositoryErrorWithOperation(t *testing.T) {
	alerts := newFakeAlertRepo()
	alerts.listErr = domain.ErrAlreadyExists
	svc := NewAlertService(alerts, &fakeEventRepo{}, nil, NoopAuditRecorder{})
	_, _, err := svc.List(context.Background(), AlertListQuery{})
	if apperr.FromError(err).Code != apperr.CodeAlreadyExists {
		t.Fatalf("expected already exists, got %v", err)
	}

	alerts.listErr = assertErr("database offline")
	_, _, err = svc.List(context.Background(), AlertListQuery{})
	app := apperr.FromError(err)
	if app.Code != apperr.CodeInternal || app.Message != "list alerts failed" {
		t.Fatalf("expected list operation internal error, got code=%s message=%q", app.Code, app.Message)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func TestAlertService_RequestAIAnalysis(t *testing.T) {
	alerts := newFakeAlertRepo()
	events := &fakeEventRepo{}
	now := time.Now()
	alert := &domain.Alert{
		ID: "a1", SourceID: "src1", DedupKey: "dk1", Name: "HighCPU",
		Severity: domain.SeverityP1, Status: domain.StatusNew,
		Labels: map[string]string{}, Annotations: map[string]string{},
		OccurrenceCount: 1, FirstSeenAt: now, LastSeenAt: now,
	}
	_ = alerts.Create(context.Background(), alert)
	svc := NewAlertService(alerts, events, nil, NoopAuditRecorder{})
	actor := Actor{UserID: "u1", DisplayName: "alice"}

	ev, err := svc.RequestAIAnalysis(context.Background(), "a1", actor, AIAnalysisInput{
		TimeRange:      "30m",
		IncludeLogs:    true,
		IncludeMetrics: true,
		IncludeChanges: true,
	})
	if err != nil {
		t.Fatalf("request ai analysis: %v", err)
	}
	if ev.EventType != string(domain.EventAIAnalysisRequested) {
		t.Fatalf("expected ai_analysis_requested, got %s", ev.EventType)
	}
	if ev.Payload["time_range"] != "30m" {
		t.Fatalf("unexpected payload: %#v", ev.Payload)
	}

	alert.Status = domain.StatusClosed
	_ = alerts.Update(context.Background(), alert)
	_, err = svc.RequestAIAnalysis(context.Background(), "a1", actor, AIAnalysisInput{})
	if err == nil {
		t.Fatal("expected error for closed alert")
	}
	if apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", apperr.FromError(err).Code)
	}
}
