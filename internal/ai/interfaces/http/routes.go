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
	handler     *Handler
	chatHandler *ChatHandler
	authorizer  routeAuthorizer
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

func NewRegistrar(handler *Handler, chatHandler *ChatHandler, authorizer routeAuthorizer) *Registrar {
	return &Registrar{handler: handler, chatHandler: chatHandler, authorizer: authorizer}
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
		if r.chatHandler != nil {
			authed.POST("/chat/sessions", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.CreateSession)
			authed.GET("/chat/sessions", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.ListSessions)
			authed.GET("/chat/sessions/:session_id", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.GetSession)
			authed.POST("/chat/sessions/:session_id/messages", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.SendMessage)
			authed.POST("/chat/sessions/:session_id/messages/:message_id/feedback",
				middleware.AuthorizeStatic(authz, "ai", "chat:feedback"), r.chatHandler.SubmitFeedback)
		}
		return
	}
	authed.GET("/providers", r.handler.ListProviders)
	authed.POST("/providers", r.handler.UpsertProvider)
	authed.DELETE("/providers/:id", r.handler.DeleteProvider)
	authed.POST("/tools/invoke", r.handler.Invoke)
	if r.chatHandler != nil {
		authed.POST("/chat/sessions", r.chatHandler.CreateSession)
		authed.GET("/chat/sessions", r.chatHandler.ListSessions)
		authed.GET("/chat/sessions/:session_id", r.chatHandler.GetSession)
		authed.POST("/chat/sessions/:session_id/messages", r.chatHandler.SendMessage)
		authed.POST("/chat/sessions/:session_id/messages/:message_id/feedback", r.chatHandler.SubmitFeedback)
	}
}
