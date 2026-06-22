package http

import (
	"context"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
)

// Registrar 注册 Integration 路由到 /api/integrations。
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
		UserID: in.UserID, Resource: in.Resource, Action: in.Action,
		ObjectOwner: in.ObjectOwner, ObjectDept: in.ObjectDept, ObjectTeam: in.ObjectTeam,
		ObjectRegion: in.ObjectRegion, ObjectTags: in.ObjectTags, ToolCode: in.ToolCode,
		UserConfirmed: in.UserConfirmed, RequiredPermission: in.RequiredPermission, SkipDataScope: in.SkipDataScope,
	})
	if res == nil || err != nil {
		return nil, err
	}
	return &middleware.AuthorizationResult{
		Allowed: res.Allowed, Reason: res.Reason, MatchedRoles: res.MatchedRoleNames,
		MatchedPerms: res.MatchedPermissions, MatchedScopes: res.MatchedScopes, ToolMode: res.ToolMode,
	}, nil
}

func NewRegistrar(handler *Handler, authorizer routeAuthorizer) *Registrar {
	return &Registrar{handler: handler, authorizer: authorizer}
}

func (r *Registrar) RegisterRoutes(groups server.RouteGroups) {
	authed := groups.Authed.Group("/integrations")
	if r.authorizer == nil {
		return
	}
	authz := authorizationMiddlewareAdapter{authorizer: r.authorizer}
	authed.GET("/accounts", middleware.AuthorizeStatic(authz, integrationAuthResource, "read"), r.handler.ListAccounts)
	authed.GET("/accounts/:account_id", middleware.AuthorizeStatic(authz, integrationAuthResource, "read"), r.handler.GetAccount)
	authed.POST("/accounts", middleware.AuthorizeStatic(authz, integrationAuthResource, "create"), r.handler.CreateAccount)
	authed.PUT("/accounts/:account_id", middleware.AuthorizeStatic(authz, integrationAuthResource, "update"), r.handler.UpdateAccount)
	authed.DELETE("/accounts/:account_id", middleware.AuthorizeStatic(authz, integrationAuthResource, "delete"), r.handler.DeleteAccount)
	authed.POST("/accounts/:account_id/check", middleware.AuthorizeStatic(authz, integrationAuthResource, "check"), r.handler.CheckConnectivity)
}
