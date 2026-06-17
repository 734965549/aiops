package application

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/identity/domain"
)

type fakeAccessRepo struct {
	roles  []domain.Role
	perms  []domain.Permission
	scopes []domain.DataScope
	tools  []domain.AIToolPermission
	err    error
}

func (f *fakeAccessRepo) ListRoles(ctx context.Context, filter domain.RoleFilter) ([]domain.Role, error) {
	return nil, nil
}
func (f *fakeAccessRepo) CountRoles(ctx context.Context, filter domain.RoleFilter) (int64, error) {
	return 0, nil
}
func (f *fakeAccessRepo) FindRoleByID(ctx context.Context, id string) (*domain.Role, error) {
	return nil, nil
}
func (f *fakeAccessRepo) FindRoleByCode(ctx context.Context, code string) (*domain.Role, error) {
	return nil, nil
}
func (f *fakeAccessRepo) ListPermissions(ctx context.Context, filter domain.PermissionFilter) ([]domain.Permission, error) {
	return nil, nil
}
func (f *fakeAccessRepo) CountPermissions(ctx context.Context, filter domain.PermissionFilter) (int64, error) {
	return 0, nil
}
func (f *fakeAccessRepo) FindPermissionByID(ctx context.Context, id string) (*domain.Permission, error) {
	return nil, nil
}
func (f *fakeAccessRepo) FindPermissionByCode(ctx context.Context, code string) (*domain.Permission, error) {
	return nil, nil
}
func (f *fakeAccessRepo) ListDataScopes(ctx context.Context, filter domain.DataScopeFilter) ([]domain.DataScope, error) {
	return f.scopes, f.err
}
func (f *fakeAccessRepo) FindDataScopeByCode(ctx context.Context, code string) (*domain.DataScope, error) {
	return nil, nil
}
func (f *fakeAccessRepo) FindDataScopeByID(ctx context.Context, id string) (*domain.DataScope, error) {
	return nil, nil
}
func (f *fakeAccessRepo) ListRoleDataScopes(ctx context.Context, roleID string) ([]domain.DataScope, error) {
	return f.scopes, f.err
}
func (f *fakeAccessRepo) ListAIToolPermissions(ctx context.Context, filter domain.AIToolPermissionFilter) ([]domain.AIToolPermission, error) {
	return f.tools, f.err
}
func (f *fakeAccessRepo) CountAIToolPermissions(ctx context.Context, filter domain.AIToolPermissionFilter) (int64, error) {
	return int64(len(f.tools)), f.err
}
func (f *fakeAccessRepo) FindAIToolPermissionByCode(ctx context.Context, code string) (*domain.AIToolPermission, error) {
	return nil, nil
}
func (f *fakeAccessRepo) FindAIToolPermissionByID(ctx context.Context, id string) (*domain.AIToolPermission, error) {
	return nil, nil
}
func (f *fakeAccessRepo) ListRoleAIToolPermissions(ctx context.Context, roleID string) ([]domain.AIToolPermission, error) {
	return f.tools, f.err
}
func (f *fakeAccessRepo) ListUserRoles(ctx context.Context, userID string) ([]domain.Role, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.roles, nil
}
func (f *fakeAccessRepo) ListUserRoleBindings(ctx context.Context, userID string) ([]domain.UserRole, error) {
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}
func (f *fakeAccessRepo) BindUserRole(ctx context.Context, userID, roleID string, source domain.UserRoleSource) error {
	return f.err
}
func (f *fakeAccessRepo) UnbindUserRole(ctx context.Context, userID, roleID string) error {
	return f.err
}
func (f *fakeAccessRepo) BindRolePermission(ctx context.Context, roleID, permissionID string) error {
	return f.err
}
func (f *fakeAccessRepo) BindRoleDataScope(ctx context.Context, roleID, dataScopeID string) error {
	return f.err
}
func (f *fakeAccessRepo) BindRoleAIToolPermission(ctx context.Context, roleID, toolPermissionID string) error {
	return f.err
}
func (f *fakeAccessRepo) HasUserRole(ctx context.Context, userID, roleID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return true, nil
}
func (f *fakeAccessRepo) ListRolePermissions(ctx context.Context, roleID string) ([]domain.Permission, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.perms, nil
}
func (f *fakeAccessRepo) ReplaceUserManualRoles(ctx context.Context, userID string, roleIDs []string) error {
	return f.err
}
func (f *fakeAccessRepo) ReplaceRolePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	return f.err
}
func (f *fakeAccessRepo) ReplaceRoleDataScopes(ctx context.Context, roleID string, dataScopeIDs []string) error {
	return f.err
}
func (f *fakeAccessRepo) ReplaceRoleAIToolPermissions(ctx context.Context, roleID string, toolPermissionIDs []string) error {
	return f.err
}
func (f *fakeAccessRepo) LoadUserGrantContext(ctx context.Context, userID string) (*domain.UserGrantContext, error) {
	if f.err != nil {
		return nil, f.err
	}
	roles, err := f.ListUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &domain.UserGrantContext{
		Roles:             roles,
		Permissions:       f.perms,
		DataScopes:        f.scopes,
		AIToolPermissions: f.tools,
	}, nil
}

