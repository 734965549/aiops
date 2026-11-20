package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	httpx "github.com/734965549/aiops/pkg/transport/http"
)

type captureSnapshotRepository struct {
	ref *obsdomain.EvidenceRef
}

func (c *captureSnapshotRepository) SaveSnapshot(_ context.Context, ref *obsdomain.EvidenceRef) error {
	c.ref = ref
	return nil
}

func TestSnapshotStorePersistsExactPayloadAndHash(t *testing.T) {
	repo := &captureSnapshotRepository{}
	store := NewSnapshotStore(repo)
	evidence := &aiapp.MetricEvidenceResult{
		Status: aiapp.EvidenceStatusOK, EvidenceID: "ev-1", SchemaVersion: obsapp.MetricEvidenceSchemaV1,
		Provider: "huawei_cloud", ResourceID: "res-1", CloudResourceID: "ecs-1",
		WindowStart: 1700000000, WindowEnd: 1700003600, CollectedAt: 1700003700,
		Period: 60, Aggregator: "average", OriginalCount: 1, RetainedCount: 1,
		Metrics: []aiapp.MetricEvidence{{
			Metric: "cpu_util", EvidenceID: "ev-1", Points: [][2]float64{{1700000000, 90}},
		}},
	}
	query := evidenceQuery()
	query.ResourceID = "res-1"
	ctx := httpx.ContextWithTraceID(context.Background(), "trace-1")
	if err := store.Save(ctx, query, evidence); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	ref := repo.ref
	if ref == nil || ref.Status != aiapp.EvidenceStatusOK || ref.TraceID != "trace-1" {
		t.Fatalf("unexpected snapshot ref: %+v", ref)
	}
	if ref.PayloadHash != obsapp.HashPayload(ref.Payload) {
		t.Fatal("payload hash does not match persisted payload")
	}
	var decoded aiapp.MetricEvidenceResult
	if err := json.Unmarshal(ref.Payload, &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	want := *evidence
	want.TraceID = "trace-1"
	if !reflect.DeepEqual(&decoded, &want) {
		t.Fatalf("payload differs from model evidence:\nstored=%+v\nmodel=%+v", &decoded, &want)
	}
	var canonical any
	if err := json.Unmarshal(ref.Payload, &canonical); err != nil {
		t.Fatal(err)
	}
	canonicalRaw, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ref.Payload, canonicalRaw) {
		t.Fatal("stored payload is not canonical JSON")
	}
	if ref.QueryParams["metric"] != "cpu_util" || ref.QueryHash == "" || ref.OriginalCount == nil || *ref.OriginalCount != 1 {
		t.Fatalf("unexpected query metadata: %+v", ref)
	}
}
