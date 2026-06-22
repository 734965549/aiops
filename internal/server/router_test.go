package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/734965549/aiops/pkg/config"
	"github.com/734965549/aiops/pkg/database"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

func TestHealthz_ProcessOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := NewEngine(Options{
		Cfg:       validTestConfig(),
		StartedAt: time.Now().Add(-time.Second),
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	payload := decodeHealthData(t, w.Body.Bytes())
	if payload.Status != httpx.HealthStatusOK {
		t.Fatalf("expected status ok, got %q", payload.Status)
	}
	if len(payload.Checks) != 1 || payload.Checks[0].Name != "process" || payload.Checks[0].Status != httpx.HealthStatusOK {
		t.Fatalf("unexpected checks: %+v", payload.Checks)
	}
	if payload.UptimeMS <= 0 {
		t.Fatalf("expected positive uptime_ms, got %d", payload.UptimeMS)
	}
}

func TestReadyz_WithoutDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := NewEngine(Options{
		Cfg:       validTestConfig(),
		StartedAt: time.Now(),
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	payload := decodeHealthData(t, w.Body.Bytes())
	if payload.Status != httpx.HealthStatusNotReady {
		t.Fatalf("expected top-level not_ready, got %q", payload.Status)
	}

	checks := checksByName(payload.Checks)
	for _, name := range []string{"process", "config"} {
		if checks[name].Status != httpx.HealthStatusOK {
			t.Fatalf("expected %s ok, got %+v", name, checks[name])
		}
	}
	for _, name := range []string{"migration", "db"} {
		if checks[name].Status != httpx.HealthStatusDown {
			t.Fatalf("expected %s down, got %+v", name, checks[name])
		}
	}
	if checks["redis"].Status != httpx.HealthStatusDegraded {
		t.Fatalf("expected redis degraded when optional and not connected, got %+v", checks["redis"])
	}
}

func TestReadyz_InvalidConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	badCfg := validTestConfig()
	badCfg.Server.Port = 0

	engine := NewEngine(Options{Cfg: badCfg, StartedAt: time.Now()})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	payload := decodeHealthData(t, w.Body.Bytes())
	if payload.Status != httpx.HealthStatusNotReady {
		t.Fatalf("expected not_ready, got %q", payload.Status)
	}
	if checksByName(payload.Checks)["config"].Status != httpx.HealthStatusDown {
		t.Fatalf("expected config down, got %+v", checksByName(payload.Checks)["config"])
	}
}

func TestReadyz_Integration(t *testing.T) {
	root := repoRoot(t)
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}

	cfg := validTestConfig()
	cfg.Database.Host = envOrDefault("AIOPS_TEST_DATABASE_HOST", "127.0.0.1")
	cfg.Database.Port = envIntOrDefault("AIOPS_TEST_DATABASE_PORT", 5432)
	cfg.Database.User = envOrDefault("AIOPS_TEST_DATABASE_USER", "aiops")
	cfg.Database.Password = envOrDefault("AIOPS_TEST_DATABASE_PASSWORD", "aiops")
	cfg.Database.Name = envOrDefault("AIOPS_TEST_DATABASE_NAME", "aiops")
	cfg.Redis.Addr = envOrDefault("AIOPS_TEST_REDIS_ADDR", "127.0.0.1:6379")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.NewPostgres(ctx, cfg.Database, cfg.App.Timezone)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer database.ClosePostgres(db)

	if err := database.RunMigrations(ctx, db, database.MigrateOptions{Dir: database.ResolveMigrationDir()}); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	redisClient, err := database.NewRedis(ctx, cfg.Redis)
	if err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	defer redisClient.Close()

	startedAt := time.Now().Add(-2 * time.Second)
	engine := NewEngine(Options{
		Cfg:          cfg,
		DB:           db,
		Redis:        redisClient,
		MigrationDir: database.ResolveMigrationDir(),
		StartedAt:    startedAt,
	})

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	payload := decodeHealthData(t, w.Body.Bytes())
	if payload.Status != httpx.HealthStatusReady {
		t.Fatalf("expected ready, got %q checks=%+v", payload.Status, payload.Checks)
	}

	checks := checksByName(payload.Checks)
	for _, name := range []string{"process", "config", "migration", "db", "redis"} {
		if checks[name].Status != httpx.HealthStatusOK {
			t.Fatalf("expected %s ok, got %+v", name, checks[name])
		}
	}
	if payload.UptimeMS < 1000 {
		t.Fatalf("expected uptime_ms >= 1000, got %d", payload.UptimeMS)
	}
}

type apiHealthEnvelope struct {
	Code string               `json:"code"`
	Data httpx.HealthResponse `json:"data"`
}

func decodeHealthData(t *testing.T, body []byte) httpx.HealthResponse {
	t.Helper()
	var env apiHealthEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, string(body))
	}
	if env.Code != "OK" {
		t.Fatalf("expected code OK, got %q body=%s", env.Code, string(body))
	}
	return env.Data
}

func checksByName(checks []httpx.HealthCheck) map[string]httpx.HealthCheck {
	out := make(map[string]httpx.HealthCheck, len(checks))
	for _, c := range checks {
		out[c.Name] = c
	}
	return out
}

func validTestConfig() *config.Config {
	return &config.Config{
		App:      config.AppConfig{Env: "dev", Timezone: "Asia/Shanghai"},
		Server:   config.ServerConfig{Port: 8080},
		Database: config.DatabaseConfig{Host: "127.0.0.1", Name: "aiops", SSLMode: "disable"},
		Auth:     config.AuthConfig{JWTSecret: config.DefaultJWTSecretPlaceholder},
		Integration: config.IntegrationConfig{
			CredentialEncryptionKey:        config.DefaultCredentialEncryptionKeyPlaceholder,
			CredentialEncryptionKeyVersion: 1,
		},
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
