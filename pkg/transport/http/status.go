package http

import (
	"net/http"

	apperr "github.com/734965549/aiops/pkg/errors"
)

// HTTPStatus 将平台错误码映射到 HTTP 状态码。
func HTTPStatus(c apperr.Code) int {
	switch c {
	case apperr.CodeOK:
		return http.StatusOK
	case apperr.CodeInvalidArgument:
		return http.StatusBadRequest
	case apperr.CodeUnauthenticated:
		return http.StatusUnauthorized
	case apperr.CodePermissionDenied:
		return http.StatusForbidden
	case apperr.CodeNotFound:
		return http.StatusNotFound
	case apperr.CodeAlreadyExists:
		return http.StatusConflict
	case apperr.CodeFailedPrecondition:
		return http.StatusPreconditionFailed
	case apperr.CodeAborted:
		return http.StatusConflict
	case apperr.CodeResourceExhausted:
		return http.StatusTooManyRequests
	case apperr.CodePayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case apperr.CodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
