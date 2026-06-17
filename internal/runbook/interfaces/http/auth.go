package http

import (
	rbapp "github.com/734965549/aiops/internal/runbook/application"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

const runbookAuthResource = "runbooks"

func actorFromContext(c *gin.Context) rbapp.Actor {
	return rbapp.Actor{
		UserID:      c.GetString(middleware.CtxKeyUserID),
		DisplayName: c.GetString(middleware.CtxKeyUserID),
	}
}
