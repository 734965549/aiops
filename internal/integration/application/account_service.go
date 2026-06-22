package application

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/integration/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// AccountService 管理云账号/观测平台账号接入（ops/cloud-observability-contract.md §4）。
type AccountService struct {
	accounts     domain.AccountRepository
	credentials  domain.CredentialRepository
	capabilities domain.CapabilityRepository
	checks       domain.CheckResultRepository
	vault        domain.CredentialVault
	checkers     map[domain.ProviderType]domain.ProviderChecker
	audit        AuditRecorder
	uow          domain.UnitOfWork
}

func NewAccountService(
	accounts domain.AccountRepository,
	credentials domain.CredentialRepository,
	capabilities domain.CapabilityRepository,
	checks domain.CheckResultRepository,
	vault domain.CredentialVault,
	checkers []domain.ProviderChecker,
	audit AuditRecorder,
	uow domain.UnitOfWork,
) *AccountService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	m := make(map[domain.ProviderType]domain.ProviderChecker, len(checkers))
	for _, c := range checkers {
		if c == nil {
			continue
		}
		m[c.Provider()] = c
	}
	return &AccountService{
		accounts: accounts, credentials: credentials, capabilities: capabilities,
		checks: checks, vault: vault, checkers: m, audit: audit, uow: uow,
	}
}

type CreateAccountInput struct {
	AccountID   string
	Name        string
	Provider    string
	AuthType    string
	Regions     []string
	ProjectID   string
	Credential  map[string]string
	Enabled     *bool
	OwnerTeam   string
	Description string
}

type UpdateAccountInput struct {
	Name        *string
	Provider    *string
	AuthType    *string
	Regions     []string
	RegionsSet  bool
	ProjectID   *string
	Credential  map[string]string
	Enabled     *bool
	OwnerTeam   *string
	Description *string
}

type ListAccountsQuery struct {
	Page     int
	PageSize int
	Provider string
	Enabled  *bool
	Keyword  string
}

func (s *AccountService) List(ctx context.Context, q ListAccountsQuery) ([]AccountDTO, int64, error) {
	if s == nil || s.accounts == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "integration account service is not enabled")
	}
	filter := domain.AccountFilter{
		Provider: strings.TrimSpace(q.Provider),
		Enabled:  q.Enabled,
		Keyword:  strings.TrimSpace(q.Keyword),
		Limit:    q.PageSize,
		Offset:   (q.Page - 1) * q.PageSize,
	}
	total, err := s.accounts.Count(ctx, filter)
	if err != nil {
		return nil, 0, wrapIntegrationError(err, "count integration accounts failed")
	}
	rows, err := s.accounts.List(ctx, filter)
	if err != nil {
		return nil, 0, wrapIntegrationError(err, "list integration accounts failed")
	}
	out := make([]AccountDTO, 0, len(rows))
	for _, row := range rows {
		dto, err := s.toAccountDTO(ctx, row)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, dto)
	}
	return out, total, nil
}

