package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/internal/integration/infrastructure/credential"
)

type fakeUnitOfWork struct {
	repos domain.TransactionRepositories
}

func (f *fakeUnitOfWork) WithinTransaction(ctx context.Context, fn func(ctx context.Context, repos domain.TransactionRepositories) error) error {
	return fn(ctx, f.repos)
}

type memAccountRepo struct {
	byID map[string]*domain.IntegrationAccount
}

func newMemAccountRepo() *memAccountRepo {
	return &memAccountRepo{byID: map[string]*domain.IntegrationAccount{}}
}

func (r *memAccountRepo) Create(_ context.Context, account *domain.IntegrationAccount) error {
	cp := *account
	r.byID[account.AccountID] = &cp
	return nil
}

func (r *memAccountRepo) Update(_ context.Context, account *domain.IntegrationAccount) error {
	if _, ok := r.byID[account.AccountID]; !ok {
		return domain.ErrNotFound
	}
	cp := *account
	r.byID[account.AccountID] = &cp
	return nil
}

func (r *memAccountRepo) GetByID(_ context.Context, accountID string) (*domain.IntegrationAccount, error) {
	acc, ok := r.byID[accountID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *acc
	return &cp, nil
}

func (r *memAccountRepo) List(context.Context, domain.AccountFilter) ([]domain.IntegrationAccount, error) {
	return nil, nil
}

func (r *memAccountRepo) Count(context.Context, domain.AccountFilter) (int64, error) {
	return 0, nil
}

func (r *memAccountRepo) SoftDelete(context.Context, string) error { return nil }

type memCredentialRepo struct {
	byAccount map[string]*domain.CredentialRef
}

func newMemCredentialRepo() *memCredentialRepo {
	return &memCredentialRepo{byAccount: map[string]*domain.CredentialRef{}}
}

func (r *memCredentialRepo) Create(_ context.Context, ref *domain.CredentialRef) error {
	cp := *ref
	r.byAccount[ref.AccountID] = &cp
	return nil
}

func (r *memCredentialRepo) Update(_ context.Context, ref *domain.CredentialRef) error {
	if _, ok := r.byAccount[ref.AccountID]; !ok {
		return domain.ErrNotFound
	}
	cp := *ref
	r.byAccount[ref.AccountID] = &cp
	return nil
}

func (r *memCredentialRepo) GetByAccountID(_ context.Context, accountID string) (*domain.CredentialRef, error) {
	ref, ok := r.byAccount[accountID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *ref
	return &cp, nil
}

func (r *memCredentialRepo) DeleteByAccountID(context.Context, string) error { return nil }

type memCapabilityRepo struct {
	byAccount map[string][]domain.Capability
}

func newMemCapabilityRepo() *memCapabilityRepo {
	return &memCapabilityRepo{byAccount: map[string][]domain.Capability{}}
}

func (r *memCapabilityRepo) ReplaceForAccount(_ context.Context, accountID string, caps []domain.Capability) error {
	cp := append([]domain.Capability(nil), caps...)
	r.byAccount[accountID] = cp
	return nil
}

func (r *memCapabilityRepo) ListByAccountID(_ context.Context, accountID string) ([]domain.Capability, error) {
	return r.byAccount[accountID], nil
}

type memCheckRepo struct {
	latest map[string]*domain.ConnectivityCheck
}

func newMemCheckRepo() *memCheckRepo {
	return &memCheckRepo{latest: map[string]*domain.ConnectivityCheck{}}
}

func (r *memCheckRepo) Create(_ context.Context, check *domain.ConnectivityCheck) error {
	cp := *check
	r.latest[check.AccountID] = &cp
	return nil
}

func (r *memCheckRepo) LatestByAccountID(_ context.Context, accountID string) (*domain.ConnectivityCheck, error) {
	check, ok := r.latest[accountID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *check
	return &cp, nil
}

func newTestAccountService(t *testing.T) (*AccountService, *memAccountRepo, *memCredentialRepo) {
	t.Helper()
	accounts := newMemAccountRepo()
	creds := newMemCredentialRepo()
	caps := newMemCapabilityRepo()
	checks := newMemCheckRepo()
	vault, err := credential.NewVault("unit-test-credential-key", 1)
	if err != nil {
		t.Fatalf("new vault: %v", err)
	}
	uow := &fakeUnitOfWork{repos: domain.TransactionRepositories{
		Accounts: accounts, Credentials: creds, Capabilities: caps, Checks: checks,
	}}
	svc := NewAccountService(accounts, creds, caps, checks, vault, nil, nil, uow)
	return svc, accounts, creds
}

func TestCreateAuthNoneSkipsCredentialVault(t *testing.T) {
	svc, accounts, creds := newTestAccountService(t)
	dto, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "prom-no-auth", Provider: string(domain.ProviderPrometheus), AuthType: string(domain.AuthNone),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if dto.HasCredential {
		t.Fatal("expected has_credential=false for auth_type=none without credential")
	}
	acc := accounts.byID[dto.AccountID]
	if acc.CredentialRefID != "" {
		t.Fatalf("expected empty credential_ref_id, got %q", acc.CredentialRefID)
	}
	if len(creds.byAccount) != 0 {
		t.Fatal("expected no credential ref stored")
	}
}

func TestCreateAuthNoneStoresOptionalConfig(t *testing.T) {
	svc, accounts, creds := newTestAccountService(t)
	dto, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "prom-config", Provider: string(domain.ProviderPrometheus), AuthType: string(domain.AuthNone),
		Credential: map[string]string{"base_url": "http://127.0.0.1:9090"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !dto.HasCredential {
		t.Fatal("expected has_credential=true when optional config provided")
	}
	if accounts.byID[dto.AccountID].CredentialRefID == "" {
		t.Fatal("expected credential_ref_id set")
	}
	if len(creds.byAccount) != 1 {
		t.Fatalf("expected one credential ref, got %d", len(creds.byAccount))
	}
}

func TestCreateStoresExtraConfig(t *testing.T) {
	svc, accounts, _ := newTestAccountService(t)
	dto, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "hw-ces", Provider: string(domain.ProviderHuaweiCloud), AuthType: string(domain.AuthNone),
		ExtraConfig: map[string]any{"sync_mode": "ces", "max_resources": float64(5000)},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	acc := accounts.byID[dto.AccountID]
	if string(acc.ExtraConfig) != `{"max_resources":5000,"sync_mode":"ces"}` {
		t.Fatalf("extra_config = %s", string(acc.ExtraConfig))
	}
	if dto.ExtraConfig["sync_mode"] != "ces" {
		t.Fatalf("dto extra_config = %+v", dto.ExtraConfig)
	}
}

func TestCreateRejectsSecretInExtraConfig(t *testing.T) {
	svc, _, _ := newTestAccountService(t)
	_, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "hw-bad", Provider: string(domain.ProviderHuaweiCloud), AuthType: string(domain.AuthNone),
		ExtraConfig: map[string]any{"secret_key": "do-not-store"},
	})
	if err == nil {
		t.Fatal("expected extra_config secret rejection")
	}
}

func TestCheckConnectivityAuthNoneWithoutCredential(t *testing.T) {
	svc, _, _ := newTestAccountService(t)
	dto, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "hw-none", Provider: string(domain.ProviderHuaweiCloud), AuthType: string(domain.AuthNone),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	svc.checkers = map[domain.ProviderType]domain.ProviderChecker{
		domain.ProviderHuaweiCloud: &noopChecker{provider: domain.ProviderHuaweiCloud},
	}
	check, err := svc.CheckConnectivity(context.Background(), dto.AccountID, Actor{UserID: "u1"})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if check.Status != string(domain.ConnectivityOK) {
		t.Fatalf("expected ok status, got %q", check.Status)
	}
}

type noopChecker struct{ provider domain.ProviderType }

func (n *noopChecker) Provider() domain.ProviderType { return n.provider }

func (n *noopChecker) CheckConnectivity(_ context.Context, account domain.IntegrationAccount, _ domain.CredentialMaterial) (*domain.ConnectivityCheck, error) {
	return &domain.ConnectivityCheck{
		AccountID: account.AccountID, Status: domain.ConnectivityOK, Provider: account.Provider,
		Capabilities: domain.DefaultCapabilitiesForProvider(account.Provider), Message: "ok",
	}, nil
}

func TestValidateCredentialInputPrometheusNoneAllowsEmptyBaseURL(t *testing.T) {
	if err := validateCredentialInput(domain.ProviderPrometheus, domain.AuthNone, nil); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateCredentialInputPrometheusAKSKRequiresBaseURL(t *testing.T) {
	err := validateCredentialInput(domain.ProviderPrometheus, domain.AuthAKSK, map[string]string{
		"access_key": "AK", "secret_key": "SK",
	})
	if err == nil {
		t.Fatal("expected base_url required error")
	}
}

func TestCreateRejectsCustomProvider(t *testing.T) {
	svc, _, _ := newTestAccountService(t)
	_, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "custom", Provider: "custom", AuthType: string(domain.AuthNone),
	})
	if err == nil {
		t.Fatal("expected invalid provider error for custom")
	}
}

func TestCreateRejectsUnsupportedProviderAuthType(t *testing.T) {
	svc, _, _ := newTestAccountService(t)
	_, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "hw-token", Provider: string(domain.ProviderHuaweiCloud), AuthType: string(domain.AuthAPIToken),
		Credential: map[string]string{"api_token": "tok"},
	})
	if err == nil {
		t.Fatal("expected error for huawei_cloud + api_token")
	}
	_, err = svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "signoz-aksk", Provider: string(domain.ProviderSigNoz), AuthType: string(domain.AuthAKSK),
		Credential: map[string]string{"access_key": "AK", "secret_key": "SK"},
	})
	if err == nil {
		t.Fatal("expected error for signoz + ak_sk")
	}
}

