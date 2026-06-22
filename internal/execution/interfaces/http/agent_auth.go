package http

import (
	"strings"

	execapp "github.com/734965549/aiops/internal/execution/application"
	"github.com/734965549/aiops/internal/execution/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

func AgentAuth(agents *execapp.AgentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if agents == nil {
			httpx.FailWith(c, apperr.CodeUnavailable, "agent service is not enabled")
			c.Abort()
			return
		}
		token := extractBearerToken(c.GetHeader("Authorization"))
		if token == "" {
			token = strings.TrimSpace(c.GetHeader("X-Agent-Token"))
		}
		agent, err := agents.AuthenticateByToken(c.Request.Context(), token)
		if err != nil {
			httpx.Fail(c, err)
			c.Abort()
			return
		}
		c.Set(ctxKeyAgentID, agent)
		c.Next()
	}
}

func extractBearerToken(header string) string {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func agentFromContext(c *gin.Context) (*domain.ExecutionAgent, bool) {
	raw, ok := c.Get(ctxKeyAgentID)
	if !ok {
		httpx.FailWith(c, apperr.CodeUnauthenticated, "missing agent identity")
		return nil, false
	}
	agent, ok := raw.(*domain.ExecutionAgent)
	if !ok || agent == nil {
		httpx.FailWith(c, apperr.CodeUnauthenticated, "invalid agent identity")
		return nil, false
	}
	if paramID := strings.TrimSpace(c.Param("agent_id")); paramID != "" && paramID != agent.AgentID {
		httpx.FailWith(c, apperr.CodePermissionDenied, "agent id mismatch")
		return nil, false
	}
	return agent, true
}
