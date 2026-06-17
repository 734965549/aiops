package middleware

import (
	"strings"

	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

const (
	// CtxKeyUserID 当前请求用户 ID。
	CtxKeyUserID = "user_id"
	// CtxKeyUsername 当前请求用户名。
	CtxKeyUsername = "username"
	// CtxKeyRoles 当前请求用户角色列表。
	CtxKeyRoles = "roles"
)

// Authenticator 描述 token 解析与用户加载的能力，由 Identity 模块在第二步实现注入。
//
// 第一阶段保持接口稳定，避免后续替换鉴权方案（账号密码 / SSO / OIDC）时影响其它中间件。
type Authenticator interface {
	// Authenticate 解析请求中的凭证，返回身份信息或错误。
	Authenticate(token string) (Identity, error)
}

// Identity 是当前请求所代表的用户身份摘要。
type Identity struct {
	UserID   string
	Username string
	Roles    []string
}

// AuthRequired 强制鉴权；未提供 Authenticator 时直接返回 503，提示尚未接入。
func AuthRequired(a Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if a == nil {
			httpx.FailWith(c, apperr.CodeUnavailable, "authenticator is not configured")
			c.Abort()
			return
		}

		token := extractBearerToken(c)
		if token == "" {
			httpx.FailWith(c, apperr.CodeUnauthenticated, "missing access token")
			c.Abort()
			return
		}

		id, err := a.Authenticate(token)
		if err != nil {
			httpx.FailWith(c, apperr.CodeUnauthenticated, "invalid or expired token")
			c.Abort()
			return
		}

		c.Set(CtxKeyUserID, id.UserID)
		c.Set(CtxKeyUsername, id.Username)
		c.Set(CtxKeyRoles, id.Roles)
		c.Next()
	}
}

// AuthOptional 解析存在的 token，但不强制要求登录，便于公共接口共享中间件链。
func AuthOptional(a Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if a == nil {
			c.Next()
			return
		}
		token := extractBearerToken(c)
		if token == "" {
			c.Next()
			return
		}
		if id, err := a.Authenticate(token); err == nil {
			c.Set(CtxKeyUserID, id.UserID)
			c.Set(CtxKeyUsername, id.Username)
			c.Set(CtxKeyRoles, id.Roles)
		}
		c.Next()
	}
}

func extractBearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
