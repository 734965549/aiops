package middleware

import (
	"context"
	"strings"

	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

const (
	// CtxKeyAuthDecision 保存一次统一授权结果。
	CtxKeyAuthDecision = "auth_decision"
)

// AuthorizationInput 描述统一授权输入，避免中间件层反向依赖具体业务应用层。
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
	// UserConfirmed 表示调用方已确认本次高风险工具执行。
	UserConfirmed      bool
	RequiredPermission string
	// SkipDataScope 为 true 时仅校验 RBAC/工具权限，不校验数据范围（适用于无资源实例的静态路由）。
	SkipDataScope bool
}

// AuthorizationResult 描述统一授权结果。
type AuthorizationResult struct {
	Allowed       bool
	Reason        string
	MatchedRoles  []string
	MatchedPerms  []string
	MatchedScopes []string
	ToolMode      string
}

// AuthorizationService 描述统一授权能力。
type AuthorizationService interface {
	Authorize(ctx context.Context, in AuthorizationInput) (*AuthorizationResult, error)
}

// AuthorizeStatic 适合静态路由权限，例如只要求固定资源/动作的接口。
func AuthorizeStatic(svc AuthorizationService, resource, action string) gin.HandlerFunc {
	return AuthorizeRequired(func(c *gin.Context) (AuthorizationInput, bool) {
		return AuthorizationInput{
			UserID:        c.GetString(CtxKeyUserID),
			Resource:      resource,
			Action:        action,
			SkipDataScope: true,
		}, true
	}, svc)
}

// AuthorizeDynamic 适合需要从请求参数中动态解析数据权限/工具权限的接口。
func AuthorizeDynamic(svc AuthorizationService, resolver func(*gin.Context) (AuthorizationInput, bool)) gin.HandlerFunc {
	return AuthorizeRequired(resolver, svc)
}

// AuthorizeRequired 在进入 handler 前统一执行 RBAC / 数据权限 / 操作权限校验。
func AuthorizeRequired(resolver func(*gin.Context) (AuthorizationInput, bool), svc AuthorizationService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil {
			httpx.FailWith(c, apperr.CodeUnavailable, "authorization service is not configured")
			c.Abort()
			return
		}
		in, ok := resolver(c)
		if !ok {
			httpx.FailWith(c, apperr.CodeInvalidArgument, "missing authorization context")
			c.Abort()
			return
		}
		in.UserID = strings.TrimSpace(in.UserID)
		res, err := svc.Authorize(c.Request.Context(), in)
		if err != nil {
			httpx.Fail(c, err)
			c.Abort()
			return
		}
		if res != nil && !res.Allowed {
			httpx.FailWith(c, apperr.CodePermissionDenied, res.Reason)
			c.Abort()
			return
		}
		c.Set(CtxKeyAuthDecision, res)
		c.Next()
	}
}
