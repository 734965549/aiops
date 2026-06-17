package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeAccessControlRepo struct {
	roles        []domain.Role
	permissions  []domain.Permission
	dataScopes   []domain.DataScope
	tools        []domain.AIToolPermission
	userRoles    []domain.Role
	rolePerms    []domain.Permission
	roleScopes   []domain.DataScope
	roleTools    []domain.AIToolPermission
	roleBindings []domain.UserRole
}

func (f *fakeAccessControlRepo) ListRoles(ctx context.Context, filter domain.RoleFilter) ([]domain.Role, error) {
	return f.roles, nil
}
func (f *fakeAccessControlRepo) CountRoles(ctx context.Context, filter domain.RoleFilter) (int64, error) {
	return int64(len(f.roles)), nil
}
func (f *fakeAccessControlRepo) FindRoleByID(ctx context.Context, id string) (*domain.Role, error) {
	for i := range f.roles {
		if f.roles[i].ID == id {
			return &f.roles[i], nil
		}
	}
	return nil, nil
}
func (f *fakeAccessControlRepo) FindRoleByCode(ctx context.Context, code string) (*domain.Role, error) {
	for i := range f.roles {
		if f.roles[i].Code == code {
			return &f.roles[i], nil
		}
	}
	return nil, nil
}
func (f *fakeAccessControlRepo) ListPermissions(ctx context.Context, filter domain.PermissionFilter) ([]domain.Permission, error) {
	return f.permissions, nil
}
func (f *fakeAccessControlRepo) CountPermissions(ctx context.Context, filter domain.PermissionFilter) (int64, error) {
	return int64(len(f.permissions)), nil
}
func (f *fakeAccessControlRepo) FindPermissionByID(ctx context.Context, id string) (*domain.Permission, error) {
	for i := range f.permissions {
		if f.permissions[i].ID == id {
			return &f.permissions[i], nil
		}
	}
	return nil, nil
}
func (f *fakeAccessControlRepo) FindPermissionByCode(ctx context.Context, code string) (*domain.Permission, error) {
	for i := range f.permissions {
		if f.permissions[i].Code == code {
			return &f.permissions[i], nil
		}
	}
	return nil, nil
}
func (f *fakeAccessControlRepo) ListUserRoles(ctx context.Context, userID string) ([]domain.Role, error) {
	return f.userRoles, nil
}
func (f *fakeAccessControlRepo) ListUserRoleBindings(ctx context.Context, userID string) ([]domain.UserRole, error) {
	out := make([]domain.UserRole, 0, len(f.roleBindings))
	for _, binding := range f.roleBindings {
		if binding.UserID == userID {
			out = append(out, binding)
		}
	}
	return out, nil
}
func (f *fakeAccessControlRepo) BindUserRole(ctx context.Context, userID, roleID string, source domain.UserRoleSource) error {
	return nil
}
func (f *fakeAccessControlRepo) UnbindUserRole(ctx context.Context, userID, roleID string) error {
	return nil
}
func (f *fakeAccessControlRepo) BindRolePermission(ctx context.Context, roleID, permissionID string) error {
	return nil
}
func (f *fakeAccessControlRepo) BindRoleDataScope(ctx context.Context, roleID, dataScopeID string) error {
	return nil
}
func (f *fakeAccessControlRepo) BindRoleAIToolPermission(ctx context.Context, roleID, toolPermissionID string) error {
	return nil
}
func (f *fakeAccessControlRepo) HasUserRole(ctx context.Context, userID, roleID string) (bool, error) {
	return true, nil
}
func (f *fakeAccessControlRepo) ListRolePermissions(ctx context.Context, roleID string) ([]domain.Permission, error) {
	return f.rolePerms, nil
}
func (f *fakeAccessControlRepo) ListDataScopes(ctx context.Context, filter domain.DataScopeFilter) ([]domain.DataScope, error) {
	return f.dataScopes, nil
}
func (f *fakeAccessControlRepo) FindDataScopeByCode(ctx context.Context, code string) (*domain.DataScope, error) {
	for i := range f.dataScopes {
		if f.dataScopes[i].Code == code {
			return &f.dataScopes[i], nil
		}
	}
	return nil, nil
}
func (f *fakeAccessControlRepo) FindDataScopeByID(ctx context.Context, id string) (*domain.DataScope, error) {
	for i := range f.dataScopes {
		if f.dataScopes[i].ID == id {
			return &f.dataScopes[i], nil
		}
	}
	return nil, nil
}
func (f *fakeAccessControlRepo) ListRoleDataScopes(ctx context.Context, roleID string) ([]domain.DataScope, error) {
	return f.roleScopes, nil
}
func (f *fakeAccessControlRepo) ListAIToolPermissions(ctx context.Context, filter domain.AIToolPermissionFilter) ([]domain.AIToolPermission, error) {
	return f.tools, nil
}
func (f *fakeAccessControlRepo) CountAIToolPermissions(ctx context.Context, filter domain.AIToolPermissionFilter) (int64, error) {
	return int64(len(f.tools)), nil
}
func (f *fakeAccessControlRepo) FindAIToolPermissionByCode(ctx context.Context, code string) (*domain.AIToolPermission, error) {
	for i := range f.tools {
		if f.tools[i].ToolCode == code {
			return &f.tools[i], nil
		}
	}
	return nil, nil
}
func (f *fakeAccessControlRepo) FindAIToolPermissionByID(ctx context.Context, id string) (*domain.AIToolPermission, error) {
	for i := range f.tools {
		if f.tools[i].ID == id {
			return &f.tools[i], nil
		}
	}
	return nil, nil
}
func (f *fakeAccessControlRepo) ListRoleAIToolPermissions(ctx context.Context, roleID string) ([]domain.AIToolPermission, error) {
	return f.roleTools, nil
}
func (f *fakeAccessControlRepo) ReplaceUserManualRoles(ctx context.Context, userID string, roleIDs []string) error {
	next := make([]domain.UserRole, 0, len(f.roleBindings)+len(roleIDs))
	preserved := map[string]struct{}{}
	for _, binding := range f.roleBindings {
		if binding.UserID == userID && domain.NormalizeUserRoleSource(binding.Source) == domain.UserRoleSourceManual {
			continue
		}
		if binding.UserID == userID {
			preserved[binding.RoleID] = struct{}{}
		}
		next = append(next, binding)
	}
	for _, roleID := range roleIDs {
		if _, ok := preserved[roleID]; ok {
			continue
		}
		next = append(next, domain.UserRole{UserID: userID, RoleID: roleID, Source: domain.UserRoleSourceManual})
	}
	f.roleBindings = next
	return nil
}
func (f *fakeAccessControlRepo) ReplaceRolePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	f.rolePerms = f.rolePerms[:0]
	for _, permissionID := range permissionIDs {
		if p, _ := f.FindPermissionByID(ctx, permissionID); p != nil {
			f.rolePerms = append(f.rolePerms, *p)
		}
	}
	return nil
}
func (f *fakeAccessControlRepo) ReplaceRoleDataScopes(ctx context.Context, roleID string, dataScopeIDs []string) error {
	f.roleScopes = f.roleScopes[:0]
	for _, dataScopeID := range dataScopeIDs {
		if sc, _ := f.FindDataScopeByID(ctx, dataScopeID); sc != nil {
			f.roleScopes = append(f.roleScopes, *sc)
		}
	}
	return nil
}
func (f *fakeAccessControlRepo) ReplaceRoleAIToolPermissions(ctx context.Context, roleID string, toolPermissionIDs []string) error {
	f.roleTools = f.roleTools[:0]
	for _, toolPermissionID := range toolPermissionIDs {
		if tp, _ := f.FindAIToolPermissionByID(ctx, toolPermissionID); tp != nil {
			f.roleTools = append(f.roleTools, *tp)
		}
	}
	return nil
}
func (f *fakeAccessControlRepo) LoadUserGrantContext(ctx context.Context, userID string) (*domain.UserGrantContext, error) {
	return &domain.UserGrantContext{}, nil
}

