// Package alert 将 alert 仓储适配为 AI AlertReader port（§9.2）。
package alert

import (
	"context"
	"errors"
	"strings"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	alertdomain "github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// ReaderAdapter 实现 AI AlertReader。
type ReaderAdapter struct {
	alerts alertdomain.AlertRepository
}

// NewReaderAdapter 构造适配器。
func NewReaderAdapter(alerts alertdomain.AlertRepository) *ReaderAdapter {
	return &ReaderAdapter{alerts: alerts}
}

func (a *ReaderAdapter) GetForAnalysis(ctx context.Context, alertID string) (*aiapp.AlertContext, error) {
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
		Environment:     alert.Environment,
		ApplicationID:   alert.ApplicationID,
		ApplicationName: alert.ApplicationName,
		ResourceID:      alert.ResourceID,
		ResourceName:    alert.ResourceName,
		Labels:          labels,
		Annotations:     annotations,
	}, nil
}
