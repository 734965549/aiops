package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/auth"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeAuthUserRepo struct {
	mu         sync.Mutex
	byID       map[string]*domain.User
	byUsername map[string]*domain.User
}

func newFakeAuthUserRepo(users ...*domain.User) *fakeAuthUserRepo {
	repo := &fakeAuthUserRepo{
		byID:       make(map[string]*domain.User),
		byUsername: make(map[string]*domain.User),
	}
	for _, u := range users {
		repo.byID[u.ID] = u
		repo.byUsername[strings.ToLower(u.Username)] = u
	}
	return repo
}

func (f *fakeAuthUserRepo) FindByID(_ context.Context, id string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byID[id], nil
}

func (f *fakeAuthUserRepo) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byUsername[strings.ToLower(strings.TrimSpace(username))], nil
}

func (f *fakeAuthUserRepo) Create(_ context.Context, u *domain.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := strings.ToLower(u.Username)
	if _, ok := f.byUsername[key]; ok {
		return domain.ErrAlreadyExists
	}
	f.byUsername[key] = u
	f.byID[u.ID] = u
	return nil
}

func (f *fakeAuthUserRepo) Update(_ context.Context, u *domain.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.byID[u.ID]; ok {
		existing.DisplayName = u.DisplayName
		existing.Email = u.Email
		existing.Status = u.Status
	}
	return nil
}

func (f *fakeAuthUserRepo) DeleteByID(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return nil
	}
	delete(f.byID, id)
	delete(f.byUsername, strings.ToLower(u.Username))
	return nil
}

type fakeAuthAccessRepo struct {
	roles []domain.Role
}

func (f *fakeAuthAccessRepo) ListUserRoles(_ context.Context, _ string) ([]domain.Role, error) {
	return f.roles, nil
}

func (f *fakeAuthAccessRepo) LoadUserGrantContext(ctx context.Context, userID string) (*domain.UserGrantContext, error) {
	roles, err := f.ListUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &domain.UserGrantContext{Roles: roles}, nil
}

func (f *fakeAuthAccessRepo) ListRoles(context.Context, domain.RoleFilter) ([]domain.Role, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) CountRoles(context.Context, domain.RoleFilter) (int64, error) {
	return 0, nil
}
func (f *fakeAuthAccessRepo) FindRoleByID(context.Context, string) (*domain.Role, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) FindRoleByCode(context.Context, string) (*domain.Role, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) ListPermissions(context.Context, domain.PermissionFilter) ([]domain.Permission, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) CountPermissions(context.Context, domain.PermissionFilter) (int64, error) {
	return 0, nil
}
func (f *fakeAuthAccessRepo) FindPermissionByID(context.Context, string) (*domain.Permission, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) FindPermissionByCode(context.Context, string) (*domain.Permission, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) ListUserRoleBindings(context.Context, string) ([]domain.UserRole, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) BindUserRole(context.Context, string, string, domain.UserRoleSource) error {
	return nil
}
func (f *fakeAuthAccessRepo) UnbindUserRole(context.Context, string, string) error { return nil }
func (f *fakeAuthAccessRepo) BindRolePermission(context.Context, string, string) error {
	return nil
}
func (f *fakeAuthAccessRepo) BindRoleDataScope(context.Context, string, string) error { return nil }
func (f *fakeAuthAccessRepo) BindRoleAIToolPermission(context.Context, string, string) error {
	return nil
}
func (f *fakeAuthAccessRepo) HasUserRole(context.Context, string, string) (bool, error) {
	return true, nil
}
func (f *fakeAuthAccessRepo) ListRolePermissions(context.Context, string) ([]domain.Permission, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) ListDataScopes(context.Context, domain.DataScopeFilter) ([]domain.DataScope, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) FindDataScopeByCode(context.Context, string) (*domain.DataScope, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) FindDataScopeByID(context.Context, string) (*domain.DataScope, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) ListRoleDataScopes(context.Context, string) ([]domain.DataScope, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) ListAIToolPermissions(context.Context, domain.AIToolPermissionFilter) ([]domain.AIToolPermission, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) CountAIToolPermissions(context.Context, domain.AIToolPermissionFilter) (int64, error) {
	return 0, nil
}
func (f *fakeAuthAccessRepo) FindAIToolPermissionByCode(context.Context, string) (*domain.AIToolPermission, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) FindAIToolPermissionByID(context.Context, string) (*domain.AIToolPermission, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) ListRoleAIToolPermissions(context.Context, string) ([]domain.AIToolPermission, error) {
	return nil, nil
}
func (f *fakeAuthAccessRepo) ReplaceUserManualRoles(context.Context, string, []string) error {
	return nil
}
func (f *fakeAuthAccessRepo) ReplaceRolePermissions(context.Context, string, []string) error {
	return nil
}
func (f *fakeAuthAccessRepo) ReplaceRoleDataScopes(context.Context, string, []string) error {
	return nil
}
func (f *fakeAuthAccessRepo) ReplaceRoleAIToolPermissions(context.Context, string, []string) error {
	return nil
}

