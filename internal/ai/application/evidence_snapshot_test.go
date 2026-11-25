package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	httpx "github.com/734965549/aiops/pkg/transport/http"
)

func TestFinalizeMetricEvidencePersistsExactModelSnapshot(t *testing.T) {
	points := make([][2]float64, 720)
	for i := range points {
		points[i] = [2]float64{float64(1700000000 + i*3600), float64(i)}
	}
	input := &MetricEvidenceResult{Status: EvidenceStatusOK, Metrics: []MetricEvidence{{
		Metric: "cpu_util", EvidenceID: "ev-1", Points: points,
	}}}
	store := &fakeEvidenceSnapshotStore{}
	out := finalizeMetricEvidence(context.Background(), store, MetricEvidenceQuery{
		From: 1700000000, To: 1700003600, Period: 3600, Aggregator: "average",
	}, input, evidenceSnapshotOptions{MaxPointsPerSeries: chatMetricPromptMaxPoints, MaxBytes: maxEvidencePayloadBytes})

	if out.Status != EvidenceStatusOK || out.EvidenceID != "ev-1" {
		t.Fatalf("unexpected finalized evidence: %+v", out)
	}
	if out.OriginalCount != 720 || out.RetainedCount != chatMetricPromptMaxPoints || !out.Truncated || out.Sampling == nil {
		t.Fatalf("unexpected truncation metadata: %+v", out)
	}
	if len(input.Metrics[0].Points) != 720 {
		t.Fatal("finalization must not mutate the collector result")
	}
	if !reflect.DeepEqual(out, store.saved) {
		t.Fatal("persisted snapshot differs from the object returned for model input")
	}
	storedJSON, err := json.Marshal(store.saved)
	if err != nil {
		t.Fatal(err)
	}
	modelJSON, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(storedJSON) != string(modelJSON) {
		t.Fatal("persisted JSON differs from model JSON")
	}
}

func TestFinalizeMetricEvidenceFailsClosedWhenPersistFails(t *testing.T) {
	store := &fakeEvidenceSnapshotStore{err: errors.New("database unavailable")}
	out := finalizeMetricEvidence(context.Background(), store, MetricEvidenceQuery{}, &MetricEvidenceResult{
		Status:  EvidenceStatusOK,
		Metrics: []MetricEvidence{{Metric: "cpu_util", EvidenceID: "ev-1", Points: [][2]float64{{1, 90}}}},
	}, evidenceSnapshotOptions{})

	if out.Status != EvidenceStatusError || out.Reason != "evidence_snapshot_persist_failed" {
		t.Fatalf("unexpected failure result: %+v", out)
	}
	if len(out.Metrics) != 0 || len(evidenceIDs(out)) != 0 || len(evidenceReferences(out)) != 0 {
		t.Fatalf("unpersisted evidence must not support model facts: %+v", out)
	}
}

func TestFinalizeMetricEvidenceRecordsFailureWithoutSupportingReference(t *testing.T) {
	store := &fakeEvidenceSnapshotStore{}
	out := finalizeMetricEvidence(context.Background(), store, MetricEvidenceQuery{}, &MetricEvidenceResult{
		Status: EvidenceStatusPermissionDenied, Reason: "denied",
	}, evidenceSnapshotOptions{})

	if out.Status != EvidenceStatusPermissionDenied || out.EvidenceID == "" || store.saved == nil {
		t.Fatalf("failure evidence was not persisted: %+v", out)
	}
	if len(evidenceIDs(out)) != 1 || len(evidenceReferences(out)) != 0 {
		t.Fatalf("failure evidence must be traceable but not supportive: %+v", out)
	}
}

func TestFinalizeMetricEvidenceEncodeFailureDoesNotPanic(t *testing.T) {
	out := finalizeMetricEvidence(context.Background(), &fakeEvidenceSnapshotStore{}, MetricEvidenceQuery{}, &MetricEvidenceResult{
		Status:  EvidenceStatusOK,
		Metrics: []MetricEvidence{{Metric: "cpu_util", Points: [][2]float64{{1, math.NaN()}}}},
	}, evidenceSnapshotOptions{})
	if out.Status != EvidenceStatusError || out.Reason != "evidence_snapshot_encode_failed" || len(out.Metrics) != 0 {
		t.Fatalf("unexpected encode failure result: %+v", out)
	}
}

