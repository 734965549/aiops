package http

import (
	"context"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
)

const dashboardAuthResource = "dashboard"

// Registrar 注册 Dashboard 路由。
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

// NewRegistrar 构造路由注册器。
func NewRegistrar(handler *Handler, authorizer routeAuthorizer) *Registrar {
	return &Registrar{handler: handler, authorizer: authorizer}
}

func (r *Registrar) RegisterRoutes(groups server.RouteGroups) {
	authed := groups.Authed.Group("/dashboard")
	if r.authorizer == nil {
		return
	}
	authz := authorizationMiddlewareAdapter{authorizer: r.authorizer}
	authed.GET("/summary", middleware.AuthorizeStatic(authz, dashboardAuthResource, "read"), r.handler.GetSummary)
}