func (s *AccountService) Get(ctx context.Context, accountID string) (*AccountDTO, error) {
	if s == nil || s.accounts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "integration account service is not enabled")
	}
	acc, err := s.loadActiveAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	dto, err := s.toAccountDTO(ctx, *acc)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *AccountService) Create(ctx context.Context, actor Actor, in CreateAccountInput) (*AccountDTO, error) {
	if s == nil || s.accounts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "integration account service is not enabled")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "name is required")
	}
	provider := domain.ProviderType(strings.TrimSpace(in.Provider))
	if !provider.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid provider")
	}
	authType := domain.AuthType(strings.TrimSpace(in.AuthType))
	if authType == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "auth_type is required")
	}
	if !authType.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid auth_type")
	}
	if err := validateProviderAuthType(provider, authType); err != nil {
		return nil, err
	}
	if err := validateCredentialInput(provider, authType, in.Credential); err != nil {
		return nil, err
	}
	accountID := strings.TrimSpace(in.AccountID)
	if accountID == "" {
		accountID = "acc-" + uuid.NewString()
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	acc := &domain.IntegrationAccount{
		AccountID: accountID,
		Name:      name,
		Provider:  provider,
		AuthType:  authType,
		Regions:   normalizeRegions(in.Regions),
		ProjectID: strings.TrimSpace(in.ProjectID),
		Enabled:   enabled,
		OwnerTeam: strings.TrimSpace(in.OwnerTeam),
		Description: strings.TrimSpace(in.Description),
	}
	defaultCaps := domain.DefaultCapabilitiesForProvider(provider)
	if err := s.runAccountWriteTransaction(ctx, func(ctx context.Context, repos domain.TransactionRepositories) error {
		if err := repos.Accounts.Create(ctx, acc); err != nil {
			return wrapIntegrationError(err, "create integration account failed")
		}
		if err := s.storeCredentialWithRepos(ctx, repos, acc, in.Credential); err != nil {
			return err
		}
		if err := repos.Capabilities.ReplaceForAccount(ctx, acc.AccountID, defaultCaps); err != nil {
			return wrapIntegrationError(err, "save integration capabilities failed")
		}
		return nil
	}); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, acc.AccountID, actor.UserID, AuditAccountCreate, map[string]any{
		"name": acc.Name, "provider": string(acc.Provider), "enabled": acc.Enabled, "result": "success",
	})
	dto := ToAccountDTO(*acc, defaultCaps, nil)
	return &dto, nil
}

func (s *AccountService) Update(ctx context.Context, accountID string, actor Actor, in UpdateAccountInput) (*AccountDTO, error) {
	if s == nil || s.accounts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "integration account service is not enabled")
	}
	acc, err := s.loadActiveAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	originalAuthType := acc.AuthType
	providerChanged := false
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "name cannot be empty")
		}
		acc.Name = name
	}
	if in.Provider != nil {
		p := domain.ProviderType(strings.TrimSpace(*in.Provider))
		if !p.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid provider")
		}
		if p != acc.Provider {
			providerChanged = true
		}
		acc.Provider = p
	}
	if in.AuthType != nil {
		a := domain.AuthType(strings.TrimSpace(*in.AuthType))
		if !a.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid auth_type")
		}
		acc.AuthType = a
	}
	if in.RegionsSet {
		acc.Regions = normalizeRegions(in.Regions)
	}
	if in.ProjectID != nil {
		acc.ProjectID = strings.TrimSpace(*in.ProjectID)
	}
	if in.Enabled != nil {
		acc.Enabled = *in.Enabled
	}
	if in.OwnerTeam != nil {
		acc.OwnerTeam = strings.TrimSpace(*in.OwnerTeam)
	}
	if in.Description != nil {
		acc.Description = strings.TrimSpace(*in.Description)
	}
	if err := validateProviderAuthType(acc.Provider, acc.AuthType); err != nil {
		return nil, err
	}
	authTypeChanged := in.AuthType != nil && acc.AuthType != originalAuthType
	credentialValidationNeeded := providerChanged || authTypeChanged || len(in.Credential) > 0
	if acc.AuthType != domain.AuthNone && credentialValidationNeeded {
		credToValidate := in.Credential
		if len(credToValidate) == 0 {
			stored, err := s.storedCredentialMaterial(ctx, acc.AccountID)
			if err != nil {
				return nil, err
			}
			credToValidate = stored
		}
		if err := validateCredentialInput(acc.Provider, acc.AuthType, credToValidate); err != nil {
			return nil, err
		}
	}
	writeAccount := func(ctx context.Context, repos domain.TransactionRepositories) error {
		if len(in.Credential) > 0 {
			if err := s.storeCredentialWithRepos(ctx, repos, acc, in.Credential); err != nil {
				return err
			}
		}
		if err := repos.Accounts.Update(ctx, acc); err != nil {
			return wrapIntegrationError(err, "update integration account failed")
		}
		if providerChanged {
			if err := repos.Capabilities.ReplaceForAccount(ctx, acc.AccountID, domain.DefaultCapabilitiesForProvider(acc.Provider)); err != nil {
				return wrapIntegrationError(err, "refresh integration capabilities failed")
			}
		}
		return nil
	}
	if len(in.Credential) > 0 || providerChanged {
		if err := s.runAccountWriteTransaction(ctx, writeAccount); err != nil {
			return nil, err
		}
	} else if err := s.accounts.Update(ctx, acc); err != nil {
		return nil, wrapIntegrationError(err, "update integration account failed")
	}
	s.recordAudit(ctx, acc.AccountID, actor.UserID, AuditAccountUpdate, map[string]any{
		"name": acc.Name, "enabled": acc.Enabled, "result": "success",
	})
	dto, err := s.toAccountDTO(ctx, *acc)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

