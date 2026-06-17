package persistence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	"github.com/734965549/aiops/pkg/config"
	"github.com/734965549/aiops/pkg/database"
	"github.com/google/uuid"
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

func alertTestAlert(alertID, sourceID, dedupKey string, lifecycleSeq int, status domain.AlertStatus) *domain.Alert {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &domain.Alert{
		ID:              alertID,
		Source:          string(domain.SourcePrometheusAlertmanager),
		SourceID:        sourceID,
		DedupKey:        dedupKey,
		LifecycleSeq:    lifecycleSeq,
		Name:            "TestAlert",
		Severity:        domain.SeverityP1,
		Status:          status,
		Labels:          map[string]string{},
		Annotations:     map[string]string{},
		OccurrenceCount: 1,
		FirstSeenAt:     now,
		LastSeenAt:      now,
	}
}

func uniqueDedupKey(t *testing.T) string {
	t.Helper()
	return "dk-" + uuid.NewString()
}

func deleteTestAlerts(t *testing.T, db *gorm.DB, alertIDs ...string) {
	t.Helper()
	if len(alertIDs) == 0 {
		return
	}
	if err := db.Exec("DELETE FROM alert_alert WHERE alert_id IN ?", alertIDs).Error; err != nil {
		t.Fatalf("cleanup alerts: %v", err)
	}
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
