package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/alert/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestAlertSourceRepository_IntegrationSourceIDUnique(t *testing.T) {
	db := openTestPostgres(t)
	repo := NewAlertSourceRepository(db)
	ctx := context.Background()

	sourceID := "src-" + uuid.NewString()[:8]
	t.Cleanup(func() { deleteTestSource(t, db, sourceID) })

	src := &domain.AlertSource{
		ID:         sourceID,
		Name:       "Prod AM",
		Type:       domain.SourcePrometheusAlertmanager,
		Enabled:    true,
		SecretHash: "hash",
	}
	if err := repo.Create(ctx, src); err != nil {
		t.Fatalf("create source: %v", err)
	}

	dup := &domain.AlertSource{
		ID:         sourceID,
		Name:       "Dup",
		Type:       domain.SourcePrometheusAlertmanager,
		Enabled:    true,
		SecretHash: "hash2",
	}
	err := repo.Create(ctx, dup)
	if err == nil {
		t.Fatal("expected source_id unique violation")
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	got, err := repo.GetByID(ctx, sourceID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if got.Name != "Prod AM" {
		t.Fatalf("expected original source preserved, got name=%q", got.Name)
	}
}

func deleteTestSource(t *testing.T, db *gorm.DB, sourceID string) {
	t.Helper()
	if err := db.Exec("DELETE FROM alert_source WHERE source_id = ?", sourceID).Error; err != nil {
		t.Fatalf("cleanup source: %v", err)
	}
}
