package http

import (
	"context"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
)

const assetAuthResource = "assets"

// Registrar 注册 Asset 路由到 /api/assets。
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
	authed := groups.Authed.Group("/assets")
	if r.authorizer == nil {
		return
	}
	authz := authorizationMiddlewareAdapter{authorizer: r.authorizer}
	authed.GET("/applications", middleware.AuthorizeStatic(authz, assetAuthResource, "read"), r.handler.ListApplications)
	authed.POST("/applications", middleware.AuthorizeStatic(authz, assetAuthResource, "write"), r.handler.CreateApplication)
	authed.PUT("/applications/:id", middleware.AuthorizeStatic(authz, assetAuthResource, "write"), r.handler.UpdateApplication)
	authed.DELETE("/applications/:id", middleware.AuthorizeStatic(authz, assetAuthResource, "write"), r.handler.DeleteApplication)
	authed.GET("/applications/:application_id/resources", middleware.AuthorizeStatic(authz, assetAuthResource, "read"), r.handler.ListResources)
	authed.POST("/resources", middleware.AuthorizeStatic(authz, assetAuthResource, "write"), r.handler.CreateResource)
	authed.PUT("/resources/:id", middleware.AuthorizeStatic(authz, assetAuthResource, "write"), r.handler.UpdateResource)
	authed.DELETE("/resources/:id", middleware.AuthorizeStatic(authz, assetAuthResource, "write"), r.handler.DeleteResource)
	authed.POST("/sync", middleware.AuthorizeStatic(authz, assetAuthResource, "write"), r.handler.TriggerSync)
	authed.GET("/sync/batches", middleware.AuthorizeStatic(authz, assetAuthResource, "read"), r.handler.ListSyncBatches)
	authed.GET("/sync/batches/:batch_id", middleware.AuthorizeStatic(authz, assetAuthResource, "read"), r.handler.GetSyncBatch)
	authed.GET("/match-rules", middleware.AuthorizeStatic(authz, assetAuthResource, "read"), r.handler.ListMatchRules)
	authed.POST("/match-rules", middleware.AuthorizeStatic(authz, assetAuthResource, "write"), r.handler.CreateMatchRule)
	authed.PUT("/match-rules/:id", middleware.AuthorizeStatic(authz, assetAuthResource, "write"), r.handler.UpdateMatchRule)
	authed.DELETE("/match-rules/:id", middleware.AuthorizeStatic(authz, assetAuthResource, "write"), r.handler.DeleteMatchRule)
}
