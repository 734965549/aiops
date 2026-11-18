package asset

import (
	"context"
	"testing"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	assetapp "github.com/734965549/aiops/internal/asset/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeResourceLister struct {
	rows   []assetapp.ResourceAnalysisSnapshot
	err    error
	userID string
	query  assetapp.ListCloudResourcesQuery
	calls  int
}

func (f *fakeResourceLister) ListCloudResourcesForAgent(_ context.Context, userID string, q assetapp.ListCloudResourcesQuery) ([]assetapp.ResourceAnalysisSnapshot, error) {
	f.userID = userID
	f.query = q
	f.calls++
	return f.rows, f.err
}

type fakeAccountResolver struct {
	snapshot *obsdomain.AccountSnapshot
	err      error
}

func (f *fakeAccountResolver) ResolveAccount(_ context.Context, _ string) (*obsdomain.AccountSnapshot, error) {
	return f.snapshot, f.err
}

type fakeResourceAccess struct {
	err  error
	ref  aiapp.AccountRef
	used bool
}

func (f *fakeResourceAccess) Authorize(_ context.Context, _ string, ref aiapp.AccountRef) error {
	f.ref = ref
	f.used = true
	return f.err
}

func TestDiscoveryAdapterMapsResources(t *testing.T) {
	lister := &fakeResourceLister{rows: []assetapp.ResourceAnalysisSnapshot{{
		ResourceID: "res-1", Name: "orders-rds", CloudResourceType: "rds",
		CloudResourceID: "cluster-1", Region: "cn-north-4", IntegrationAccountID: "acc-1",
		Labels: map[string]string{"namespace": "SYS.RDS"},
	}}}
	access := &fakeResourceAccess{}
	adapter := NewDiscoveryAdapter(lister, &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", OwnerTeam: "sre", Regions: []string{"cn-north-4"},
	}}, access, time.Second)

	result, err := adapter.List(context.Background(), "u-1", aiapp.ResourceQuery{
		AccountID: "acc-1", ResourceType: "rds", Region: "cn-north-4", Limit: 20,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusOK || len(result.Resources) != 1 {
		t.Fatalf("result = %+v", result)
	}
	item := result.Resources[0]
	if item.ResourceID != "res-1" || item.CloudResourceID != "cluster-1" || item.AccountID != "acc-1" {
		t.Fatalf("resource = %+v", item)
	}
	if !access.used || access.ref.OwnerTeam != "sre" {
		t.Fatalf("account access not checked: %+v", access)
	}
	if lister.calls != 1 || lister.userID != "u-1" || lister.query.AccountID != "acc-1" || lister.query.ResourceType != "rds" {
		t.Fatalf("lister query = %+v", lister.query)
	}
}

func TestDiscoveryAdapterPermissionDenied(t *testing.T) {
	lister := &fakeResourceLister{}
	access := &fakeResourceAccess{err: apperr.New(apperr.CodePermissionDenied, "denied")}
	adapter := NewDiscoveryAdapter(lister, &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", OwnerTeam: "dba",
	}}, access, time.Second)

	result, err := adapter.List(context.Background(), "u-1", aiapp.ResourceQuery{AccountID: "acc-1"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusPermissionDenied {
		t.Fatalf("status = %q", result.Status)
	}
	if lister.calls != 0 {
		t.Fatal("denied account must not hit asset repository")
	}
}
