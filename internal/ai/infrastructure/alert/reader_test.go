package alert

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	alertdomain "github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeAlertRepo struct {
	alert *alertdomain.Alert
	err   error
}

func (f *fakeAlertRepo) Create(context.Context, *alertdomain.Alert) error { return nil }
func (f *fakeAlertRepo) Update(context.Context, *alertdomain.Alert) error { return nil }
func (f *fakeAlertRepo) GetByID(context.Context, string) (*alertdomain.Alert, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.alert, nil
}
func (f *fakeAlertRepo) FindActiveByDedupKey(context.Context, string, string) (*alertdomain.Alert, error) {
	return nil, alertdomain.ErrNotFound
}
func (f *fakeAlertRepo) FindLatestByDedupKey(context.Context, string, string) (*alertdomain.Alert, error) {
	return nil, alertdomain.ErrNotFound
}
func (f *fakeAlertRepo) MaxLifecycleSeq(context.Context, string) (int, error) { return 0, nil }
func (f *fakeAlertRepo) List(context.Context, alertdomain.AlertFilter) ([]alertdomain.Alert, error) {
	return nil, nil
}
func (f *fakeAlertRepo) Count(context.Context, alertdomain.AlertFilter) (int64, error) { return 0, nil }

// snapshotPayload 校验快照的 provider 序列化形态（与 alertSnapshot 等价）。
func snapshotPayload(t *testing.T, alert *aiapp.AlertContext) map[string]any {
	t.Helper()
	raw, err := json.Marshal(alert)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	return out
}

// fakeScope 记录入参并返回预设结果。
type fakeScope struct {
	err     error
	called  bool
	userID  string
	alertID string
	context aiapp.AlertObjectContext
}

func (f *fakeScope) AuthorizeAlert(_ context.Context, userID string, alert *aiapp.AlertContext) error {
	f.called = true
	f.userID = userID
	if alert != nil {
		f.alertID = alert.ID
		f.context = aiapp.AlertObjectContextOf(alert)
	}
	return f.err
}

func testAlert() *alertdomain.Alert {
	recovered := time.Unix(1700000900, 0).UTC()
	return &alertdomain.Alert{
		ID: "a1", Name: "HighCPU", Summary: "CPU > 85%", Severity: "p1", Status: "firing",
		BusinessLine: "payments", Environment: "prod", Source: "huawei_ces",
		OwnerUserID: "user-9",
		Labels:      map[string]string{"region": "cn-north-4", "team": "sre"},
		Annotations: map[string]string{"summary": "cpu"},
		FirstSeenAt: time.Unix(1700000000, 0).UTC(),
		LastSeenAt:  time.Unix(1700000600, 0).UTC(),
		RecoveredAt: &recovered,
	}
}

func TestReaderAdapterReturnsSnapshotAfterScopePasses(t *testing.T) {
	scope := &fakeScope{}
	reader := NewReaderAdapter(&fakeAlertRepo{alert: testAlert()}, scope)

	snapshot, err := reader.GetForAnalysis(context.Background(), "user-1", " a1 ")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if snapshot.ID != "a1" || snapshot.Name != "HighCPU" || snapshot.BusinessLine != "payments" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	// owner 只用于数据范围校验，必须在对象上下文中可见但不出现在序列化快照里。
	if snapshot.OwnerUserID != "user-9" {
		t.Fatalf("owner_user_id = %q", snapshot.OwnerUserID)
	}
	if !scope.called || scope.userID != "user-1" || scope.alertID != "a1" {
		t.Fatalf("scope guard not called correctly: %+v", scope)
	}
	if scope.context.Team != "sre" || scope.context.Region != "cn-north-4" {
		t.Fatalf("scope object context = %+v", scope.context)
	}
	raw := snapshotPayload(t, snapshot)
	if _, ok := raw["owner_user_id"]; ok {
		t.Fatal("owner_user_id must not be sent to provider payload")
	}
}

func TestReaderAdapterPropagatesScopeDenial(t *testing.T) {
	scope := &fakeScope{err: apperr.New(apperr.CodePermissionDenied, "data scope denied")}
	reader := NewReaderAdapter(&fakeAlertRepo{alert: testAlert()}, scope)

	snapshot, err := reader.GetForAnalysis(context.Background(), "user-1", "a1")
	if apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if snapshot != nil {
		t.Fatal("denied read must not return alert content")
	}
	if !scope.called {
		t.Fatal("scope guard must run before returning the snapshot")
	}
}

func TestReaderAdapterRejectsUnconfiguredScopeGuard(t *testing.T) {
	reader := NewReaderAdapter(&fakeAlertRepo{alert: testAlert()}, nil)
	if _, err := reader.GetForAnalysis(context.Background(), "user-1", "a1"); apperr.CodeOf(err) != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable (fail-closed), got %v", err)
	}
}

func TestReaderAdapterNotFound(t *testing.T) {
	reader := NewReaderAdapter(&fakeAlertRepo{err: alertdomain.ErrNotFound}, &fakeScope{})
	if _, err := reader.GetForAnalysis(context.Background(), "user-1", "missing"); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestReaderAdapterWrapsRepositoryError(t *testing.T) {
	reader := NewReaderAdapter(&fakeAlertRepo{err: errors.New("db down")}, &fakeScope{})
	if _, err := reader.GetForAnalysis(context.Background(), "user-1", "a1"); apperr.CodeOf(err) != apperr.CodeInternal {
		t.Fatalf("expected internal error, got %v", err)
	}
}
