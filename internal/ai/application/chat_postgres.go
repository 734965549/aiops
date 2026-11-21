package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/pkg/logger"
)

func (s *ChatService) collectPostgresObservation(ctx context.Context, userID string, alert *AlertContext) (*ChatObservation, *PostgresEvidenceResult) {
	if s.postgres == nil || !isPostgresLongTransactionAlert(alert) {
		return nil, nil
	}
	query, skip := derivePostgresEvidenceQuery(alert, nil)
	obs := s.checkTool(ctx, userID, chatToolPostgresActivity)
	if !obs.Allowed {
		return &obs, nil
	}
	if skip != "" {
		result := finalizePostgresEvidence(ctx, s.postgresStore, query, &PostgresEvidenceResult{
			Status: EvidenceStatusSkipped, Reason: skip,
		}, s.postgresPayloadMaxBytes)
		obs.Status = result.Status
		obs.Reason = result.Reason
		obs.EvidenceID = result.EvidenceID
		return &obs, result
	}
	metricObs, evidence := s.executePostgresObservation(ctx, userID, query)
	if metricObs == nil {
		obs.Status = EvidenceStatusSkipped
		obs.Reason = "postgres evidence collector is not configured"
		return &obs, nil
	}
	metricObs.Mode = obs.Mode
	return metricObs, evidence
}

func (s *ChatService) executePostgresObservation(ctx context.Context, userID string, query PostgresEvidenceQuery) (*ChatObservation, *PostgresEvidenceResult) {
	if s.postgres == nil {
		return nil, nil
	}
	result, err := s.postgres.Collect(ctx, userID, query)
	if err != nil {
		logger.From(ctx).Warn("postgres evidence collection failed", logger.Error(err))
		result = &PostgresEvidenceResult{Status: EvidenceStatusError, Reason: "postgres evidence collector failed"}
	}
	result = finalizePostgresEvidence(ctx, s.postgresStore, query, result, s.postgresPayloadMaxBytes)
	obs := &ChatObservation{
		ToolCode: chatToolPostgresActivity, Allowed: true,
		Status: result.Status, Reason: result.Reason, EvidenceID: result.EvidenceID,
		Summary: fmt.Sprintf("PostgreSQL 历史会话 %s（%d 会话 / %d 等待事件 / %d Top SQL）",
			result.Status, len(result.Sessions), len(result.WaitEvents), len(result.TopSQL)),
		Data: map[string]any{
			"status": result.Status, "reason": result.Reason,
			"evidence_id": result.EvidenceID, "schema_version": result.SchemaVersion,
			"provider": result.Provider, "trace_id": result.TraceID,
			"project_id": result.ProjectID, "resource_id": result.ResourceID,
			"instance_id": result.InstanceID, "collected_at": result.CollectedAt,
			"window_start": result.WindowStart, "window_end": result.WindowEnd,
			"original_count": result.OriginalCount, "retained_count": result.RetainedCount,
			"truncated":              result.Truncated,
			"session_total_count":    result.SessionTotalCount,
			"wait_event_total_count": result.WaitEventTotalCount,
			"top_sql_total_count":    result.TopSQLTotalCount,
			"missing_evidence":       result.MissingEvidence, "collection_errors": result.CollectionErrors,
			"sessions": result.Sessions, "wait_events": result.WaitEvents, "top_sql": result.TopSQL,
		},
	}
	return obs, result
}

func (s *ChatService) refreshPostgresObservation(ctx context.Context, userID string, session *domain.Session, data *chatContextData, raw map[string]any) bool {
	obs := s.checkTool(ctx, userID, chatToolPostgresActivity)
	if !obs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	query, skip := derivePostgresEvidenceQuery(data.Alert, data.Resource)
	if skip == "" {
		var params struct {
			From  *int64 `json:"from"`
			To    *int64 `json:"to"`
			Limit int    `json:"limit"`
		}
		if _, err := decodeToolParams(raw, &params); err != nil {
			skip = "invalid postgres.activity.query params"
		} else {
			if (params.From == nil) != (params.To == nil) {
				skip = "from and to must be provided together"
			} else if params.From != nil {
				query.From, query.To = *params.From, *params.To
			}
			if params.Limit < 0 {
				skip = "invalid limit"
			} else if params.Limit > 0 {
				query.Limit = params.Limit
			}
		}
	}
	if skip != "" {
		obs.Status = EvidenceStatusSkipped
		obs.Reason = skip
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	metricObs, evidence := s.executePostgresObservation(ctx, userID, query)
	if metricObs == nil {
		obs.Status = EvidenceStatusSkipped
		obs.Reason = "postgres evidence collector is not configured"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	metricObs.Mode = obs.Mode
	s.appendToolResult(ctx, userID, session.SessionID, "", data, *metricObs)
	if evidence == nil || metricObs.Status != EvidenceStatusOK {
		data.Postgres = evidence
		return false
	}
	data.Postgres = evidence
	appendPostgresEvidenceReference(data, evidence)
	return true
}

func appendPostgresEvidenceReference(data *chatContextData, evidence *PostgresEvidenceResult) {
	if data == nil || evidence == nil || evidence.Status != EvidenceStatusOK {
		return
	}
	appendObservabilityEvidenceReference(data, evidence.EvidenceID, "PostgreSQL 历史会话证据 "+evidence.EvidenceID)
}

func postgresObservationEvidenceID(evidence *PostgresEvidenceResult) string {
	if evidence == nil || evidence.Status != EvidenceStatusOK {
		return ""
	}
	return strings.TrimSpace(evidence.EvidenceID)
}
