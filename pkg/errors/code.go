// Package errors 定义平台统一错误模型与错误码。
//
// 与 gRPC / Google API Design Guide 的 Code 命名保持一致，便于将来切换 Transport。
// 协议相关的状态码映射（如 HTTP）放在 pkg/transport 各子包，本包只保留 Code/Error。
package errors

// Code 是平台错误码字符串枚举。
type Code string

const (
	CodeOK                 Code = "OK"
	CodeInvalidArgument    Code = "INVALID_ARGUMENT"
	CodeUnauthenticated    Code = "UNAUTHENTICATED"
	CodePermissionDenied   Code = "PERMISSION_DENIED"
	CodeNotFound           Code = "NOT_FOUND"
	CodeAlreadyExists      Code = "ALREADY_EXISTS"
	CodeFailedPrecondition Code = "FAILED_PRECONDITION"
	CodeAborted            Code = "ABORTED"
	CodeResourceExhausted  Code = "RESOURCE_EXHAUSTED"
	CodePayloadTooLarge    Code = "PAYLOAD_TOO_LARGE"
	CodeUnavailable        Code = "UNAVAILABLE"
	CodeInternal           Code = "INTERNAL"
)
