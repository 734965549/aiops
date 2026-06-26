package integration

import (
	"context"
	"strings"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// AccountAdapter 实现 IntegrationAccountPort，俾 Observability 只读取账号摘要同能力。
//
// 这个 adapter 不解密凭据，只传 credential_ref_id，避免明文凭据跨上下文流动。
type AccountAdapter struct {
	accounts     integdomain.AccountRepository
	capabilities integdomain.CapabilityRepository
}

func NewAccountAdapter(
	accounts integdomain.AccountRepository,
	capabilities integdomain.CapabilityRepository,
) *AccountAdapter {
	return &AccountAdapter{
		accounts: accounts, capabilities: capabilities,
	}
}

func (a *AccountAdapter) ResolveAccount(ctx context.Context, accountID string) (*domain.AccountSnapshot, error) {
	if a == nil || a.accounts == nil || a.capabilities == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "observability account adapter is not configured")
	}
	accountID = strings.TrimSpace(accountID)
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
		AccountID:       acc.AccountID,
		Provider:        string(acc.Provider),
		AuthType:        string(acc.AuthType),
		Regions:         append([]string(nil), acc.Regions...),
		ProjectID:       acc.ProjectID,
		CredentialRefID: acc.CredentialRefID,
		OwnerTeam:       acc.OwnerTeam,
		Capabilities:    capStrs,
		ExtraConfig:     acc.ExtraConfig,
	}, nil
}
