// Package main 是数据库迁移的独立入口，供本地联调、CI 与发布流水线显式执行 schema 变更。
//
// 平台仅使用 pkg/database 自研 runner，禁止与 golang-migrate 等外部迁移工具混用。
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/734965549/aiops/internal/bootstrap"
	"github.com/734965549/aiops/pkg/config"
	"github.com/734965549/aiops/pkg/logger"
)

func main() {
	configPath := flag.String("config", "", "path to config file (default: ./configs/config.yaml)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.ReportError("migrate failed", err)
		os.Exit(1)
	}
	timeout := time.Duration(cfg.Database.MigrateTimeoutS) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := bootstrap.Migrate(ctx, *configPath); err != nil {
		logger.ReportError("migrate failed", err)
		os.Exit(1)
	}
}
