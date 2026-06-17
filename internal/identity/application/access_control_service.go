package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/734965549/aiops/internal/identity/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type AccessControlService struct {
	ac    domain.AccessControlRepository
	users userAdminRepository
	audit AccessAuditRecorder
}

type userAdminRepository interface {
	FindByID(ctx context.Context, id string) (*domain.User, error)
	List(ctx context.Context, filter domain.UserFilter) ([]domain.User, error)
	Count(ctx context.Context, filter domain.UserFilter) (int64, error)
}

type Actor struct {
	UserID      string
	DisplayName string
}

type AccessAuditAction string

const (
	AuditSetUserRoles        AccessAuditAction = "set_user_roles"
	AuditSetRolePermissions  AccessAuditAction = "set_role_permissions"
	AuditSetRoleDataScopes   AccessAuditAction = "set_role_data_scopes"
	AuditSetRoleAIToolPerms  AccessAuditAction = "set_role_ai_tools"
	resourceTypeIdentityUser                   = "identity_user"
	resourceTypeIdentityRole                   = "identity_role"
)

type AccessAuditRecord struct {
	ResourceType string
	ResourceID   string
	Action       AccessAuditAction
	UserID       string
	Payload      map[string]any
}

type AccessAuditRecorder interface {
	Record(ctx context.Context, rec AccessAuditRecord) error
}

type NoopAccessAuditRecorder struct{}

func (NoopAccessAuditRecorder) Record(context.Context, AccessAuditRecord) error { return nil }

func NewAccessControlService(ac domain.AccessControlRepository, users userAdminRepository, audit AccessAuditRecorder) *AccessControlService {
	if audit == nil {
		audit = NoopAccessAuditRecorder{}
	}
	return &AccessControlService{ac: ac, users: users, audit: audit}
}

type UserDTO struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type UserRoleBindingDTO struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	IsSystem    bool   `json:"is_system"`
	Source      string `json:"source"`
}

func (s *AccessControlService) ListUsers(ctx context.Context, filter domain.UserFilter) ([]UserDTO, int64, error) {
	if s == nil || s.users == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "user service is not configured")
	}
	rows, err := s.users.List(ctx, filter)
	if err != nil {
		return nil, 0, apperr.Wrap(err, apperr.CodeInternal, "list users failed")
	}
	total, err := s.users.Count(ctx, filter)
	if err != nil {
		return nil, 0, apperr.Wrap(err, apperr.CodeInternal, "count users failed")
	}
	out := make([]UserDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, toUserDTO(row))
	}
	return out, total, nil
}

func (s *AccessControlService) ListRoles(ctx context.Context, filter domain.RoleFilter) ([]domain.Role, error) {
	if s == nil || s.ac == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "access control service is not configured")
	}
	rows, err := s.ac.ListRoles(ctx, filter)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list roles failed")
	}
	return rows, nil
}

func (s *AccessControlService) CountRoles(ctx context.Context, filter domain.RoleFilter) (int64, error) {
	if s == nil || s.ac == nil {
		return 0, apperr.New(apperr.CodeUnavailable, "access control service is not configured")
	}
	n, err := s.ac.CountRoles(ctx, filter)
	if err != nil {
		return 0, apperr.Wrap(err, apperr.CodeInternal, "count roles failed")
	}
	return n, nil
}

func (s *AccessControlService) ListPermissions(ctx context.Context, filter domain.PermissionFilter) ([]domain.Permission, error) {
	if s == nil || s.ac == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "access control service is not configured")
	}
	rows, err := s.ac.ListPermissions(ctx, filter)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list permissions failed")
	}
	return rows, nil
}

func (s *AccessControlService) CountPermissions(ctx context.Context, filter domain.PermissionFilter) (int64, error) {
	if s == nil || s.ac == nil {
		return 0, apperr.New(apperr.CodeUnavailable, "access control service is not configured")
	}
	n, err := s.ac.CountPermissions(ctx, filter)
	if err != nil {
		return 0, apperr.Wrap(err, apperr.CodeInternal, "count permissions failed")
	}
	return n, nil
}

func (s *AccessControlService) ListUserRoles(ctx context.Context, userID string) ([]domain.Role, error) {
	if s == nil || s.ac == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "access control service is not configured")
	}
	rows, err := s.ac.ListUserRoles(ctx, userID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list user roles failed")
	}
	return rows, nil
}

