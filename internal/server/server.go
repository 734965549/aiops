package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/734965549/aiops/pkg/config"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/gin-gonic/gin"
)

// Server 封装 http.Server，提供阻塞 Run 与优雅 Shutdown。
type Server struct {
	cfg     config.ServerConfig
	httpSrv *http.Server
}

// New 构造 Server。engine 通常由 NewEngine 产生。
func New(cfg config.ServerConfig, engine *gin.Engine) *Server {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	readHeaderTimeout := time.Duration(cfg.ReadHeaderTimeoutS) * time.Second
	if readHeaderTimeout <= 0 {
		readHeaderTimeout = 5 * time.Second
	}
	maxHeaderBytes := cfg.MaxHeaderBytes
	if maxHeaderBytes <= 0 {
		maxHeaderBytes = 1 << 20 // 1MB 默认
	}

	return &Server{
		cfg: cfg,
		httpSrv: &http.Server{
			Addr:              addr,
			Handler:           engine,
			ReadTimeout:       time.Duration(cfg.ReadTimeoutS) * time.Second,
			ReadHeaderTimeout: readHeaderTimeout,
			WriteTimeout:      time.Duration(cfg.WriteTimeoutS) * time.Second,
			MaxHeaderBytes:    maxHeaderBytes,
		},
	}
}

// Run 阻塞监听端口，直到 Shutdown 或发生致命错误。
func (s *Server) Run() error {
	logger.L().Info("http server listening", logger.String("addr", s.httpSrv.Addr))
	if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen: %w", err)
	}
	return nil
}

// Shutdown 在指定超时内优雅关闭。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}
