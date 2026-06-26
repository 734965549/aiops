package integration

import (
	"context"
	"errors"
	"strings"

	assetapp "github.com/734965549/aiops/internal/asset/application"
	integdomain "github.com/734965549/aiops/internal/integration/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// AccountAdapter 将 Integration 仓储适配为 Asset Sync 账号端口。
type AccountAdapter struct {
	accounts     integdomain.AccountRepository
	capabilities integdomain.CapabilityRepository
}

func NewAccountAdapter(accounts integdomain.AccountRepository, capabilities integdomain.CapabilityRepository) *AccountAdapter {
	return &AccountAdapter{accounts: accounts, capabilities: capabilities}
}

func (a *AccountAdapter) ResolveSyncAccount(ctx context.Context, accountID string) (*assetapp.SyncAccountSnapshot, error) {
	if a == nil || a.accounts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "integration account adapter is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "account_id is required")
	}
	acc, err := a.accounts.GetByID(ctx, accountID)
	if err != nil {
		return nil, mapAccountError(err)
	}
	if acc == nil {
		return nil, apperr.New(apperr.CodeNotFound, "integration account not found")
	}
	if !acc.Enabled {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "integration account is disabled")
	}
	if a.capabilities != nil {
		caps, capErr := a.capabilities.ListByAccountID(ctx, accountID)
		if capErr != nil {
			return nil, apperr.Wrap(capErr, apperr.CodeInternal, "list integration capabilities failed")
		}
		if !hasCapability(caps, integdomain.CapabilityAssets) {
			return nil, apperr.New(apperr.CodeFailedPrecondition, "integration account does not support assets capability")
		}
	}
	return &assetapp.SyncAccountSnapshot{
		AccountID:   acc.AccountID,
		Provider:    string(acc.Provider),
		Regions:     append([]string(nil), acc.Regions...),
		Enabled:     acc.Enabled,
		ExtraConfig: acc.ExtraConfig,
	}, nil
}

func hasCapability(caps []integdomain.Capability, want integdomain.Capability) bool {
	for _, cap := range caps {
		if cap == want {
			return true
		}
	}
	return false
}

func mapAccountError(err error) error {
	if err == nil {
		return nil
	}
	if err == integdomain.ErrNotFound || errors.Is(err, integdomain.ErrNotFound) {
		return apperr.New(apperr.CodeNotFound, "integration account not found")
	}
	return apperr.Wrap(err, apperr.CodeInternal, "load integration account failed")
}

var _ assetapp.IntegrationAccountPort = (*AccountAdapter)(nil)
