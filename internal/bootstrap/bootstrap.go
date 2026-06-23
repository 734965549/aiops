// Package bootstrap 负责装配平台运行所需的基础组件（配置、日志、数据库、缓存）。
//
// main 包从此处拿到一个就绪的 App，再装配业务模块；这样可避免 main 函数承担过多职责，
// 也方便在集成测试中复用同一份初始化流程。
package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/734965549/aiops/pkg/config"
	"github.com/734965549/aiops/pkg/database"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// App 聚合启动期初始化得到的运行时依赖。
type App struct {
	Cfg          *config.Config
	DB           *gorm.DB
	Redis        *redis.Client
	MigrationDir string
}

// Close 释放资源，幂等。
func (a *App) Close() {
	if a == nil {
		return
	}
	if a.Redis != nil {
		_ = a.Redis.Close()
	}
	_ = database.ClosePostgres(a.DB)
	logger.Sync()
}

// Migrate 仅连接 PostgreSQL 并执行 migrations/*.up.sql（自研 runner）。
// 供 make migrate、go run ./cmd/migrate 与 CI/发布流水线显式调用；生产环境应优先使用此路径，
// 而非在 API 启动时隐式迁移。
func Migrate(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	if err := logger.Init(logger.Options{
		Level:       cfg.Logger.Level,
		Format:      cfg.Logger.Format,
		Output:      cfg.Logger.Output,
		AppEnv:      cfg.App.Env,
		ServiceName: cfg.App.Name,
		FilePath:    cfg.Logger.FilePath,
		MaxSizeMB:   cfg.Logger.MaxSizeMB,
		MaxBackups:  cfg.Logger.MaxBackups,
		MaxAgeDays:  cfg.Logger.MaxAgeDays,
	}); err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer logger.Sync()

	db, err := database.NewPostgres(ctx, cfg.Database, cfg.App.Timezone)
	if err != nil {
		return fmt.Errorf("init postgres: %w", err)
	}
	defer func() { _ = database.ClosePostgres(db) }()

	dir := database.ResolveMigrationDir()
	if err := database.RunMigrations(ctx, db, database.MigrateOptions{Dir: dir}); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// Init 加载配置、初始化日志、连接数据库与缓存。
//
// 启动顺序对外部副作用敏感：
//  1. Load config -> Validate（避免基础组件起来后才发现致命配置错误）；
//  2. Init logger（后续步骤的日志都带 trace 字段）；
//  3. Postgres：连接；仅当 database.auto_migrate=true 时执行迁移（仅限受控 dev/test）；
//  4. Redis。
func Init(ctx context.Context, configPath string) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	migrationDir := database.ResolveMigrationDir()

	if err := logger.Init(logger.Options{
		Level:       cfg.Logger.Level,
		Format:      cfg.Logger.Format,
		Output:      cfg.Logger.Output,
		AppEnv:      cfg.App.Env,
		ServiceName: cfg.App.Name,
		FilePath:    cfg.Logger.FilePath,
		MaxSizeMB:   cfg.Logger.MaxSizeMB,
		MaxBackups:  cfg.Logger.MaxBackups,
		MaxAgeDays:  cfg.Logger.MaxAgeDays,
	}); err != nil {
		return nil, fmt.Errorf("init logger: %w", err)
	}

	logger.L().Info("bootstrap start",
		logger.String("app", cfg.App.Name),
		logger.String("env", cfg.App.Env),
	)

	db, err := database.NewPostgres(ctx, cfg.Database, cfg.App.Timezone)
	if err != nil {
		return nil, fmt.Errorf("init postgres: %w", err)
	}

	if cfg.Database.AutoMigrate {
		migTimeout := time.Duration(cfg.Database.MigrateTimeoutS) * time.Second
		if migTimeout <= 0 {
			migTimeout = 5 * time.Minute
		}
		migCtx, migCancel := context.WithTimeout(context.Background(), migTimeout)
		defer migCancel()
		if err := database.RunMigrations(migCtx, db, database.MigrateOptions{Dir: migrationDir}); err != nil {
			_ = database.ClosePostgres(db)
			return nil, fmt.Errorf("run migrations: %w", err)
		}
	} else {
		logger.L().Warn("database.auto_migrate disabled, skipping migrations")
	}

	var rdb *redis.Client
	if cfg.Redis.Required {
		rdb, err = database.NewRedis(ctx, cfg.Redis)
		if err != nil {
			_ = database.ClosePostgres(db)
			return nil, fmt.Errorf("init redis: %w", err)
		}
	} else {
		rdb, err = database.NewRedis(ctx, cfg.Redis)
		if err != nil {
			logger.L().Warn("redis optional and unavailable, continuing without cache/session store", logger.Error(err))
			rdb = nil
		}
	}

	return &App{Cfg: cfg, DB: db, Redis: rdb, MigrationDir: migrationDir}, nil
}
