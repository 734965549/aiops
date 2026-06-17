// Package middleware 收敛平台 HTTP 通用中间件，便于各模块路由组复用。
package middleware

import (
	"context"
	"strings"

	"github.com/734965549/aiops/pkg/logger"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Trace 中间件：
//   - 优先使用上游 X-Trace-Id（须为合法 UUID，长度不超过 128）；否则生成 UUID；
//   - 写入响应头、Gin 上下文与标准 context（同 key），
//     使下游服务通过 context.Context（如 application/domain 层）也能拿到 trace_id；
//   - 同时把带 trace_id 的 logger 注入 ctx，logger.From(ctx) 自动携带。
func Trace() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := normalizeTraceID(c.GetHeader(httpx.HeaderTraceID))
		c.Writer.Header().Set(httpx.HeaderTraceID, traceID)
		c.Set(httpx.CtxKeyTraceID, traceID)

		// 把 trace_id 与带 trace_id 字段的 logger 一并放入标准 context，
		// 业务层只持有 context.Context 也能拿到完整链路信息。
		ctx := httpx.ContextWithTraceID(c.Request.Context(), traceID)
		ctx = logger.WithContext(ctx, logger.With(zap.String("trace_id", traceID)))
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// TraceIDFromContext 从标准 ctx 中读取 trace_id（若不存在返回空串）。
//
// 推荐 application / domain / repository 层统一通过本函数取 trace_id，
// 不要依赖 *gin.Context，避免上层泄漏到下层。
func TraceIDFromContext(ctx context.Context) string {
	return httpx.TraceIDFromCtx(ctx)
}

// ContextWithTraceID 给非 HTTP 入口（如定时任务、消息消费）准备的辅助函数，
// 用于在入口处显式把 trace_id 写入标准 ctx，保持与 HTTP 路径一致的链路。
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if traceID == "" {
		traceID = uuid.NewString()
	}
	ctx = httpx.ContextWithTraceID(ctx, traceID)
	ctx = logger.WithContext(ctx, logger.With(zap.String("trace_id", traceID)))
	return ctx
}

const maxTraceIDLen = 128

// normalizeTraceID 校验上游 trace id：合法 UUID 且长度不超过 maxTraceIDLen，否则生成新 UUID。
func normalizeTraceID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxTraceIDLen {
		return uuid.NewString()
	}
	if _, err := uuid.Parse(raw); err != nil {
		return uuid.NewString()
	}
	return raw
}
