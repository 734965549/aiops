package http

import (
	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

const observabilityAuthResource = "observability"

func actorFromContext(c *gin.Context) obsapp.Actor {
	return obsapp.Actor{
		UserID:      c.GetString(middleware.CtxKeyUserID),
		DisplayName: c.GetString(middleware.CtxKeyUsername),
	}
}
