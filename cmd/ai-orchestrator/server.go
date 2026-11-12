package main

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/734965549/aiops/pkg/logger"
)

const (
	toolCodeAlarmAnalyze = "alarm.analyze"
	maxRequestBytes      = 2 << 20
)

// Config 是编排服务运行配置；模型凭据只从环境变量读取，不进入日志。
type Config struct {
	Addr         string
	APIKey       string
	ModelBaseURL string
	ModelAPIKey  string
	ModelName    string
	ModelPath    string
	Timeout      time.Duration
}

type analysisServer struct {
	cfg    Config
	client *http.Client
}

type toolInvokeRequest struct {
	ToolCode string         `json:"tool_code"`
	Resource string         `json:"resource"`
	Action   string         `json:"action"`
	UserID   string         `json:"user_id"`
	Payload  map[string]any `json:"payload"`
}

func NewRouter(cfg Config) http.Handler {
	server := &analysisServer{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", server.handleHealth)
	invoke := server.withAPIKey(server.handleInvoke)
	// 平台 type=c executor 默认使用 /api/tool/invoke；同时兼容编排方案中的 /v1/tools/invoke。
	mux.HandleFunc("/api/tool/invoke", invoke)
	mux.HandleFunc("/v1/tools/invoke", invoke)
	return mux
}

func (s *analysisServer) withAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		supplied := strings.TrimSpace(r.Header.Get("X-API-Key"))
		// 平台 HTTPToolExecutor 统一写入 "Bearer <api_key>"，这里同时兼容裸 key。
		if len(supplied) > 7 && strings.EqualFold(supplied[:7], "bearer ") {
			supplied = strings.TrimSpace(supplied[7:])
		}
		if strings.TrimSpace(s.cfg.APIKey) == "" || subtle.ConstantTimeCompare([]byte(supplied), []byte(s.cfg.APIKey)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid api key"})
			return
		}
		next(w, r)
	}
}

func (s *analysisServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *analysisServer) handleInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	var req toolInvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.ToolCode) != toolCodeAlarmAnalyze {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported tool_code"})
		return
	}

	started := time.Now()
	output, tokens, err := s.analyze(r.Context(), req.Payload)
	if err != nil {
		logger.L().Warn("ai orchestrator analysis degraded",
			logger.String("tool_code", req.ToolCode),
			logger.Int64("duration_ms", time.Since(started).Milliseconds()),
			logger.Error(err),
		)
		output = fallbackAnalysis(req.Payload)
	}
	logger.L().Info("ai orchestrator analysis completed",
		logger.String("tool_code", req.ToolCode),
		logger.Int64("duration_ms", time.Since(started).Milliseconds()),
		logger.Int("tokens", tokens),
		logger.Bool("degraded", err != nil),
	)
	writeJSON(w, http.StatusOK, output)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
