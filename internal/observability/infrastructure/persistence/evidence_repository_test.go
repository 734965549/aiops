package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	"github.com/734965549/aiops/pkg/config"
	"github.com/734965549/aiops/pkg/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestEvidenceRepositorySnapshotLifecycle(t *testing.T) {
	db := openEvidenceTestPostgres(t)
	repo := NewEvidenceRepository(db)
	ctx := context.Background()
	evidenceID := "ev-" + uuid.NewString()
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM obs_evidence_ref WHERE evidence_id = ?", evidenceID).Error
	})
	start := time.Unix(1700000000, 0).UTC()
	end := start.Add(time.Hour)
	collected := end.Add(time.Minute)
	base := &domain.EvidenceRef{
		EvidenceID: evidenceID, TraceID: "trace-1", AccountID: "acc-1",
		EvidenceType: "metrics", Provider: "huawei_cloud", Collector: obsapp.MetricEvidenceSchemaV1,
		QueryType: "metrics", QueryHash: "query-hash-1",
		QueryParams: map[string]any{"metric": "cpu_util", "period": 60},
		ResourceID:  "res-1", CloudResourceID: "ecs-1",
		WindowStart: &start, WindowEnd: &end, CollectedAt: &collected,
		Status: "collecting", Summary: map[string]any{"series_count": 1},
	}
	if err := repo.Create(ctx, base); err != nil {
		t.Fatalf("create collecting evidence: %v", err)
	}
	payload, err := json.Marshal(map[string]any{
		"status": "ok", "evidence_id": evidenceID,
		"original_count": 1, "retained_count": 1,
		"metrics": []any{map[string]any{"metric": "cpu_util", "points": []any{[]any{1700000000, 1e-7}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	originalCount := 1
	snapshot := *base
	snapshot.Status = "ok"
	snapshot.Payload = payload
	snapshot.PayloadHash = obsapp.HashPayload(payload)
	snapshot.SchemaVersion = obsapp.MetricEvidenceSchemaV1
	snapshot.OriginalCount = &originalCount
	if err := repo.SaveSnapshot(ctx, &snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	loaded, err := repo.GetByID(ctx, evidenceID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if loaded.Status != "ok" || loaded.QueryHash != "query-hash-1" || loaded.SchemaVersion != obsapp.MetricEvidenceSchemaV1 {
		t.Fatalf("unexpected loaded snapshot: %+v", loaded)
	}
	canonical, err := obsapp.CanonicalJSON(loaded.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if obsapp.HashPayload(canonical) != loaded.PayloadHash {
		t.Fatal("loaded payload content does not match payload_hash")
	}
	if loaded.OriginalCount == nil || *loaded.OriginalCount != 1 || loaded.QueryParams["metric"] != "cpu_util" {
		t.Fatalf("snapshot metadata missing: %+v", loaded)
	}
	var count int64
	if err := db.Table("obs_evidence_ref").Where("evidence_id = ?", evidenceID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("snapshot upsert count=%d err=%v", count, err)
	}

	invalid := snapshot
	invalid.PayloadHash = strings.Repeat("0", 64)
	if err := repo.SaveSnapshot(ctx, &invalid); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected payload hash mismatch, got %v", err)
	}
}

func openEvidenceTestPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping postgres evidence repository test in short mode")
	}
	cfg := config.DatabaseConfig{
		Host:     evidenceTestEnv("AIOPS_TEST_DATABASE_HOST", "127.0.0.1"),
		Port:     evidenceTestPort(t, evidenceTestEnv("AIOPS_TEST_DATABASE_PORT", "5432")),
		User:     evidenceTestEnv("AIOPS_TEST_DATABASE_USER", "aiops"),
		Password: evidenceTestEnv("AIOPS_TEST_DATABASE_PASSWORD", "aiops"),
		Name:     evidenceTestEnv("AIOPS_TEST_DATABASE_NAME", "aiops"),
		SSLMode:  "disable",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.NewPostgres(ctx, cfg, "Asia/Shanghai")
	if err != nil {
		if os.Getenv("AIOPS_TEST_REQUIRE_PG") == "1" {
			t.Fatalf("postgres required but unavailable: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() { _ = database.ClosePostgres(db) })
	migrationsDir := filepath.Join(evidenceRepoRoot(t), "migrations")
	if err := database.RunMigrations(ctx, db, database.MigrateOptions{Dir: migrationsDir}); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func evidenceRepoRoot(t *testing.T) string {
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

func evidenceTestEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func evidenceTestPort(t *testing.T, raw string) int {
	t.Helper()
	var port int
	if _, err := fmt.Sscanf(raw, "%d", &port); err != nil || port <= 0 {
		return 5432
	}
	return port
}