func TestFinalizeMetricEvidenceFinalJSONRespectsLimit(t *testing.T) {
	for _, limit := range []int{512, 1024, 2048, 4096, 8192} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			points := make([][2]float64, 150)
			for i := range points {
				points[i] = [2]float64{float64(1700000000 + i*60), float64(i)}
			}
			out := finalizeMetricEvidence(context.Background(), &fakeEvidenceSnapshotStore{}, MetricEvidenceQuery{}, &MetricEvidenceResult{
				Status:  EvidenceStatusOK,
				Metrics: []MetricEvidence{{Metric: "cpu_util", EvidenceID: "ev-1", Points: points}},
			}, evidenceSnapshotOptions{MaxPointsPerSeries: 96, MaxBytes: limit})
			if out.Status != EvidenceStatusOK {
				t.Fatalf("finalization failed for limit %d: %+v", limit, out)
			}
			raw, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) > limit {
				t.Fatalf("final payload size=%d exceeds limit=%d", len(raw), limit)
			}
			if out.RetainedCount != metricEvidencePointCount(out) {
				t.Fatalf("retained_count=%d actual=%d", out.RetainedCount, metricEvidencePointCount(out))
			}
		})
	}
}

func TestFinalizeMetricEvidenceNonOKSnapshotAlsoRespectsLimit(t *testing.T) {
	store := &fakeEvidenceSnapshotStore{}
	out := finalizeMetricEvidence(context.Background(), store, MetricEvidenceQuery{}, &MetricEvidenceResult{
		Status: EvidenceStatusPermissionDenied, Reason: strings.Repeat("x", 1024),
	}, evidenceSnapshotOptions{MaxBytes: 128})
	if out.Status != EvidenceStatusError || out.Reason != "evidence_snapshot_encode_failed" || store.calls != 0 {
		t.Fatalf("oversized failure snapshot must not persist: %+v calls=%d", out, store.calls)
	}
}

func TestFilterAnswerEvidenceRejectsNonOKAndUnknownIDs(t *testing.T) {
	data := chatContextData{
		Observations: []ChatObservation{
			{ToolCode: chatToolMetricsQuery, Status: EvidenceStatusPermissionDenied, EvidenceID: "ev-denied"},
			{ToolCode: chatToolMetricsQuery, Status: EvidenceStatusOK, EvidenceID: "ev-ok"},
		},
	}
	lines := filterAnswerEvidence([]string{
		"observability evidence ev-denied",
		"ev-denied",
		"根因可能是 ev-denied",
		"observability evidence ev-unknown",
		"observability evidence ev-ok",
		"CPU 峰值 90%",
	}, data)
	if !reflect.DeepEqual(lines, []string{"observability evidence ev-ok", "CPU 峰值 90%"}) {
		t.Fatalf("unexpected filtered evidence: %+v", lines)
	}
}

func TestObservationEvidenceLinesOnlySupportsOKSnapshots(t *testing.T) {
	data := chatContextData{Evidence: &MetricEvidenceResult{
		Status: EvidenceStatusPermissionDenied, EvidenceID: "ev-denied",
	}}
	if lines := observationEvidenceLines(data); len(lines) != 0 {
		t.Fatalf("failure snapshot must not support answer evidence: %+v", lines)
	}
	data.Evidence = &MetricEvidenceResult{Status: EvidenceStatusOK, EvidenceID: "ev-ok"}
	if lines := observationEvidenceLines(data); len(lines) != 1 || lines[0] != "observability evidence ev-ok" {
		t.Fatalf("ok snapshot evidence missing: %+v", lines)
	}
}