func (s *AccountService) Delete(ctx context.Context, accountID string, actor Actor) error {
	if s == nil || s.accounts == nil {
		return apperr.New(apperr.CodeUnavailable, "integration account service is not enabled")
	}
	acc, err := s.loadActiveAccount(ctx, accountID)
	if err != nil {
		return err
	}
	if err := s.accounts.SoftDelete(ctx, acc.AccountID); err != nil {
		return wrapIntegrationError(err, "delete integration account failed")
	}
	s.recordAudit(ctx, acc.AccountID, actor.UserID, AuditAccountDelete, map[string]any{"result": "success"})
	return nil
}

func (s *AccountService) CheckConnectivity(ctx context.Context, accountID string, actor Actor) (*ConnectivityCheckDTO, error) {
	if s == nil || s.accounts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "integration account service is not enabled")
	}
	acc, err := s.loadActiveAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !acc.Enabled {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "integration account is disabled")
	}
	checker := s.checkers[acc.Provider]
	if checker == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "provider checker is not configured")
	}
	material, err := s.loadCredentialMaterial(ctx, acc)
	if err != nil {
		return nil, err
	}
	check, err := checker.CheckConnectivity(ctx, *acc, material)
	if err != nil {
		safeMsg := sanitizeConnectivityMessage(err.Error())
		check = &domain.ConnectivityCheck{
			CheckID:   "chk-" + uuid.NewString(),
			AccountID: acc.AccountID,
			Status:    domain.ConnectivityFailed,
			Provider:  acc.Provider,
			Message:   safeMsg,
			CheckedAt: timeNow(),
		}
	}
	if check.CheckID == "" {
		check.CheckID = "chk-" + uuid.NewString()
	}
	if check.AccountID == "" {
		check.AccountID = acc.AccountID
	}
	if len(check.Capabilities) == 0 {
		check.Capabilities = domain.DefaultCapabilitiesForProvider(acc.Provider)
	}
	if err := s.runConnectivityWriteTransaction(ctx, func(ctx context.Context, repos domain.TransactionRepositories) error {
		if err := repos.Checks.Create(ctx, check); err != nil {
			return wrapIntegrationError(err, "save connectivity check failed")
		}
		if err := repos.Capabilities.ReplaceForAccount(ctx, acc.AccountID, check.Capabilities); err != nil {
			return wrapIntegrationError(err, "update integration capabilities failed")
		}
		return nil
	}); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, acc.AccountID, actor.UserID, AuditAccountCheck, map[string]any{
		"status": check.Status, "provider": string(check.Provider), "result": "success",
	})
	dto := ToConnectivityCheckDTO(*check)
	return &dto, nil
}

