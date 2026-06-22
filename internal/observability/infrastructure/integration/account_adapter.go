package integration

import (
	"context"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// AccountAdapter 实现 IntegrationAccountPort，复用 Integration 账号与能力仓储。
type AccountAdapter struct {
	accounts     integdomain.AccountRepository
	capabilities integdomain.CapabilityRepository
}

func NewAccountAdapter(
	accounts integdomain.AccountRepository,
	credentials integdomain.CredentialRepository,
	capabilities integdomain.CapabilityRepository,
	vault integdomain.CredentialVault,
) *AccountAdapter {
	return &AccountAdapter{
		accounts: accounts, capabilities: capabilities,
	}
}

func (a *AccountAdapter) ResolveAccount(ctx context.Context, accountID string) (*domain.AccountSnapshot, error) {
	if a == nil || a.accounts == nil || a.capabilities == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "observability account adapter is not configured")
	}
	if accountID == "" {
		return nil, integdomain.ErrNotFound
	}
	acc, err := a.accounts.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if acc == nil || acc.Deleted {
		return nil, integdomain.ErrNotFound
	}
	if !acc.Enabled {
		return nil, integdomain.ErrAccountDisabled
	}
	caps, err := a.capabilities.ListByAccountID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	capStrs := make([]string, 0, len(caps))
	for _, c := range caps {
		capStrs = append(capStrs, string(c))
	}
	return &domain.AccountSnapshot{
		AccountID:    acc.AccountID,
		Provider:     string(acc.Provider),
		AuthType:     string(acc.AuthType),
		Regions:      append([]string(nil), acc.Regions...),
		ProjectID:    acc.ProjectID,
		OwnerTeam:    acc.OwnerTeam,
		Capabilities: capStrs,
	}, nil
}
