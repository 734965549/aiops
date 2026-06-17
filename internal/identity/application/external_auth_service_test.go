package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/internal/identity/infrastructure/identityprovider"
	"github.com/734965549/aiops/internal/identity/infrastructure/oauthstate"
	"github.com/734965549/aiops/pkg/auth"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakePasswordProvider struct {
	info domain.ProviderInfo
	user *domain.AuthenticatedExternalUser
	err  error
	opts identityprovider.ProvisioningOptions
}

func (f *fakePasswordProvider) Info() domain.ProviderInfo { return f.info }
func (f *fakePasswordProvider) ProvisioningOptions() identityprovider.ProvisioningOptions {
	return f.opts
}
func (f *fakePasswordProvider) Authenticate(_ context.Context, _, _ string) (*domain.AuthenticatedExternalUser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.user, nil
}

type fakeOAuthProvider struct {
	info domain.ProviderInfo
	user *domain.AuthenticatedExternalUser
	err  error
}

func (f *fakeOAuthProvider) Info() domain.ProviderInfo { return f.info }
func (f *fakeOAuthProvider) AuthorizationURL(state string) (string, error) {
	return "https://idp.example/authorize?state=" + state, nil
}
func (f *fakeOAuthProvider) ExchangeCode(_ context.Context, _ string) (*domain.AuthenticatedExternalUser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.user, nil
}

type fakeExternalIDRepo struct {
	bySubject map[string]*domain.ExternalIdentity
	byUser    map[string]*domain.ExternalIdentity
}

func newFakeExternalIDRepo() *fakeExternalIDRepo {
	return &fakeExternalIDRepo{
		bySubject: make(map[string]*domain.ExternalIdentity),
		byUser:    make(map[string]*domain.ExternalIdentity),
	}
}

func (f *fakeExternalIDRepo) key(providerID, subject string) string {
	return providerID + "\x00" + subject
}

func (f *fakeExternalIDRepo) userKey(userID, providerID string) string {
	return userID + "\x00" + providerID
}

func (f *fakeExternalIDRepo) FindByProviderSubject(_ context.Context, providerID, subject string) (*domain.ExternalIdentity, error) {
	return f.bySubject[f.key(providerID, subject)], nil
}

func (f *fakeExternalIDRepo) FindByUserAndProvider(_ context.Context, userID, providerID string) (*domain.ExternalIdentity, error) {
	return f.byUser[f.userKey(userID, providerID)], nil
}

func (f *fakeExternalIDRepo) Create(_ context.Context, ext *domain.ExternalIdentity) error {
	if _, ok := f.bySubject[f.key(ext.ProviderID, ext.ExternalSubject)]; ok {
		return domain.ErrAlreadyExists
	}
	f.bySubject[f.key(ext.ProviderID, ext.ExternalSubject)] = ext
	f.byUser[f.userKey(ext.UserID, ext.ProviderID)] = ext
	return nil
}

func (f *fakeExternalIDRepo) Update(_ context.Context, ext *domain.ExternalIdentity) error {
	f.bySubject[f.key(ext.ProviderID, ext.ExternalSubject)] = ext
	return nil
}

func (f *fakeExternalIDRepo) DeleteByProviderSubject(_ context.Context, providerID, subject string) error {
	ext, ok := f.bySubject[f.key(providerID, subject)]
	if !ok {
		return nil
	}
	delete(f.bySubject, f.key(providerID, subject))
	delete(f.byUser, f.userKey(ext.UserID, providerID))
	return nil
}

