package huawei

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdkerr"
)

// mapCESError 将华为云 SDK/网络错误映射为平台领域错误，不向外暴露原始敏感错误文案。
func mapCESError(err error) error {
	if err == nil {
		return nil
	}
	var svcErr *sdkerr.ServiceResponseError
	if errors.As(err, &svcErr) {
		return mapServiceResponseError(svcErr)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: huawei ces request timed out", domain.ErrProviderUnavailable)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: huawei ces request canceled", domain.ErrProviderUnavailable)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%w: huawei ces request timed out", domain.ErrProviderUnavailable)
	}
	return fmt.Errorf("%w: huawei ces request failed", domain.ErrProviderUnavailable)
}

func mapServiceResponseError(svcErr *sdkerr.ServiceResponseError) error {
	if svcErr == nil {
		return fmt.Errorf("%w: huawei ces request failed", domain.ErrProviderUnavailable)
	}
	switch svcErr.StatusCode {
	case 400:
		return mapCESClientError(svcErr)
	case 401:
		return fmt.Errorf("%w: provider authentication failed", domain.ErrProviderUnavailable)
	case 403:
		return fmt.Errorf("%w: provider permission denied", domain.ErrProviderUnavailable)
	case 404:
		return fmt.Errorf("%w: metric or resource not found", domain.ErrNotFound)
	case 429:
		return apperr.New(apperr.CodeResourceExhausted, "huawei ces rate limit exceeded")
	case 502, 503, 504:
		return fmt.Errorf("%w: huawei ces service unavailable", domain.ErrProviderUnavailable)
	default:
		if svcErr.StatusCode >= 500 {
			return fmt.Errorf("%w: huawei ces service unavailable", domain.ErrProviderUnavailable)
		}
		return mapCESClientError(svcErr)
	}
}

func mapCESClientError(svcErr *sdkerr.ServiceResponseError) error {
	code := strings.ToUpper(strings.TrimSpace(svcErr.ErrorCode))
	switch {
	case strings.Contains(code, "RATE"):
		return apperr.New(apperr.CodeResourceExhausted, "huawei ces rate limit exceeded")
	case strings.Contains(code, "NOT_FOUND"), strings.Contains(code, "NOTFOUND"):
		return fmt.Errorf("%w: metric or resource not found", domain.ErrNotFound)
	case strings.Contains(code, "AUTH"), strings.Contains(code, "TOKEN"), strings.Contains(code, "CREDENTIAL"):
		return fmt.Errorf("%w: provider authentication failed", domain.ErrProviderUnavailable)
	case strings.Contains(code, "PERMISSION"), strings.Contains(code, "FORBIDDEN"):
		return fmt.Errorf("%w: provider permission denied", domain.ErrProviderUnavailable)
	default:
		return fmt.Errorf("%w: invalid ces query parameters", domain.ErrInvalidArgument)
	}
}