type fakeUserAdminRepo struct {
	users []domain.User
}

func (f *fakeUserAdminRepo) FindByID(ctx context.Context, id string) (*domain.User, error) {
	for i := range f.users {
		if f.users[i].ID == id {
			return &f.users[i], nil
		}
	}
	return nil, nil
}

func (f *fakeUserAdminRepo) List(ctx context.Context, filter domain.UserFilter) ([]domain.User, error) {
	return f.users, nil
}

func (f *fakeUserAdminRepo) Count(ctx context.Context, filter domain.UserFilter) (int64, error) {
	return int64(len(f.users)), nil
}

type captureAccessAuditRecorder struct {
	records []AccessAuditRecord
}

func (r *captureAccessAuditRecorder) Record(ctx context.Context, rec AccessAuditRecord) error {
	r.records = append(r.records, rec)
	return nil
}

func TestAccessControlService_ListRoles(t *testing.T) {
	repo := &fakeAccessControlRepo{roles: []domain.Role{{ID: "r1", Code: "admin"}}}
	svc := NewAccessControlService(repo, nil, nil)
	rows, err := svc.ListRoles(context.Background(), domain.RoleFilter{})
	if err != nil {
		t.Fatalf("ListRoles error: %v", err)
	}
	if len(rows) != 1 || rows[0].Code != "admin" {
		t.Fatalf("unexpected roles: %#v", rows)
	}
}

