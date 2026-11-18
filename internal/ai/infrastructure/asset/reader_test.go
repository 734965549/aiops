package asset

import (
	"context"
	"testing"

	assetapp "github.com/734965549/aiops/internal/asset/application"
)

type fakeResourceReader struct {
	snapshot   *assetapp.ResourceAnalysisSnapshot
	err        error
	userID     string
	resourceID string
}

func (f *fakeResourceReader) GetResourceForAnalysis(_ context.Context, userID, resourceID string) (*assetapp.ResourceAnalysisSnapshot, error) {
	f.userID = userID
	f.resourceID = resourceID
	return f.snapshot, f.err
}

func TestReaderAdapterMapsSnapshot(t *testing.T) {
	reader := &fakeResourceReader{snapshot: &assetapp.ResourceAnalysisSnapshot{
		ResourceID: "res-1", Name: "orders-rds", ApplicationID: "app-1",
		ResourceType: "database", CloudResourceType: "rds", CloudResourceID: "cluster-1",
		Region: "cn-north-4", IntegrationAccountID: "acc-1",
		Labels: map[string]string{"namespace": "SYS.RDS"},
	}}
	adapter := NewReaderAdapter(reader)

	got, err := adapter.GetForAnalysis(context.Background(), "u-1", "cluster-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if reader.userID != "u-1" || reader.resourceID != "cluster-1" {
		t.Fatalf("reader args = %+v", reader)
	}
	if got.ResourceID != "res-1" || got.AccountID != "acc-1" || got.CloudResourceID != "cluster-1" {
		t.Fatalf("context = %+v", got)
	}
	namespace, dimension, value := got.CloudSelector()
	if namespace != "SYS.RDS" || dimension != "rds_cluster_id" || value != "cluster-1" {
		t.Fatalf("selector = %s/%s/%s", namespace, dimension, value)
	}
}
