package identity

import (
	"context"
	"strings"

	alertapp "github.com/734965549/aiops/internal/alert/application"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// AccountAccessChecker 使用统一授权服务校验用户对绑定账号的数据范围。
type AccountAccessChecker struct {
	authorizer *identityapp.AuthorizationService
}

func NewAccountAccessChecker(authorizer *identityapp.AuthorizationService) *AccountAccessChecker {
	return &AccountAccessChecker{authorizer: authorizer}
}

var _ alertapp.AccountAccessChecker = (*AccountAccessChecker)(nil)

func (c *AccountAccessChecker) CheckAccountAccess(ctx context.Context, userID string, account *alertapp.IntegrationAccountInfo) error {
	if c == nil || c.authorizer == nil || account == nil {
		return nil
	}
	if strings.TrimSpace(userID) == "" {
		return nil
	}
	in := identityapp.AuthorizationInput{UserID: userID, ObjectTeam: account.OwnerTeam}
	if len(account.Regions) > 0 {
		in.ObjectRegion = account.Regions[0]
	}
	res, err := c.authorizer.Authorize(ctx, in)
	if err != nil {
		return err
	}
	if res == nil || !res.Allowed {
		return apperr.New(apperr.CodePermissionDenied, "no access to bound integration account")
	}
	return nil
}
