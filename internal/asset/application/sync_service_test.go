package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
)

type fakeSyncBatchRepo struct {
	rows []domain.SyncBatch
}

func (r *fakeSyncBatchRepo) Create(_ context.Context, batch *domain.SyncBatch) error {
	now := time.Now().UTC()
	batch.CreatedAt = now
	batch.UpdatedAt = now
	r.rows = append(r.rows, *batch)
	return nil
}

func (r *fakeSyncBatchRepo) Update(_ context.Context, batch *domain.SyncBatch) error {
	for i := range r.rows {
		if r.rows[i].BatchID == batch.BatchID {
			r.rows[i] = *batch
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeSyncBatchRepo) GetByID(_ context.Context, batchID string) (*domain.SyncBatch, error) {
	for i := range r.rows {
		if r.rows[i].BatchID == batchID {
			cp := r.rows[i]
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeSyncBatchRepo) List(_ context.Context, filter domain.SyncBatchFilter) ([]domain.SyncBatch, int64, error) {
	out := make([]domain.SyncBatch, 0)
	for _, row := range r.rows {
		if filter.IntegrationAccountID != "" && row.IntegrationAccountID != filter.IntegrationAccountID {
			continue
		}
		out = append(out, row)
	}
	total := int64(len(out))
	if filter.Offset >= len(out) {
		return []domain.SyncBatch{}, total, nil
	}
	end := filter.Offset + filter.Limit
	if end > len(out) {
		end = len(out)
	}
	return out[filter.Offset:end], total, nil
}

type fakeDiscoveryPort struct {
	resources []obsdomain.CloudResource
	errors    map[string]error
}

func (p *fakeDiscoveryPort) ListResources(_ context.Context, _ obsapp.Actor, q obsdomain.AssetDiscoveryQuery) (*obsapp.AssetDiscoveryResult, error) {
	if p.errors != nil {
		if err := p.errors[q.Region+"/"+q.ResourceType]; err != nil {
			return nil, err
		}
	}
	out := make([]obsdomain.CloudResource, 0)
	for _, item := range p.resources {
		if q.ResourceType != "" && item.Type != q.ResourceType {
			continue
		}
		if q.Region != "" && item.Region != q.Region {
			continue
		}
		out = append(out, item)
	}
	return &obsapp.AssetDiscoveryResult{Resources: out, EvidenceID: "ev-fake"}, nil
}

type fakeIntegrationAccountPort struct {
	account *SyncAccountSnapshot
}

func (p *fakeIntegrationAccountPort) ResolveSyncAccount(_ context.Context, accountID string) (*SyncAccountSnapshot, error) {
	if p.account == nil || p.account.AccountID != accountID {
		return nil, domain.ErrNotFound
	}
	cp := *p.account
	return &cp, nil
}

func TestSyncService_TriggerSyncCreatesCloudResources(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-1", Name: "ecs-demo-1", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-1"},
			{ResourceID: "res-fake-rds-1", Name: "rds-demo-1", Type: "rds", Region: "cn-north-4", Status: "ACTIVE", ProviderRef: "rds-demo-1"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	if out.Status != domain.SyncBatchStatusSuccess {
		t.Fatalf("expected success batch, got %+v", out)
	}
	if out.CreatedCount < 2 {
		t.Fatalf("expected at least 2 created resources across types, got created=%d updated=%d", out.CreatedCount, out.UpdatedCount)
	}
	if len(resources.rows) < 2 {
		t.Fatalf("expected synced resources in repo, got %d", len(resources.rows))
	}
	for _, row := range resources.rows {
		if row.Source != domain.ResourceSourceCloudSync {
			t.Fatalf("expected cloud_sync source, got %s", row.Source)
		}
		if row.SyncStatus != domain.SyncStatusActive {
			t.Fatalf("expected active sync status, got %s", row.SyncStatus)
		}
	}
}

func TestSyncService_TriggerSyncMarksStale(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	resources := &fakeResRepo{rows: []domain.Resource{
		{
			ID: "old-res", ApplicationID: appID, Name: "old-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
	}}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-2", Name: "ecs-demo-2", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-2"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	if out.StaleCount < 1 {
		t.Fatalf("expected stale count >= 1, got %+v", out)
	}
	var staleFound bool
	for _, row := range resources.rows {
		if row.CloudResourceID == "gone-ecs" && row.SyncStatus == domain.SyncStatusStale {
			staleFound = true
		}
	}
	if !staleFound {
		t.Fatal("expected gone-ecs to be marked stale")
	}
}

func TestSyncService_TriggerSyncOnlyMarksStaleForSuccessfulScopes(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	resources := &fakeResRepo{rows: []domain.Resource{
		{
			ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
		{
			ID: "old-rds", ApplicationID: appID, Name: "old-rds",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "rds", CloudResourceID: "keep-rds", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
	}}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-2", Name: "ecs-demo-2", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-2"},
		},
		errors: map[string]error{
			"cn-north-4/rds": errors.New("rds temporarily unavailable"),
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	if out.Status != domain.SyncBatchStatusPartial {
		t.Fatalf("expected partial batch, got %+v", out)
	}
	statusByID := map[string]string{}
	for _, row := range resources.rows {
		statusByID[row.ID] = row.SyncStatus
	}
	if statusByID["old-ecs"] != domain.SyncStatusStale {
		t.Fatalf("expected old ecs stale, got %s", statusByID["old-ecs"])
	}
	if statusByID["old-rds"] != domain.SyncStatusActive {
		t.Fatalf("rds scope failed and must remain active, got %s", statusByID["old-rds"])
	}
}
