package integration

import (
	"context"
	"testing"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type accountRepoStub struct {
	acc *integdomain.IntegrationAccount
}

func (r accountRepoStub) Create(context.Context, *integdomain.IntegrationAccount) error { return nil }
func (r accountRepoStub) Update(context.Context, *integdomain.IntegrationAccount) error { return nil }
func (r accountRepoStub) GetByID(context.Context, string) (*integdomain.IntegrationAccount, error) {
	return r.acc, nil
}
func (r accountRepoStub) List(context.Context, integdomain.AccountFilter) ([]integdomain.IntegrationAccount, error) {
	return nil, nil
}
func (r accountRepoStub) Count(context.Context, integdomain.AccountFilter) (int64, error) {
	return 0, nil
}
func (r accountRepoStub) SoftDelete(context.Context, string) error { return nil }

type capabilityRepoStub struct {
	caps []integdomain.Capability
}

func (r capabilityRepoStub) ReplaceForAccount(context.Context, string, []integdomain.Capability) error {
	return nil
}
func (r capabilityRepoStub) ListByAccountID(context.Context, string) ([]integdomain.Capability, error) {
	return r.caps, nil
}

type panicCredentialRepo struct {
	t *testing.T
}

func (r panicCredentialRepo) Create(context.Context, *integdomain.CredentialRef) error { return nil }
func (r panicCredentialRepo) Update(context.Context, *integdomain.CredentialRef) error { return nil }
func (r panicCredentialRepo) GetByAccountID(context.Context, string) (*integdomain.CredentialRef, error) {
	r.t.Fatal("observability account adapter must not load decrypted credentials")
	return nil, nil
}
func (r panicCredentialRepo) DeleteByAccountID(context.Context, string) error { return nil }

type panicVault struct {
	t *testing.T
}

func (v panicVault) Encrypt(integdomain.CredentialMaterial) ([]byte, string, error) {
	return nil, "", nil
}
func (v panicVault) Decrypt([]byte) (integdomain.CredentialMaterial, error) {
	v.t.Fatal("observability account adapter must not decrypt credentials")
	return nil, nil
}
func (v panicVault) Fingerprint(integdomain.CredentialMaterial) string { return "" }

func TestAccountAdapterResolveAccountDoesNotDecryptCredentials(t *testing.T) {
	adapter := NewAccountAdapter(
		accountRepoStub{acc: &integdomain.IntegrationAccount{
			AccountID: "acc-1", Provider: integdomain.ProviderHuaweiCloud, AuthType: integdomain.AuthAKSK,
			CredentialRefID: "cred-1", Enabled: true,
		}},
		panicCredentialRepo{t: t},
		capabilityRepoStub{caps: []integdomain.Capability{integdomain.CapabilityMetrics}},
		panicVault{t: t},
	)

	acc, err := adapter.ResolveAccount(context.Background(), "acc-1")
	if err != nil {
		t.Fatalf("ResolveAccount: %v", err)
	}
	if acc.AccountID != "acc-1" || acc.Provider != string(integdomain.ProviderHuaweiCloud) {
		t.Fatalf("unexpected account snapshot: %+v", acc)
	}
	if len(acc.Capabilities) != 1 || acc.Capabilities[0] != string(integdomain.CapabilityMetrics) {
		t.Fatalf("unexpected capabilities: %+v", acc.Capabilities)
	}
}

func TestAccountAdapterResolveAccountMissingCapabilityRepoReturnsUnavailable(t *testing.T) {
	adapter := NewAccountAdapter(accountRepoStub{}, nil, nil, nil)

	_, err := adapter.ResolveAccount(context.Background(), "acc-1")
	if err == nil {
		t.Fatal("expected configuration error")
	}
	if ae := apperr.FromError(err); ae.Code != apperr.CodeUnavailable {
		t.Fatalf("expected UNAVAILABLE, got %s", ae.Code)
	}
}
