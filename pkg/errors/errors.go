package errors

import (
	"errors"
	"fmt"
)

// InternalMessage 是未包装业务错误对外展示的默认文案；底层细节应写入日志而非响应。
const InternalMessage = "internal server error"

// Error 是平台业务错误的统一表达。
//
// 设计建议：
//   - domain 层只构造 Error；
//   - application 层尽量使用 Wrap 携带底层错误链（便于排障），
//     同时通过 Code 决定对外返回的语义；
//   - interfaces 层（HTTP/gRPC）只读 Code/Message，不应再判定具体的下层 error 类型。
type Error struct {
	Code    Code
	Message string
	cause   error
}

// New 构造一个带 Code 与 Message 的业务错误。
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Newf 类似 New，但支持 fmt 风格格式化。
func Newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap 把底层 error 包裹成业务错误，保留链路信息。
func Wrap(err error, code Code, message string) *Error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Message: message, cause: err}
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 暴露原始错误，支持 errors.Is / errors.As。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// FromError 把任意 error 转为 *Error；若已是则原样返回，否则包装成 Internal。
// 非 *Error 的底层细节保留在 cause 中，对外 Message 固定为 InternalMessage。
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: CodeInternal, Message: InternalMessage, cause: err}
}
