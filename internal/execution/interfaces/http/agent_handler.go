package http

import (
	execapp "github.com/734965549/aiops/internal/execution/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
	"strings"
)

const ctxKeyAgentID = "execution_agent_id"

// AgentHandler 执行代理 HTTP 层。
type AgentHandler struct {
	agents   *execapp.AgentService
	dispatch *execapp.DispatchService
	register func() string
}

func NewAgentHandler(agents *execapp.AgentService, dispatch *execapp.DispatchService, registerToken string) *AgentHandler {
	return &AgentHandler{agents: agents, dispatch: dispatch, register: func() string { return registerToken }}
}

type registerAgentRequest struct {
	AgentID      string   `json:"agent_id"`
	MediumID     string   `json:"medium_id" binding:"required"`
	PublicKey    string   `json:"public_key"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

type heartbeatRequest struct {
	Status       string `json:"status"`
	RunningTasks int    `json:"running_tasks"`
	FreeSlots    int    `json:"free_slots"`
	Version      string `json:"version"`
	ObservedAt   int64  `json:"observed_at"`
}

type appendLogRequest struct {
	LeaseID    string `json:"lease_id" binding:"required"`
	StepID     string `json:"step_id" binding:"required"`
	Stream     string `json:"stream" binding:"required"`
	Sequence   int    `json:"sequence"`
	Content    string `json:"content"`
	Truncated  bool   `json:"truncated"`
	ObservedAt int64  `json:"observed_at"`
}

type reportResultRequest struct {
	LeaseID       string `json:"lease_id" binding:"required"`
	StepID        string `json:"step_id" binding:"required"`
	Status        string `json:"status" binding:"required"`
	ExitCode      int    `json:"exit_code"`
	ResultSummary string `json:"result_summary"`
	StartedAt     int64  `json:"started_at"`
	FinishedAt    int64  `json:"finished_at"`
}

func (h *AgentHandler) Register(c *gin.Context) {
	if h.agents == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "agent service is not enabled")
		return
	}
	token := strings.TrimSpace(c.GetHeader("X-Register-Token"))
	if token == "" {
		parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token = strings.TrimSpace(parts[1])
		}
	}
	if h.register != nil && token != h.register() {
		httpx.FailWith(c, apperr.CodePermissionDenied, "invalid register token")
		return
	}
	var req registerAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid register agent request")
		return
	}
	out, err := h.agents.Register(c.Request.Context(), execapp.RegisterAgentInput{
		AgentID: req.AgentID, MediumID: req.MediumID, PublicKey: req.PublicKey,
		Version: req.Version, Capabilities: req.Capabilities,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *AgentHandler) Heartbeat(c *gin.Context) {
	agent, ok := agentFromContext(c)
	if !ok {
		return
	}
	var req heartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid heartbeat request")
		return
	}
	out, err := h.agents.Heartbeat(c.Request.Context(), agent.AgentID, execapp.HeartbeatInput{
		Status: req.Status, RunningTasks: req.RunningTasks, FreeSlots: req.FreeSlots,
		Version: req.Version, ObservedAt: req.ObservedAt,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *AgentHandler) Lease(c *gin.Context) {
	agent, ok := agentFromContext(c)
	if !ok {
		return
	}
	if h.dispatch == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "dispatch service is not enabled")
		return
	}
	out, err := h.dispatch.Lease(c.Request.Context(), agent)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *AgentHandler) AppendLog(c *gin.Context) {
	agent, ok := agentFromContext(c)
	if !ok {
		return
	}
	if h.dispatch == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "dispatch service is not enabled")
		return
	}
	var req appendLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid log request")
		return
	}
	if err := h.dispatch.AppendLog(c.Request.Context(), agent, c.Param("task_id"), execapp.AppendLogInput{
		LeaseID: req.LeaseID, StepID: req.StepID, Stream: req.Stream, Sequence: req.Sequence,
		Content: req.Content, Truncated: req.Truncated, ObservedAt: req.ObservedAt,
	}); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"accepted": true})
}

func (h *AgentHandler) ReportResult(c *gin.Context) {
	agent, ok := agentFromContext(c)
	if !ok {
		return
	}
	if h.dispatch == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "dispatch service is not enabled")
		return
	}
	var req reportResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid result request")
		return
	}
	out, err := h.dispatch.ReportResult(c.Request.Context(), agent, c.Param("task_id"), execapp.ReportResultInput{
		LeaseID: req.LeaseID, StepID: req.StepID, Status: req.Status, ExitCode: req.ExitCode,
		ResultSummary: req.ResultSummary, StartedAt: req.StartedAt, FinishedAt: req.FinishedAt,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
