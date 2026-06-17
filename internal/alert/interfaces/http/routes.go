package http

import (
	"context"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
)

// Registrar 实现 server.RouteRegistrar，将 Alert 路由挂到 /api/alerts 下。
//
// 鉴权（ops/alert-contract.md §3）：
//   - §3.1 管理/查询：Authed 组 + Bearer + AuthorizeStatic(alerts, action)，见 auth.go 权限表
//   - §3.2 Webhook：Public 组，X-AIOPS-Webhook-Token，不经用户登录体系
//
// 路由范围对应 §1 第一阶段能力；所有 Handler 输出走 httpx.OK / httpx.Fail（§2）。
type Registrar struct {
	handler       *Handler
	ingestHandler *IngestHandler
	authorizer    routeAuthorizer
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
func NewRegistrar(handler *Handler, ingestHandler *IngestHandler, authorizer routeAuthorizer) *Registrar {
	return &Registrar{handler: handler, ingestHandler: ingestHandler, authorizer: authorizer}
}

// RegisterRoutes 满足 internal/server.RouteRegistrar 接口。
//
// Public 路由仅 Webhook ingest（§3.2 不用 Bearer）；Authed 路由要求 Bearer（§3.1）。
func (r *Registrar) RegisterRoutes(groups server.RouteGroups) {
	// §3.2：外部监控系统接入，共享密钥鉴权，不走 Authed/Bearer。
	public := groups.Public.Group("/alerts")
	public.POST("/ingest/alertmanager/:source_id", r.ingestHandler.IngestAlertmanager)
	public.POST("/ingest/webhook/:source_id", r.ingestHandler.IngestWebhook)

	// §3.1：除 Webhook 外全部 /api/alerts/** 要求 Bearer + app:alerts:{action}。
	authed := groups.Authed.Group("/alerts")
	if r.authorizer != nil {
		authz := authorizationMiddlewareAdapter{authorizer: r.authorizer}
		authed.GET("/sources", middleware.AuthorizeStatic(authz, alertAuthResource, "ingest"), r.handler.ListSources)
		authed.GET("/sources/:source_id", middleware.AuthorizeStatic(authz, alertAuthResource, "ingest"), r.handler.GetSource)
		authed.POST("/sources", middleware.AuthorizeStatic(authz, alertAuthResource, "ingest"), r.handler.CreateSource)
		authed.PUT("/sources/:source_id", middleware.AuthorizeStatic(authz, alertAuthResource, "ingest"), r.handler.UpdateSource)
		authed.DELETE("/sources/:source_id", middleware.AuthorizeStatic(authz, alertAuthResource, "ingest"), r.handler.DeleteSource)
		authed.GET("", middleware.AuthorizeStatic(authz, alertAuthResource, "read"), r.handler.ListAlerts)
		authed.GET("/:alert_id", middleware.AuthorizeStatic(authz, alertAuthResource, "read"), r.handler.GetAlert)
		authed.POST("/:alert_id/acknowledge", middleware.AuthorizeStatic(authz, alertAuthResource, "acknowledge"), r.handler.Acknowledge)
		authed.POST("/:alert_id/assign", middleware.AuthorizeStatic(authz, alertAuthResource, "assign"), r.handler.Assign)
		authed.POST("/:alert_id/start-processing", middleware.AuthorizeStatic(authz, alertAuthResource, "update"), r.handler.StartProcessing)
		authed.POST("/:alert_id/recover", middleware.AuthorizeStatic(authz, alertAuthResource, "update"), r.handler.Recover)
		authed.POST("/:alert_id/close", middleware.AuthorizeStatic(authz, alertAuthResource, "close"), r.handler.Close)
		authed.POST("/:alert_id/silence", middleware.AuthorizeStatic(authz, alertAuthResource, "silence"), r.handler.Silence)
		authed.POST("/:alert_id/unsilence", middleware.AuthorizeStatic(authz, alertAuthResource, "silence"), r.handler.Unsilence)
		authed.POST("/:alert_id/comments", middleware.AuthorizeStatic(authz, alertAuthResource, "update"), r.handler.Comment)
		authed.POST("/:alert_id/ai-analysis", middleware.AuthorizeStatic(authz, alertAuthResource, "update"), r.handler.RequestAIAnalysis)
		return
	}
	// 无 authorizer 时仅注册只读路由（仍须 Bearer）；完整 RBAC 由 cmd/api 注入 authorizationSvc。
	authed.GET("", r.handler.ListAlerts)
	authed.GET("/:alert_id", r.handler.GetAlert)
}
