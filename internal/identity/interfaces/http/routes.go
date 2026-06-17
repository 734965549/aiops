package http

import (
	"context"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
)

// Registrar 实现 internal/server.RouteRegistrar，把 Identity 模块路由挂到 /api 下。
type Registrar struct {
	handler    *Handler
	authorizer routeAuthorizer
}

type routeAuthorizer interface {
	Authorize(ctx context.Context, input identityapp.AuthorizationInput) (*identityapp.AuthorizationResult, error)
}

type authorizationMiddlewareAdapter struct{ authorizer routeAuthorizer }

func (a authorizationMiddlewareAdapter) Authorize(ctx context.Context, in middleware.AuthorizationInput) (*middleware.AuthorizationResult, error) {
	res, err := a.authorizer.Authorize(ctx, identityapp.AuthorizationInput{
		UserID:             in.UserID,
		Resource:           in.Resource,
		Action:             in.Action,
		ObjectOwner:        in.ObjectOwner,
		ObjectDept:         in.ObjectDept,
		ObjectTeam:         in.ObjectTeam,
		ObjectRegion:       in.ObjectRegion,
		ObjectTags:         in.ObjectTags,
		ToolCode:           in.ToolCode,
		UserConfirmed:      in.UserConfirmed,
		RequiredPermission: in.RequiredPermission,
		SkipDataScope:      in.SkipDataScope,
	})
	if res == nil || err != nil {
		return nil, err
	}
	return &middleware.AuthorizationResult{
		Allowed:       res.Allowed,
		Reason:        res.Reason,
		MatchedRoles:  res.MatchedRoleNames,
		MatchedPerms:  res.MatchedPermissions,
		MatchedScopes: res.MatchedScopes,
		ToolMode:      res.ToolMode,
	}, nil
}

// NewRegistrar 构造路由注册器。
func NewRegistrar(handler *Handler, authorizer routeAuthorizer) *Registrar {
	return &Registrar{handler: handler, authorizer: authorizer}
}

