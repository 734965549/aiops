package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/asset/domain"
)

func TestApplicationRepository_IntegrationCreateAndFindByNameEnv(t *testing.T) {
	db := openAssetTestPostgres(t)
	repo := NewApplicationRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })
	appName := "payment-service-" + appID[:8]

	app := &domain.Application{
		ID:          appID,
		Name:        appName,
		Environment: "prod",
		Namespace:   "payment",
		Description: "E2E payment app",
	}
	if err := repo.Create(ctx, app); err != nil {
		t.Fatalf("create application: %v", err)
	}

	found, err := repo.FindByNameEnv(ctx, appName, "prod")
	if err != nil {
		t.Fatalf("find by name env: %v", err)
	}
	if found.ID != appID || found.Namespace != "payment" {
		t.Fatalf("unexpected application: %+v", found)
	}

	if _, err := repo.FindByNameEnv(ctx, "missing-app", "prod"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestApplicationRepository_IntegrationCount(t *testing.T) {
	db := openAssetTestPostgres(t)
	repo := NewApplicationRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := repo.Create(ctx, &domain.Application{ID: appID, Name: "count-test-app", Environment: "dev"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count < 1 {
		t.Fatalf("expected count >= 1, got %d", count)
	}
}

func TestApplicationRepository_IntegrationList(t *testing.T) {
	db := openAssetTestPostgres(t)
	repo := NewApplicationRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := repo.Create(ctx, &domain.Application{ID: appID, Name: "list-test-app", Environment: "dev"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, item := range items {
		if item.ID == appID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("created application not in list")
	}
}

func TestApplicationRepository_IntegrationFindByNameEnvExactEnvPriority(t *testing.T) {
	db := openAssetTestPostgres(t)
	repo := NewApplicationRepository(db)
	ctx := context.Background()

	wildcardID := uniqueAssetAppID(t)
	exactID := uniqueAssetAppID(t)
	t.Cleanup(func() { deleteTestApplications(t, db, wildcardID, exactID) })
	appName := "env-priority-app-" + exactID[:8]

	if err := repo.Create(ctx, &domain.Application{
		ID: wildcardID, Name: appName, Environment: "",
	}); err != nil {
		t.Fatalf("create wildcard application: %v", err)
	}
	if err := repo.Create(ctx, &domain.Application{
		ID: exactID, Name: appName, Environment: "prod",
	}); err != nil {
		t.Fatalf("create exact application: %v", err)
	}

	found, err := repo.FindByNameEnv(ctx, appName, "prod")
	if err != nil {
		t.Fatalf("find by name env: %v", err)
	}
	if found.ID != exactID {
		t.Fatalf("expected exact env app %s, got %s", exactID, found.ID)
	}
}
