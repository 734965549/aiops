package http

import (
	execapp "github.com/734965549/aiops/internal/execution/application"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

const executionAuthResource = "executions"

func actorFromContext(c *gin.Context) execapp.Actor {
	return execapp.Actor{
		UserID:      c.GetString(middleware.CtxKeyUserID),
		DisplayName: c.GetString(middleware.CtxKeyUserID),
	}
}