func TestAccessControlService_ListPermissions(t *testing.T) {
	repo := &fakeAccessControlRepo{permissions: []domain.Permission{{ID: "p1", Code: "app:user:read"}}}
	svc := NewAccessControlService(repo, nil, nil)
	rows, err := svc.ListPermissions(context.Background(), domain.PermissionFilter{})
	if err != nil {
		t.Fatalf("ListPermissions error: %v", err)
	}
	if len(rows) != 1 || rows[0].Code != "app:user:read" {
		t.Fatalf("unexpected permissions: %#v", rows)
	}
}

func TestAccessControlService_ListUserRoles(t *testing.T) {
	repo := &fakeAccessControlRepo{userRoles: []domain.Role{{ID: "r1", Code: "operator"}}}
	svc := NewAccessControlService(repo, nil, nil)
	rows, err := svc.ListUserRoles(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListUserRoles error: %v", err)
	}
	if len(rows) != 1 || rows[0].Code != "operator" {
		t.Fatalf("unexpected user roles: %#v", rows)
	}
}

func TestAccessControlService_ListUsers(t *testing.T) {
	now := time.Now()
	users := &fakeUserAdminRepo{users: []domain.User{{
		ID: "u1", Username: "alice", DisplayName: "Alice", Email: "alice@example.com",
		Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}}}
	svc := NewAccessControlService(&fakeAccessControlRepo{}, users, nil)
	rows, total, err := svc.ListUsers(context.Background(), domain.UserFilter{})
	if err != nil {
		t.Fatalf("ListUsers error: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != "u1" || rows[0].Username != "alice" {
		t.Fatalf("unexpected users: total=%d rows=%#v", total, rows)
	}
}

func TestAccessControlService_ReplaceUserManualRolesPreservesDelegatedBindingsAndAudits(t *testing.T) {
	now := time.Now()
	repo := &fakeAccessControlRepo{
		roles: []domain.Role{
			{ID: "r1", Code: "old-manual", Name: "Old manual"},
			{ID: "r2", Code: "viewer", Name: "Viewer"},
			{ID: "r3", Code: "ldap", Name: "LDAP"},
		},
		roleBindings: []domain.UserRole{
			{UserID: "u1", RoleID: "r1", Source: domain.UserRoleSourceManual},
			{UserID: "u1", RoleID: "r3", Source: domain.UserRoleSourceLDAPImport},
		},
	}
	users := &fakeUserAdminRepo{users: []domain.User{{ID: "u1", Username: "alice", Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now}}}
	audit := &captureAccessAuditRecorder{}
	svc := NewAccessControlService(repo, users, audit)

	rows, err := svc.ReplaceUserManualRoles(context.Background(), Actor{UserID: "admin-user"}, "u1", []string{"r2", "r3", "r2"})
	if err != nil {
		t.Fatalf("ReplaceUserManualRoles error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two effective bindings, got %#v", rows)
	}
	sources := map[string]string{}
	for _, row := range rows {
		sources[row.ID] = row.Source
	}
	if sources["r2"] != string(domain.UserRoleSourceManual) {
		t.Fatalf("expected r2 manual binding, got %#v", rows)
	}
	if sources["r3"] != string(domain.UserRoleSourceLDAPImport) {
		t.Fatalf("expected r3 ldap binding to be preserved, got %#v", rows)
	}
	if _, ok := sources["r1"]; ok {
		t.Fatalf("expected old manual role to be removed, got %#v", rows)
	}
	if len(audit.records) != 1 {
		t.Fatalf("expected one audit record, got %#v", audit.records)
	}
	rec := audit.records[0]
	if rec.ResourceType != resourceTypeIdentityUser || rec.Action != AuditSetUserRoles || rec.UserID != "admin-user" {
		t.Fatalf("unexpected audit record: %#v", rec)
	}
	if _, ok := rec.Payload["password"]; ok {
		t.Fatalf("audit payload must not contain password: %#v", rec.Payload)
	}
}

func TestAccessControlService_ReplaceRolePermissionsValidatesReferencesAndClears(t *testing.T) {
	repo := &fakeAccessControlRepo{
		roles:       []domain.Role{{ID: "role-1", Code: "viewer"}},
		permissions: []domain.Permission{{ID: "perm-1", Code: "app:alerts:read"}},
		rolePerms:   []domain.Permission{{ID: "perm-1", Code: "app:alerts:read"}},
	}
	svc := NewAccessControlService(repo, nil, nil)

	rows, err := svc.ReplaceRolePermissions(context.Background(), Actor{UserID: "admin-user"}, "role-1", nil)
	if err != nil {
		t.Fatalf("ReplaceRolePermissions clear error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected permissions to be cleared, got %#v", rows)
	}
	_, err = svc.ReplaceRolePermissions(context.Background(), Actor{UserID: "admin-user"}, "role-1", []string{"missing"})
	if err == nil {
		t.Fatal("expected reference validation error")
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Code != apperr.CodeNotFound {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestAccessControlService_ReplaceRoleDataScopesAndAITools(t *testing.T) {
	repo := &fakeAccessControlRepo{
		roles:      []domain.Role{{ID: "role-1", Code: "viewer"}},
		dataScopes: []domain.DataScope{{ID: "scope-1", Code: "all-data", ScopeType: domain.DataScopeAll}},
		tools:      []domain.AIToolPermission{{ID: "tool-1", ToolCode: "alert.analyze", PermissionMode: domain.AIToolPermissionReadOnly}},
	}
	svc := NewAccessControlService(repo, nil, nil)

	scopes, err := svc.ReplaceRoleDataScopes(context.Background(), Actor{UserID: "admin-user"}, "role-1", []string{"scope-1", "scope-1"})
	if err != nil {
		t.Fatalf("ReplaceRoleDataScopes error: %v", err)
	}
	if len(scopes) != 1 || scopes[0].ID != "scope-1" {
		t.Fatalf("unexpected data scopes: %#v", scopes)
	}
	tools, err := svc.ReplaceRoleAIToolPermissions(context.Background(), Actor{UserID: "admin-user"}, "role-1", []string{"tool-1", "tool-1"})
	if err != nil {
		t.Fatalf("ReplaceRoleAIToolPermissions error: %v", err)
	}
	if len(tools) != 1 || tools[0].ID != "tool-1" {
		t.Fatalf("unexpected ai tools: %#v", tools)
	}
}

type ensureRoleBindingRepo struct {
	fakeAccessControlRepo
	role    *domain.Role
	bindErr error
	hasRole bool
}

func (f *ensureRoleBindingRepo) FindRoleByCode(ctx context.Context, code string) (*domain.Role, error) {
	if f.role != nil && f.role.Code == code {
		return f.role, nil
	}
	return nil, nil
}

func (f *ensureRoleBindingRepo) ListUserRoleBindings(ctx context.Context, userID string) ([]domain.UserRole, error) {
	return nil, nil
}
func (f *ensureRoleBindingRepo) BindUserRole(ctx context.Context, userID, roleID string, source domain.UserRoleSource) error {
	return f.bindErr
}
func (f *ensureRoleBindingRepo) UnbindUserRole(ctx context.Context, userID, roleID string) error {
	return nil
}

func (f *ensureRoleBindingRepo) HasUserRole(ctx context.Context, userID, roleID string) (bool, error) {
	return f.hasRole, nil
}

func TestEnsureUserHasRoleByCode_RoleMissing(t *testing.T) {
	svc := NewAccessControlService(&ensureRoleBindingRepo{}, nil, nil)
	err := svc.EnsureUserHasRoleByCode(context.Background(), "u1", "admin")
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestEnsureUserHasRoleByCode_UserReferenceNotFound(t *testing.T) {
	svc := NewAccessControlService(&ensureRoleBindingRepo{
		role:    &domain.Role{ID: "r1", Code: "admin"},
		bindErr: fmt.Errorf("%w: user %q", domain.ErrReferenceNotFound, "missing-user"),
	}, nil, nil)
	err := svc.EnsureUserHasRoleByCode(context.Background(), "missing-user", "admin")
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}
