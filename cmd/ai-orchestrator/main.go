// Package main 提供 type c 内部 AI 编排服务参考实现。
//
// 平台将本服务注册为 AI provider（type=c，base_url=http://<host>:8090，
// 鉴权头 X-API-Key），由 toolgateway 调用 POST /api/tool/invoke。
// 服务只读取 payload.alert / payload.evidence 做分析建议，不落库、不执行任何动作。
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/734965549/aiops/pkg/logger"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ai-orchestrator config:", err)
		os.Exit(1)
	}
	if err := logger.Init(logger.Options{
		Level:       "info",
		Format:      "json",
		Output:      "stdout",
		AppEnv:      "prod",
		ServiceName: "aiops-ai-orchestrator",
	}); err != nil {
		fmt.Fprintln(os.Stderr, "ai-orchestrator logger:", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           NewRouter(cfg),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.L().Info("ai orchestrator listening", logger.String("addr", cfg.Addr))
		errCh <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.L().Error("ai orchestrator stopped unexpectedly", logger.Error(err))
			os.Exit(1)
		}
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.L().Warn("ai orchestrator shutdown failed", logger.Error(err))
	}
}

func loadConfig() (Config, error) {
	timeoutS, err := strconv.Atoi(envOrDefault("AIOPS_AI_ORCH_TIMEOUT_S", "30"))
	if err != nil || timeoutS <= 0 {
		return Config{}, errors.New("AIOPS_AI_ORCH_TIMEOUT_S must be a positive integer")
	}
	cfg := Config{
		Addr:         envOrDefault("AIOPS_AI_ORCH_ADDR", ":8090"),
		APIKey:       strings.TrimSpace(os.Getenv("AIOPS_AI_ORCH_API_KEY")),
		ModelBaseURL: strings.TrimSpace(os.Getenv("AIOPS_AI_ORCH_MODEL_BASE_URL")),
		ModelAPIKey:  strings.TrimSpace(os.Getenv("AIOPS_AI_ORCH_MODEL_API_KEY")),
		ModelName:    strings.TrimSpace(os.Getenv("AIOPS_AI_ORCH_MODEL_NAME")),
		ModelPath:    envOrDefault("AIOPS_AI_ORCH_MODEL_PATH", "/v1/chat/completions"),
		Timeout:      time.Duration(timeoutS) * time.Second,
	}
	if cfg.APIKey == "" {
		return Config{}, errors.New("AIOPS_AI_ORCH_API_KEY is required")
	}
	if cfg.ModelBaseURL == "" || cfg.ModelAPIKey == "" || cfg.ModelName == "" {
		return Config{}, errors.New("AIOPS_AI_ORCH_MODEL_BASE_URL / MODEL_API_KEY / MODEL_NAME are required")
	}
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