func TestAuthorizationServiceDeniedWithoutUser(t *testing.T) {
	svc := NewAuthorizationService(&fakeAccessRepo{})
	res, err := svc.Authorize(context.Background(), AuthorizationInput{})
	if err == nil || res == nil || res.Allowed {
		t.Fatalf("expected unauthenticated denial, got res=%+v err=%v", res, err)
	}
}

func TestAuthorizationServiceAllowsWithPermissionDataScopeAndTool(t *testing.T) {
	repo := &fakeAccessRepo{
		roles: []domain.Role{{ID: "r1", Code: "admin"}},
		perms: []domain.Permission{{Code: "app:alarm:read"}},
		scopes: []domain.DataScope{{
			Code: "dept-scope", ScopeType: domain.DataScopeDepartment,
			ScopeConfig: map[string]any{"department_ids": []any{"platform", "ops"}},
		}},
		tools: []domain.AIToolPermission{{ToolCode: "alarm.analyze", PermissionMode: domain.AIToolPermissionReadOnly}},
	}
	svc := NewAuthorizationService(repo)
	res, err := svc.Authorize(context.Background(), AuthorizationInput{UserID: "u1", Resource: "alarm", Action: "read", ObjectDept: "platform", ToolCode: "alarm.analyze"})
	if err != nil || res == nil || !res.Allowed {
		t.Fatalf("expected allowed, got res=%+v err=%v", res, err)
	}
}

func TestAuthorizationServiceStaticRouteSkipsDataScope(t *testing.T) {
	repo := &fakeAccessRepo{
		roles: []domain.Role{{ID: "r1", Code: "operator"}},
		perms: []domain.Permission{{Code: "app:identity.roles:read"}},
		scopes: []domain.DataScope{{
			Code: "dept-scope", ScopeType: domain.DataScopeDepartment,
			ScopeConfig: map[string]any{"department_ids": []any{"platform"}},
		}},
	}
	svc := NewAuthorizationService(repo)
	res, err := svc.Authorize(context.Background(), AuthorizationInput{
		UserID: "u1", Resource: "identity.roles", Action: "read", SkipDataScope: true,
	})
	if err != nil || res == nil || !res.Allowed {
		t.Fatalf("expected static route allowed without object scope, got res=%+v err=%v", res, err)
	}
}

func TestAuthorizationServiceDeniesRestrictedScopeWithoutObjectContext(t *testing.T) {
	repo := &fakeAccessRepo{
		roles: []domain.Role{{ID: "r1", Code: "operator"}},
		perms: []domain.Permission{{Code: "app:alarm:read"}},
		scopes: []domain.DataScope{{
			Code: "dept-scope", ScopeType: domain.DataScopeDepartment,
			ScopeConfig: map[string]any{"department_ids": []any{"platform"}},
		}},
	}
	svc := NewAuthorizationService(repo)
	res, err := svc.Authorize(context.Background(), AuthorizationInput{UserID: "u1", Resource: "alarm", Action: "read"})
	if err == nil || res == nil || res.Allowed {
		t.Fatalf("expected denial without object context, got res=%+v err=%v", res, err)
	}
}

func TestAuthorizationServiceAllowsAllScopeWithoutObjectContext(t *testing.T) {
	repo := &fakeAccessRepo{
		roles:  []domain.Role{{ID: "r1", Code: "admin"}},
		perms:  []domain.Permission{{Code: "app:alarm:read"}},
		scopes: []domain.DataScope{{Code: "all-data", ScopeType: domain.DataScopeAll}},
	}
	svc := NewAuthorizationService(repo)
	res, err := svc.Authorize(context.Background(), AuthorizationInput{UserID: "u1", Resource: "alarm", Action: "read"})
	if err != nil || res == nil || !res.Allowed {
		t.Fatalf("expected all scope allowed without object context, got res=%+v err=%v", res, err)
	}
}

func TestAuthorizationServiceDeniedMissingPermission(t *testing.T) {
	repo := &fakeAccessRepo{
		roles: []domain.Role{{ID: "r1", Code: "admin"}},
		perms: []domain.Permission{{Code: "app:user:read"}},
		scopes: []domain.DataScope{{
			Code: "dept-scope", ScopeType: domain.DataScopeDepartment,
			ScopeConfig: map[string]any{"department_ids": []any{"platform"}},
		}},
	}
	svc := NewAuthorizationService(repo)
	res, err := svc.Authorize(context.Background(), AuthorizationInput{UserID: "u1", Resource: "alarm", Action: "read", ObjectDept: "platform"})
	if err == nil || res == nil || res.Allowed {
		t.Fatalf("expected permission denied, got res=%+v err=%v", res, err)
	}
}

func TestMatchDataScope_AllPassesWithoutObjectContext(t *testing.T) {
	scopes := []domain.DataScope{{Code: "all-data", ScopeType: domain.DataScopeAll}}
	if !matchDataScopeForTest(AuthorizationInput{}, scopes) {
		t.Fatal("expected all scope to pass without object context")
	}
}

