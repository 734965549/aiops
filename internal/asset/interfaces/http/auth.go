package http

import (
	assetapp "github.com/734965549/aiops/internal/asset/application"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

func actorFromContext(c *gin.Context) assetapp.Actor {
	return assetapp.Actor{
		UserID:      c.GetString(middleware.CtxKeyUserID),
		DisplayName: c.GetString(middleware.CtxKeyUserID),
	}
}