func (s *AccessControlService) ListUserRoleBindings(ctx context.Context, userID string) ([]UserRoleBindingDTO, error) {
	if err := s.ensureUserExists(ctx, userID); err != nil {
		return nil, err
	}
	bindings, err := s.ac.ListUserRoleBindings(ctx, userID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list user role bindings failed")
	}
	roles, err := s.ac.ListRoles(ctx, domain.RoleFilter{})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list roles failed")
	}
	roleByID := make(map[string]domain.Role, len(roles))
	for _, role := range roles {
		roleByID[role.ID] = role
	}
	out := make([]UserRoleBindingDTO, 0, len(bindings))
	for _, binding := range bindings {
		role, ok := roleByID[binding.RoleID]
		if !ok {
			continue
		}
		out = append(out, toUserRoleBindingDTO(role, binding.Source))
	}
	return out, nil
}

func (s *AccessControlService) ReplaceUserManualRoles(ctx context.Context, actor Actor, userID string, roleIDs []string) ([]UserRoleBindingDTO, error) {
	if err := s.ensureUserExists(ctx, userID); err != nil {
		return nil, err
	}
	roleIDs = uniqueTrimmed(roleIDs)
	if err := s.ensureRolesExist(ctx, roleIDs); err != nil {
		return nil, err
	}
	oldBindings, err := s.ac.ListUserRoleBindings(ctx, userID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list user role bindings failed")
	}
	oldManual := manualRoleIDs(oldBindings)
	roleIDs = excludeNonManualRoleIDs(roleIDs, oldBindings)
	if err := s.ac.ReplaceUserManualRoles(ctx, userID, roleIDs); err != nil {
		return nil, wrapAccessWriteError(err, "replace user roles failed")
	}
	s.recordAudit(ctx, resourceTypeIdentityUser, userID, actor.UserID, AuditSetUserRoles, map[string]any{
		"old_role_ids": oldManual,
		"new_role_ids": roleIDs,
	})
	return s.ListUserRoleBindings(ctx, userID)
}

func (s *AccessControlService) ListRolePermissions(ctx context.Context, roleID string) ([]domain.Permission, error) {
	if err := s.ensureRoleExists(ctx, roleID); err != nil {
		return nil, err
	}
	rows, err := s.ac.ListRolePermissions(ctx, roleID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list role permissions failed")
	}
	return rows, nil
}

func (s *AccessControlService) ReplaceRolePermissions(ctx context.Context, actor Actor, roleID string, permissionIDs []string) ([]domain.Permission, error) {
	if err := s.ensureRoleExists(ctx, roleID); err != nil {
		return nil, err
	}
	permissionIDs = uniqueTrimmed(permissionIDs)
	if err := s.ensurePermissionsExist(ctx, permissionIDs); err != nil {
		return nil, err
	}
	oldRows, err := s.ac.ListRolePermissions(ctx, roleID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list role permissions failed")
	}
	if err := s.ac.ReplaceRolePermissions(ctx, roleID, permissionIDs); err != nil {
		return nil, wrapAccessWriteError(err, "replace role permissions failed")
	}
	s.recordAudit(ctx, resourceTypeIdentityRole, roleID, actor.UserID, AuditSetRolePermissions, map[string]any{
		"old_permission_ids": permissionIDsFromRows(oldRows),
		"new_permission_ids": permissionIDs,
	})
	return s.ListRolePermissions(ctx, roleID)
}

func (s *AccessControlService) ListDataScopes(ctx context.Context, filter domain.DataScopeFilter) ([]domain.DataScope, error) {
	if s == nil || s.ac == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "access control service is not configured")
	}
	rows, err := s.ac.ListDataScopes(ctx, filter)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list data scopes failed")
	}
	return rows, nil
}

func (s *AccessControlService) ListRoleDataScopes(ctx context.Context, roleID string) ([]domain.DataScope, error) {
	if err := s.ensureRoleExists(ctx, roleID); err != nil {
		return nil, err
	}
	rows, err := s.ac.ListRoleDataScopes(ctx, roleID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list role data scopes failed")
	}
	return rows, nil
}

func (s *AccessControlService) ReplaceRoleDataScopes(ctx context.Context, actor Actor, roleID string, dataScopeIDs []string) ([]domain.DataScope, error) {
	if err := s.ensureRoleExists(ctx, roleID); err != nil {
		return nil, err
	}
	dataScopeIDs = uniqueTrimmed(dataScopeIDs)
	if err := s.ensureDataScopesExist(ctx, dataScopeIDs); err != nil {
		return nil, err
	}
	oldRows, err := s.ac.ListRoleDataScopes(ctx, roleID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list role data scopes failed")
	}
	if err := s.ac.ReplaceRoleDataScopes(ctx, roleID, dataScopeIDs); err != nil {
		return nil, wrapAccessWriteError(err, "replace role data scopes failed")
	}
	s.recordAudit(ctx, resourceTypeIdentityRole, roleID, actor.UserID, AuditSetRoleDataScopes, map[string]any{
		"old_data_scope_ids": dataScopeIDsFromRows(oldRows),
		"new_data_scope_ids": dataScopeIDs,
	})
	return s.ListRoleDataScopes(ctx, roleID)
}

