package application

import (
	"context"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type syncTestSourceRepo struct {
	byID map[string]*domain.AlertSource
}

func newSyncTestSourceRepo(src *domain.AlertSource) *syncTestSourceRepo {
	return &syncTestSourceRepo{byID: map[string]*domain.AlertSource{src.ID: src}}
}

func (r *syncTestSourceRepo) Create(_ context.Context, src *domain.AlertSource) error {
	r.byID[src.ID] = src
	return nil
}
func (r *syncTestSourceRepo) Update(_ context.Context, src *domain.AlertSource) error {
	r.byID[src.ID] = src
	return nil
}
func (r *syncTestSourceRepo) GetByID(_ context.Context, id string) (*domain.AlertSource, error) {
	src, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *src
	return &cp, nil
}
func (r *syncTestSourceRepo) List(context.Context) ([]domain.AlertSource, error) { return nil, nil }
func (r *syncTestSourceRepo) Delete(context.Context, string) error               { return nil }

type syncTestPull struct {
	results []*AlertPullResult
	errs    []error
	calls   int
	reqs    []AlertPullRequest
}

func (p *syncTestPull) PullAlarms(_ context.Context, req AlertPullRequest) (*AlertPullResult, error) {
	p.reqs = append(p.reqs, req)
	i := p.calls
	p.calls++
	if i < len(p.errs) && p.errs[i] != nil {
		return nil, p.errs[i]
	}
	if i >= len(p.results) {
		return &AlertPullResult{}, nil
	}
	return p.results[i], nil
}

type syncTestLease struct {
	acquire  bool
	renew    bool
	commit   bool
	state    map[string]domain.ScopeCursor
	released bool
}

func (l *syncTestLease) Acquire(context.Context, string, string, time.Duration) (bool, error) {
	return l.acquire, nil
}
func (l *syncTestLease) Renew(context.Context, string, string, time.Duration) (bool, error) {
	return l.renew, nil
}
func (l *syncTestLease) Release(context.Context, string, string) error {
	l.released = true
	return nil
}
func (l *syncTestLease) IsHeld(context.Context, string) (bool, error) { return false, nil }
func (l *syncTestLease) CommitSyncState(_ context.Context, _, _ string, _ time.Time, status string, state map[string]domain.ScopeCursor) (bool, error) {
	if !l.commit {
		return false, nil
	}
	l.state = cloneScopeState(state)
	return true, nil
}

func newSyncTestService(t *testing.T, src *domain.AlertSource, pull AlertPullPort, lease *syncTestLease) (*AlertSyncService, *fakeAlertRepo, *fakeEventRepo) {
	t.Helper()
	alerts := newFakeAlertRepo()
	events := &fakeEventRepo{}
	sources := newSyncTestSourceRepo(src)
	ingest := NewIngestService(alerts, events, sources, nil, nil, NoopAuditRecorder{})
	return NewAlertSyncService(sources, pull, ingest, lease, nil, NoopAuditRecorder{}, "owner-1"), alerts, events
}

func newPullSource() *domain.AlertSource {
	return &domain.AlertSource{
		ID: "ces-src", Name: "CES", Type: domain.SourceHuaweiCES, Enabled: true,
		DeliveryMode: domain.DeliveryModePull, AccountID: "acc-1",
		PullConfig: domain.PullConfig{PullIntervalSeconds: 300, SeverityMap: domain.DefaultSeverityMap()},
	}
}

func pulled(recordID, status, resource string, last time.Time) PulledAlarm {
	return PulledAlarm{
		RecordID: recordID, AlarmID: "al-rule", Name: "High CPU", Status: status, Level: "2",
		Namespace: "SYS.ECS", ResourceID: "i-" + resource, ResourceName: "ecs-" + resource,
		ResourceType: "ecs", Dimensions: map[string]string{"instance_id": "i-" + resource},
		LastAlarmAt: &last,
	}
}

func oneScope(records []PulledAlarm, complete bool, windowEnd int64, code string) *AlertPullResult {
	return &AlertPullResult{
		Regions: []string{"cn-north-4"},
		Scopes: []PulledScope{{
			Region: "cn-north-4", ProjectID: "proj-1", Records: records,
			Complete: complete, WindowEnd: windowEnd, FailureCode: code,
		}},
	}
}

func TestAlertSyncService_CreatesRecordsAndAdvancesCursor(t *testing.T) {
	now := time.Now().UTC()
	pull := &syncTestPull{results: []*AlertPullResult{
		oneScope([]PulledAlarm{pulled("r-a", "alarm", "a", now), pulled("r-b", "alarm", "b", now)}, true, 1000, ""),
	}}
	lease := &syncTestLease{acquire: true, renew: true, commit: true}
	svc, alerts, events := newSyncTestService(t, newPullSource(), pull, lease)

	result, err := svc.Sync(context.Background(), "ces-src", Actor{UserID: "u-1"})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Status != syncStatusSuccess || result.Created != 2 || result.Accepted != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.FailedScopes) != 0 {
		t.Fatalf("expected empty failed scopes, got %+v", result.FailedScopes)
	}
	if len(alerts.byID) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts.byID))
	}
	if len(events.rows) != 2 {
		t.Fatalf("expected 2 triggered events, got %d", len(events.rows))
	}
	cursor, ok := lease.state[domain.ScopeKey("cn-north-4", "proj-1")]
	if !ok || cursor.CursorAt.Unix() != 1000 {
		t.Fatalf("expected cursor 1000, got %+v", lease.state)
	}
	if pull.reqs[0].Cursors != nil && len(pull.reqs[0].Cursors) != 0 {
		t.Fatalf("first round should start without cursors, got %+v", pull.reqs[0].Cursors)
	}
}

