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
	handler          *Handler
	chatHandler      *ChatHandler
	knowledgeHandler *KnowledgeHandler
	memoryHandler    *MemoryHandler
	authorizer       routeAuthorizer
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

func NewRegistrar(handler *Handler, chatHandler *ChatHandler, knowledgeHandler *KnowledgeHandler, memoryHandler *MemoryHandler, authorizer routeAuthorizer) *Registrar {
	return &Registrar{
		handler: handler, chatHandler: chatHandler, knowledgeHandler: knowledgeHandler,
		memoryHandler: memoryHandler, authorizer: authorizer,
	}
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
			// 流式对话（SSE，事件级）：与同步接口同一编排，客户端断开即取消。
			authed.POST("/chat/sessions/:session_id/messages/stream", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.SendMessageStream)
			authed.POST("/chat/sessions/:session_id/messages/:message_id/feedback",
				middleware.AuthorizeStatic(authz, "ai", "chat:feedback"), r.chatHandler.SubmitFeedback)
			// 归档/恢复/删除复用 app:ai:chat：会话是用户私有资源，归属校验由 application 层负责，
			// 不引入新权限码以免与记忆路线图的迁移编号冲突（ops/ai-contract.md §10.13）。
			authed.POST("/chat/sessions/:session_id/archive", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.ArchiveSession)
			authed.POST("/chat/sessions/:session_id/unarchive", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.UnarchiveSession)
			authed.DELETE("/chat/sessions/:session_id", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.DeleteSession)
			// 会话只读分享：只有 owner 能创建/查看/撤销；访问分享仍需登录 + app:ai:chat（契约 §10.18）。
			authed.POST("/chat/sessions/:session_id/shares", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.CreateShare)
			authed.GET("/chat/sessions/:session_id/shares", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.ListShares)
			authed.DELETE("/chat/sessions/:session_id/shares/:share_id", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.RevokeShare)
			authed.GET("/chat/shares/:token", middleware.AuthorizeStatic(authz, "ai", "chat"), r.chatHandler.ViewSharedSession)
		}
		// 知识沉淀管理（阶段 2）：draft 必须审核激活后才注入回答（ops/ai-contract.md §10.15）。
		if r.knowledgeHandler != nil {
			authed.GET("/knowledge/items", middleware.AuthorizeStatic(authz, "ai", "knowledge:manage"), r.knowledgeHandler.List)
			authed.POST("/knowledge/items", middleware.AuthorizeStatic(authz, "ai", "knowledge:manage"), r.knowledgeHandler.Create)
			authed.PUT("/knowledge/items/:item_id", middleware.AuthorizeStatic(authz, "ai", "knowledge:manage"), r.knowledgeHandler.Update)
			authed.POST("/knowledge/items/:item_id/review", middleware.AuthorizeStatic(authz, "ai", "knowledge:manage"), r.knowledgeHandler.Review)
			authed.DELETE("/knowledge/items/:item_id", middleware.AuthorizeStatic(authz, "ai", "knowledge:manage"), r.knowledgeHandler.Delete)
			// 反馈聚合：把某应用下已确认有效的案例聚合成 draft（幂等，不覆盖 active 条目）。
			authed.POST("/knowledge/aggregate", middleware.AuthorizeStatic(authz, "ai", "knowledge:manage"), r.knowledgeHandler.Aggregate)
		}
		// 记忆管理：只读检索 + 手动停用/启用；回答注入仍严格按归属用户过滤（契约 §10.14/§10.19）。
		if r.memoryHandler != nil {
			authed.GET("/memory/cases", middleware.AuthorizeStatic(authz, "ai", "memory:manage"), r.memoryHandler.List)
			authed.POST("/memory/cases/:case_id/status", middleware.AuthorizeStatic(authz, "ai", "memory:manage"), r.memoryHandler.UpdateStatus)
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
		authed.POST("/chat/sessions/:session_id/messages/stream", r.chatHandler.SendMessageStream)
		authed.POST("/chat/sessions/:session_id/messages/:message_id/feedback", r.chatHandler.SubmitFeedback)
		authed.POST("/chat/sessions/:session_id/archive", r.chatHandler.ArchiveSession)
		authed.POST("/chat/sessions/:session_id/unarchive", r.chatHandler.UnarchiveSession)
		authed.DELETE("/chat/sessions/:session_id", r.chatHandler.DeleteSession)
		authed.POST("/chat/sessions/:session_id/shares", r.chatHandler.CreateShare)
		authed.GET("/chat/sessions/:session_id/shares", r.chatHandler.ListShares)
		authed.DELETE("/chat/sessions/:session_id/shares/:share_id", r.chatHandler.RevokeShare)
		authed.GET("/chat/shares/:token", r.chatHandler.ViewSharedSession)
	}
	// 无 authorizer 时知识管理路由不注册：审核是管理员动作，不能在没有 RBAC 的环境放开。
}
