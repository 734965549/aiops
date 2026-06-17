package http

import (
	"context"

	"github.com/gin-gonic/gin"
)

const (
	// HeaderTraceID 上下游传递的 trace_id 头部名称。
	HeaderTraceID = "X-Trace-Id"
	// CtxKeyTraceID Gin 上下文中的 trace_id 键名。
	CtxKeyTraceID = "trace_id"
)

// ctxTraceIDKey 是标准 context 中 trace_id 的私有键，
// 通过私有类型避免与外部包发生 key 冲突。
type ctxTraceIDKey struct{}

// TraceIDFrom 从 Gin 上下文中读取 trace_id，便于响应结构与日志使用。
//
// 无 Trace 中间件或未写入时返回 ""；Response 仍会输出 trace_id 字段（§2 固定字段）。
//
// 优先级：标准 context（Trace 中间件已显式写入） > Gin Context Keys。
func TraceIDFrom(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if id := TraceIDFromCtx(c.Request.Context()); id != "" {
		return id
	}
	if v, ok := c.Get(CtxKeyTraceID); ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}

// TraceIDFromCtx 是给非 Gin 路径（消息队列消费、定时任务）的版本。
func TraceIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxTraceIDKey{}).(string); ok {
		return v
	}
	return ""
}

// ContextWithTraceID 把 trace_id 写入标准 context。
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxTraceIDKey{}, traceID)
}
