// Package alert 将 Alert 模块适配为 Runbook 的 AlertReader。
package alert

import (
	"context"
	"errors"
	"strings"

	alertdomain "github.com/734965549/aiops/internal/alert/domain"
	rbapp "github.com/734965549/aiops/internal/runbook/application"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// Adapter 实现 Runbook AlertReader。
type Adapter struct {
	alerts alertdomain.AlertRepository
}

// NewAdapter 构造适配器。
func NewAdapter(alerts alertdomain.AlertRepository) *Adapter {
	return &Adapter{alerts: alerts}
}

func (a *Adapter) GetForExecution(ctx context.Context, alertID string) (*rbapp.AlertContext, error) {
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
	return &rbapp.AlertContext{
		ID:           alert.ID,
		Name:         alert.Name,
		Status:       string(alert.Status),
		Environment:  alert.Environment,
		ResourceID:   alert.ResourceID,
		ResourceType: alert.ResourceType,
		ResourceName: alert.ResourceName,
		Labels:       alert.Labels,
		Annotations:  alert.Annotations,
	}, nil
}
