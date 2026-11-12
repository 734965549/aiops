// Package identity 将统一授权服务适配为 AI 证据取数的账号数据权限端口。
package identity

import (
	"context"
	"strings"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// AuthorizationChecker 是统一授权服务的最小依赖面，便于单测注入。
type AuthorizationChecker interface {
	Authorize(ctx context.Context, in identityapp.AuthorizationInput) (*identityapp.AuthorizationResult, error)
}

// AccountAccessAuthorizer 使用统一授权服务校验用户对 integration 账号的数据范围。
type AccountAccessAuthorizer struct {
	checker AuthorizationChecker
}

func NewAccountAccessAuthorizer(checker AuthorizationChecker) *AccountAccessAuthorizer {
	return &AccountAccessAuthorizer{checker: checker}
}

var _ aiapp.AccountAccessAuthorizer = (*AccountAccessAuthorizer)(nil)

func (a *AccountAccessAuthorizer) Authorize(ctx context.Context, userID string, ref aiapp.AccountRef) error {
	if a == nil || a.checker == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	in := identityapp.AuthorizationInput{UserID: strings.TrimSpace(userID), ObjectTeam: strings.TrimSpace(ref.OwnerTeam)}
	if len(ref.Regions) > 0 {
		in.ObjectRegion = strings.TrimSpace(ref.Regions[0])
	}
	res, err := a.checker.Authorize(ctx, in)
	if err != nil {
		return err
	}
	if res == nil || !res.Allowed {
		return apperr.New(apperr.CodePermissionDenied, "no access to integration account for evidence")
	}
	return nil
}