// RegisterRoutes 满足 internal/server.RouteRegistrar 接口。
//
// 路由划分：
//   - 公共：POST /api/identity/login、POST /api/identity/refresh；
//   - 鉴权：GET /api/identity/roles、GET /api/identity/permissions、GET /api/identity/me、GET /api/identity/me/roles。
//   - 统一授权显式校验：POST /api/identity/authorize。
func (r *Registrar) RegisterRoutes(groups server.RouteGroups) {
	public := groups.Public.Group("/identity")
	public.POST("/login", r.handler.Login)
	public.POST("/login/external", r.handler.LoginExternal)
	public.GET("/login/providers", r.handler.ListLoginProviders)
	public.GET("/oauth/:provider_id/authorize", r.handler.OAuthAuthorize)
	public.GET("/oauth/:provider_id/callback", r.handler.OAuthCallback)
	public.POST("/oauth/:provider_id/callback", r.handler.OAuthCallback)
	public.POST("/refresh", r.handler.Refresh)
	public.POST("/logout", r.handler.Logout)

	authed := groups.Authed.Group("/identity")
	if r.authorizer != nil {
		authz := authorizationMiddlewareAdapter{authorizer: r.authorizer}
		authed.GET("/roles", middleware.AuthorizeStatic(authz, "identity.roles", "read"), r.handler.Roles)
		authed.GET("/permissions", middleware.AuthorizeStatic(authz, "identity.permissions", "read"), r.handler.Permissions)
		authed.GET("/data-scopes", middleware.AuthorizeStatic(authz, "identity.data_scopes", "read"), r.handler.DataScopes)
		authed.GET("/ai-tool-permissions", middleware.AuthorizeStatic(authz, "identity.ai_tool_permissions", "read"), r.handler.AIToolPermissions)
		authed.GET("/me", middleware.AuthorizeStatic(authz, "identity.profile", "read"), r.handler.GetCurrentUser)
		authed.GET("/me/roles", middleware.AuthorizeStatic(authz, "identity.profile.roles", "read"), r.handler.MeRoles)
		authed.POST("/authorize", middleware.AuthorizeStatic(authz, "identity.authorization", "execute"), r.handler.Authorize)
		authed.GET("/admin/auth-audits", middleware.AuthorizeStatic(authz, "identity.auth_audits", "read"), r.handler.AuthAudits)
		authed.GET("/admin/users", middleware.AuthorizeStatic(authz, "identity.users", "read"), r.handler.AdminUsers)
		authed.POST("/admin/users", middleware.AuthorizeStatic(authz, "identity.users", "create"), r.handler.AdminCreateLocalUser)
		authed.GET("/admin/users/:user_id/roles", middleware.AuthorizeStatic(authz, "identity.users", "read"), r.handler.AdminUserRoles)
		authed.PUT("/admin/users/:user_id/roles", middleware.AuthorizeStatic(authz, "identity.access_control", "write"), r.handler.AdminReplaceUserRoles)
		authed.GET("/admin/roles/:role_id/permissions", middleware.AuthorizeStatic(authz, "identity.roles", "read"), r.handler.AdminRolePermissions)
		authed.PUT("/admin/roles/:role_id/permissions", middleware.AuthorizeStatic(authz, "identity.access_control", "write"), r.handler.AdminReplaceRolePermissions)
		authed.GET("/admin/roles/:role_id/data-scopes", middleware.AuthorizeStatic(authz, "identity.data_scopes", "read"), r.handler.AdminRoleDataScopes)
		authed.PUT("/admin/roles/:role_id/data-scopes", middleware.AuthorizeStatic(authz, "identity.access_control", "write"), r.handler.AdminReplaceRoleDataScopes)
		authed.GET("/admin/roles/:role_id/ai-tool-permissions", middleware.AuthorizeStatic(authz, "identity.ai_tool_permissions", "read"), r.handler.AdminRoleAIToolPermissions)
		authed.PUT("/admin/roles/:role_id/ai-tool-permissions", middleware.AuthorizeStatic(authz, "identity.access_control", "write"), r.handler.AdminReplaceRoleAIToolPermissions)
		authed.POST("/admin/external-identities", middleware.AuthorizeStatic(authz, "identity.external_identities", "create"), r.handler.AdminProvisionExternalIdentity)
		authed.POST("/admin/users/:user_id/external-identities", middleware.AuthorizeStatic(authz, "identity.external_identities", "create"), r.handler.AdminBindExternalIdentity)
		authed.POST("/admin/ldap/connect", middleware.AuthorizeStatic(authz, "identity.external_identities", "create"), r.handler.AdminConnectLDAPSession)
		authed.DELETE("/admin/ldap/sessions/:session_id", middleware.AuthorizeStatic(authz, "identity.external_identities", "create"), r.handler.AdminCloseLDAPSession)
		authed.GET("/admin/ldap/sessions/:session_id/organizations", middleware.AuthorizeStatic(authz, "identity.external_identities", "create"), r.handler.AdminBrowseLDAPSessionOrganizations)
		authed.GET("/admin/ldap/sessions/:session_id/users", middleware.AuthorizeStatic(authz, "identity.external_identities", "create"), r.handler.AdminPreviewLDAPSessionUsers)
		authed.POST("/admin/ldap/sessions/:session_id/import", middleware.AuthorizeStatic(authz, "identity.external_identities", "create"), r.handler.AdminImportLDAPSessionUsers)
		authed.GET("/admin/ldap/:provider_id/connection-test", middleware.AuthorizeStatic(authz, "identity.external_identities", "create"), r.handler.AdminTestLDAPConnection)
		authed.GET("/admin/ldap/:provider_id/organizations", middleware.AuthorizeStatic(authz, "identity.external_identities", "create"), r.handler.AdminBrowseLDAPOrganizations)
		authed.GET("/admin/ldap/:provider_id/users", middleware.AuthorizeStatic(authz, "identity.external_identities", "create"), r.handler.AdminPreviewLDAPUsers)
		authed.POST("/admin/ldap/:provider_id/import", middleware.AuthorizeStatic(authz, "identity.external_identities", "create"), r.handler.AdminImportLDAPUsers)
		return
	}
	authed.GET("/roles", r.handler.Roles)
	authed.GET("/permissions", r.handler.Permissions)
	authed.GET("/data-scopes", r.handler.DataScopes)
	authed.GET("/ai-tool-permissions", r.handler.AIToolPermissions)
	authed.GET("/me", r.handler.GetCurrentUser)
	authed.GET("/me/roles", r.handler.MeRoles)
	authed.POST("/authorize", r.handler.Authorize)
	authed.GET("/admin/users", r.handler.AdminUsers)
	authed.GET("/admin/users/:user_id/roles", r.handler.AdminUserRoles)
	authed.PUT("/admin/users/:user_id/roles", r.handler.AdminReplaceUserRoles)
	authed.GET("/admin/roles/:role_id/permissions", r.handler.AdminRolePermissions)
	authed.PUT("/admin/roles/:role_id/permissions", r.handler.AdminReplaceRolePermissions)
	authed.GET("/admin/roles/:role_id/data-scopes", r.handler.AdminRoleDataScopes)
	authed.PUT("/admin/roles/:role_id/data-scopes", r.handler.AdminReplaceRoleDataScopes)
	authed.GET("/admin/roles/:role_id/ai-tool-permissions", r.handler.AdminRoleAIToolPermissions)
	authed.PUT("/admin/roles/:role_id/ai-tool-permissions", r.handler.AdminReplaceRoleAIToolPermissions)
}
