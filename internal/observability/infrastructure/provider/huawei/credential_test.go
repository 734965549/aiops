package huawei

import (
	"context"
	"errors"
	"strings"
	"testing"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	integcred "github.com/734965549/aiops/internal/integration/infrastructure/credential"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type memCredentialRepo struct {
	byAccount map[string]*integdomain.CredentialRef
}

func (r *memCredentialRepo) Update(_ context.Context, ref *integdomain.CredentialRef) error {
	r.byAccount[ref.AccountID] = ref
	return nil
}

func (r *memCredentialRepo) Create(_ context.Context, ref *integdomain.CredentialRef) error {
	r.byAccount[ref.AccountID] = ref
	return nil
}

func (r *memCredentialRepo) GetByAccountID(_ context.Context, accountID string) (*integdomain.CredentialRef, error) {
	ref, ok := r.byAccount[accountID]
	if !ok {
		return nil, integdomain.ErrNotFound
	}
	cp := *ref
	return &cp, nil
}

func (r *memCredentialRepo) DeleteByAccountID(_ context.Context, accountID string) error {
	delete(r.byAccount, accountID)
	return nil
}

type memVault struct {
	vault integdomain.CredentialVault
}

func newMemVault(t *testing.T) *memVault {
	t.Helper()
	v, err := integcred.NewVault("unit-test-huawei-credential-key", 1)
	if err != nil {
		t.Fatalf("new vault: %v", err)
	}
	return &memVault{vault: v}
}

func (v *memVault) Encrypt(material integdomain.CredentialMaterial) ([]byte, string, error) {
	return v.vault.Encrypt(material)
}

func (v *memVault) Decrypt(ciphertext []byte) (integdomain.CredentialMaterial, error) {
	return v.vault.Decrypt(ciphertext)
}

func (v *memVault) Fingerprint(material integdomain.CredentialMaterial) string {
	return v.vault.Fingerprint(material)
}

func newTestCredentialProvider(t *testing.T) (*CredentialProvider, *memCredentialRepo, *memVault) {
	t.Helper()
	repo := &memCredentialRepo{byAccount: map[string]*integdomain.CredentialRef{}}
	vault := newMemVault(t)
	return NewCredentialProvider(repo, vault), repo, vault
}

func seedAKSKCredential(t *testing.T, provider *CredentialProvider, repo *memCredentialRepo, vault *memVault, accountID string) {
	t.Helper()
	material := integdomain.CredentialMaterial{
		"access_key": "AKTEST123456",
		"secret_key": "SKTEST9876543210",
	}
	ciphertext, _, err := vault.Encrypt(material)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	repo.byAccount[accountID] = &integdomain.CredentialRef{
		AccountID:  accountID,
		Ciphertext: ciphertext,
	}
}

func TestCredentialProviderResolveAKSKSuccess(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-1"
	seedAKSKCredential(t, provider, repo, vault, accountID)

	cred, err := provider.ResolveAKSK(context.Background(), domain.AccountSnapshot{
		AccountID:       accountID,
		AuthType:        string(integdomain.AuthAKSK),
		CredentialRefID: "cref-1",
	})
	if err != nil {
		t.Fatalf("ResolveAKSK: %v", err)
	}
	if cred.AccessKey != "AKTEST123456" || cred.SecretKey != "SKTEST9876543210" {
		t.Fatalf("unexpected credential: %+v", cred)
	}
}

func TestCredentialProviderResolveAKSKMissingRef(t *testing.T) {
	provider, _, _ := newTestCredentialProvider(t)
	_, err := provider.ResolveAKSK(context.Background(), domain.AccountSnapshot{
		AccountID: "acc-missing",
		AuthType:  string(integdomain.AuthAKSK),
	})
	if !errors.Is(err, integdomain.ErrCredentialRequired) && apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("expected credential required, got %v", err)
	}
	assertNoSensitiveLeak(t, err, "AKTEST", "SKTEST", "Authorization")
}

func TestCredentialProviderResolveAKSKMissingStoredRefIsMisconfiguration(t *testing.T) {
	provider, _, _ := newTestCredentialProvider(t)
	_, err := provider.ResolveAKSK(context.Background(), domain.AccountSnapshot{
		AccountID:       "acc-missing-ref",
		AuthType:        string(integdomain.AuthAKSK),
		CredentialRefID: "cred-missing",
	})
	if apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestCredentialProviderResolveAKSKIncompleteMaterial(t *testing.T) {
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-2"
	material := integdomain.CredentialMaterial{"access_key": "AKONLY"}
	ciphertext, _, err := vault.Encrypt(material)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	repo.byAccount[accountID] = &integdomain.CredentialRef{
		AccountID: accountID, Ciphertext: ciphertext,
	}

	_, err = provider.ResolveAKSK(context.Background(), domain.AccountSnapshot{
		AccountID: accountID, AuthType: string(integdomain.AuthAKSK), CredentialRefID: "cref-2",
	})
	if apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
	assertNoSensitiveLeak(t, err, "AKONLY")
}

func TestCredentialProviderResolveAKSKUnsupportedAuthType(t *testing.T) {
	provider, _, _ := newTestCredentialProvider(t)
	_, err := provider.ResolveAKSK(context.Background(), domain.AccountSnapshot{
		AccountID: "acc-3", AuthType: string(integdomain.AuthAPIToken),
	})
	if apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestCredentialProviderResolveAKSKNotConfigured(t *testing.T) {
	_, err := (*CredentialProvider)(nil).ResolveAKSK(context.Background(), domain.AccountSnapshot{
		AccountID: "acc-4", AuthType: string(integdomain.AuthAKSK),
	})
	if apperr.CodeOf(err) != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func assertNoSensitiveLeak(t *testing.T, err error, secrets ...string) {
	t.Helper()
	msg := err.Error()
	for _, s := range secrets {
		if s != "" && strings.Contains(msg, s) {
			t.Fatalf("error leaked sensitive value %q: %q", s, msg)
		}
	}
}
