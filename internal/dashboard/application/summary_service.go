// Package application 实现 Dashboard 首页聚合查询。
package application

import (
	"context"

	alertdomain "github.com/734965549/aiops/internal/alert/domain"
	execapp "github.com/734965549/aiops/internal/execution/application"
	execdomain "github.com/734965549/aiops/internal/execution/domain"
	rbdomain "github.com/734965549/aiops/internal/runbook/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

const defaultRecentExecutions = 10

// StatsReader 聚合 Dashboard 所需的跨模块统计读口。
type StatsReader interface {
	CountAlerts(ctx context.Context, filter alertdomain.AlertFilter) (int64, error)
	ListAlerts(ctx context.Context, filter alertdomain.AlertFilter) ([]alertdomain.Alert, error)
	CountTasks(ctx context.Context, filter execdomain.TaskFilter) (int64, error)
	ListTasks(ctx context.Context, filter execdomain.TaskFilter) ([]execdomain.Task, error)
	CountApplications(ctx context.Context) (int64, error)
	CountResources(ctx context.Context) (int64, error)
	CountRunbookTemplates(ctx context.Context, filter rbdomain.TemplateFilter) (int64, error)
}

// SummaryService 首页驾驶舱聚合服务。
type SummaryService struct {
	stats StatsReader
}

// NewSummaryService 构造聚合服务。
func NewSummaryService(stats StatsReader) *SummaryService {
	return &SummaryService{stats: stats}
}

// AlertMetrics 告警计数摘要。
type AlertMetrics struct {
	ActiveTotal int64 `json:"active_total"`
	P0          int64 `json:"p0"`
	P1          int64 `json:"p1"`
}

// ExecutionMetrics 执行任务摘要。
type ExecutionMetrics struct {
	PendingConfirm int64             `json:"pending_confirm"`
	Recent         []execapp.TaskDTO `json:"recent"`
}

// AssetMetrics 资产注册表摘要。
type AssetMetrics struct {
	Applications int64 `json:"applications"`
	Resources    int64 `json:"resources"`
}

// RunbookMetrics Runbook 摘要。
type RunbookMetrics struct {
	Total   int64 `json:"total"`
	Enabled int64 `json:"enabled"`
}

// ProcessingAlertHint 供前端拉取 Runbook 推荐的活跃告警提示。
type ProcessingAlertHint struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
}

// SummaryDTO Dashboard 聚合响应。
type SummaryDTO struct {
	Alerts           AlertMetrics          `json:"alerts"`
	Executions       ExecutionMetrics      `json:"executions"`
	Assets           AssetMetrics          `json:"assets"`
	Runbooks         RunbookMetrics        `json:"runbooks"`
	ProcessingAlerts []ProcessingAlertHint `json:"processing_alerts"`
}

// GetSummary 聚合首页指标；单块失败时返回 0/空列表，不阻断其它块。
func (s *SummaryService) GetSummary(ctx context.Context) (*SummaryDTO, error) {
	if s == nil || s.stats == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "dashboard summary is not enabled")
	}
	out := &SummaryDTO{
		Executions:       ExecutionMetrics{Recent: []execapp.TaskDTO{}},
		ProcessingAlerts: []ProcessingAlertHint{},
	}

	if active, err := s.stats.CountAlerts(ctx, alertdomain.AlertFilter{ActiveOnly: true}); err == nil {
		out.Alerts.ActiveTotal = active
	}

	if p0, err := s.stats.CountAlerts(ctx, alertdomain.AlertFilter{
		ActiveOnly: true,
		Severities: []alertdomain.AlertSeverity{alertdomain.SeverityP0},
	}); err == nil {
		out.Alerts.P0 = p0
	}

	if p1, err := s.stats.CountAlerts(ctx, alertdomain.AlertFilter{
		ActiveOnly: true,
		Severities: []alertdomain.AlertSeverity{alertdomain.SeverityP1},
	}); err == nil {
		out.Alerts.P1 = p1
	}

	if pending, err := s.stats.CountTasks(ctx, execdomain.TaskFilter{
		Statuses: []execdomain.TaskStatus{execdomain.StatusPendingConfirm},
	}); err == nil {
		out.Executions.PendingConfirm = pending
	}

	if recentTasks, err := s.stats.ListTasks(ctx, execdomain.TaskFilter{
		Limit: defaultRecentExecutions,
	}); err == nil {
		recent := make([]execapp.TaskDTO, 0, len(recentTasks))
		for _, row := range recentTasks {
			recent = append(recent, execapp.ToTaskDTO(row))
		}
		out.Executions.Recent = recent
	}

	if appCount, err := s.stats.CountApplications(ctx); err == nil {
		out.Assets.Applications = appCount
	}

	if resCount, err := s.stats.CountResources(ctx); err == nil {
		out.Assets.Resources = resCount
	}

	if rbTotal, err := s.stats.CountRunbookTemplates(ctx, rbdomain.TemplateFilter{}); err == nil {
		out.Runbooks.Total = rbTotal
	}

	enabled := true
	if rbEnabled, err := s.stats.CountRunbookTemplates(ctx, rbdomain.TemplateFilter{Enabled: &enabled}); err == nil {
		out.Runbooks.Enabled = rbEnabled
	}

	if processing, err := s.stats.ListAlerts(ctx, alertdomain.AlertFilter{
		ActiveOnly: true,
		Statuses: []alertdomain.AlertStatus{
			alertdomain.StatusNew,
			alertdomain.StatusProcessing,
			alertdomain.StatusAcknowledged,
		},
		Limit: 5,
	}); err == nil {
		hints := make([]ProcessingAlertHint, 0, len(processing))
		for _, row := range processing {
			hints = append(hints, ProcessingAlertHint{
				ID: row.ID, Name: row.Name, Severity: string(row.Severity), Status: string(row.Status),
			})
		}
		out.ProcessingAlerts = hints
	}

	return out, nil
}
