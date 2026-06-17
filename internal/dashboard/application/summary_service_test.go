package application

import (
	"context"
	"errors"
	"testing"
	"time"

	alertdomain "github.com/734965549/aiops/internal/alert/domain"
	execdomain "github.com/734965549/aiops/internal/execution/domain"
	rbdomain "github.com/734965549/aiops/internal/runbook/domain"
)

type fakeDashboardStats struct {
	alertCounts map[string]int64
	tasks       []execdomain.Task
	pending     int64
	apps        int64
	resources   int64
	rbTotal     int64
	rbEnabled   int64
	alerts      []alertdomain.Alert
}

func (f *fakeDashboardStats) CountAlerts(_ context.Context, filter alertdomain.AlertFilter) (int64, error) {
	if len(filter.Severities) == 1 {
		return f.alertCounts[string(filter.Severities[0])], nil
	}
	if filter.ActiveOnly {
		return f.alertCounts["active"], nil
	}
	return 0, nil
}

func (f *fakeDashboardStats) ListAlerts(_ context.Context, _ alertdomain.AlertFilter) ([]alertdomain.Alert, error) {
	return f.alerts, nil
}

func (f *fakeDashboardStats) CountTasks(_ context.Context, filter execdomain.TaskFilter) (int64, error) {
	if len(filter.Statuses) == 1 && filter.Statuses[0] == execdomain.StatusPendingConfirm {
		return f.pending, nil
	}
	return 0, nil
}

func (f *fakeDashboardStats) ListTasks(_ context.Context, _ execdomain.TaskFilter) ([]execdomain.Task, error) {
	return f.tasks, nil
}

func (f *fakeDashboardStats) CountApplications(context.Context) (int64, error) {
	return f.apps, nil
}

func (f *fakeDashboardStats) CountResources(context.Context) (int64, error) {
	return f.resources, nil
}

func (f *fakeDashboardStats) CountRunbookTemplates(_ context.Context, filter rbdomain.TemplateFilter) (int64, error) {
	if filter.Enabled != nil && *filter.Enabled {
		return f.rbEnabled, nil
	}
	return f.rbTotal, nil
}

func TestSummaryService_GetSummary(t *testing.T) {
	now := time.Now().UTC()
	stats := &fakeDashboardStats{
		alertCounts: map[string]int64{"active": 12, "p0": 2, "p1": 4},
		pending:     3,
		apps:        5,
		resources:   18,
		rbTotal:     8,
		rbEnabled:   6,
		tasks: []execdomain.Task{
			{ID: "t1", Name: "Restart pod", Status: execdomain.StatusSuccess, CreatedAt: now},
		},
		alerts: []alertdomain.Alert{
			{ID: "a1", Name: "HighCPU", Severity: alertdomain.SeverityP1, Status: alertdomain.StatusProcessing},
		},
	}
	svc := NewSummaryService(stats)
	out, err := svc.GetSummary(context.Background())
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if out.Alerts.ActiveTotal != 12 || out.Alerts.P0 != 2 || out.Alerts.P1 != 4 {
		t.Fatalf("unexpected alerts: %+v", out.Alerts)
	}
	if out.Executions.PendingConfirm != 3 || len(out.Executions.Recent) != 1 {
		t.Fatalf("unexpected executions: %+v", out.Executions)
	}
	if out.Assets.Applications != 5 || out.Assets.Resources != 18 {
		t.Fatalf("unexpected assets: %+v", out.Assets)
	}
	if out.Runbooks.Total != 8 || out.Runbooks.Enabled != 6 {
		t.Fatalf("unexpected runbooks: %+v", out.Runbooks)
	}
	if len(out.ProcessingAlerts) != 1 || out.ProcessingAlerts[0].ID != "a1" {
		t.Fatalf("unexpected processing alerts: %+v", out.ProcessingAlerts)
	}
}

type partialFailDashboardStats struct {
	fakeDashboardStats
	failAlerts bool
}

func (f *partialFailDashboardStats) CountAlerts(context.Context, alertdomain.AlertFilter) (int64, error) {
	if f.failAlerts {
		return 0, errors.New("alerts unavailable")
	}
	return f.fakeDashboardStats.CountAlerts(context.Background(), alertdomain.AlertFilter{ActiveOnly: true})
}

func TestSummaryService_GetSummaryPartialFailure(t *testing.T) {
	stats := &partialFailDashboardStats{
		fakeDashboardStats: fakeDashboardStats{
			alertCounts: map[string]int64{"active": 12},
			apps:        5,
		},
		failAlerts: true,
	}
	svc := NewSummaryService(stats)
	out, err := svc.GetSummary(context.Background())
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if out.Alerts.ActiveTotal != 0 {
		t.Fatalf("expected zero alerts on failure, got %d", out.Alerts.ActiveTotal)
	}
	if out.Assets.Applications != 5 {
		t.Fatalf("expected assets block to succeed, got %d", out.Assets.Applications)
	}
}
