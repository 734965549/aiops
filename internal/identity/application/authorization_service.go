package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/734965549/aiops/internal/identity/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// AuthorizationInput 表示一次统一权限校验请求。
type AuthorizationInput struct {
	UserID       string
	Resource     string
	Action       string
	ObjectOwner  string
	ObjectDept   string
	ObjectTeam   string
	ObjectRegion string
	ObjectTags   []string
	ToolCode     string
	// UserConfirmed 表示调用方已确认本次高风险工具执行（对应 HTTP 字段 require_confirmed）。
	UserConfirmed      bool `json:"require_confirmed"`
	RequiredPermission string
	// SkipDataScope 为 true 时跳过数据范围校验（静态路由 / 纯操作授权）。
	SkipDataScope bool
}

// AuthorizationResult 表示一次权限校验结果。
type AuthorizationResult struct {
	Allowed            bool     `json:"allowed"`
	Reason             string   `json:"reason,omitempty"`
	MatchedRoleNames   []string `json:"matched_role_names,omitempty"`
	MatchedPermissions []string `json:"matched_permissions,omitempty"`
	MatchedScopes      []string `json:"matched_scopes,omitempty"`
	ToolMode           string   `json:"tool_mode,omitempty"`
}

// AuthorizationService 统一负责 RBAC + 数据权限 + 操作权限校验。
type AuthorizationService struct {
	ac domain.AccessControlRepository
}

// NewAuthorizationService 构造统一授权服务。
func NewAuthorizationService(ac domain.AccessControlRepository) *AuthorizationService {
	return &AuthorizationService{ac: ac}
}

// Authorize 对资源、数据范围和工具权限执行统一判断。
func (s *AuthorizationService) Authorize(ctx context.Context, in AuthorizationInput) (*AuthorizationResult, error) {
	if s == nil || s.ac == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "authorization service is not configured")
	}
	if strings.TrimSpace(in.UserID) == "" {
		return &AuthorizationResult{Allowed: false, Reason: "missing user identity"}, apperr.New(apperr.CodeUnauthenticated, "missing user identity")
	}

	grant, err := s.ac.LoadUserGrantContext(ctx, in.UserID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load user grant context failed")
	}
	if grant == nil || len(grant.Roles) == 0 {
		return &AuthorizationResult{Allowed: false, Reason: "no roles bound"}, apperr.New(apperr.CodePermissionDenied, "permission denied")
	}
	roles := grant.Roles
	rolePermissions := dedupePermissions(grant.Permissions)
	dataScopes := dedupeDataScopes(grant.DataScopes)
	toolPerms := dedupeAIToolPermissions(grant.AIToolPermissions)

	res := &AuthorizationResult{
		Allowed:            true,
		MatchedRoleNames:   make([]string, 0, len(roles)),
		MatchedPermissions: make([]string, 0, len(rolePermissions)),
		MatchedScopes:      make([]string, 0, len(dataScopes)),
	}
	for _, r := range roles {
		res.MatchedRoleNames = append(res.MatchedRoleNames, r.Code)
	}
	for _, p := range rolePermissions {
		res.MatchedPermissions = append(res.MatchedPermissions, p.Code)
	}
	for _, sc := range dataScopes {
		res.MatchedScopes = append(res.MatchedScopes, sc.Code)
	}

	if strings.TrimSpace(in.RequiredPermission) != "" && !contains(res.MatchedPermissions, in.RequiredPermission) {
		return deny("missing required permission")
	}
	if in.Resource != "" && in.Action != "" {
		code := fmt.Sprintf("app:%s:%s", in.Resource, in.Action)
		if !contains(res.MatchedPermissions, code) {
			return deny("missing operation permission")
		}
	}
	if scopeRes, scopeErr := s.enforceDataScope(in, dataScopes); scopeErr != nil {
		return scopeRes, scopeErr
	}
	if strings.TrimSpace(in.ToolCode) != "" {
		mode, ok := s.matchTool(in.ToolCode, toolPerms, in.UserConfirmed)
		if !ok {
			return deny("tool permission denied")
		}
		res.ToolMode = string(mode)
	}
	return res, nil
}

func hasObjectContext(in AuthorizationInput) bool {
	if strings.TrimSpace(in.ObjectOwner) != "" ||
		strings.TrimSpace(in.ObjectDept) != "" ||
		strings.TrimSpace(in.ObjectTeam) != "" ||
		strings.TrimSpace(in.ObjectRegion) != "" ||
		len(in.ObjectTags) > 0 {
		return true
	}
	return false
}

func hasAllDataScope(scopes []domain.DataScope) bool {
	for _, sc := range scopes {
		if sc.ScopeType == domain.DataScopeAll {
			return true
		}
	}
	return false
}

// enforceDataScope 在用户配置了非 all 数据范围时，禁止客户端省略对象上下文绕过校验。
func (s *AuthorizationService) enforceDataScope(in AuthorizationInput, scopes []domain.DataScope) (*AuthorizationResult, error) {
	if in.SkipDataScope || len(scopes) == 0 {
		return nil, nil
	}
	if !hasObjectContext(in) {
		if !hasAllDataScope(scopes) {
			return deny("data scope denied: missing object context")
		}
		return nil, nil
	}
	if !s.matchDataScope(in, scopes) {
		return deny("data scope denied")
	}
	return nil, nil
}

