package middleware

import (
	"time"

	"github.com/734965549/aiops/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLog 输出每次请求的访问日志（method/path/status/latency/ip）。
//
// 复杂的访问日志（如请求体、响应体）建议在审计中间件中处理，并按风险分级脱敏。
func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		fullPath := path
		if raw != "" {
			fullPath = path + "?" + raw
		}

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", fullPath),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
		}
		if ua := c.Request.UserAgent(); ua != "" {
			fields = append(fields, zap.String("user_agent", ua))
		}

		l := logger.From(c.Request.Context())
		switch {
		case c.Writer.Status() >= 500:
			l.Error("http request", fields...)
		case c.Writer.Status() >= 400:
			l.Warn("http request", fields...)
		default:
			l.Info("http request", fields...)
		}
	}
}
