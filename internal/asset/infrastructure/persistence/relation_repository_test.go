package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/google/uuid"
)

func TestResourceRelationRepositoryLifecycle(t *testing.T) {
	db := openAssetTestPostgres(t)
	ctx := context.Background()
	appID := uniqueAssetAppID(t)
	sourceID := uniqueAssetResourceID(t)
	targetID := uniqueAssetResourceID(t)
	relationID := uuid.NewString()
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM asset_resource_relation WHERE relation_id = ?", relationID).Error
		deleteTestApplications(t, db, appID)
	})

	apps := NewApplicationRepository(db)
	if err := apps.Create(ctx, &domain.Application{ID: appID, Name: "topology-test"}); err != nil {
		t.Fatalf("create application: %v", err)
	}
	resources := NewResourceRepository(db)
	for _, id := range []string{sourceID, targetID} {
		if err := resources.Create(ctx, &domain.Resource{ID: id, ApplicationID: appID, Name: id}); err != nil {
			t.Fatalf("create resource: %v", err)
		}
	}

	repo := NewResourceRelationRepository(db)
	relation := &domain.ResourceRelation{
		ID: relationID, SourceResourceID: sourceID, TargetResourceID: targetID,
		RelationType: domain.RelationDependsOn, Origin: domain.RelationOriginManual,
	}
	if err := repo.Create(ctx, relation); err != nil {
		t.Fatalf("create relation: %v", err)
	}
	if relation.CreatedAt.IsZero() || relation.UpdatedAt.IsZero() {
		t.Fatal("gorm hooks should maintain timestamps")
	}
	if err := repo.Create(ctx, relation); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("expected unique conflict, got %v", err)
	}
	count, err := repo.CountByResourceID(ctx, sourceID)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	rows, err := repo.ListByResourceIDs(ctx, []string{targetID})
	if err != nil || len(rows) != 1 || rows[0].ID != relationID {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	loaded, err := repo.GetByID(ctx, relationID)
	if err != nil || loaded.SourceResourceID != sourceID {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	if err := repo.Delete(ctx, relationID); err != nil {
		t.Fatalf("delete relation: %v", err)
	}
	if _, err := repo.GetByID(ctx, relationID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
