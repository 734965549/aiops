package persistence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/734965549/aiops/pkg/config"
	"github.com/734965549/aiops/pkg/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func openAssetTestPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	root := assetRepoRoot(t)
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}

	cfg := config.DatabaseConfig{
		Host:     assetEnvOrDefault("AIOPS_TEST_DATABASE_HOST", "127.0.0.1"),
		Port:     assetEnvIntOrDefault("AIOPS_TEST_DATABASE_PORT", 5432),
		User:     assetEnvOrDefault("AIOPS_TEST_DATABASE_USER", "aiops"),
		Password: assetEnvOrDefault("AIOPS_TEST_DATABASE_PASSWORD", "aiops"),
		Name:     assetEnvOrDefault("AIOPS_TEST_DATABASE_NAME", "aiops"),
		SSLMode:  "disable",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := database.NewPostgres(ctx, cfg, "Asia/Shanghai")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() { database.ClosePostgres(db) })

	if err := database.RunMigrations(ctx, db, database.MigrateOptions{Dir: database.ResolveMigrationDir()}); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func assetRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func deleteTestApplications(t *testing.T, db *gorm.DB, appIDs ...string) {
	t.Helper()
	if len(appIDs) == 0 {
		return
	}
	if err := db.Exec("DELETE FROM asset_resource WHERE application_id IN ?", appIDs).Error; err != nil {
		t.Fatalf("cleanup resources: %v", err)
	}
	if err := db.Exec("DELETE FROM asset_application WHERE application_id IN ?", appIDs).Error; err != nil {
		t.Fatalf("cleanup applications: %v", err)
	}
}

func uniqueAssetAppID(t *testing.T) string {
	t.Helper()
	return uuid.NewString()
}

func uniqueAssetResourceID(t *testing.T) string {
	t.Helper()
	return uuid.NewString()
}

// createTestSyncBatch 创建一个 running 状态且租约有效的同步批次，用于仓储测试。
func createTestSyncBatch(t *testing.T, db *gorm.DB, accountID, batchID, fencingToken string) *domain.SyncBatch {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	lease := now.Add(10 * time.Minute)
	batch := &domain.SyncBatch{
		BatchID:              batchID,
		IntegrationAccountID: accountID,
		Provider:             "huawei_ces",
		Status:               domain.SyncBatchStatusRunning,
		StartedAt:            now,
		FencingToken:         fencingToken,
		LeaseExpiresAt:       &lease,
	}
	repo := NewSyncBatchRepository(db)
	if err := repo.Create(ctx, batch); err != nil {
		t.Fatalf("create sync batch %s: %v", batchID, err)
	}
	return batch
}

func assetEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func assetEnvIntOrDefault(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
