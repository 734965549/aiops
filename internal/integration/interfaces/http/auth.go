package http

import (
	integapp "github.com/734965549/aiops/internal/integration/application"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

const integrationAuthResource = "integrations"

func actorFromContext(c *gin.Context) integapp.Actor {
	return integapp.Actor{
		UserID:      c.GetString(middleware.CtxKeyUserID),
		DisplayName: c.GetString(middleware.CtxKeyUsername),
	}
}