func TestAlertSyncService_DuplicateObservationIgnored(t *testing.T) {
	now := time.Now().UTC()
	round := oneScope([]PulledAlarm{pulled("r-a", "alarm", "a", now), pulled("r-b", "alarm", "b", now)}, true, 1000, "")
	pull := &syncTestPull{results: []*AlertPullResult{round, round}}
	lease := &syncTestLease{acquire: true, renew: true, commit: true}
	svc, alerts, events := newSyncTestService(t, newPullSource(), pull, lease)

	if _, err := svc.Sync(context.Background(), "ces-src", Actor{UserID: "u-1"}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	result, err := svc.Sync(context.Background(), "ces-src", Actor{UserID: "u-1"})
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if result.Created != 0 || result.Updated != 0 || result.Ignored != 2 || result.Accepted != 2 {
		t.Fatalf("expected duplicate observation ignored, got %+v", result)
	}
	if len(alerts.byID) != 2 {
		t.Fatalf("duplicate must not create alerts, got %d", len(alerts.byID))
	}
	if len(events.rows) != 2 {
		t.Fatalf("duplicate must not append timeline events, got %d", len(events.rows))
	}
}

func TestAlertSyncService_RecoveryAndOldFiring(t *testing.T) {
	now := time.Now().UTC()
	round1 := oneScope([]PulledAlarm{
		pulled("r-a", "alarm", "a", now.Add(-10*time.Minute)),
		pulled("r-b", "alarm", "b", now.Add(-10*time.Minute)),
	}, true, 1000, "")
	round2 := oneScope([]PulledAlarm{
		pulled("r-a", "ok", "a", now.Add(-20*time.Minute)),
		pulled("r-b", "alarm", "b", now.Add(-1*time.Minute)),
	}, true, 2000, "")
	round3 := oneScope([]PulledAlarm{
		pulled("r-a", "alarm", "a", now.Add(-30*time.Minute)), // 旧 firing 快照
		pulled("r-b", "ok", "b", now.Add(-1*time.Minute)),
	}, true, 3000, "")
	pull := &syncTestPull{results: []*AlertPullResult{round1, round2, round3}}
	lease := &syncTestLease{acquire: true, renew: true, commit: true}
	svc, alerts, _ := newSyncTestService(t, newPullSource(), pull, lease)

	if _, err := svc.Sync(context.Background(), "ces-src", Actor{}); err != nil {
		t.Fatalf("round1: %v", err)
	}
	result2, err := svc.Sync(context.Background(), "ces-src", Actor{})
	if err != nil {
		t.Fatalf("round2: %v", err)
	}
	if result2.Recovered != 1 || result2.Updated != 1 || result2.Created != 0 {
		t.Fatalf("round2 expected recovered=1 updated=1, got %+v", result2)
	}
	keyA := domain.ComputeCESDedupKey("ces-src", "acc-1", "cn-north-4", "proj-1", "r-a")
	a, err := alerts.FindActiveByDedupKey(context.Background(), "ces-src", keyA)
	if err != nil || a.Status != domain.StatusRecovered {
		t.Fatalf("expected A recovered, got %+v err=%v", a, err)
	}
	result3, err := svc.Sync(context.Background(), "ces-src", Actor{})
	if err != nil {
		t.Fatalf("round3: %v", err)
	}
	if result3.Recovered != 1 || result3.Ignored < 1 {
		t.Fatalf("round3 expected B recovered and old A firing ignored, got %+v", result3)
	}
	a, _ = alerts.FindActiveByDedupKey(context.Background(), "ces-src", keyA)
	if a.Status != domain.StatusRecovered {
		t.Fatalf("old firing must not reopen A, got %s", a.Status)
	}
	cursor := lease.state[domain.ScopeKey("cn-north-4", "proj-1")]
	if cursor.CursorAt.Unix() != 3000 {
		t.Fatalf("expected cursor 3000, got %+v", cursor)
	}
}

func TestAlertSyncService_FailedScopeKeepsCursorWhilePartial(t *testing.T) {
	now := time.Now().UTC()
	result := &AlertPullResult{
		Regions: []string{"cn-north-4", "cn-south-1"},
		Scopes: []PulledScope{
			{Region: "cn-north-4", ProjectID: "proj-1", Records: []PulledAlarm{pulled("r-a", "alarm", "a", now)}, Complete: true, WindowEnd: 1000},
			{Region: "cn-south-1", ProjectID: "proj-2", Records: []PulledAlarm{pulled("r-b", "alarm", "b", now)}, Complete: false, FailureCode: "PAGE_INCOMPLETE"},
		},
	}
	pull := &syncTestPull{results: []*AlertPullResult{result}}
	lease := &syncTestLease{acquire: true, renew: true, commit: true}
	svc, _, _ := newSyncTestService(t, newPullSource(), pull, lease)

	out, err := svc.Sync(context.Background(), "ces-src", Actor{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if out.Status != syncStatusPartial {
		t.Fatalf("expected partial, got %s", out.Status)
	}
	if len(out.FailedScopes) != 1 || out.FailedScopes[0].Reason != "PAGE_INCOMPLETE" {
		t.Fatalf("expected PAGE_INCOMPLETE failure, got %+v", out.FailedScopes)
	}
	if _, ok := lease.state[domain.ScopeKey("cn-north-4", "proj-1")]; !ok {
		t.Fatal("expected successful scope cursor")
	}
	if _, ok := lease.state[domain.ScopeKey("cn-south-1", "proj-2")]; ok {
		t.Fatal("failed scope must not advance cursor")
	}
}

func TestAlertSyncService_LeaseConflictReturnsAborted(t *testing.T) {
	pull := &syncTestPull{results: []*AlertPullResult{{}}}
	lease := &syncTestLease{acquire: false, renew: true, commit: true}
	svc, _, _ := newSyncTestService(t, newPullSource(), pull, lease)

	_, err := svc.Sync(context.Background(), "ces-src", Actor{})
	if err == nil || apperr.CodeOf(err) != apperr.CodeAborted {
		t.Fatalf("expected ABORTED, got %v", err)
	}
	if pull.calls != 0 {
		t.Fatal("conflicting round must not call provider")
	}
}

func TestAlertSyncService_ExpiredLeaseCommitReturnsAborted(t *testing.T) {
	now := time.Now().UTC()
	pull := &syncTestPull{results: []*AlertPullResult{
		oneScope([]PulledAlarm{pulled("r-a", "alarm", "a", now)}, true, 1000, ""),
	}}
	lease := &syncTestLease{acquire: true, renew: true, commit: false}
	svc, _, _ := newSyncTestService(t, newPullSource(), pull, lease)

	_, err := svc.Sync(context.Background(), "ces-src", Actor{})
	if err == nil || apperr.CodeOf(err) != apperr.CodeAborted {
		t.Fatalf("expected ABORTED on expired lease commit, got %v", err)
	}
}

func TestAlertSyncService_PullFailureReturnsFailedSummary(t *testing.T) {
	pull := &syncTestPull{errs: []error{apperr.New(apperr.CodeUnavailable, "credential failed")}}
	lease := &syncTestLease{acquire: true, renew: true, commit: true}
	svc, _, _ := newSyncTestService(t, newPullSource(), pull, lease)

	out, err := svc.Sync(context.Background(), "ces-src", Actor{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if out.Status != syncStatusFailed || len(out.FailedScopes) != 1 {
		t.Fatalf("expected failed summary, got %+v", out)
	}
	if lease.state == nil {
		t.Fatal("failed round should still commit last_sync_status")
	}
}

func TestAlertSyncService_WritesCESConditionMetadata(t *testing.T) {
	now := time.Now().UTC()
	rec := pulled("r-cond", "alarm", "a", now)
	rec.Namespace = "SYS.RDS"
	rec.MetricName = "disk_usedPercent"
	rec.ResourceName = "prod-db"
	rec.ResourceType = "rds"
	rec.Condition = &AlarmCondition{
		Filter: "average", ComparisonOperator: ">", Threshold: 80, Unit: "%",
		PeriodSeconds: 300, TriggerCount: 3,
	}
	rec.RuleName = "RDS 磁盘使用率告警"
	rec.RuleDescription = "RDS disk usage exceeds 80%"
	rec.RuleType = "MULTI_INSTANCE"
	pull := &syncTestPull{results: []*AlertPullResult{
		oneScope([]PulledAlarm{rec}, true, 1000, ""),
	}}
	lease := &syncTestLease{acquire: true, renew: true, commit: true}
	svc, alerts, _ := newSyncTestService(t, newPullSource(), pull, lease)

	result, err := svc.Sync(context.Background(), "ces-src", Actor{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("expected 1 created, got %+v", result)
	}
	if len(alerts.byID) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts.byID))
	}
	var alert *domain.Alert
	for _, item := range alerts.byID {
		alert = item
	}
	if got := alert.Annotations["ces_condition"]; got != "> 80% (average, 300s x3)" {
		t.Fatalf("ces_condition = %q", got)
	}
	if got := alert.Annotations["ces_metric_name"]; got != "disk_usedPercent" {
		t.Fatalf("ces_metric_name = %q", got)
	}
	if got := alert.Annotations["ces_level"]; got != "2" {
		t.Fatalf("ces_level = %q", got)
	}
	if got := alert.Labels["metric_name"]; got != "disk_usedPercent" {
		t.Fatalf("labels.metric_name = %q", got)
	}
	if got := alert.Labels["rule_name"]; got != "RDS 磁盘使用率告警" {
		t.Fatalf("labels.rule_name = %q", got)
	}
	if got := alert.Labels["resource_type"]; got != "rds" {
		t.Fatalf("labels.resource_type = %q", got)
	}
	if got := alert.Annotations["ces_rule_description"]; got != "RDS disk usage exceeds 80%" {
		t.Fatalf("ces_rule_description = %q", got)
	}
	if got := alert.Annotations["ces_rule_type"]; got != "MULTI_INSTANCE" {
		t.Fatalf("ces_rule_type = %q", got)
	}
	wantDesc := "SYS.RDS disk_usedPercent > 80% (average, 300s x3) · prod-db"
	if alert.Description != wantDesc {
		t.Fatalf("description = %q, want %q", alert.Description, wantDesc)
	}
	if alert.Summary != "prod-db" {
		t.Fatalf("summary should stay resource name, got %q", alert.Summary)
	}
}

func TestAlertSyncService_RuleLookupMissKeepsAlertAndMarksAnnotation(t *testing.T) {
	now := time.Now().UTC()
	rec := pulled("r-rule-miss", "alarm", "a", now)
	rec.Namespace = ""
	rec.MetricName = ""
	rec.ResourceName = ""
	rec.RuleLookup = "not_found"
	pull := &syncTestPull{results: []*AlertPullResult{
		oneScope([]PulledAlarm{rec}, true, 1000, ""),
	}}
	lease := &syncTestLease{acquire: true, renew: true, commit: true}
	svc, alerts, _ := newSyncTestService(t, newPullSource(), pull, lease)

	if _, err := svc.Sync(context.Background(), "ces-src", Actor{}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var alert *domain.Alert
	for _, item := range alerts.byID {
		alert = item
	}
	if alert == nil {
		t.Fatal("expected alert to be created")
	}
	if got := alert.Annotations["ces_rule_lookup"]; got != "not_found" {
		t.Fatalf("ces_rule_lookup = %q", got)
	}
	if alert.Summary != "i-a" {
		t.Fatalf("summary should fall back to resource id, got %q", alert.Summary)
	}
	if alert.Description != "i-a" {
		t.Fatalf("description should fall back to resource id, got %q", alert.Description)
	}
}