func (s *AccountService) toAccountDTO(ctx context.Context, acc domain.IntegrationAccount) (AccountDTO, error) {
	caps, err := s.capabilities.ListByAccountID(ctx, acc.AccountID)
	if err != nil {
		return AccountDTO{}, wrapIntegrationError(err, "list integration capabilities failed")
	}
	last, err := s.checks.LatestByAccountID(ctx, acc.AccountID)
	if err != nil && !isNotFound(err) {
		return AccountDTO{}, wrapIntegrationError(err, "load latest connectivity check failed")
	}
	return ToAccountDTO(acc, caps, last), nil
}

func (s *AccountService) runAccountWriteTransaction(ctx context.Context, fn func(ctx context.Context, repos domain.TransactionRepositories) error) error {
	if s.uow == nil {
		return apperr.New(apperr.CodeUnavailable, "integration unit of work is not configured")
	}
	return fnWithRepos(ctx, s.uow, fn)
}

func (s *AccountService) runConnectivityWriteTransaction(ctx context.Context, fn func(ctx context.Context, repos domain.TransactionRepositories) error) error {
	if s.uow == nil {
		return apperr.New(apperr.CodeUnavailable, "integration unit of work is not configured")
	}
	return fnWithRepos(ctx, s.uow, fn)
}

func fnWithRepos(ctx context.Context, uow domain.UnitOfWork, fn func(ctx context.Context, repos domain.TransactionRepositories) error) error {
	return uow.WithinTransaction(ctx, fn)
}

func (s *AccountService) storeCredentialWithRepos(ctx context.Context, repos domain.TransactionRepositories, acc *domain.IntegrationAccount, input map[string]string) error {
	material := normalizeCredential(input)
	if len(material) == 0 {
		if acc.AuthType == domain.AuthNone {
			return nil
		}
		return apperr.New(apperr.CodeInvalidArgument, "credential is required")
	}
	if s.vault == nil || repos.Credentials == nil {
		return apperr.New(apperr.CodeUnavailable, "credential vault is not configured")
	}
	ciphertext, fingerprint, err := s.vault.Encrypt(material)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "encrypt credential failed")
	}
	existing, err := repos.Credentials.GetByAccountID(ctx, acc.AccountID)
	if err != nil && !isNotFound(err) {
		return wrapIntegrationError(err, "load credential ref failed")
	}
	if existing == nil {
		ref := &domain.CredentialRef{
			CredentialRefID: "cred-" + uuid.NewString(),
			AccountID:       acc.AccountID,
			StoreType:       domain.StoreLocalEncrypted,
			Ciphertext:      ciphertext,
			Fingerprint:     fingerprint,
		}
		if err := repos.Credentials.Create(ctx, ref); err != nil {
			return wrapIntegrationError(err, "create credential ref failed")
		}
		acc.CredentialRefID = ref.CredentialRefID
		return repos.Accounts.Update(ctx, acc)
	}
	existing.Ciphertext = ciphertext
	existing.Fingerprint = fingerprint
	if err := repos.Credentials.Update(ctx, existing); err != nil {
		return wrapIntegrationError(err, "update credential ref failed")
	}
	acc.CredentialRefID = existing.CredentialRefID
	return nil
}

func (s *AccountService) storedCredentialMaterial(ctx context.Context, accountID string) (domain.CredentialMaterial, error) {
	ref, err := s.credentials.GetByAccountID(ctx, accountID)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, wrapIntegrationError(err, "load credential ref failed")
	}
	if s.vault == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "credential vault is not configured")
	}
	material, err := s.vault.Decrypt(ref.Ciphertext)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "decrypt credential failed")
	}
	return material, nil
}

func (s *AccountService) loadCredentialMaterial(ctx context.Context, acc *domain.IntegrationAccount) (domain.CredentialMaterial, error) {
	if acc.CredentialRefID == "" {
		if acc.AuthType == domain.AuthNone {
			return domain.CredentialMaterial{}, nil
		}
		return nil, apperr.New(apperr.CodeFailedPrecondition, "credential is not configured")
	}
	ref, err := s.credentials.GetByAccountID(ctx, acc.AccountID)
	if err != nil {
		return nil, wrapIntegrationError(err, "load credential ref failed")
	}
	if s.vault == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "credential vault is not configured")
	}
	material, err := s.vault.Decrypt(ref.Ciphertext)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "decrypt credential failed")
	}
	return material, nil
}