func (s *AccessControlService) ListAIToolPermissions(ctx context.Context, filter domain.AIToolPermissionFilter) ([]domain.AIToolPermission, error) {
	if s == nil || s.ac == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "access control service is not configured")
	}
	rows, err := s.ac.ListAIToolPermissions(ctx, filter)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list ai tool permissions failed")
	}
	return rows, nil
}

func (s *AccessControlService) CountAIToolPermissions(ctx context.Context, filter domain.AIToolPermissionFilter) (int64, error) {
	if s == nil || s.ac == nil {
		return 0, apperr.New(apperr.CodeUnavailable, "access control service is not configured")
	}
	n, err := s.ac.CountAIToolPermissions(ctx, filter)
	if err != nil {
		return 0, apperr.Wrap(err, apperr.CodeInternal, "count ai tool permissions failed")
	}
	return n, nil
}

func (s *AccessControlService) ListRoleAIToolPermissions(ctx context.Context, roleID string) ([]domain.AIToolPermission, error) {
	if err := s.ensureRoleExists(ctx, roleID); err != nil {
		return nil, err
	}
	rows, err := s.ac.ListRoleAIToolPermissions(ctx, roleID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list role ai tool permissions failed")
	}
	return rows, nil
}

func (s *AccessControlService) ReplaceRoleAIToolPermissions(ctx context.Context, actor Actor, roleID string, toolPermissionIDs []string) ([]domain.AIToolPermission, error) {
	if err := s.ensureRoleExists(ctx, roleID); err != nil {
		return nil, err
	}
	toolPermissionIDs = uniqueTrimmed(toolPermissionIDs)
	if err := s.ensureAIToolPermissionsExist(ctx, toolPermissionIDs); err != nil {
		return nil, err
	}
	oldRows, err := s.ac.ListRoleAIToolPermissions(ctx, roleID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list role ai tool permissions failed")
	}
	if err := s.ac.ReplaceRoleAIToolPermissions(ctx, roleID, toolPermissionIDs); err != nil {
		return nil, wrapAccessWriteError(err, "replace role ai tool permissions failed")
	}
	s.recordAudit(ctx, resourceTypeIdentityRole, roleID, actor.UserID, AuditSetRoleAIToolPerms, map[string]any{
		"old_tool_permission_ids": aiToolPermissionIDsFromRows(oldRows),
		"new_tool_permission_ids": toolPermissionIDs,
	})
	return s.ListRoleAIToolPermissions(ctx, roleID)
}

func (s *AccessControlService) EnsureBootstrapUserRoleByUsername(ctx context.Context, users domain.UserRepository, username, roleCode string) error {
	if s == nil || s.ac == nil || users == nil {
		return apperr.New(apperr.CodeUnavailable, "access control service is not configured")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}
	u, err := users.FindByUsername(ctx, username)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "find bootstrap user failed")
	}
	if u == nil {
		return apperr.New(apperr.CodeFailedPrecondition, fmt.Sprintf("bootstrap user %q not found", username))
	}
	return s.EnsureUserHasRoleByCode(ctx, u.ID, roleCode)
}

func (s *AccessControlService) EnsureUserHasRoleByCode(ctx context.Context, userID, roleCode string) error {
	if s == nil || s.ac == nil {
		return apperr.New(apperr.CodeUnavailable, "access control service is not configured")
	}
	userID = strings.TrimSpace(userID)
	roleCode = strings.TrimSpace(roleCode)
	if userID == "" || roleCode == "" {
		return nil
	}
	role, err := s.ac.FindRoleByCode(ctx, roleCode)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "find role failed")
	}
	if role == nil {
		return apperr.New(apperr.CodeFailedPrecondition, fmt.Sprintf("role %q not found; ensure migration 0002 has been applied", roleCode))
	}
	if err := s.ac.BindUserRole(ctx, userID, role.ID, domain.UserRoleSourceManual); err != nil {
		if errors.Is(err, domain.ErrReferenceNotFound) {
			return apperr.Wrap(err, apperr.CodeFailedPrecondition, "bind user role failed")
		}
		return apperr.Wrap(err, apperr.CodeInternal, "bind user role failed")
	}
	ok, err := s.ac.HasUserRole(ctx, userID, role.ID)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "verify user role failed")
	}
	if !ok {
		return apperr.New(apperr.CodeInternal, fmt.Sprintf("failed to bind role %q to user %q", roleCode, userID))
	}
	return nil
}

