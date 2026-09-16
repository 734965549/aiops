package huawei

import (
	"context"
	"errors"
	"strings"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// AKSKCredential 华为云 AK/SK 凭据，仅在 infrastructure 内部流转，禁止写入日志或响应。
type AKSKCredential struct {
	AccessKey string
	SecretKey string
}

// CredentialProvider 通过 integration_credential_ref 与 Vault 解密华为云 AK/SK。
type CredentialProvider struct {
	credentials integdomain.CredentialRepository
	vault       integdomain.CredentialVault
}

func NewCredentialProvider(
	credentials integdomain.CredentialRepository,
	vault integdomain.CredentialVault,
) *CredentialProvider {
	return &CredentialProvider{credentials: credentials, vault: vault}
}

// ResolveAKSK 根据 ProviderContext 中的账号摘要加载并解密 AK/SK。
func (p *CredentialProvider) ResolveAKSK(ctx context.Context, account domain.AccountSnapshot) (AKSKCredential, error) {
	if p == nil || p.credentials == nil || p.vault == nil {
		return AKSKCredential{}, apperr.New(apperr.CodeUnavailable, "huawei credential provider is not configured")
	}
	accountID := strings.TrimSpace(account.AccountID)
	if accountID == "" {
		return AKSKCredential{}, domain.ErrInvalidArgument
	}
	authType := integdomain.AuthType(strings.TrimSpace(account.AuthType))
	switch authType {
	case integdomain.AuthNone:
		return AKSKCredential{}, apperr.New(apperr.CodeFailedPrecondition, "huawei account has no credential configured")
	case integdomain.AuthAgency:
		return AKSKCredential{}, apperr.New(apperr.CodeFailedPrecondition, "agency auth is not supported for ces metrics yet")
	case integdomain.AuthAKSK:
	default:
		return AKSKCredential{}, apperr.New(apperr.CodeFailedPrecondition, "unsupported auth type for huawei ces")
	}
	refID := strings.TrimSpace(account.CredentialRefID)
	if refID == "" {
		return AKSKCredential{}, apperr.New(apperr.CodeFailedPrecondition, "credential is not configured")
	}
	ref, err := p.credentials.GetByAccountID(ctx, accountID)
	if err != nil {
		return AKSKCredential{}, wrapCredentialError(err)
	}
	if ref == nil || len(ref.Ciphertext) == 0 {
		return AKSKCredential{}, integdomain.ErrCredentialRequired
	}
	material, err := p.vault.Decrypt(ref.Ciphertext)
	if err != nil {
		return AKSKCredential{}, apperr.Wrap(err, apperr.CodeInternal, "decrypt huawei credential failed")
	}
	ak := strings.TrimSpace(material["access_key"])
	sk := strings.TrimSpace(material["secret_key"])
	if ak == "" || sk == "" {
		return AKSKCredential{}, apperr.New(apperr.CodeFailedPrecondition, "access_key and secret_key are required")
	}
	return AKSKCredential{AccessKey: ak, SecretKey: sk}, nil
}

func wrapCredentialError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, integdomain.ErrNotFound) {
		return apperr.New(apperr.CodeFailedPrecondition, "credential is not configured")
	}
	return apperr.MapSentinels(err, "load credential ref failed",
		apperr.Sentinel{Err: integdomain.ErrCredentialRequired, Code: apperr.CodeFailedPrecondition},
	)
}
