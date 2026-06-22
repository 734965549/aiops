// Package database 提供 PostgreSQL（GORM）与 Redis 的初始化封装。
//
// 这里只关注「连接」的建立与生命周期管理，业务模块的仓储应在 internal/<context>/
// 内自行注入 *gorm.DB / *redis.Client，不要把表名等业务概念放到 pkg。
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/734965549/aiops/pkg/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// NewPostgres 根据配置建立 GORM 连接并执行一次 ping。
func NewPostgres(ctx context.Context, cfg config.DatabaseConfig, tz string) (*gorm.DB, error) {
	level := parseGormLogLevel(cfg.LogLevel)
	gdb, err := gorm.Open(postgres.Open(cfg.DSN(tz)), &gorm.Config{
		Logger:                 newGormLogger(level),
		PrepareStmt:            true,
		SkipDefaultTransaction: true, // 业务上未显式使用事务时省去隐式 begin/commit
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("get *sql.DB: %w", err)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if lifetime := cfg.ConnMaxLifetime(); lifetime > 0 {
		sqlDB.SetConnMaxLifetime(lifetime)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return gdb, nil
}

// ClosePostgres 关闭底层 *sql.DB，安全可被多次调用。
func ClosePostgres(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func parseGormLogLevel(s string) gormlogger.LogLevel {
	switch s {
	case "silent":
		return gormlogger.Silent
	case "error":
		return gormlogger.Error
	case "warn", "":
		return gormlogger.Warn
	case "info":
		return gormlogger.Info
	default:
		return gormlogger.Warn
	}
}
