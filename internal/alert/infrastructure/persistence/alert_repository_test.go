package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/alert/domain"
	"github.com/google/uuid"
)

func TestAlertRepository_IntegrationDedupConstraints(t *testing.T) {
	db := openTestPostgres(t)
	repo := NewAlertRepository(db)
	ctx := context.Background()

	sourceID := "src-" + uuid.NewString()[:8]
	dedupKey := uniqueDedupKey(t)
	id1 := uuid.NewString()
	id2 := uuid.NewString()
	id3 := uuid.NewString()
	t.Cleanup(func() { deleteTestAlerts(t, db, id1, id2, id3) })

	first := alertTestAlert(id1, sourceID, dedupKey, 1, domain.StatusNew)
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first lifecycle: %v", err)
	}

	dupLifecycle := alertTestAlert(id2, sourceID, dedupKey, 1, domain.StatusNew)
	err := repo.Create(ctx, dupLifecycle)
	if err == nil {
		t.Fatal("expected dedup_key+lifecycle_seq unique violation")
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	secondActive := alertTestAlert(id3, sourceID, dedupKey, 2, domain.StatusNew)
	err = repo.Create(ctx, secondActive)
	if err == nil {
		t.Fatal("expected active dedup unique violation while first lifecycle still active")
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists for active dedup, got %v", err)
	}
}

func TestAlertRepository_IntegrationClosedAllowsNewLifecycle(t *testing.T) {
	db := openTestPostgres(t)
	repo := NewAlertRepository(db)
	ctx := context.Background()

	sourceID := "src-" + uuid.NewString()[:8]
	dedupKey := uniqueDedupKey(t)
	id1 := uuid.NewString()
	id2 := uuid.NewString()
	t.Cleanup(func() { deleteTestAlerts(t, db, id1, id2) })

	closed := alertTestAlert(id1, sourceID, dedupKey, 1, domain.StatusClosed)
	if err := repo.Create(ctx, closed); err != nil {
		t.Fatalf("create closed lifecycle: %v", err)
	}

	next := alertTestAlert(id2, sourceID, dedupKey, 2, domain.StatusNew)
	if err := repo.Create(ctx, next); err != nil {
		t.Fatalf("create new lifecycle after closed: %v", err)
	}

	active, err := repo.FindActiveByDedupKey(ctx, sourceID, dedupKey)
	if err != nil {
		t.Fatalf("find active: %v", err)
	}
	if active.ID != id2 || active.LifecycleSeq != 2 {
		t.Fatalf("unexpected active alert: id=%s seq=%d", active.ID, active.LifecycleSeq)
	}

	if _, err := repo.FindActiveByDedupKey(ctx, sourceID, dedupKey+"-missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found for unknown dedup key, got %v", err)
	}
}

func TestAlertRepository_IntegrationFindActiveExcludesClosed(t *testing.T) {
	db := openTestPostgres(t)
	repo := NewAlertRepository(db)
	ctx := context.Background()

	sourceID := "src-" + uuid.NewString()[:8]
	dedupKey := uniqueDedupKey(t)
	id1 := uuid.NewString()
	t.Cleanup(func() { deleteTestAlerts(t, db, id1) })

	closed := alertTestAlert(id1, sourceID, dedupKey, 1, domain.StatusClosed)
	if err := repo.Create(ctx, closed); err != nil {
		t.Fatalf("create closed alert: %v", err)
	}

	if _, err := repo.FindActiveByDedupKey(ctx, sourceID, dedupKey); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected closed alert excluded from active lookup, got %v", err)
	}

	maxSeq, err := repo.MaxLifecycleSeq(ctx, dedupKey)
	if err != nil {
		t.Fatalf("max lifecycle seq: %v", err)
	}
	if maxSeq != 1 {
		t.Fatalf("expected max lifecycle seq 1, got %d", maxSeq)
	}
}
