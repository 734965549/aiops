package application

import (
	"errors"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/auth"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// mapDomainError 将 identity domain 哨兵错误映射为 pkg/errors 统一错误码。
func mapDomainError(err error) error {
	return apperr.MapSentinels(err, "identity operation failed",
		apperr.Sentinel{Err: domain.ErrAlreadyExists, Code: apperr.CodeAlreadyExists},
		apperr.Sentinel{Err: domain.ErrSessionNotFound, Code: apperr.CodeNotFound},
		apperr.Sentinel{Err: domain.ErrOAuthStateNotFound, Code: apperr.CodeUnauthenticated},
		apperr.Sentinel{Err: domain.ErrOAuthStateInvalid, Code: apperr.CodeUnauthenticated},
		apperr.Sentinel{Err: domain.ErrInvalidCredentials, Code: apperr.CodeUnauthenticated},
	)
}

// mapAlreadyExists 将唯一冲突映射为 ALREADY_EXISTS，并保留对外 message。
func mapAlreadyExists(err error, message string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrAlreadyExists) {
		return apperr.New(apperr.CodeAlreadyExists, message)
	}
	return err
}

// mapAuthError 将 pkg/auth 哨兵错误映射为对外认证语义。
func mapAuthError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, auth.ErrPasswordMismatch):
		return apperr.New(apperr.CodeUnauthenticated, "invalid username or password")
	case errors.Is(err, auth.ErrInvalidToken):
		return apperr.New(apperr.CodeUnauthenticated, "invalid or expired token")
	default:
		return err
	}
}

// wrapIdentityOpError 先尝试 domain 哨兵映射，否则 Wrap 为 INTERNAL。
func wrapIdentityOpError(err error, op string) error {
	if err == nil {
		return nil
	}
	if mapped := mapDomainError(err); apperr.FromError(mapped).Code != apperr.CodeInternal {
		return mapped
	}
	return apperr.Wrap(err, apperr.CodeInternal, op)
}
