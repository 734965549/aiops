// Package server 装配 HTTP 引擎与全局路由。
//
// 各限界上下文（identity / alert / asset / ...）通过 RouteRegistrar 接口将自身路由
// 注册到 /api 下的对应分组，避免 server 包反向依赖业务模块。
package server

import (
	"context"
	"time"

	"github.com/734965549/aiops/internal/version"
	"github.com/734965549/aiops/pkg/config"
	"github.com/734965549/aiops/pkg/database"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// RouteGroups 是 Registrar 在注册路由时拿到的两类路由组。
//
//   - Public 走 AuthOptional，允许匿名访问（如登录接口、版本信息等）；
//   - Authed 走 AuthRequired，未携带合法 token 直接 401，不进入业务 handler。
//
// 由各模块自行决定路由该挂到哪个 group 下，避免 server 反向感知业务路由细节。
type RouteGroups struct {
	Public *gin.RouterGroup
	Authed *gin.RouterGroup
}

// RouteRegistrar 由各限界上下文实现，向 /api 路由组注册自身路由。
type RouteRegistrar interface {
	RegisterRoutes(groups RouteGroups)
}

// Options 用于构造 Gin 引擎所需的依赖。
type Options struct {
	Cfg           *config.Config
	DB            *gorm.DB
	Redis         *redis.Client
	MigrationDir  string
	Authenticator middleware.Authenticator
	Registrars    []RouteRegistrar
	StartedAt     time.Time
}

// NewEngine 构造带通用中间件与系统路由的 Gin 引擎。
func NewEngine(opt Options) *gin.Engine {
	cfg := opt.Cfg
	if cfg == nil {
		cfg = &config.Config{}
	}
	if cfg.App.Env != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()

	// 通用中间件顺序：Trace -> Recovery -> RequestLog -> CORS。
	// Trace 必须在最外层，确保后续日志与响应都能拿到 trace_id。
	r.Use(middleware.Trace())
	r.Use(middleware.Recovery())
	r.Use(middleware.RequestLog())
	r.Use(middleware.CORS(cfg.CORS))

	registerSystemRoutes(r, cfg, opt.DB, opt.Redis, opt.MigrationDir, opt.StartedAt)

	// /api 下分两个子组：
	//   public：AuthOptional，允许匿名（登录、注册等暴露给前端的公共接口）；
	//   authed：AuthRequired，强制要求合法 Bearer token，未通过直接 401。
	publicAPI := r.Group("/api")
	publicAPI.Use(middleware.AuthOptional(opt.Authenticator))

	authedAPI := r.Group("/api")
	authedAPI.Use(middleware.AuthRequired(opt.Authenticator))

	groups := RouteGroups{Public: publicAPI, Authed: authedAPI}
	for _, reg := range opt.Registrars {
		if reg == nil {
			continue
		}
		reg.RegisterRoutes(groups)
	}

	// 兜底 404：保证非业务路径仍走平台统一响应格式。
	r.NoRoute(func(c *gin.Context) {
		httpx.FailWith(c, apperr.CodeNotFound, "route not found")
	})

	return r
}

func registerSystemRoutes(r *gin.Engine, cfg *config.Config, db *gorm.DB, redisClient *redis.Client, migrationDir string, startedAt time.Time) {
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	healthPayload := func(status httpx.HealthStatus, checks []httpx.HealthCheck) httpx.HealthResponse {
		return httpx.HealthResponse{Status: status, Checks: checks, UptimeMS: time.Since(startedAt).Milliseconds()}
	}

	r.GET("/healthz", func(c *gin.Context) {
		httpx.OK(c, healthPayload(httpx.HealthStatusOK, []httpx.HealthCheck{{Name: "process", Status: httpx.HealthStatusOK}}))
	})

	r.GET("/readyz", func(c *gin.Context) {
		checks := []httpx.HealthCheck{{Name: "process", Status: httpx.HealthStatusOK}}
		status := httpx.HealthStatusReady

		cfgCheck := readinessConfig(cfg)
		if cfgCheck.Status != httpx.HealthStatusOK {
			status = httpx.HealthStatusNotReady
		}
		checks = append(checks, cfgCheck)

		migCheck := readinessMigration(c.Request.Context(), db, migrationDir)
		if migCheck.Status != httpx.HealthStatusOK {
			status = httpx.HealthStatusNotReady
		}
		checks = append(checks, migCheck)

		dbCheck := readinessPostgres(c.Request.Context(), db)
		if dbCheck.Status != httpx.HealthStatusOK {
			status = httpx.HealthStatusNotReady
		}
		checks = append(checks, dbCheck)

		redisCheck := readinessRedis(c.Request.Context(), redisClient, cfg.Redis.Required)
		switch redisCheck.Status {
		case httpx.HealthStatusDown:
			if cfg.Redis.Required {
				status = httpx.HealthStatusNotReady
			}
		case httpx.HealthStatusDegraded:
			// 可选依赖不可用时不阻塞就绪。
		case httpx.HealthStatusOK:
		default:
			if cfg.Redis.Required {
				status = httpx.HealthStatusNotReady
			}
		}
		checks = append(checks, redisCheck)

		httpx.OK(c, healthPayload(status, checks))
	})
	r.GET("/version", func(c *gin.Context) {
		httpx.OK(c, gin.H{
			"app":     cfg.App.Name,
			"env":     cfg.App.Env,
			"version": version.Get(),
		})
	})
}

func readinessConfig(cfg *config.Config) httpx.HealthCheck {
	if cfg == nil {
		return httpx.HealthCheck{Name: "config", Status: httpx.HealthStatusDown, Error: "config is nil"}
	}
	if err := cfg.Validate(); err != nil {
		return httpx.HealthCheck{Name: "config", Status: httpx.HealthStatusDown, Error: err.Error()}
	}
	return httpx.HealthCheck{Name: "config", Status: httpx.HealthStatusOK}
}

func readinessMigration(ctx context.Context, db *gorm.DB, migrationDir string) httpx.HealthCheck {
	if db == nil {
		return httpx.HealthCheck{Name: "migration", Status: httpx.HealthStatusDown, Error: "database is not initialized"}
	}
	if migrationDir == "" {
		migrationDir = database.ResolveMigrationDir()
	}
	status, err := database.ReadMigrationStatus(ctx, db, database.MigrateOptions{Dir: migrationDir})
	if err != nil {
		return httpx.HealthCheck{Name: "migration", Status: httpx.HealthStatusDown, Error: err.Error()}
	}
	details := &httpx.HealthMigrationDetails{
		Dir:            status.Dir,
		LatestVersion:  status.LatestVersion,
		AppliedVersion: status.AppliedVersion,
		PendingCount:   status.PendingCount,
		UpToDate:       status.UpToDate,
	}
	if status.UpToDate {
		return httpx.HealthCheck{Name: "migration", Status: httpx.HealthStatusOK, Details: details}
	}
	return httpx.HealthCheck{Name: "migration", Status: httpx.HealthStatusDegraded, Error: "pending migrations exist", Details: details}
}

func readinessPostgres(ctx context.Context, db *gorm.DB) httpx.HealthCheck {
	if db == nil {
		return httpx.HealthCheck{Name: "db", Status: httpx.HealthStatusDown, Error: "database is not initialized"}
	}
	sqlDB, err := db.DB()
	if err != nil {
		return httpx.HealthCheck{Name: "db", Status: httpx.HealthStatusDown, Error: err.Error()}
	}
	stats := sqlDB.Stats()
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return httpx.HealthCheck{Name: "db", Status: httpx.HealthStatusDown, Error: err.Error(), Details: &httpx.HealthDBDetails{
			MaxOpenConns:      stats.MaxOpenConnections,
			MaxIdleConns:      stats.Idle,
			OpenConnections:   stats.OpenConnections,
			InUseConnections:  stats.InUse,
			IdleConnections:   stats.Idle,
			WaitCount:         stats.WaitCount,
			WaitDurationMS:    stats.WaitDuration.Milliseconds(),
			MaxIdleClosed:     stats.MaxIdleClosed,
			MaxLifetimeClosed: stats.MaxLifetimeClosed,
			PingTimeoutMS:     2000,
		}}
	}
	return httpx.HealthCheck{Name: "db", Status: httpx.HealthStatusOK, Details: &httpx.HealthDBDetails{
		MaxOpenConns:      stats.MaxOpenConnections,
		MaxIdleConns:      stats.Idle,
		OpenConnections:   stats.OpenConnections,
		InUseConnections:  stats.InUse,
		IdleConnections:   stats.Idle,
		WaitCount:         stats.WaitCount,
		WaitDurationMS:    stats.WaitDuration.Milliseconds(),
		MaxIdleClosed:     stats.MaxIdleClosed,
		MaxLifetimeClosed: stats.MaxLifetimeClosed,
		PingTimeoutMS:     time.Since(start).Milliseconds(),
	}}
}

