package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/pkg/logger"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/google/uuid"
)

const metricEvidenceSchemaVersion = "ai.metric_evidence.v1"

// EvidenceSnapshotStore 持久化已经定稿、即将进入模型输入的证据。
type EvidenceSnapshotStore interface {
	Save(ctx context.Context, query MetricEvidenceQuery, evidence *MetricEvidenceResult) error
}

type evidenceSnapshotOptions struct {
	MaxPointsPerSeries int
	MaxBytes           int
	// Timezone 是本地时间文本与 breach_by_hour 使用的时区；空值回退 UTC。
	Timezone string
}

func finalizeMetricEvidence(ctx context.Context, store EvidenceSnapshotStore, query MetricEvidenceQuery, result *MetricEvidenceResult, opts evidenceSnapshotOptions) *MetricEvidenceResult {
	evidence := cloneMetricEvidenceResult(result)
	if evidence == nil {
		evidence = &MetricEvidenceResult{Status: EvidenceStatusError, Reason: "empty evidence result"}
	}
	if evidence.Status == "" {
		evidence.Status = EvidenceStatusOK
	}
	prepareMetricEvidenceMetadata(evidence, query)
	loc := EvidenceLocation(opts.Timezone)
	evidence.Timezone = loc.String()
	if evidence.Status == EvidenceStatusOK {
		// 峰值时刻与破限片段必须在降采样前基于全量序列计算。
		annotateMetricEvidenceDerivedStats(evidence, query, loc)
	}
	if evidence.TraceID == "" {
		evidence.TraceID = httpx.TraceIDFromCtx(ctx)
	}
	evidence.OriginalCount = metricEvidencePointCount(evidence)
	if evidence.Status != EvidenceStatusOK {
		evidence.Metrics = nil
		evidence.RetainedCount = 0
		if store != nil {
			ensureMetricEvidenceID(evidence)
		}
	} else {
		ensureMetricEvidenceID(evidence)
		if opts.MaxPointsPerSeries > 0 && metricEvidenceNeedsDownsample(evidence.Metrics, opts.MaxPointsPerSeries) {
			evidence.Metrics = downsampleMetricsForPrompt(evidence.Metrics, opts.MaxPointsPerSeries)
			evidence.Truncated = true
			evidence.Sampling = &MetricEvidenceSampling{
				Strategy: "stride_first_last", MaxPointsPerSeries: opts.MaxPointsPerSeries,
			}
		}
		evidence.RetainedCount = metricEvidencePointCount(evidence)
		if evidence.Truncated {
			evidence.Reason = "truncated"
		}
	}
	capped, err := capMetricEvidenceBytes(evidence, opts.MaxBytes)
	if err != nil {
		return metricEvidenceSnapshotFailure(evidence, "evidence_snapshot_encode_failed")
	}
	evidence = capped
	if store == nil {
		if evidence.Status == EvidenceStatusOK {
			return metricEvidenceSnapshotFailure(evidence, "evidence_snapshot_store_disabled")
		}
		clearMetricEvidenceIDs(evidence)
		return evidence
	}
	if err := store.Save(ctx, query, evidence); err != nil {
		logger.From(ctx).Warn("ai metric evidence snapshot persist failed",
			logger.String("evidence_id", evidence.EvidenceID),
			logger.String("status", evidence.Status),
			logger.Error(err))
		if evidence.Status == EvidenceStatusOK {
			return metricEvidenceSnapshotFailure(evidence, "evidence_snapshot_persist_failed")
		}
		clearMetricEvidenceIDs(evidence)
	}
	return evidence
}

func prepareMetricEvidenceMetadata(evidence *MetricEvidenceResult, query MetricEvidenceQuery) {
	evidence.SchemaVersion = metricEvidenceSchemaVersion
	evidence.EvidenceID = strings.TrimSpace(evidence.EvidenceID)
	if evidence.WindowStart == 0 {
		evidence.WindowStart = query.From
	}
	if evidence.WindowEnd == 0 {
		evidence.WindowEnd = query.To
	}
	if evidence.CollectedAt == 0 {
		evidence.CollectedAt = time.Now().Unix()
	}
	if evidence.Period == 0 {
		evidence.Period = query.Period
	}
	if evidence.Aggregator == "" {
		evidence.Aggregator = query.Aggregator
	}
	if evidence.ProjectID == "" {
		evidence.ProjectID = query.ProjectID
	}
	if evidence.ResourceID == "" {
		evidence.ResourceID = query.ResourceID
	}
	if evidence.CloudResourceID == "" {
		evidence.CloudResourceID = query.CloudResourceID
	}
	if evidence.EvidenceID == "" {
		for _, metric := range evidence.Metrics {
			if id := strings.TrimSpace(metric.EvidenceID); id != "" {
				evidence.EvidenceID = id
				break
			}
		}
	}
}

func ensureMetricEvidenceID(evidence *MetricEvidenceResult) {
	if evidence.EvidenceID == "" {
		evidence.EvidenceID = "ev-" + uuid.NewString()
	}
	for i := range evidence.Metrics {
		evidence.Metrics[i].EvidenceID = evidence.EvidenceID
	}
}

func clearMetricEvidenceIDs(evidence *MetricEvidenceResult) {
	evidence.EvidenceID = ""
	for i := range evidence.Metrics {
		evidence.Metrics[i].EvidenceID = ""
	}
}

func metricEvidenceSnapshotFailure(evidence *MetricEvidenceResult, reason string) *MetricEvidenceResult {
	evidence.Status = EvidenceStatusError
	evidence.Reason = reason
	evidence.Metrics = nil
	evidence.RetainedCount = 0
	clearMetricEvidenceIDs(evidence)
	return evidence
}

func cloneMetricEvidenceResult(evidence *MetricEvidenceResult) *MetricEvidenceResult {
	if evidence == nil {
		return nil
	}
	out := *evidence
	if evidence.Sampling != nil {
		sampling := *evidence.Sampling
		out.Sampling = &sampling
	}
	out.Metrics = make([]MetricEvidence, len(evidence.Metrics))
	for i, metric := range evidence.Metrics {
		metric.Points = append([][2]float64(nil), metric.Points...)
		if metric.Labels != nil {
			labels := make(map[string]string, len(metric.Labels))
			for key, value := range metric.Labels {
				labels[key] = value
			}
			metric.Labels = labels
		}
		out.Metrics[i] = metric
	}
	return &out
}

func metricEvidenceNeedsDownsample(metrics []MetricEvidence, max int) bool {
	for _, metric := range metrics {
		if len(metric.Points) > max {
			return true
		}
	}
	return false
}

func capMetricEvidenceBytes(evidence *MetricEvidenceResult, maxBytes int) (*MetricEvidenceResult, error) {
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
	trimmed := cloneMetricEvidenceResult(evidence)
	for i := range trimmed.Metrics {
		trimmed.Metrics[i].Points = nil
	}
	trimmed.Truncated = true
	trimmed.Reason = "truncated"
	trimmed.RetainedCount = 0
	raw, err = json.Marshal(trimmed)
	if err != nil {
		return nil, err
	}
	if len(raw) <= maxBytes {
		return trimmed, nil
	}
	trimmed.Metrics = nil
	raw, err = json.Marshal(trimmed)
	if err != nil {
		return nil, err
	}
	if len(raw) <= maxBytes {
		return trimmed, nil
	}
	return nil, errors.New("evidence snapshot metadata exceeds payload limit")
}
