package infrastructure

import (
	"context"

	alertdomain "github.com/734965549/aiops/internal/alert/domain"
	assetdomain "github.com/734965549/aiops/internal/asset/domain"
	dashapp "github.com/734965549/aiops/internal/dashboard/application"
	execdomain "github.com/734965549/aiops/internal/execution/domain"
	rbdomain "github.com/734965549/aiops/internal/runbook/domain"
)

// RepoStatsReader 基于各模块仓储实现 Dashboard 统计读口。
type RepoStatsReader struct {
	Alerts    alertdomain.AlertRepository
	Tasks     execdomain.TaskRepository
	Apps      assetdomain.ApplicationRepository
	Resources assetdomain.ResourceRepository
	Templates rbdomain.TemplateRepository
}

var _ dashapp.StatsReader = (*RepoStatsReader)(nil)

func (r *RepoStatsReader) CountAlerts(ctx context.Context, filter alertdomain.AlertFilter) (int64, error) {
	if r == nil || r.Alerts == nil {
		return 0, nil
	}
	return r.Alerts.Count(ctx, filter)
}

func (r *RepoStatsReader) ListAlerts(ctx context.Context, filter alertdomain.AlertFilter) ([]alertdomain.Alert, error) {
	if r == nil || r.Alerts == nil {
		return nil, nil
	}
	return r.Alerts.List(ctx, filter)
}

func (r *RepoStatsReader) CountTasks(ctx context.Context, filter execdomain.TaskFilter) (int64, error) {
	if r == nil || r.Tasks == nil {
		return 0, nil
	}
	return r.Tasks.Count(ctx, filter)
}

func (r *RepoStatsReader) ListTasks(ctx context.Context, filter execdomain.TaskFilter) ([]execdomain.Task, error) {
	if r == nil || r.Tasks == nil {
		return nil, nil
	}
	return r.Tasks.List(ctx, filter)
}

func (r *RepoStatsReader) CountApplications(ctx context.Context) (int64, error) {
	if r == nil || r.Apps == nil {
		return 0, nil
	}
	return r.Apps.Count(ctx)
}

func (r *RepoStatsReader) CountResources(ctx context.Context) (int64, error) {
	if r == nil || r.Resources == nil {
		return 0, nil
	}
	return r.Resources.Count(ctx)
}

func (r *RepoStatsReader) CountRunbookTemplates(ctx context.Context, filter rbdomain.TemplateFilter) (int64, error) {
	if r == nil || r.Templates == nil {
		return 0, nil
	}
	return r.Templates.Count(ctx, filter)
}
