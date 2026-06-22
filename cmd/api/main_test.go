package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	identityhttp "github.com/734965549/aiops/internal/identity/interfaces/http"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/auth"
	"github.com/734965549/aiops/pkg/config"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

// TestAssemblySmoke_SystemProbes 模拟 cmd/api 装配后的系统探针行为：
// 未注入 DB/Redis 时 /healthz 可用、/readyz 为 not_ready。
func TestAssemblySmoke_SystemProbes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := assembleSmokeEngine(t)

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthW := httptest.NewRecorder()
	engine.ServeHTTP(healthW, healthReq)
	if healthW.Code != http.StatusOK {
		t.Fatalf("healthz status=%d body=%s", healthW.Code, healthW.Body.String())
	}
	health := decodeHealthPayload(t, healthW.Body.Bytes())
	if health.Status != httpx.HealthStatusOK {
		t.Fatalf("expected healthz ok, got %q", health.Status)
	}

	readyReq := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	readyW := httptest.NewRecorder()
	engine.ServeHTTP(readyW, readyReq)
	if readyW.Code != http.StatusOK {
		t.Fatalf("readyz status=%d body=%s", readyW.Code, readyW.Body.String())
	}
	ready := decodeHealthPayload(t, readyW.Body.Bytes())
	if ready.Status != httpx.HealthStatusNotReady {
		t.Fatalf("expected readyz not_ready without dependencies, got %q checks=%+v", ready.Status, ready.Checks)
	}
	checks := indexChecks(ready.Checks)
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
		t.Fatalf("expected optional redis degraded, got %+v", checks["redis"])
	}
}

// assembleSmokeEngine 复刻 cmd/api 中与系统路由相关的装配路径（Options 字段与 main 对齐）。
func assembleSmokeEngine(t *testing.T) *gin.Engine {
	t.Helper()
	cfg := &config.Config{
		App:      config.AppConfig{Env: "dev", Timezone: "Asia/Shanghai"},
		Server:   config.ServerConfig{Port: 8080},
		Database: config.DatabaseConfig{Host: "127.0.0.1", Name: "aiops", SSLMode: "disable"},
		Auth:     config.AuthConfig{JWTSecret: config.DefaultJWTSecretPlaceholder},
		Integration: config.IntegrationConfig{
			CredentialEncryptionKey:        config.DefaultCredentialEncryptionKeyPlaceholder,
			CredentialEncryptionKeyVersion: 1,
		},
		Redis: config.RedisConfig{Required: false},
	}
	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret:     cfg.Auth.JWTSecret,
		Issuer:     cfg.Auth.JWTIssuer,
		AccessTTL:  cfg.Auth.AccessTTL(),
		RefreshTTL: cfg.Auth.RefreshTTL(),
	})
	if err != nil {
		t.Fatalf("init jwt: %v", err)
	}
	authenticator := auth.NewJWTAuthenticator(jwtMgr)
	authSvc := identityapp.NewAuthService(nil, jwtMgr, auth.NoopRefreshTokenStore{}, nil, nil, nil, nil, nil, "")
	userSvc := identityapp.NewUserService(nil)
	identityHandler := identityhttp.NewHandler(userSvc, authSvc, nil, nil, auth.NoopLoginAttemptLimiter{}, nil, nil)

	return server.NewEngine(server.Options{
		Cfg:           cfg,
		DB:            nil,
		Redis:         nil,
		MigrationDir:  "",
		Authenticator: authenticator,
		Registrars: []server.RouteRegistrar{
			identityhttp.NewRegistrar(identityHandler, nil),
		},
		StartedAt: time.Now(),
	})
}

type healthAPIEnvelope struct {
	Code string               `json:"code"`
	Data httpx.HealthResponse `json:"data"`
}

func decodeHealthPayload(t *testing.T, body []byte) httpx.HealthResponse {
	t.Helper()
	var env healthAPIEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal health: %v body=%s", err, string(body))
	}
	if env.Code != "OK" {
		t.Fatalf("expected code OK, got %q body=%s", env.Code, string(body))
	}
	return env.Data
}

func indexChecks(checks []httpx.HealthCheck) map[string]httpx.HealthCheck {
	out := make(map[string]httpx.HealthCheck, len(checks))
	for _, c := range checks {
		out[c.Name] = c
	}
	return out
}
