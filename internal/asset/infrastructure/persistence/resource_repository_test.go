package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/asset/domain"
)

func TestResourceRepository_IntegrationFindBestMatchPod(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	resID := uniqueAssetResourceID(t)
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "order-service", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	if err := resRepo.Create(ctx, &domain.Resource{
		ID: resID, ApplicationID: appID, Name: "order-pod-1",
		ResourceType: "pod", Namespace: "order", Pod: "order-xxx-1",
	}); err != nil {
		t.Fatalf("create resource: %v", err)
	}

	matched, err := resRepo.FindBestMatch(ctx, domain.ResourceMatchQuery{
		ApplicationID: appID,
		Namespace:     "order",
		Pod:           "order-xxx-1",
	})
	if err != nil {
		t.Fatalf("find best match: %v", err)
	}
	if matched.ID != resID {
		t.Fatalf("unexpected resource id: %s", matched.ID)
	}

	if _, err := resRepo.FindBestMatch(ctx, domain.ResourceMatchQuery{
		ApplicationID: appID,
		Pod:           "missing-pod",
	}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestResourceRepository_IntegrationFindBestMatchInstance(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	resID := uniqueAssetResourceID(t)
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "metrics-app", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	if err := resRepo.Create(ctx, &domain.Resource{
		ID: resID, ApplicationID: appID, ResourceType: "host", Instance: "node-1",
	}); err != nil {
		t.Fatalf("create resource: %v", err)
	}

	matched, err := resRepo.FindBestMatch(ctx, domain.ResourceMatchQuery{
		ApplicationID: appID,
		Instance:      "node-1",
	})
	if err != nil {
		t.Fatalf("find by instance: %v", err)
	}
	if matched.ID != resID {
		t.Fatalf("unexpected resource id: %s", matched.ID)
	}
}

func TestResourceRepository_IntegrationListByApplicationID(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "list-res-app", Environment: "dev"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	if err := resRepo.Create(ctx, &domain.Resource{ID: uniqueAssetResourceID(t), ApplicationID: appID, Pod: "p1"}); err != nil {
		t.Fatalf("create resource 1: %v", err)
	}
	if err := resRepo.Create(ctx, &domain.Resource{ID: uniqueAssetResourceID(t), ApplicationID: appID, Pod: "p2"}); err != nil {
		t.Fatalf("create resource 2: %v", err)
	}

	items, err := resRepo.ListByApplicationID(ctx, appID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(items))
	}
}