type memoryRefreshStore struct {
	mu     sync.Mutex
	active map[string]map[string]struct{}
}

func newMemoryRefreshStore() *memoryRefreshStore {
	return &memoryRefreshStore{active: make(map[string]map[string]struct{})}
}

func (s *memoryRefreshStore) Register(_ context.Context, userID, jti string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[userID] == nil {
		s.active[userID] = make(map[string]struct{})
	}
	s.active[userID][jti] = struct{}{}
	return nil
}

func (s *memoryRefreshStore) Validate(_ context.Context, userID, jti string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.active[userID][jti]
	return ok, nil
}

func (s *memoryRefreshStore) Revoke(_ context.Context, userID, jti string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active[userID], jti)
	return nil
}

func (s *memoryRefreshStore) RevokeAll(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, userID)
	return nil
}

func newTestAuthService(t *testing.T, refreshStore auth.RefreshTokenStore) (*AuthService, *fakeAuthUserRepo) {
	t.Helper()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	repo := newFakeAuthUserRepo(&domain.User{
		ID: "user-1", Username: "alice", PasswordHash: hash, Status: domain.UserStatusActive,
	})
	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret:     "auth-service-test-secret-with-length",
		Issuer:     "aiops-test",
		AccessTTL:  time.Hour,
		RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt manager: %v", err)
	}
	ac := &fakeAuthAccessRepo{roles: []domain.Role{{ID: "r1", Code: "admin"}}}
	return NewAuthService(repo, jwtMgr, refreshStore, ac, nil, nil, nil, nil, ""), repo
}

func TestAuthServiceLoginSuccess(t *testing.T) {
	svc, _ := newTestAuthService(t, auth.NoopRefreshTokenStore{})
	pair, err := svc.Login(context.Background(), LoginInput{Username: "alice", Password: "secret123"})
	if err != nil || pair == nil || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("login: pair=%+v err=%v", pair, err)
	}
	if pair.User == nil || pair.User.Username != "alice" {
		t.Fatalf("unexpected user: %+v", pair.User)
	}
}

func TestAuthServiceLoginWrongPassword(t *testing.T) {
	svc, _ := newTestAuthService(t, auth.NoopRefreshTokenStore{})
	_, err := svc.Login(context.Background(), LoginInput{Username: "alice", Password: "bad"})
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
}

func TestAuthServiceLoginUnknownUser(t *testing.T) {
	svc, _ := newTestAuthService(t, auth.NoopRefreshTokenStore{})
	_, err := svc.Login(context.Background(), LoginInput{Username: "ghost", Password: "secret123"})
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
}

func TestAuthServiceLoginDisabledUser(t *testing.T) {
	svc, repo := newTestAuthService(t, auth.NoopRefreshTokenStore{})
	repo.byUsername["bob"] = &domain.User{ID: "user-2", Username: "bob", Status: domain.UserStatusDisabled}
	_, err := svc.Login(context.Background(), LoginInput{Username: "bob", Password: "secret123"})
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
}

func TestAuthServiceRefreshSuccess(t *testing.T) {
	svc, _ := newTestAuthService(t, auth.NoopRefreshTokenStore{})
	login, err := svc.Login(context.Background(), LoginInput{Username: "alice", Password: "secret123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	refreshed, err := svc.Refresh(context.Background(), login.RefreshToken)
	if err != nil || refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatalf("refresh: pair=%+v err=%v", refreshed, err)
	}
	if refreshed.RefreshToken == login.RefreshToken {
		t.Fatal("expected new refresh token after refresh")
	}
}

func TestAuthServiceRefreshRotatesWithStore(t *testing.T) {
	store := newMemoryRefreshStore()
	svc, _ := newTestAuthService(t, store)
	login, err := svc.Login(context.Background(), LoginInput{Username: "alice", Password: "secret123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	firstRefresh, err := svc.Refresh(context.Background(), login.RefreshToken)
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if _, err := svc.Refresh(context.Background(), login.RefreshToken); err == nil {
		t.Fatal("expected old refresh token to be rejected after rotation")
	}
	if _, err := svc.Refresh(context.Background(), firstRefresh.RefreshToken); err != nil {
		t.Fatalf("second refresh with rotated token: %v", err)
	}
}

func TestAuthServiceEnsureBootstrapUserIdempotent(t *testing.T) {
	svc, repo := newTestAuthService(t, auth.NoopRefreshTokenStore{})
	ctx := context.Background()
	if err := svc.EnsureBootstrapUser(ctx, "bootstrap", "password123", "Admin"); err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	if _, ok := repo.byUsername["bootstrap"]; !ok {
		t.Fatal("expected bootstrap user created")
	}
	if err := svc.EnsureBootstrapUser(ctx, "bootstrap", "password123", "Admin"); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
}
