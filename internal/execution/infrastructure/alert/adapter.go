// Package alert 将 Alert 模块适配为 Execution 的 AlertReader / AlertTimelineWriter。
package alert

import (
	"context"
	"errors"
	"fmt"
	"strings"

	alertapp "github.com/734965549/aiops/internal/alert/application"
	alertdomain "github.com/734965549/aiops/internal/alert/domain"
	execapp "github.com/734965549/aiops/internal/execution/application"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// Adapter 实现 AlertReader 与 AlertTimelineWriter。
type Adapter struct {
	alerts   alertdomain.AlertRepository
	timeline *alertapp.AlertService
}

// NewAdapter 构造适配器。
func NewAdapter(alerts alertdomain.AlertRepository, timeline *alertapp.AlertService) *Adapter {
	return &Adapter{alerts: alerts, timeline: timeline}
}

func (a *Adapter) GetForExecution(ctx context.Context, alertID string) (*execapp.AlertContext, error) {
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
	return &execapp.AlertContext{
		ID:              alert.ID,
		Name:            alert.Name,
		Status:          string(alert.Status),
		Environment:     alert.Environment,
		ApplicationID:   alert.ApplicationID,
		ApplicationName: alert.ApplicationName,
		ResourceID:      alert.ResourceID,
		ResourceType:    alert.ResourceType,
		ResourceName:    alert.ResourceName,
		Labels:          alert.Labels,
		Annotations:     alert.Annotations,
	}, nil
}

func (a *Adapter) RecordExecutionCreated(ctx context.Context, alertID string, actor execapp.Actor, taskID string, payload map[string]any) error {
	return a.record(ctx, alertID, alertdomain.EventExecutionCreated, actor, fmt.Sprintf("创建执行任务 %s", taskID), payload, taskID)
}

func (a *Adapter) RecordExecutionStarted(ctx context.Context, alertID string, actor execapp.Actor, taskID string, payload map[string]any) error {
	return a.record(ctx, alertID, alertdomain.EventExecutionStarted, actor, fmt.Sprintf("开始执行任务 %s", taskID), payload, taskID)
}

func (a *Adapter) RecordExecutionFinished(ctx context.Context, alertID string, actor execapp.Actor, taskID string, payload map[string]any) error {
	status, _ := payload["status"].(string)
	msg := fmt.Sprintf("执行任务 %s 完成", taskID)
	if status == "failed" {
		msg = fmt.Sprintf("执行任务 %s 失败", taskID)
	}
	return a.record(ctx, alertID, alertdomain.EventExecutionFinished, actor, msg, payload, taskID)
}

func (a *Adapter) record(ctx context.Context, alertID string, eventType alertdomain.AlertEventType, actor execapp.Actor, message string, payload map[string]any, taskID string) error {
	if a == nil || a.timeline == nil {
		return nil
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["execution_id"] = taskID
	return a.timeline.RecordExecutionTimelineEvent(ctx, alertID, eventType, alertapp.Actor{
		UserID:      actor.UserID,
		DisplayName: actor.DisplayName,
	}, message, payload)
}