func (s *AccessControlService) ensureUserExists(ctx context.Context, userID string) error {
	if s == nil || s.users == nil {
		return apperr.New(apperr.CodeUnavailable, "user service is not configured")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return apperr.New(apperr.CodeInvalidArgument, "user_id is required")
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "load user failed")
	}
	if u == nil {
		return apperr.New(apperr.CodeNotFound, "user not found")
	}
	return nil
}

func (s *AccessControlService) ensureRoleExists(ctx context.Context, roleID string) error {
	if s == nil || s.ac == nil {
		return apperr.New(apperr.CodeUnavailable, "access control service is not configured")
	}
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return apperr.New(apperr.CodeInvalidArgument, "role_id is required")
	}
	role, err := s.ac.FindRoleByID(ctx, roleID)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "load role failed")
	}
	if role == nil {
		return apperr.New(apperr.CodeNotFound, "role not found")
	}
	return nil
}

func (s *AccessControlService) ensureRolesExist(ctx context.Context, roleIDs []string) error {
	for _, id := range roleIDs {
		if err := s.ensureRoleExists(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *AccessControlService) ensurePermissionsExist(ctx context.Context, ids []string) error {
	for _, id := range ids {
		p, err := s.ac.FindPermissionByID(ctx, id)
		if err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "load permission failed")
		}
		if p == nil {
			return apperr.New(apperr.CodeNotFound, "permission not found")
		}
	}
	return nil
}

func (s *AccessControlService) ensureDataScopesExist(ctx context.Context, ids []string) error {
	for _, id := range ids {
		sc, err := s.ac.FindDataScopeByID(ctx, id)
		if err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "load data scope failed")
		}
		if sc == nil {
			return apperr.New(apperr.CodeNotFound, "data scope not found")
		}
	}
	return nil
}

func (s *AccessControlService) ensureAIToolPermissionsExist(ctx context.Context, ids []string) error {
	for _, id := range ids {
		tp, err := s.ac.FindAIToolPermissionByID(ctx, id)
		if err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "load ai tool permission failed")
		}
		if tp == nil {
			return apperr.New(apperr.CodeNotFound, "ai tool permission not found")
		}
	}
	return nil
}

func (s *AccessControlService) recordAudit(ctx context.Context, resourceType, resourceID, userID string, action AccessAuditAction, payload map[string]any) {
	if s == nil || s.audit == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["actor_user_id"] = strings.TrimSpace(userID)
	_ = s.audit.Record(ctx, AccessAuditRecord{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		UserID:       userID,
		Payload:      payload,
	})
}

func wrapAccessWriteError(err error, op string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrReferenceNotFound) {
		return apperr.Wrap(err, apperr.CodeFailedPrecondition, op)
	}
	return apperr.Wrap(err, apperr.CodeInternal, op)
}

func toUserDTO(u domain.User) UserDTO {
	return UserDTO{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		Status:      string(u.Status),
		CreatedAt:   u.CreatedAt.Unix(),
		UpdatedAt:   u.UpdatedAt.Unix(),
	}
}

func toUserRoleBindingDTO(role domain.Role, source domain.UserRoleSource) UserRoleBindingDTO {
	return UserRoleBindingDTO{
		ID:          role.ID,
		Code:        role.Code,
		Name:        role.Name,
		Description: role.Description,
		Status:      string(role.Status),
		IsSystem:    role.IsSystem,
		Source:      string(domain.NormalizeUserRoleSource(source)),
	}
}

func uniqueTrimmed(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func manualRoleIDs(bindings []domain.UserRole) []string {
	out := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if domain.NormalizeUserRoleSource(binding.Source) == domain.UserRoleSourceManual {
			out = append(out, binding.RoleID)
		}
	}
	return out
}

func excludeNonManualRoleIDs(roleIDs []string, bindings []domain.UserRole) []string {
	blocked := make(map[string]struct{})
	for _, binding := range bindings {
		if domain.NormalizeUserRoleSource(binding.Source) != domain.UserRoleSourceManual {
			blocked[binding.RoleID] = struct{}{}
		}
	}
	if len(blocked) == 0 {
		return roleIDs
	}
	out := make([]string, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		if _, ok := blocked[roleID]; ok {
			continue
		}
		out = append(out, roleID)
	}
	return out
}

func permissionIDsFromRows(rows []domain.Permission) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out
}

func dataScopeIDsFromRows(rows []domain.DataScope) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out
}

func aiToolPermissionIDsFromRows(rows []domain.AIToolPermission) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out
}
