package http

import (
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

func authorizePermission(authz authorizationMiddlewareAdapter, permission string) gin.HandlerFunc {
	return middleware.AuthorizeRequired(func(c *gin.Context) (middleware.AuthorizationInput, bool) {
		return middleware.AuthorizationInput{
			UserID:             c.GetString(middleware.CtxKeyUserID),
			RequiredPermission: permission,
			SkipDataScope:      true,
		}, true
	}, authz)
}