func (s *AccountService) loadActiveAccount(ctx context.Context, accountID string) (*domain.IntegrationAccount, error) {
	acc, err := s.accounts.GetByID(ctx, strings.TrimSpace(accountID))
	if err != nil {
		return nil, wrapIntegrationError(err, "load integration account failed")
	}
	if acc.Deleted {
		return nil, apperr.New(apperr.CodeNotFound, "integration account not found")
	}
	return acc, nil
}

func (s *AccountService) recordAudit(ctx context.Context, accountID, userID string, action AuditAction, payload map[string]any) {
	if s.audit == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "integration_account",
		ResourceID:   accountID,
		Action:       action,
		UserID:       userID,
		Payload:      payload,
	})
}

func validateProviderAuthType(provider domain.ProviderType, authType domain.AuthType) error {
	if !provider.SupportsAuthType(authType) {
		return apperr.New(apperr.CodeInvalidArgument, "auth_type is not supported for provider")
	}
	return nil
}

func validateCredentialInput(provider domain.ProviderType, authType domain.AuthType, cred map[string]string) error {
	if authType == domain.AuthNone {
		return nil
	}
	material := normalizeCredential(cred)
	if len(material) == 0 {
		return apperr.New(apperr.CodeInvalidArgument, "credential is required")
	}
	switch authType {
	case domain.AuthAKSK:
		if material["access_key"] == "" || material["secret_key"] == "" {
			return apperr.New(apperr.CodeInvalidArgument, "access_key and secret_key are required")
		}
	case domain.AuthAgency:
		if material["agency_name"] == "" || material["domain_name"] == "" {
			return apperr.New(apperr.CodeInvalidArgument, "agency_name and domain_name are required")
		}
	case domain.AuthAPIToken:
		if material["api_token"] == "" && material["access_token"] == "" {
			return apperr.New(apperr.CodeInvalidArgument, "api_token is required")
		}
	}
	if provider == domain.ProviderPrometheus && authType != domain.AuthNone && material["base_url"] == "" {
		return apperr.New(apperr.CodeInvalidArgument, "base_url is required for prometheus")
	}
	return nil
}

func normalizeCredential(input map[string]string) domain.CredentialMaterial {
	if len(input) == 0 {
		return nil
	}
	out := domain.CredentialMaterial{}
	for k, v := range input {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRegions(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, r := range in {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

func sanitizeConnectivityMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "connectivity check failed"
	}
	lower := strings.ToLower(msg)
	for _, token := range []string{"access_key", "secret_key", "api_token", "authorization", "bearer ", "ak", "sk"} {
		if strings.Contains(lower, token) {
			return "connectivity check failed"
		}
	}
	if len(msg) > 256 {
		return msg[:256]
	}
	return msg
}

func mapIntegrationError(err error) error {
	return apperr.MapSentinels(err, "integration operation failed",
		apperr.Sentinel{Err: domain.ErrNotFound, Code: apperr.CodeNotFound},
		apperr.Sentinel{Err: domain.ErrAlreadyExists, Code: apperr.CodeAlreadyExists},
	)
}

func wrapIntegrationError(err error, op string) error {
	if err == nil {
		return nil
	}
	if mapped := mapIntegrationError(err); apperr.FromError(mapped).Code != apperr.CodeInternal {
		return mapped
	}
	return apperr.Wrap(err, apperr.CodeInternal, op)
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if err == domain.ErrNotFound {
		return true
	}
 ae := apperr.FromError(err)
	return ae.Code == apperr.CodeNotFound
}

func timeNow() time.Time {
	return timeNowFunc()
}

var timeNowFunc = func() time.Time {
	return time.Now()
}