func readinessRedis(ctx context.Context, client *redis.Client, required bool) httpx.HealthCheck {
	if client == nil {
		if required {
			return httpx.HealthCheck{Name: "redis", Status: httpx.HealthStatusDown, Error: "redis client is not initialized"}
		}
		return httpx.HealthCheck{Name: "redis", Status: httpx.HealthStatusDegraded, Error: "redis optional and not connected"}
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	if err := client.Ping(pingCtx).Err(); err != nil {
		st := httpx.HealthStatusDown
		if !required {
			st = httpx.HealthStatusDegraded
		}
		return httpx.HealthCheck{Name: "redis", Status: st, Error: err.Error(), Details: &httpx.HealthRedisDetails{
			Addr:          client.Options().Addr,
			DB:            client.Options().DB,
			PoolSize:      client.Options().PoolSize,
			PingLatencyMS: time.Since(start).Milliseconds(),
			Endpoint:      client.Options().Addr,
		}}
	}
	return httpx.HealthCheck{Name: "redis", Status: httpx.HealthStatusOK, Details: &httpx.HealthRedisDetails{
		Addr:          client.Options().Addr,
		DB:            client.Options().DB,
		PoolSize:      client.Options().PoolSize,
		PingLatencyMS: time.Since(start).Milliseconds(),
		Endpoint:      client.Options().Addr,
	}}
}