func TestUpdateRejectsUnsupportedProviderAuthType(t *testing.T) {
	svc, _, _ := newTestAccountService(t)
	dto, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "signoz", Provider: string(domain.ProviderSigNoz), AuthType: string(domain.AuthNone),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	authType := string(domain.AuthAKSK)
	_, err = svc.Update(context.Background(), dto.AccountID, Actor{UserID: "u1"}, UpdateAccountInput{
		AuthType: &authType,
	})
	if err == nil {
		t.Fatal("expected error when switching signoz to ak_sk")
	}
}

func TestUpdateAuthTypeToNonNoneRequiresCredential(t *testing.T) {
	svc, _, _ := newTestAccountService(t)
	dto, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "prom", Provider: string(domain.ProviderPrometheus), AuthType: string(domain.AuthNone),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	authType := string(domain.AuthAPIToken)
	_, err = svc.Update(context.Background(), dto.AccountID, Actor{UserID: "u1"}, UpdateAccountInput{
		AuthType: &authType,
	})
	if err == nil {
		t.Fatal("expected error when switching to api_token without credential")
	}
}

func TestUpdateAuthTypeToNonNoneWithCredential(t *testing.T) {
	svc, _, _ := newTestAccountService(t)
	dto, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "prom", Provider: string(domain.ProviderPrometheus), AuthType: string(domain.AuthNone),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	authType := string(domain.AuthAPIToken)
	updated, err := svc.Update(context.Background(), dto.AccountID, Actor{UserID: "u1"}, UpdateAccountInput{
		AuthType:   &authType,
		Credential: map[string]string{"api_token": "tok", "base_url": "http://127.0.0.1:9090"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.AuthType != string(domain.AuthAPIToken) {
		t.Fatalf("expected auth_type api_token, got %q", updated.AuthType)
	}
	if !updated.HasCredential {
		t.Fatal("expected has_credential=true after storing credential")
	}
}

func TestUpdateProviderToPrometheusWithoutBaseURLRejectsStoredCredential(t *testing.T) {
	svc, _, _ := newTestAccountService(t)
	dto, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "hw", Provider: string(domain.ProviderHuaweiCloud), AuthType: string(domain.AuthAKSK),
		Credential: map[string]string{"access_key": "AK", "secret_key": "SK"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	provider := string(domain.ProviderPrometheus)
	authType := string(domain.AuthAKSK)
	_, err = svc.Update(context.Background(), dto.AccountID, Actor{UserID: "u1"}, UpdateAccountInput{
		Provider: &provider,
		AuthType: &authType,
	})
	if err == nil {
		t.Fatal("expected error when switching to prometheus without base_url in stored credential")
	}
}

func TestUpdateProviderRefreshesCapabilities(t *testing.T) {
	svc, _, _ := newTestAccountService(t)
	dto, err := svc.Create(context.Background(), Actor{UserID: "u1"}, CreateAccountInput{
		Name: "prom", Provider: string(domain.ProviderPrometheus), AuthType: string(domain.AuthNone),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	provider := string(domain.ProviderSigNoz)
	updated, err := svc.Update(context.Background(), dto.AccountID, Actor{UserID: "u1"}, UpdateAccountInput{
		Provider: &provider,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	want := []string{"metrics", "logs", "traces", "topology", "alerts"}
	if len(updated.Capabilities) != len(want) {
		t.Fatalf("expected %d capabilities, got %v", len(want), updated.Capabilities)
	}
	for i, c := range want {
		if updated.Capabilities[i] != c {
			t.Fatalf("expected capabilities %v, got %v", want, updated.Capabilities)
		}
	}
}

func TestSanitizeConnectivityMessageRedactsCredentialValues(t *testing.T) {
	got := sanitizeConnectivityMessage("provider rejected secret-value", "secret-value")
	if got != "connectivity check failed" {
		t.Fatalf("expected generic message, got %q", got)
	}
	got = sanitizeConnectivityMessage("provider timeout")
	if got != "provider timeout" {
		t.Fatalf("expected benign message preserved, got %q", got)
	}
}
