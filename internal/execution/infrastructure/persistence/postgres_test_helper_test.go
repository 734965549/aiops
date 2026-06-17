package persistence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/734965549/aiops/pkg/config"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

func openTestPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	root := repoRoot(t)
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}

	cfg := config.DatabaseConfig{
		Host:     envOrDefault("AIOPS_TEST_DATABASE_HOST", "127.0.0.1"),
		Port:     envIntOrDefault("AIOPS_TEST_DATABASE_PORT", 5432),
		User:     envOrDefault("AIOPS_TEST_DATABASE_USER", "aiops"),
		Password: envOrDefault("AIOPS_TEST_DATABASE_PASSWORD", "aiops"),
		Name:     envOrDefault("AIOPS_TEST_DATABASE_NAME", "aiops"),
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

func repoRoot(t *testing.T) string {
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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

func deleteExecTasks(t *testing.T, db *gorm.DB, taskIDs ...string) {
	t.Helper()
	if len(taskIDs) == 0 {
		return
	}
	if err := db.Exec("DELETE FROM exec_step WHERE task_id IN ?", taskIDs).Error; err != nil {
		t.Fatalf("cleanup steps: %v", err)
	}
	if err := db.Exec("DELETE FROM exec_task WHERE task_id IN ?", taskIDs).Error; err != nil {
		t.Fatalf("cleanup tasks: %v", err)
	}
}
