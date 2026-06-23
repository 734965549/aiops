package http

import (
	inspectionapp "github.com/734965549/aiops/internal/inspection/application"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

const inspectionAuthResource = "inspections"

func actorFromContext(c *gin.Context) inspectionapp.Actor {
	return inspectionapp.Actor{
		UserID:      c.GetString(middleware.CtxKeyUserID),
		DisplayName: c.GetString(middleware.CtxKeyUsername),
	}
}
