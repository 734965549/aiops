package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/gin-gonic/gin"
)

type fakeHTTPUserService struct{}

func (f *fakeHTTPUserService) GetCurrentUser(ctx context.Context, userID string) (*identityapp.CurrentUserDTO, error) {
	return &identityapp.CurrentUserDTO{ID: userID, Username: "alice"}, nil
}

type fakeHTTPAccessControlService struct {
	roles        []domain.Role
	permissions  []domain.Permission
	users        []identityapp.UserDTO
	userBindings []identityapp.UserRoleBindingDTO
	scopes       []domain.DataScope
	tools        []domain.AIToolPermission
	savedRoleIDs []string
}

func (f *fakeHTTPAccessControlService) ListRoles(ctx context.Context, filter domain.RoleFilter) ([]domain.Role, error) {
	return f.roles, nil
}
func (f *fakeHTTPAccessControlService) CountRoles(ctx context.Context, filter domain.RoleFilter) (int64, error) {
	return int64(len(f.roles)), nil
}
func (f *fakeHTTPAccessControlService) ListPermissions(ctx context.Context, filter domain.PermissionFilter) ([]domain.Permission, error) {
	return f.permissions, nil
}
func (f *fakeHTTPAccessControlService) CountPermissions(ctx context.Context, filter domain.PermissionFilter) (int64, error) {
	return int64(len(f.permissions)), nil
}
func (f *fakeHTTPAccessControlService) ListUserRoles(ctx context.Context, userID string) ([]domain.Role, error) {
	return f.roles, nil
}
func (f *fakeHTTPAccessControlService) ListUsers(ctx context.Context, filter domain.UserFilter) ([]identityapp.UserDTO, int64, error) {
	return f.users, int64(len(f.users)), nil
}
func (f *fakeHTTPAccessControlService) ListUserRoleBindings(ctx context.Context, userID string) ([]identityapp.UserRoleBindingDTO, error) {
	return f.userBindings, nil
}
func (f *fakeHTTPAccessControlService) ReplaceUserManualRoles(ctx context.Context, actor identityapp.Actor, userID string, roleIDs []string) ([]identityapp.UserRoleBindingDTO, error) {
	f.savedRoleIDs = append([]string(nil), roleIDs...)
	return f.userBindings, nil
}
func (f *fakeHTTPAccessControlService) ListRolePermissions(ctx context.Context, roleID string) ([]domain.Permission, error) {
	return f.permissions, nil
}
func (f *fakeHTTPAccessControlService) ReplaceRolePermissions(ctx context.Context, actor identityapp.Actor, roleID string, permissionIDs []string) ([]domain.Permission, error) {
	return f.permissions, nil
}
func (f *fakeHTTPAccessControlService) ListDataScopes(ctx context.Context, filter domain.DataScopeFilter) ([]domain.DataScope, error) {
	return f.scopes, nil
}
func (f *fakeHTTPAccessControlService) ListRoleDataScopes(ctx context.Context, roleID string) ([]domain.DataScope, error) {
	return f.scopes, nil
}
func (f *fakeHTTPAccessControlService) ReplaceRoleDataScopes(ctx context.Context, actor identityapp.Actor, roleID string, dataScopeIDs []string) ([]domain.DataScope, error) {
	return f.scopes, nil
}
func (f *fakeHTTPAccessControlService) ListAIToolPermissions(ctx context.Context, filter domain.AIToolPermissionFilter) ([]domain.AIToolPermission, error) {
	return f.tools, nil
}
func (f *fakeHTTPAccessControlService) ListRoleAIToolPermissions(ctx context.Context, roleID string) ([]domain.AIToolPermission, error) {
	return f.tools, nil
}
func (f *fakeHTTPAccessControlService) ReplaceRoleAIToolPermissions(ctx context.Context, actor identityapp.Actor, roleID string, toolPermissionIDs []string) ([]domain.AIToolPermission, error) {
	return f.tools, nil
}

func TestHandlerRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&fakeHTTPUserService{}, nil, &fakeHTTPAccessControlService{roles: []domain.Role{{ID: "r1", Code: "admin", Name: "管理员", Status: domain.RoleStatusActive}}}, nil, nil, nil, nil)
	r := gin.New()
	r.GET("/roles", h.Roles)

	req := httptest.NewRequest(http.MethodGet, "/roles?page=1&page_size=10&status=active", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Data struct {
			Items    []map[string]any `json:"items"`
			Total    int64            `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Data.Total != 1 || resp.Data.Page != 1 || resp.Data.PageSize != 10 || len(resp.Data.Items) != 1 {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}

func TestHandlerPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&fakeHTTPUserService{}, nil, &fakeHTTPAccessControlService{permissions: []domain.Permission{{ID: "p1", Code: "app:user:read", Resource: "user", Action: "read"}}}, nil, nil, nil, nil)
	r := gin.New()
	r.GET("/permissions", h.Permissions)

	req := httptest.NewRequest(http.MethodGet, "/permissions?page=1&page_size=10&resource=user&action=read", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Data struct {
			Items    []map[string]any `json:"items"`
			Total    int64            `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Data.Total != 1 || resp.Data.Page != 1 || resp.Data.PageSize != 10 || len(resp.Data.Items) != 1 {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}

func TestHandlerAdminReplaceUserRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	access := &fakeHTTPAccessControlService{
		userBindings: []identityapp.UserRoleBindingDTO{{ID: "r1", Code: "viewer", Source: "manual"}},
	}
	h := NewHandler(&fakeHTTPUserService{}, nil, access, nil, nil, nil, nil)
	r := gin.New()
	r.PUT("/users/:user_id/roles", h.AdminReplaceUserRoles)

	req := httptest.NewRequest(http.MethodPut, "/users/u1/roles", bytes.NewBufferString(`{"role_ids":["r1"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if len(access.savedRoleIDs) != 1 || access.savedRoleIDs[0] != "r1" {
		t.Fatalf("unexpected saved role ids: %#v", access.savedRoleIDs)
	}
}

func TestHandlerAdminReplaceUserRolesInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&fakeHTTPUserService{}, nil, &fakeHTTPAccessControlService{}, nil, nil, nil, nil)
	r := gin.New()
	r.PUT("/users/:user_id/roles", h.AdminReplaceUserRoles)

	req := httptest.NewRequest(http.MethodPut, "/users/u1/roles", bytes.NewBufferString(`{`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}
