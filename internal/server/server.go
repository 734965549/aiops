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
	"go.uber.org/zap"
)

// Server 封装 http.Server，提供阻塞 Run 与优雅 Shutdown。
type Server struct {
	cfg     config.ServerConfig
	httpSrv *http.Server
}

// New 构造 Server。engine 通常由 NewEngine 产生。
func New(cfg config.ServerConfig, engine *gin.Engine) *Server {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	return &Server{
		cfg: cfg,
		httpSrv: &http.Server{
			Addr:         addr,
			Handler:      engine,
			ReadTimeout:  time.Duration(cfg.ReadTimeoutS) * time.Second,
			WriteTimeout: time.Duration(cfg.WriteTimeoutS) * time.Second,
		},
	}
}

// Run 阻塞监听端口，直到 Shutdown 或发生致命错误。
func (s *Server) Run() error {
	logger.L().Info("http server listening", zap.String("addr", s.httpSrv.Addr))
	if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen: %w", err)
	}
	return nil
}

// Shutdown 在指定超时内优雅关闭。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}
