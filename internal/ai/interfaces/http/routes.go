package http

import (
	"context"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// Registrar 将 AI 模块路由挂到 /api 下。
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

func NewRegistrar(handler *Handler, authorizer routeAuthorizer) *Registrar {
	return &Registrar{handler: handler, authorizer: authorizer}
}

func (r *Registrar) RegisterRoutes(groups server.RouteGroups) {
	authed := groups.Authed.Group("/ai")
	if r.authorizer != nil {
		authz := authorizationMiddlewareAdapter{authorizer: r.authorizer}
		authed.GET("/providers", middleware.AuthorizeStatic(authz, "ai.providers", "read"), r.handler.ListProviders)
		authed.POST("/providers", middleware.AuthorizeStatic(authz, "ai.providers", "write"), r.handler.UpsertProvider)
		authed.DELETE("/providers/:id", middleware.AuthorizeStatic(authz, "ai.providers", "delete"), r.handler.DeleteProvider)
		authed.POST("/tools/invoke", middleware.AuthorizeDynamic(authz, func(c *gin.Context) (middleware.AuthorizationInput, bool) {
			return middleware.AuthorizationInput{UserID: c.GetString(middleware.CtxKeyUserID), Resource: "ai.tools", Action: "invoke"}, true
		}), r.handler.Invoke)
		authed.POST("/analyze-alert", middleware.AuthorizeStatic(authz, "ai.analysis", "analyze"), r.handler.AnalyzeAlert)
		return
	}
	authed.GET("/providers", r.handler.ListProviders)
	authed.POST("/providers", r.handler.UpsertProvider)
	authed.DELETE("/providers/:id", r.handler.DeleteProvider)
	authed.POST("/tools/invoke", r.handler.Invoke)
}
