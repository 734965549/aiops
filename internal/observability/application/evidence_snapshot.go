package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/734965549/aiops/internal/observability/domain"
	httpx "github.com/734965549/aiops/pkg/transport/http"
)

const MetricEvidenceSchemaV1 = "ai.metric_evidence.v1"

func HashQuery(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return HashPayload(raw), nil
}

func HashPayload(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// CanonicalJSON 统一结构体/map 与数据库 JSONB 回读的 hash 口径。
func CanonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var canonical any
	if err := decoder.Decode(&canonical); err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

func effectiveMetricQuery(q domain.MetricQuery, series []domain.MetricSeries) domain.MetricQuery {
	if len(series) > 0 {
		if series[0].QueryProjectID != "" {
			q.ProjectID = series[0].QueryProjectID
		}
		if series[0].QueryPeriod > 0 {
			q.Period = series[0].QueryPeriod
		}
		if series[0].QueryAggregator != "" {
			q.Aggregator = series[0].QueryAggregator
		}
	}
	q.CollectedAt = time.Now().UTC()
	return q
}

func enrichMetricEvidenceRef(ctx context.Context, ref *domain.EvidenceRef, q domain.MetricQuery) {
	ref.TraceID = httpx.TraceIDFromCtx(ctx)
	ref.Provider = q.Provider
	ref.QueryParams = metricQueryParams(q)
	ref.ResourceID = q.ResourceID
	ref.CloudResourceID = q.CloudResourceID
	start := time.Unix(q.From, 0).UTC()
	end := time.Unix(q.To, 0).UTC()
	collected := q.CollectedAt.UTC()
	ref.WindowStart = &start
	ref.WindowEnd = &end
	ref.CollectedAt = &collected
	ref.Status = "ok"
	if q.SnapshotRequired {
		ref.Status = "collecting"
		ref.Collector = MetricEvidenceSchemaV1
	}
}

func metricQueryParams(q domain.MetricQuery) map[string]any {
	return map[string]any{
		"account_id": q.AccountID,
		"provider":   q.Provider,
		"region":     q.Region,
		"project_id": q.ProjectID,
		"namespace":  q.Namespace,
		"metric":     q.Metric,
		"dimensions": q.Dimensions,
		"from":       q.From,
		"to":         q.To,
		"period":     q.Period,
		"aggregator": q.Aggregator,
	}
}