func TestMatchDataScope_DepartmentRequiresExplicitMatch(t *testing.T) {
	scopes := []domain.DataScope{{
		Code: "dept-scope", ScopeType: domain.DataScopeDepartment,
		ScopeConfig: map[string]any{"department_ids": []any{"platform"}},
	}}
	in := AuthorizationInput{ObjectDept: "platform"}
	if !matchDataScopeForTest(in, scopes) {
		t.Fatal("expected matching department to pass")
	}
	if matchDataScopeForTest(AuthorizationInput{ObjectDept: "other"}, scopes) {
		t.Fatal("expected non-matching department to fail")
	}
	if matchDataScopeForTest(AuthorizationInput{}, scopes) {
		t.Fatal("expected empty object dept to fail")
	}
}

func TestMatchDataScope_DepartmentWithoutConfigDenied(t *testing.T) {
	scopes := []domain.DataScope{{Code: "dept-scope", ScopeType: domain.DataScopeDepartment}}
	if matchDataScopeForTest(AuthorizationInput{ObjectDept: "platform"}, scopes) {
		t.Fatal("expected department scope without config to fail")
	}
}

func TestMatchDataScope_TeamRegionTagAndCustom(t *testing.T) {
	cases := []struct {
		name  string
		scope domain.DataScope
		in    AuthorizationInput
		allow bool
	}{
		{
			name:  "team match",
			scope: domain.DataScope{ScopeType: domain.DataScopeTeam, ScopeConfig: map[string]any{"team_ids": []any{"sre"}}},
			in:    AuthorizationInput{ObjectTeam: "sre"},
			allow: true,
		},
		{
			name:  "team mismatch",
			scope: domain.DataScope{ScopeType: domain.DataScopeTeam, ScopeConfig: map[string]any{"team_ids": []any{"sre"}}},
			in:    AuthorizationInput{ObjectTeam: "dba"},
			allow: false,
		},
		{
			name:  "region match",
			scope: domain.DataScope{ScopeType: domain.DataScopeRegion, ScopeConfig: map[string]any{"regions": []any{"cn-east"}}},
			in:    AuthorizationInput{ObjectRegion: "cn-east"},
			allow: true,
		},
		{
			name:  "tag intersection",
			scope: domain.DataScope{ScopeType: domain.DataScopeTag, ScopeConfig: map[string]any{"tags": []any{"prod", "core"}}},
			in:    AuthorizationInput{ObjectTags: []string{"staging", "core"}},
			allow: true,
		},
		{
			name:  "tag no overlap",
			scope: domain.DataScope{ScopeType: domain.DataScopeTag, ScopeConfig: map[string]any{"tags": []any{"prod"}}},
			in:    AuthorizationInput{ObjectTags: []string{"staging"}},
			allow: false,
		},
		{
			name:  "custom owner match",
			scope: domain.DataScope{ScopeType: domain.DataScopeCustom, ScopeConfig: map[string]any{"owner_ids": []any{"user-1"}}},
			in:    AuthorizationInput{ObjectOwner: "user-1"},
			allow: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchDataScopeForTest(tc.in, []domain.DataScope{tc.scope})
			if got != tc.allow {
				t.Fatalf("expected allow=%v, got %v", tc.allow, got)
			}
		})
	}
}

func matchDataScopeForTest(in AuthorizationInput, scopes []domain.DataScope) bool {
	return NewAuthorizationService(&fakeAccessRepo{}).matchDataScope(in, scopes)
}

func TestAuthorizationServiceWrapsRepoError(t *testing.T) {
	repo := &fakeAccessRepo{err: errors.New("boom")}
	svc := NewAuthorizationService(repo)
	_, err := svc.Authorize(context.Background(), AuthorizationInput{UserID: "u1"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestMatchToolRequireConfirmSemantics(t *testing.T) {
	svc := NewAuthorizationService(&fakeAccessRepo{})
	tools := []domain.AIToolPermission{{
		ToolCode: "execution.runbook", PermissionMode: domain.AIToolPermissionRequireConfirm,
	}}

	mode, ok := svc.matchTool("execution.runbook", tools, true)
	if !ok || mode != domain.AIToolPermissionRequireConfirm {
		t.Fatalf("expected allow when user confirmed, got mode=%q ok=%v", mode, ok)
	}

	mode, ok = svc.matchTool("execution.runbook", tools, false)
	if ok {
		t.Fatalf("expected deny without confirmation when PermitsUnconfirmedInvoke=false")
	}

	tools[0].PermitsUnconfirmedInvoke = true
	mode, ok = svc.matchTool("execution.runbook", tools, false)
	if !ok || mode != domain.AIToolPermissionRequireConfirm {
		t.Fatalf("expected allow for trusted role without confirmation, got mode=%q ok=%v", mode, ok)
	}
}

func TestMatchToolDenyMode(t *testing.T) {
	svc := NewAuthorizationService(&fakeAccessRepo{})
	tools := []domain.AIToolPermission{{ToolCode: "danger", PermissionMode: domain.AIToolPermissionDeny}}
	if _, ok := svc.matchTool("danger", tools, true); ok {
		t.Fatal("expected deny mode to reject")
	}
}
