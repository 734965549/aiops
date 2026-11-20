package observability

import (
	"context"
	"errors"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	httpx "github.com/734965549/aiops/pkg/transport/http"
)

// SnapshotStore 将 AI 定稿结果写回 Observability 证据账本。
type SnapshotStore struct {
	repo obsdomain.EvidenceSnapshotRepository
}

func NewSnapshotStore(repo obsdomain.EvidenceSnapshotRepository) *SnapshotStore {
	return &SnapshotStore{repo: repo}
}

var _ aiapp.EvidenceSnapshotStore = (*SnapshotStore)(nil)

func (s *SnapshotStore) Save(ctx context.Context, query aiapp.MetricEvidenceQuery, evidence *aiapp.MetricEvidenceResult) error {
	if s == nil || s.repo == nil {
		return errors.New("evidence snapshot repository is not configured")
	}
	if evidence == nil {
		return errors.New("evidence snapshot is nil")
	}
	payloadEvidence := *evidence
	if payloadEvidence.TraceID == "" {
		payloadEvidence.TraceID = httpx.TraceIDFromCtx(ctx)
	}
	payload, err := obsapp.CanonicalJSON(&payloadEvidence)
	if err != nil {
		return err
	}
	params := snapshotQueryParams(query)
	queryHash, err := obsapp.HashQuery(params)
	if err != nil {
		return err
	}
	collectedAt := time.Unix(evidence.CollectedAt, 0).UTC()
	originalCount := evidence.OriginalCount
	traceID := payloadEvidence.TraceID
	ref := &obsdomain.EvidenceRef{
		EvidenceID: evidence.EvidenceID, TraceID: traceID, AccountID: query.AccountID,
		EvidenceType: "metrics", Provider: evidence.Provider, Collector: obsapp.MetricEvidenceSchemaV1,
		QueryType: "metrics", QueryHash: queryHash, QueryParams: params,
		ResourceID: evidence.ResourceID, CloudResourceID: evidence.CloudResourceID,
		WindowStart: evidenceTime(evidence.WindowStart), WindowEnd: evidenceTime(evidence.WindowEnd),
		CollectedAt: &collectedAt, Status: evidence.Status, Reason: truncateEvidenceReason(evidence.Reason),
		Summary: map[string]any{
			"metric": query.Metric, "namespace": query.Namespace,
			"series_count": len(evidence.Metrics), "point_count": evidence.RetainedCount,
			"original_count": evidence.OriginalCount, "truncated": evidence.Truncated,
		},
		Payload: payload, PayloadHash: obsapp.HashPayload(payload),
		SchemaVersion: evidence.SchemaVersion, Truncated: evidence.Truncated, OriginalCount: &originalCount,
	}
	return s.repo.SaveSnapshot(ctx, ref)
}

func snapshotQueryParams(query aiapp.MetricEvidenceQuery) map[string]any {
	params := map[string]any{
		"account_id": query.AccountID,
		"region":     query.Region,
		"project_id": query.ProjectID,
		"namespace":  query.Namespace,
		"metric":     query.Metric,
		"dimensions": query.Dimensions,
		"from":       query.From,
		"to":         query.To,
		"period":     query.Period,
		"aggregator": query.Aggregator,
	}
	if query.Threshold != nil {
		params["threshold"] = *query.Threshold
	}
	if query.Operator != "" {
		params["operator"] = query.Operator
	}
	return params
}

func evidenceTime(unix int64) *time.Time {
	if unix <= 0 {
		return nil
	}
	value := time.Unix(unix, 0).UTC()
	return &value
}

func truncateEvidenceReason(reason string) string {
	runes := []rune(reason)
	if len(runes) <= 512 {
		return reason
	}
	return string(runes[:512])
}
