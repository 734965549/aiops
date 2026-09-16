package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/google/uuid"
)

func TestApplicationDeleteExecutor_DeleteApplicationAtomic_NotFound(t *testing.T) {
	db := openAssetTestPostgres(t)
	exec := NewApplicationDeleteExecutor(db)
	err := exec.DeleteApplicationAtomic(context.Background(), uuid.NewString())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestApplicationDeleteExecutor_DeleteApplicationAtomic_DetachesClosedAlerts(t *testing.T) {
	db := openAssetTestPostgres(t)
	ctx := context.Background()
	appID := "app-del-" + uuid.NewString()
	alertID := "alert-del-" + uuid.NewString()

	appRepo := NewApplicationRepository(db)
	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "delete-test"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM asset_application WHERE application_id = ?", appID).Error
	})

	createTestAlert(t, db, alertID, appID, "closed")
	t.Cleanup(func() { deleteTestAlerts(t, db, alertID) })

	exec := NewApplicationDeleteExecutor(db)
	if err := exec.DeleteApplicationAtomic(ctx, appID); err != nil {
		t.Fatalf("delete application: %v", err)
	}

	var appCount int64
	if err := db.Table("asset_application").Where("application_id = ?", appID).Count(&appCount).Error; err != nil {
		t.Fatalf("count app: %v", err)
	}
	if appCount != 0 {
		t.Fatal("application should be deleted")
	}

	var alertAppID string
	if err := db.Raw("SELECT application_id FROM alert_alert WHERE alert_id = ?", alertID).Scan(&alertAppID).Error; err != nil {
		t.Fatalf("load alert: %v", err)
	}
	if alertAppID != "" {
		t.Fatalf("closed alert should be detached, got application_id=%q", alertAppID)
	}
}

func TestApplicationDeleteExecutor_DeleteApplicationAtomic_BlockedByOpenAlert(t *testing.T) {
	db := openAssetTestPostgres(t)
	ctx := context.Background()
	appID := "app-block-" + uuid.NewString()
	alertID := "alert-block-" + uuid.NewString()

	appRepo := NewApplicationRepository(db)
	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "block-test"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM asset_application WHERE application_id = ?", appID).Error
	})

	createTestAlert(t, db, alertID, appID, "open")
	t.Cleanup(func() { deleteTestAlerts(t, db, alertID) })

	exec := NewApplicationDeleteExecutor(db)
	err := exec.DeleteApplicationAtomic(ctx, appID)
	if !errors.Is(err, domain.ErrHasAlertReferences) {
		t.Fatalf("expected alert reference block, got %v", err)
	}

	var appCount int64
	if err := db.Table("asset_application").Where("application_id = ?", appID).Count(&appCount).Error; err != nil {
		t.Fatalf("count app: %v", err)
	}
	if appCount != 1 {
		t.Fatal("application should remain when delete is blocked")
	}
}
