package middleware

import (
	"runtime/debug"

	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery 捕获 handler panic，写入日志并返回统一错误响应。
//
// 注意：业务错误请使用 *apperr.Error，本中间件只兜底未预期 panic。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.From(c.Request.Context()).Error("panic recovered",
					zap.Any("panic", r),
					zap.ByteString("stack", debug.Stack()),
					zap.String("path", c.FullPath()),
					zap.String("method", c.Request.Method),
				)
				c.Abort()
				httpx.FailWith(c, apperr.CodeInternal, "internal server error")
			}
		}()
		c.Next()
	}
}
