// Package http 提供平台 HTTP 响应封装与通用中间件。
//
// 所有业务接口必须使用 OK / Fail / FailWith 输出，以保证响应结构与错误码统一。
// 字段约定见 ops/alert-contract.md §2、ops/auth-contract.md §2、ops/health-contract.md。
package http

import (
	"errors"

	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Response 是平台对外统一响应结构（§2：code / message / trace_id / data）。
//
//   - 成功：code = "OK"（字符串，非数字 0），message = "ok"，data 为业务载荷；
//   - 失败：code 为业务错误码（如 INVALID_ARGUMENT），message 为可展示原因，无 data；
//   - trace_id 为固定字段，无 Trace 中间件时输出空字符串 ""，不省略。
type Response struct {
	Code    apperr.Code `json:"code"`
	Message string      `json:"message"`
	TraceID string      `json:"trace_id"`
	Data    any         `json:"data,omitempty"`
}

// OK 输出 HTTP 200 成功响应（§2：code="OK"、message="ok"、trace_id 来自 Trace 中间件）。
func OK(c *gin.Context, data any) {
	c.JSON(200, Response{
		Code:    apperr.CodeOK,
		Message: "ok",
		TraceID: TraceIDFrom(c),
		Data:    data,
	})
}

// Fail 输出标准错误响应；err 应为 *apperr.Error，其它 error 会被包装为 INTERNAL。
// 未显式包装的业务错误（如 DB/Redis 原始 error）不会把底层文案暴露给调用方，详情写入日志。
func Fail(c *gin.Context, err error) {
	if err == nil {
		c.JSON(200, Response{Code: apperr.CodeOK, Message: "ok", TraceID: TraceIDFrom(c)})
		return
	}

	var typed *apperr.Error
	wasTyped := errors.As(err, &typed)
	e := apperr.FromError(err)
	if shouldLogInternalError(wasTyped, e) {
		logger.From(c.Request.Context()).Error("request failed", zap.Error(err))
	}

	c.JSON(HTTPStatus(e.Code), Response{
		Code:    e.Code,
		Message: e.Message,
		TraceID: TraceIDFrom(c),
	})
}

func shouldLogInternalError(wasTyped bool, e *apperr.Error) bool {
	if e == nil {
		return false
	}
	if !wasTyped {
		return true
	}
	return e.Code == apperr.CodeInternal && e.Unwrap() != nil
}

// FailWith 直接以 code + message 输出失败响应。
func FailWith(c *gin.Context, code apperr.Code, message string) {
	c.JSON(HTTPStatus(code), Response{
		Code:    code,
		Message: message,
		TraceID: TraceIDFrom(c),
	})
}
