package observability

import (
	"context"

	alertapp "github.com/734965549/aiops/internal/alert/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
)

// PullAdapter 将 alert 定义的拉取端口桥接到 observability 应用服务。
type PullAdapter struct {
	query *obsapp.QueryService
}

func NewPullAdapter(query *obsapp.QueryService) *PullAdapter {
	return &PullAdapter{query: query}
}

var _ alertapp.AlertPullPort = (*PullAdapter)(nil)

func (a *PullAdapter) PullAlarms(ctx context.Context, req alertapp.AlertPullRequest) (*alertapp.AlertPullResult, error) {
	if a == nil || a.query == nil {
		return nil, nil
	}
	pulled, err := a.query.PullAlarmRecords(ctx, obsapp.Actor{}, obsapp.AlarmPullQuery{
		AccountID:   req.AccountID,
		Regions:     req.Regions,
		Cursors:     req.Cursors,
		RoundCutoff: req.RoundCutoff,
		Deadline:    req.Deadline,
	})
	if err != nil {
		return nil, err
	}
	out := &alertapp.AlertPullResult{
		Regions:  append([]string(nil), pulled.PlannedRegions...),
		Scopes:   make([]alertapp.PulledScope, 0, len(pulled.Scopes)),
		Failures: make([]alertapp.AlertPullScopeFailure, 0, len(pulled.Failures)),
	}
	for _, failure := range pulled.Failures {
		out.Failures = append(out.Failures, alertapp.AlertPullScopeFailure{
			Region: failure.Region, ProjectID: failure.ProjectID, Reason: failure.Reason,
		})
	}
	for _, scope := range pulled.Scopes {
		item := alertapp.PulledScope{
			Region: scope.Region, ProjectID: scope.ProjectID,
			WindowEnd: scope.WindowEnd, Complete: scope.Complete,
			FailureCode: scope.FailureCode, FailureReason: scope.FailureReason,
		}
		for _, record := range scope.Records {
			labels := make(map[string]string, len(record.Dimensions))
			for k, v := range record.Dimensions {
				labels[k] = v
			}
			item.Records = append(item.Records, alertapp.PulledAlarm{
				RecordID:        record.RecordID,
				AlarmID:         record.AlarmID,
				Name:            record.Name,
				Status:          record.Status,
				Level:           record.Level,
				Namespace:       record.Namespace,
				MetricName:      record.MetricName,
				Condition:       mapAlarmCondition(record.Condition),
				ResourceID:      record.ResourceID,
				ResourceName:    record.ResourceName,
				ResourceType:    record.ResourceType,
				Dimensions:      labels,
				RuleName:        record.RuleName,
				RuleDescription: record.RuleDescription,
				RuleType:        record.RuleType,
				RuleLookup:      record.RuleLookup,
				FirstSeenAt:     record.FirstSeenAt,
				LastAlarmAt:     record.LastAlarmAt,
				RecoveredAt:     record.RecoveredAt,
			})
		}
		out.Scopes = append(out.Scopes, item)
	}
	return out, nil
}

func mapAlarmCondition(in *obsdomain.AlarmCondition) *alertapp.AlarmCondition {
	if in == nil {
		return nil
	}
	return &alertapp.AlarmCondition{
		Filter:             in.Filter,
		ComparisonOperator: in.ComparisonOperator,
		Threshold:          in.Threshold,
		Unit:               in.Unit,
		PeriodSeconds:      in.PeriodSeconds,
		TriggerCount:       in.TriggerCount,
		SuppressDurationS:  in.SuppressDurationS,
	}
}
