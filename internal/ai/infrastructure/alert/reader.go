// Package alert 将 alert 仓储适配为 AI AlertReader port（§9.2）。
package alert

import (
	"context"
	"errors"
	"strings"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	alertdomain "github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// ReaderAdapter 实现 AI AlertReader。
//
// 数据范围是告警读取的唯一入口校验点：/api/ai/analyze-alert 与 AI 助手会话
// 都经此处读取告警快照，因此对象级数据范围在这里统一收口（ops/ai-contract.md §10.6.4）。
type ReaderAdapter struct {
	alerts alertdomain.AlertRepository
	scope  aiapp.AlertScopeAuthorizer
}

// NewReaderAdapter 构造适配器；scope 为空时读取按未装配拒绝（fail-closed）。
func NewReaderAdapter(alerts alertdomain.AlertRepository, scope aiapp.AlertScopeAuthorizer) *ReaderAdapter {
	return &ReaderAdapter{alerts: alerts, scope: scope}
}

func (a *ReaderAdapter) GetForAnalysis(ctx context.Context, userID, alertID string) (*aiapp.AlertContext, error) {
	if a == nil || a.alerts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert reader is not configured")
	}
	alert, err := a.alerts.GetByID(ctx, strings.TrimSpace(alertID))
	if err != nil {
		if errors.Is(err, alertdomain.ErrNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "alert not found")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load alert failed")
	}
	snapshot := toAlertContext(alert)
	// 先加载对象属性、再做数据范围校验：授权需要对象上下文，拒绝时不返回任何快照字段。
	if err := a.authorizeScope(ctx, userID, snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// authorizeScope 按告警对象上下文校验当前用户的数据范围。
// 守卫未装配时拒绝读取，避免漏注入后静默放开告警快照。
func (a *ReaderAdapter) authorizeScope(ctx context.Context, userID string, alert *aiapp.AlertContext) error {
	if a.scope == nil {
		return apperr.New(apperr.CodeUnavailable, "alert data scope guard is not configured")
	}
	return a.scope.AuthorizeAlert(ctx, userID, alert)
}

func toAlertContext(alert *alertdomain.Alert) *aiapp.AlertContext {
	labels := alert.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	annotations := map[string]any{}
	for k, v := range alert.Annotations {
		annotations[k] = v
	}
	return &aiapp.AlertContext{
		ID:              alert.ID,
		Name:            alert.Name,
		Summary:         alert.Summary,
		Description:     alert.Description,
		Severity:        string(alert.Severity),
		Status:          string(alert.Status),
		Source:          alert.Source,
		RuleName:        alert.RuleName,
		BusinessLine:    alert.BusinessLine,
		Environment:     alert.Environment,
		ApplicationID:   alert.ApplicationID,
		ApplicationName: alert.ApplicationName,
		ResourceID:      alert.ResourceID,
		ResourceName:    alert.ResourceName,
		ResourceType:    alert.ResourceType,
		OccurrenceCount: alert.OccurrenceCount,
		FirstSeenAt:     alert.FirstSeenAt.Unix(),
		LastSeenAt:      alert.LastSeenAt.Unix(),
		RecoveredAt:     unixOrZero(alert.RecoveredAt),
		Labels:          labels,
		Annotations:     annotations,
		OwnerUserID:     alert.OwnerUserID,
	}
}

func unixOrZero(t *time.Time) int64 {
	if t == nil || t.IsZero() {
		return 0
	}
	return t.Unix()
}