func TestFinalizeMetricEvidenceBreachHourProfile(t *testing.T) {
	threshold := 600.0
	// UTC 18:00 对应 Asia/Shanghai 次日 02:00。
	base := time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC).Unix()
	points := [][2]float64{
		{float64(base), 700},
		{float64(base + 3600), 100},
		{float64(base + 24*3600), 800},
		{float64(base + 25*3600), 100},
		{float64(base + 48*3600), 900},
	}
	input := &MetricEvidenceResult{Status: EvidenceStatusOK, Metrics: []MetricEvidence{{
		Metric: "rds046_replication_lag", EvidenceID: "ev-1", Points: points,
	}}}
	out := finalizeMetricEvidence(context.Background(), &fakeEvidenceSnapshotStore{}, MetricEvidenceQuery{
		From: base, To: base + 48*3600, Period: 3600, Aggregator: "max",
		Threshold: &threshold, Operator: ">",
	}, input, evidenceSnapshotOptions{
		MaxPointsPerSeries: 288, MaxBytes: maxEvidencePayloadBytes, Timezone: "Asia/Shanghai",
	})
	if out.Timezone != "Asia/Shanghai" {
		t.Fatalf("timezone = %q", out.Timezone)
	}
	if len(out.Metrics) != 1 {
		t.Fatalf("metrics = %d", len(out.Metrics))
	}
	metric := out.Metrics[0]
	if metric.BreachPoints != 3 || metric.BreachDays != 3 {
		t.Fatalf("breach stats = %+v", metric)
	}
	if len(metric.BreachByHour) != 24 || metric.BreachByHour[2] != 3 {
		t.Fatalf("breach_by_hour = %v", metric.BreachByHour)
	}
	if !strings.Contains(metric.BreachTopHours, "02:00-02:59=3") {
		t.Fatalf("breach_top_hours = %q", metric.BreachTopHours)
	}
	if !strings.Contains(metric.PeakAtLocal, "2026-11-23 02:00") {
		t.Fatalf("peak_at_local = %q", metric.PeakAtLocal)
	}
	if !strings.Contains(metric.BreachFirstAtLocal, "2026-11-21 02:00") {
		t.Fatalf("breach_first_at_local = %q", metric.BreachFirstAtLocal)
	}
}

func TestFinalizeMetricEvidenceUsesRequestTrace(t *testing.T) {
	ctx := httpx.ContextWithTraceID(context.Background(), "trace-finalize")
	out := finalizeMetricEvidence(ctx, &fakeEvidenceSnapshotStore{}, MetricEvidenceQuery{}, &MetricEvidenceResult{
		Status: EvidenceStatusOK,
	}, evidenceSnapshotOptions{})
	if out.TraceID != "trace-finalize" {
		t.Fatalf("trace_id = %q", out.TraceID)
	}
}

func TestFinalizeMetricEvidenceRequiresSnapshotStore(t *testing.T) {
	out := finalizeMetricEvidence(context.Background(), nil, MetricEvidenceQuery{}, &MetricEvidenceResult{
		Status: EvidenceStatusOK, Metrics: []MetricEvidence{{Metric: "cpu_util", EvidenceID: "ev-1"}},
	}, evidenceSnapshotOptions{})
	if out.Status != EvidenceStatusError || out.Reason != "evidence_snapshot_store_disabled" || len(out.Metrics) != 0 {
		t.Fatalf("missing store must fail closed: %+v", out)
	}
}

func TestFinalizeMetricEvidenceKeepsDerivedStatsAfterDownsample(t *testing.T) {
	points := make([][2]float64, 150)
	for i := range points {
		value := 70.0
		if i == 100 {
			value = 95
		}
		points[i] = [2]float64{float64(1700000000 + i*60), value}
	}
	threshold := 80.0
	input := &MetricEvidenceResult{Status: EvidenceStatusOK, Metrics: []MetricEvidence{{
		Metric: "cpu_util", EvidenceID: "ev-1", Points: points,
	}}}
	out := finalizeMetricEvidence(context.Background(), &fakeEvidenceSnapshotStore{}, MetricEvidenceQuery{
		From: 1700000000, To: 1700003600, Period: 60, Aggregator: "average",
		Threshold: &threshold, Operator: ">",
	}, input, evidenceSnapshotOptions{MaxPointsPerSeries: 96, MaxBytes: maxEvidencePayloadBytes})

	if out.Status != EvidenceStatusOK || !out.Truncated || len(out.Metrics) != 1 {
		t.Fatalf("unexpected evidence: %+v", out)
	}
	metric := out.Metrics[0]
	if len(metric.Points) != 96 {
		t.Fatalf("points = %d, want downsample to 96", len(metric.Points))
	}
	if metric.PeakAt == nil || *metric.PeakAt != 1700000000+100*60 {
		t.Fatalf("peak_at must come from full series, got %+v", metric.PeakAt)
	}
	if metric.BreachPoints != 1 || metric.BreachEpisodes != 1 ||
		metric.BreachDurationSeconds != 60 || metric.LongestBreachSeconds != 60 {
		t.Fatalf("breach stats = %+v", metric)
	}
}
