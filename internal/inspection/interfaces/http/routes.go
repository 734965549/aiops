package http

import (
	"context"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
)

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
	authed := groups.Authed.Group("/inspections")
	if r.authorizer == nil {
		return
	}
	authz := authorizationMiddlewareAdapter{authorizer: r.authorizer}
	read := middleware.AuthorizeStatic(authz, inspectionAuthResource, "read")
	write := middleware.AuthorizeStatic(authz, inspectionAuthResource, "write")

	authed.GET("/policies", read, r.handler.ListPolicies)
	authed.POST("/policies", write, r.handler.CreatePolicy)
	authed.GET("/policies/:policy_id", read, r.handler.GetPolicy)
	authed.PUT("/policies/:policy_id", write, r.handler.UpdatePolicy)
	authed.DELETE("/policies/:policy_id", write, r.handler.DeletePolicy)
	authed.POST("/policies/:policy_id/runs", write, r.handler.TriggerRun)

	authed.GET("/runs", read, r.handler.ListRuns)
	authed.GET("/runs/:run_id", read, r.handler.GetRun)
	authed.GET("/findings", read, r.handler.ListFindings)
	authed.POST("/recommendations/:recommendation_id/execution", middleware.AuthorizeStatic(authz, "executions", "create"), r.handler.CreateExecutionFromRecommendation)
}