func deny(reason string) (*AuthorizationResult, error) {
	return &AuthorizationResult{Allowed: false, Reason: reason}, apperr.New(apperr.CodePermissionDenied, reason)
}

func contains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), target) {
			return true
		}
	}
	return false
}

func (s *AuthorizationService) matchDataScope(in AuthorizationInput, scopes []domain.DataScope) bool {
	if len(scopes) == 0 {
		return false
	}
	for _, sc := range scopes {
		if matchSingleDataScope(in, sc) {
			return true
		}
	}
	return false
}

func matchSingleDataScope(in AuthorizationInput, sc domain.DataScope) bool {
	switch sc.ScopeType {
	case domain.DataScopeAll:
		return true
	case domain.DataScopeDepartment:
		return matchScopeValue(in.ObjectDept, scopeConfigStrings(sc.ScopeConfig, "department_ids", "dept_ids", "departments"))
	case domain.DataScopeTeam:
		return matchScopeValue(in.ObjectTeam, scopeConfigStrings(sc.ScopeConfig, "team_ids", "teams"))
	case domain.DataScopeRegion:
		return matchScopeValue(in.ObjectRegion, scopeConfigStrings(sc.ScopeConfig, "region_ids", "regions"))
	case domain.DataScopeTag:
		return matchScopeTags(in.ObjectTags, scopeConfigStrings(sc.ScopeConfig, "tags", "tag_ids"))
	case domain.DataScopeCustom:
		return matchScopeValue(in.ObjectOwner, scopeConfigStrings(sc.ScopeConfig, "owner_ids", "user_ids", "owners"))
	default:
		return false
	}
}

func matchScopeValue(objectValue string, allowed []string) bool {
	objectValue = strings.TrimSpace(objectValue)
	if objectValue == "" || len(allowed) == 0 {
		return false
	}
	for _, v := range allowed {
		if strings.EqualFold(strings.TrimSpace(v), objectValue) {
			return true
		}
	}
	return false
}

func matchScopeTags(objectTags, allowed []string) bool {
	if len(objectTags) == 0 || len(allowed) == 0 {
		return false
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, v := range allowed {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		allowedSet[strings.ToLower(v)] = struct{}{}
	}
	if len(allowedSet) == 0 {
		return false
	}
	for _, tag := range objectTags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := allowedSet[strings.ToLower(tag)]; ok {
			return true
		}
	}
	return false
}

func scopeConfigStrings(cfg map[string]any, keys ...string) []string {
	if len(cfg) == 0 {
		return nil
	}
	for _, key := range keys {
		if raw, ok := cfg[key]; ok {
			return normalizeStringList(raw)
		}
	}
	return nil
}

func normalizeStringList(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return trimStringSlice(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return trimStringSlice(out)
	case string:
		if s := strings.TrimSpace(v); s != "" {
			return []string{s}
		}
	}
	return nil
}

func trimStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func dedupePermissions(perms []domain.Permission) []domain.Permission {
	seen := map[string]struct{}{}
	out := make([]domain.Permission, 0, len(perms))
	for _, p := range perms {
		if _, ok := seen[p.Code]; ok {
			continue
		}
		seen[p.Code] = struct{}{}
		out = append(out, p)
	}
	return out
}

func dedupeDataScopes(scopes []domain.DataScope) []domain.DataScope {
	seen := map[string]struct{}{}
	out := make([]domain.DataScope, 0, len(scopes))
	for _, sc := range scopes {
		if _, ok := seen[sc.Code]; ok {
			continue
		}
		seen[sc.Code] = struct{}{}
		out = append(out, sc)
	}
	return out
}

func dedupeAIToolPermissions(tools []domain.AIToolPermission) []domain.AIToolPermission {
	seen := map[string]struct{}{}
	out := make([]domain.AIToolPermission, 0, len(tools))
	for _, tp := range tools {
		if _, ok := seen[tp.ToolCode]; ok {
			continue
		}
		seen[tp.ToolCode] = struct{}{}
		out = append(out, tp)
	}
	return out
}

// matchTool 判断用户是否具备指定 tool_code 的调用权限。
//
// 语义：
//   - deny：直接拒绝；
//   - read_only：无需 UserConfirmed；
//   - require_confirm：默认要求 UserConfirmed=true；若 PermitsUnconfirmedInvoke=true，
//     则允许高权限角色在未确认时直接调用。
func (s *AuthorizationService) matchTool(code string, tools []domain.AIToolPermission, userConfirmed bool) (domain.AIToolPermissionMode, bool) {
	for _, tp := range tools {
		if !strings.EqualFold(tp.ToolCode, code) {
			continue
		}
		if tp.PermissionMode == domain.AIToolPermissionDeny {
			return tp.PermissionMode, false
		}
		if tp.PermissionMode == domain.AIToolPermissionRequireConfirm && !userConfirmed && !tp.PermitsUnconfirmedInvoke {
			return tp.PermissionMode, false
		}
		return tp.PermissionMode, true
	}
	return "", false
}