func TestOAuthCallbackRequiresValidState(t *testing.T) {
	userRepo := newFakeAuthUserRepo(&domain.User{
		ID: "user-alice", Username: "corp-oauth:alice", Status: domain.UserStatusActive,
	})
	extRepo := newFakeExternalIDRepo()
	_ = extRepo.Create(context.Background(), &domain.ExternalIdentity{
		ID: "bind-oauth", UserID: "user-alice", ProviderID: "corp-oauth", ExternalSubject: "sub-alice",
	})
	reg := identityprovider.NewRegistry()
	reg.RegisterOAuth(&fakeOAuthProvider{
		info: domain.ProviderInfo{ID: "corp-oauth", Type: domain.ProviderTypeOAuth2, Enabled: true},
		user: &domain.AuthenticatedExternalUser{
			ProviderID: "corp-oauth", ExternalSubject: "sub-alice", Username: "alice",
		},
	})
	jwtMgr, _ := auth.NewJWTManager(auth.Options{
		Secret: "external-auth-test-secret-with-length", Issuer: "test",
		AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	stateStore := oauthstate.NewMemoryStore()
	svc := NewAuthService(userRepo, jwtMgr, auth.NoopRefreshTokenStore{}, nil, extRepo, reg, nil, stateStore, "")

	_, _, err := svc.OAuthAuthorizeURL(context.Background(), "corp-oauth")
	if err != nil {
		t.Fatalf("authorize url: %v", err)
	}
	_, err = svc.LoginOAuthCallback(context.Background(), OAuthCallbackInput{
		ProviderID: "corp-oauth", Code: "auth-code", State: "forged-state",
	})
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated for forged state, got %v", err)
	}
}

func TestOAuthCallbackConsumesStateOnce(t *testing.T) {
	userRepo := newFakeAuthUserRepo(&domain.User{
		ID: "user-alice", Username: "corp-oauth:alice", Status: domain.UserStatusActive,
	})
	extRepo := newFakeExternalIDRepo()
	_ = extRepo.Create(context.Background(), &domain.ExternalIdentity{
		ID: "bind-oauth", UserID: "user-alice", ProviderID: "corp-oauth", ExternalSubject: "sub-alice",
	})
	reg := identityprovider.NewRegistry()
	reg.RegisterOAuth(&fakeOAuthProvider{
		info: domain.ProviderInfo{ID: "corp-oauth", Type: domain.ProviderTypeOAuth2, Enabled: true},
		user: &domain.AuthenticatedExternalUser{
			ProviderID: "corp-oauth", ExternalSubject: "sub-alice", Username: "alice",
		},
	})
	jwtMgr, _ := auth.NewJWTManager(auth.Options{
		Secret: "external-auth-test-secret-with-length", Issuer: "test",
		AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	stateStore := oauthstate.NewMemoryStore()
	svc := NewAuthService(userRepo, jwtMgr, auth.NoopRefreshTokenStore{}, nil, extRepo, reg, nil, stateStore, "")

	_, state, err := svc.OAuthAuthorizeURL(context.Background(), "corp-oauth")
	if err != nil || state == "" {
		t.Fatalf("authorize url: state=%q err=%v", state, err)
	}
	pair, err := svc.LoginOAuthCallback(context.Background(), OAuthCallbackInput{
		ProviderID: "corp-oauth", Code: "auth-code", State: state,
	})
	if err != nil || pair == nil || pair.AccessToken == "" {
		t.Fatalf("oauth callback: pair=%+v err=%v", pair, err)
	}
	_, err = svc.LoginOAuthCallback(context.Background(), OAuthCallbackInput{
		ProviderID: "corp-oauth", Code: "auth-code", State: state,
	})
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeUnauthenticated {
		t.Fatalf("expected replay rejection, got %v", err)
	}
}

func TestOAuthCallbackRejectsStateBindingMismatch(t *testing.T) {
	userRepo := newFakeAuthUserRepo(&domain.User{
		ID: "user-alice", Username: "corp-oauth:alice", Status: domain.UserStatusActive,
	})
	extRepo := newFakeExternalIDRepo()
	_ = extRepo.Create(context.Background(), &domain.ExternalIdentity{
		ID: "bind-oauth", UserID: "user-alice", ProviderID: "corp-oauth", ExternalSubject: "sub-alice",
	})
	reg := identityprovider.NewRegistry()
	reg.RegisterOAuth(&fakeOAuthProvider{
		info: domain.ProviderInfo{ID: "corp-oauth", Type: domain.ProviderTypeOAuth2, Enabled: true},
		user: &domain.AuthenticatedExternalUser{
			ProviderID: "corp-oauth", ExternalSubject: "sub-alice", Username: "alice",
		},
	})
	jwtMgr, _ := auth.NewJWTManager(auth.Options{
		Secret: "external-auth-test-secret-with-length", Issuer: "test",
		AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	stateStore := oauthstate.NewMemoryStore()
	svc := NewAuthService(userRepo, jwtMgr, auth.NoopRefreshTokenStore{}, nil, extRepo, reg, nil, stateStore, "")

	_, state, err := svc.OAuthAuthorizeURLWithContext(context.Background(), OAuthAuthorizeInput{
		ProviderID: "corp-oauth",
		ClientIP:   "192.0.2.1",
		UserAgent:  "browser-a",
	})
	if err != nil {
		t.Fatalf("authorize url: %v", err)
	}
	_, err = svc.LoginOAuthCallback(context.Background(), OAuthCallbackInput{
		ProviderID: "corp-oauth",
		Code:       "auth-code",
		State:      state,
		ClientIP:   "192.0.2.2",
		UserAgent:  "browser-a",
	})
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeUnauthenticated {
		t.Fatalf("expected binding mismatch rejection, got %v", err)
	}
	pair, err := svc.LoginOAuthCallback(context.Background(), OAuthCallbackInput{
		ProviderID: "corp-oauth",
		Code:       "auth-code",
		State:      state,
		ClientIP:   "192.0.2.1",
		UserAgent:  "browser-a",
	})
	if err != nil || pair == nil || pair.AccessToken == "" {
		t.Fatalf("expected original binding to remain consumable: pair=%+v err=%v", pair, err)
	}
}

func TestLoginExternalWithPreProvisionedBinding(t *testing.T) {
	userRepo := newFakeAuthUserRepo(&domain.User{
		ID: "user-alice", Username: "corp-ldap:alice@example.com", Status: domain.UserStatusActive,
	})
	extRepo := newFakeExternalIDRepo()
	_ = extRepo.Create(context.Background(), &domain.ExternalIdentity{
		ID: "bind-1", UserID: "user-alice", ProviderID: "corp-ldap",
		ExternalSubject: "uid=alice,dc=example,dc=com",
	})

	reg := identityprovider.NewRegistry()
	provider := &fakePasswordProvider{
		info: domain.ProviderInfo{ID: "corp-ldap", Type: domain.ProviderTypeLDAP, Enabled: true},
		user: &domain.AuthenticatedExternalUser{
			ProviderID: "corp-ldap", ExternalSubject: "uid=alice,dc=example,dc=com",
			Username: "alice@example.com", DisplayName: "Alice", Email: "alice@example.com",
		},
	}
	reg.RegisterPassword(provider)

	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret: "external-auth-test-secret-with-length", Issuer: "test",
		AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	svc := NewAuthService(userRepo, jwtMgr, auth.NoopRefreshTokenStore{}, nil, extRepo, reg, nil, nil, "")

	pair, err := svc.LoginExternal(context.Background(), ExternalLoginInput{
		ProviderID: "corp-ldap", Username: "alice@example.com", Password: "secret",
	})
	if err != nil || pair == nil || pair.AccessToken == "" {
		t.Fatalf("login external: pair=%+v err=%v", pair, err)
	}
	if pair.User == nil || pair.User.Username != "corp-ldap:alice@example.com" {
		t.Fatalf("unexpected user: %+v", pair.User)
	}
}

func TestLoginExternalRejectWithoutBinding(t *testing.T) {
	userRepo := newFakeAuthUserRepo()
	extRepo := newFakeExternalIDRepo()
	reg := identityprovider.NewRegistry()
	provider := &fakePasswordProvider{
		info: domain.ProviderInfo{ID: "corp-ldap", Type: domain.ProviderTypeLDAP, Enabled: true},
		user: &domain.AuthenticatedExternalUser{
			ProviderID: "corp-ldap", ExternalSubject: "uid=bob,dc=example,dc=com", Username: "bob",
		},
	}
	reg.RegisterPassword(provider)

	jwtMgr, _ := auth.NewJWTManager(auth.Options{
		Secret: "external-auth-test-secret-with-length", Issuer: "test",
		AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	svc := NewAuthService(userRepo, jwtMgr, auth.NoopRefreshTokenStore{}, nil, extRepo, reg, nil, nil, "")
	_, err := svc.LoginExternal(context.Background(), ExternalLoginInput{
		ProviderID: "corp-ldap", Username: "bob", Password: "secret",
	})
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
}

func TestLoginExternalRejectAutoLinkByUsername(t *testing.T) {
	// 本地已有 alice，外部 bob@evil.com 规范化后不应接管 alice。
	userRepo := newFakeAuthUserRepo(&domain.User{
		ID: "local-alice", Username: "alice", Status: domain.UserStatusActive,
	})
	extRepo := newFakeExternalIDRepo()
	reg := identityprovider.NewRegistry()
	provider := &fakePasswordProvider{
		info: domain.ProviderInfo{ID: "corp-ldap", Type: domain.ProviderTypeLDAP, Enabled: true},
		opts: identityprovider.ProvisioningOptions{AutoCreateUser: true},
		user: &domain.AuthenticatedExternalUser{
			ProviderID: "corp-ldap", ExternalSubject: "uid=attacker,dc=evil,dc=com",
			Username: "alice@evil.com",
		},
	}
	reg.RegisterPassword(provider)

	jwtMgr, _ := auth.NewJWTManager(auth.Options{
		Secret: "external-auth-test-secret-with-length", Issuer: "test",
		AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	svc := NewAuthService(userRepo, jwtMgr, auth.NoopRefreshTokenStore{}, nil, extRepo, reg, nil, nil, "")
	_, err := svc.LoginExternal(context.Background(), ExternalLoginInput{
		ProviderID: "corp-ldap", Username: "alice@evil.com", Password: "secret",
	})
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated (no auto-link), got %v", err)
	}
	if len(userRepo.byUsername) != 1 {
		t.Fatalf("expected no new user created, repo=%+v", userRepo.byUsername)
	}
}

func TestMapRoleCodesFromGroups(t *testing.T) {
	codes := mapRoleCodesFromGroups(
		[]string{"CN=Operators,OU=Groups,DC=example,DC=com"},
		map[string]string{"CN=Operators,OU=Groups,DC=example,DC=com": "operator"},
	)
	if len(codes) != 1 || codes[0] != "operator" {
		t.Fatalf("unexpected codes: %+v", codes)
	}
}

func TestNamespacedExternalUsername(t *testing.T) {
	got := namespacedExternalUsername("corp-ldap", "alice@example.com")
	want := "corp-ldap:alice@example.com"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

type trackingRoleAccessRepo struct {
	mu       sync.Mutex
	roles    map[string]*domain.Role
	bindings map[string]domain.UserRoleSource
}

func newTrackingRoleAccessRepo(roles ...domain.Role) *trackingRoleAccessRepo {
	repo := &trackingRoleAccessRepo{
		roles:    make(map[string]*domain.Role),
		bindings: make(map[string]domain.UserRoleSource),
	}
	for i := range roles {
		repo.roles[roles[i].Code] = &roles[i]
	}
	return repo
}

func (f *trackingRoleAccessRepo) bindingKey(userID, roleID string) string {
	return userID + "\x00" + roleID
}

func (f *trackingRoleAccessRepo) seedBinding(userID, roleID string, source domain.UserRoleSource) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bindings[f.bindingKey(userID, roleID)] = source
}

func (f *trackingRoleAccessRepo) ListUserRoles(context.Context, string) ([]domain.Role, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) ListUserRoleBindings(_ context.Context, userID string) ([]domain.UserRole, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.UserRole, 0, len(f.bindings))
	for key, source := range f.bindings {
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 || parts[0] != userID {
			continue
		}
		out = append(out, domain.UserRole{UserID: parts[0], RoleID: parts[1], Source: source})
	}
	return out, nil
}
func (f *trackingRoleAccessRepo) BindUserRole(_ context.Context, userID, roleID string, source domain.UserRoleSource) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := f.bindingKey(userID, roleID)
	if current, ok := f.bindings[key]; ok {
		f.bindings[key] = domain.PreserveUserRoleSource(current, source)
		return nil
	}
	f.bindings[key] = domain.NormalizeUserRoleSource(source)
	return nil
}
func (f *trackingRoleAccessRepo) UnbindUserRole(_ context.Context, userID, roleID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.bindings, f.bindingKey(userID, roleID))
	return nil
}
func (f *trackingRoleAccessRepo) FindRoleByCode(_ context.Context, code string) (*domain.Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.roles[code], nil
}
func (f *trackingRoleAccessRepo) HasUserRole(_ context.Context, userID, roleID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.bindings[f.bindingKey(userID, roleID)]
	return ok, nil
}
func (f *trackingRoleAccessRepo) roleIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.bindings))
	for key := range f.bindings {
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) == 2 {
			out = append(out, parts[1])
		}
	}
	return out
}

func (f *trackingRoleAccessRepo) ListRoles(context.Context, domain.RoleFilter) ([]domain.Role, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) CountRoles(context.Context, domain.RoleFilter) (int64, error) {
	return 0, nil
}
func (f *trackingRoleAccessRepo) FindRoleByID(context.Context, string) (*domain.Role, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) ListPermissions(context.Context, domain.PermissionFilter) ([]domain.Permission, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) CountPermissions(context.Context, domain.PermissionFilter) (int64, error) {
	return 0, nil
}
func (f *trackingRoleAccessRepo) FindPermissionByID(context.Context, string) (*domain.Permission, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) FindPermissionByCode(context.Context, string) (*domain.Permission, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) BindRolePermission(context.Context, string, string) error {
	return nil
}
func (f *trackingRoleAccessRepo) BindRoleDataScope(context.Context, string, string) error { return nil }
func (f *trackingRoleAccessRepo) BindRoleAIToolPermission(context.Context, string, string) error {
	return nil
}
func (f *trackingRoleAccessRepo) ListRolePermissions(context.Context, string) ([]domain.Permission, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) ListDataScopes(context.Context, domain.DataScopeFilter) ([]domain.DataScope, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) FindDataScopeByCode(context.Context, string) (*domain.DataScope, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) FindDataScopeByID(context.Context, string) (*domain.DataScope, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) ListRoleDataScopes(context.Context, string) ([]domain.DataScope, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) ListAIToolPermissions(context.Context, domain.AIToolPermissionFilter) ([]domain.AIToolPermission, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) CountAIToolPermissions(context.Context, domain.AIToolPermissionFilter) (int64, error) {
	return 0, nil
}
func (f *trackingRoleAccessRepo) FindAIToolPermissionByCode(context.Context, string) (*domain.AIToolPermission, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) FindAIToolPermissionByID(context.Context, string) (*domain.AIToolPermission, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) ListRoleAIToolPermissions(context.Context, string) ([]domain.AIToolPermission, error) {
	return nil, nil
}
func (f *trackingRoleAccessRepo) ReplaceUserManualRoles(context.Context, string, []string) error {
	return nil
}
func (f *trackingRoleAccessRepo) ReplaceRolePermissions(context.Context, string, []string) error {
	return nil
}
func (f *trackingRoleAccessRepo) ReplaceRoleDataScopes(context.Context, string, []string) error {
	return nil
}
func (f *trackingRoleAccessRepo) ReplaceRoleAIToolPermissions(context.Context, string, []string) error {
	return nil
}
func (f *trackingRoleAccessRepo) LoadUserGrantContext(context.Context, string) (*domain.UserGrantContext, error) {
	return &domain.UserGrantContext{}, nil
}

func TestSyncMappedRolesReconcilesExternalGroupRoles(t *testing.T) {
	ac := newTrackingRoleAccessRepo(
		domain.Role{ID: "role-admin", Code: "admin"},
		domain.Role{ID: "role-operator", Code: "operator"},
	)
	ac.seedBinding("user-1", "role-admin", domain.UserRoleSourceExternalGroup)
	ac.seedBinding("user-1", "role-operator", domain.UserRoleSourceExternalGroup)

	svc := &AuthService{ac: ac}
	groupDN := "CN=Operators,OU=Groups,DC=example,DC=com"
	err := svc.syncMappedRoles(context.Background(), "user-1", []string{groupDN}, identityprovider.ProvisioningOptions{
		GroupRoleMap: map[string]string{groupDN: "operator"},
	})
	if err != nil {
		t.Fatalf("syncMappedRoles: %v", err)
	}
	ids := ac.roleIDs()
	if len(ids) != 1 || ids[0] != "role-operator" {
		t.Fatalf("expected only operator role, got %+v", ids)
	}
}

func TestSyncMappedRolesPreservesManualAndLDAPImportRoles(t *testing.T) {
	ac := newTrackingRoleAccessRepo(
		domain.Role{ID: "role-admin", Code: "admin"},
		domain.Role{ID: "role-operator", Code: "operator"},
		domain.Role{ID: "role-viewer", Code: "viewer"},
	)
	ac.seedBinding("user-1", "role-admin", domain.UserRoleSourceManual)
	ac.seedBinding("user-1", "role-operator", domain.UserRoleSourceLDAPImport)
	ac.seedBinding("user-1", "role-viewer", domain.UserRoleSourceExternalGroup)

	svc := &AuthService{ac: ac}
	groupDN := "CN=Operators,OU=Groups,DC=example,DC=com"
	err := svc.syncMappedRoles(context.Background(), "user-1", []string{groupDN}, identityprovider.ProvisioningOptions{
		GroupRoleMap: map[string]string{groupDN: "operator"},
	})
	if err != nil {
		t.Fatalf("syncMappedRoles: %v", err)
	}

	want := map[string]domain.UserRoleSource{
		"role-admin":    domain.UserRoleSourceManual,
		"role-operator": domain.UserRoleSourceLDAPImport,
	}
	for roleID, wantSource := range want {
		has, err := ac.HasUserRole(context.Background(), "user-1", roleID)
		if err != nil || !has {
			t.Fatalf("expected role %s to remain: has=%v err=%v", roleID, has, err)
		}
		bindings, _ := ac.ListUserRoleBindings(context.Background(), "user-1")
		var found domain.UserRoleSource
		for _, b := range bindings {
			if b.RoleID == roleID {
				found = b.Source
				break
			}
		}
		if found != wantSource {
			t.Fatalf("role %s source=%q want %q", roleID, found, wantSource)
		}
	}
	hasViewer, err := ac.HasUserRole(context.Background(), "user-1", "role-viewer")
	if err != nil {
		t.Fatalf("check viewer role: %v", err)
	}
	if hasViewer {
		t.Fatal("expected stale external_group viewer role to be removed")
	}
}

func TestBindUserRolesByCodesUsesLDAPImportSource(t *testing.T) {
	ac := newTrackingRoleAccessRepo(domain.Role{ID: "role-operator", Code: "operator"})
	svc := &AuthService{ac: ac}
	if err := svc.bindUserRolesByCodes(context.Background(), "user-1", []string{"operator"}); err != nil {
		t.Fatalf("bindUserRolesByCodes: %v", err)
	}
	bindings, err := ac.ListUserRoleBindings(context.Background(), "user-1")
	if err != nil || len(bindings) != 1 {
		t.Fatalf("bindings=%+v err=%v", bindings, err)
	}
	if bindings[0].Source != domain.UserRoleSourceLDAPImport {
		t.Fatalf("unexpected source: %q", bindings[0].Source)
	}
}
