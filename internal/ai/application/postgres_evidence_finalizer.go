package application

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/734965549/aiops/pkg/logger"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/google/uuid"
)

func finalizePostgresEvidence(ctx context.Context, store PostgresEvidenceStore, query PostgresEvidenceQuery, result *PostgresEvidenceResult, maxBytes int) *PostgresEvidenceResult {
	evidence := clonePostgresEvidence(result)
	if evidence == nil {
		evidence = &PostgresEvidenceResult{Status: EvidenceStatusError, Reason: "empty postgres evidence result"}
	}
	if evidence.Status == "" {
		evidence.Status = EvidenceStatusOK
	}
	preparePostgresEvidenceMetadata(ctx, evidence, query)
	evidence.OriginalCount = postgresRecordCount(evidence)
	evidence.RetainedCount = evidence.OriginalCount
	if evidence.Status != EvidenceStatusOK {
		evidence.Sessions = nil
		evidence.WaitEvents = nil
		evidence.TopSQL = nil
		evidence.RetainedCount = 0
		evidence.MissingEvidence = appendMissingEvidence(evidence.MissingEvidence, "postgres_activity")
	} else {
		sort.SliceStable(evidence.Sessions, func(i, j int) bool {
			left, right := evidence.Sessions[i], evidence.Sessions[j]
			if postgresSessionHasLockWait(left) != postgresSessionHasLockWait(right) {
				return postgresSessionHasLockWait(left)
			}
			if left.TransactionExecutionTime != right.TransactionExecutionTime {
				return left.TransactionExecutionTime > right.TransactionExecutionTime
			}
			if left.SampleTime != right.SampleTime {
				return left.SampleTime > right.SampleTime
			}
			return left.SessionID < right.SessionID
		})
	}
	capped, err := capPostgresEvidenceBytes(evidence, maxBytes)
	if err != nil {
		return postgresEvidenceFailure(evidence, "postgres_snapshot_encode_failed")
	}
	evidence = capped
	if store == nil {
		if evidence.Status == EvidenceStatusOK {
			return postgresEvidenceFailure(evidence, "postgres_snapshot_store_disabled")
		}
		clearPostgresEvidenceID(evidence)
		return evidence
	}
	if err := store.SavePostgres(ctx, query, evidence); err != nil {
		logger.From(ctx).Warn("postgres evidence snapshot persist failed",
			logger.String("evidence_id", evidence.EvidenceID), logger.String("status", evidence.Status), logger.Error(err))
		if evidence.Status == EvidenceStatusOK {
			return postgresEvidenceFailure(evidence, "postgres_snapshot_persist_failed")
		}
		clearPostgresEvidenceID(evidence)
	}
	return evidence
}

func preparePostgresEvidenceMetadata(ctx context.Context, evidence *PostgresEvidenceResult, query PostgresEvidenceQuery) {
	evidence.SchemaVersion = PostgresEvidenceSchemaV1
	evidence.EvidenceID = strings.TrimSpace(evidence.EvidenceID)
	if evidence.EvidenceID == "" {
		evidence.EvidenceID = "ev-" + uuid.NewString()
	}
	if evidence.TraceID == "" {
		evidence.TraceID = httpx.TraceIDFromCtx(ctx)
	}
	if evidence.ProjectID == "" {
		evidence.ProjectID = query.ProjectID
	}
	if evidence.ResourceID == "" {
		evidence.ResourceID = query.ResourceID
	}
	if evidence.InstanceID == "" {
		evidence.InstanceID = query.InstanceID
	}
	if evidence.WindowStart == 0 {
		evidence.WindowStart = query.From
	}
	if evidence.WindowEnd == 0 {
		evidence.WindowEnd = query.To
	}
	if evidence.CollectedAt == 0 {
		evidence.CollectedAt = time.Now().Unix()
	}
}

func clonePostgresEvidence(evidence *PostgresEvidenceResult) *PostgresEvidenceResult {
	if evidence == nil {
		return nil
	}
	out := *evidence
	out.Sessions = append([]PostgresSession(nil), evidence.Sessions...)
	out.WaitEvents = append([]PostgresWaitEvent(nil), evidence.WaitEvents...)
	out.TopSQL = append([]PostgresTopSQL(nil), evidence.TopSQL...)
	out.MissingEvidence = append([]string(nil), evidence.MissingEvidence...)
	out.CollectionErrors = append([]string(nil), evidence.CollectionErrors...)
	return &out
}

func postgresRecordCount(evidence *PostgresEvidenceResult) int {
	return len(evidence.Sessions) + len(evidence.WaitEvents) + len(evidence.TopSQL)
}

func capPostgresEvidenceBytes(evidence *PostgresEvidenceResult, maxBytes int) (*PostgresEvidenceResult, error) {
	if maxBytes <= 0 {
		maxBytes = maxEvidencePayloadBytes
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return nil, err
	}
	if len(raw) <= maxBytes {
		return evidence, nil
	}
	trimmed := clonePostgresEvidence(evidence)
	trimmed.Truncated = true
	trimmed.Reason = "truncated"
	trimmed.TopSQL = nil
	trimmed.RetainedCount = postgresRecordCount(trimmed)
	raw, err = json.Marshal(trimmed)
	if err != nil {
		return nil, err
	}
	if len(raw) <= maxBytes {
		return trimmed, nil
	}
	trimmed.WaitEvents = nil
	trimmed.RetainedCount = postgresRecordCount(trimmed)
	for {
		raw, err = json.Marshal(trimmed)
		if err != nil {
			return nil, err
		}
		if len(raw) <= maxBytes {
			return trimmed, nil
		}
		if len(trimmed.Sessions) == 0 {
			return nil, errors.New("postgres evidence metadata exceeds payload limit")
		}
		trimmed.Sessions = trimmed.Sessions[:len(trimmed.Sessions)-1]
		trimmed.RetainedCount = postgresRecordCount(trimmed)
	}
}

func postgresEvidenceFailure(evidence *PostgresEvidenceResult, reason string) *PostgresEvidenceResult {
	evidence.Status = EvidenceStatusError
	evidence.Reason = reason
	evidence.Sessions = nil
	evidence.WaitEvents = nil
	evidence.TopSQL = nil
	evidence.RetainedCount = 0
	clearPostgresEvidenceID(evidence)
	return evidence
}

func clearPostgresEvidenceID(evidence *PostgresEvidenceResult) {
	evidence.EvidenceID = ""
}

func postgresSessionHasLockWait(session PostgresSession) bool {
	if strings.EqualFold(strings.TrimSpace(session.WaitEventType), "lock") {
		return true
	}
	return postgresHasBlockedProcess(session.BlockedProcessID)
}

func postgresHasBlockedProcess(value string) bool {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]{}\" \t\r\n")
	return value != "" && value != "0" && value != "-1"
}

func appendMissingEvidence(existing []string, missing string) []string {
	for _, item := range existing {
		if item == missing {
			return existing
		}
	}
	return append(existing, missing)
}
