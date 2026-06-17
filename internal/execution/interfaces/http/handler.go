package http

import (
	execapp "github.com/734965549/aiops/internal/execution/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/pagination"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
	"strings"
)

// Handler Execution HTTP 层。
type Handler struct {
	tasks *execapp.TaskService
}

// NewHandler 构造 Handler。
func NewHandler(tasks *execapp.TaskService) *Handler {
	return &Handler{tasks: tasks}
}

type createTaskRequest struct {
	Name              string         `json:"name"`
	SourceType        string         `json:"source_type" binding:"required"`
	SourceID          string         `json:"source_id"`
	OperationType     string         `json:"operation_type"`
	TargetType        string         `json:"target_type"`
	TargetID          string         `json:"target_id"`
	TargetName        string         `json:"target_name"`
	Environment       string         `json:"environment"`
	Parameters        map[string]any `json:"parameters"`
	RollbackPlan      map[string]any `json:"rollback_plan"`
	RiskLevel         string         `json:"risk_level"`
	RunbookTemplateID string         `json:"runbook_template_id"`
	DryRun            bool           `json:"dry_run"`
}

type confirmTaskRequest struct {
	Confirm     bool   `json:"confirm"`
	ConfirmText string `json:"confirm_text"`
}

// CreateTask POST /api/executions/tasks
func (h *Handler) CreateTask(c *gin.Context) {
	if h.tasks == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "execution service is not enabled")
		return
	}
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid create task request")
		return
	}
	if strings.TrimSpace(req.OperationType) == "" && strings.TrimSpace(req.RunbookTemplateID) == "" {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "operation_type or runbook_template_id is required")
		return
	}
	out, err := h.tasks.Create(c.Request.Context(), actorFromContext(c), execapp.CreateTaskInput{
		Name: req.Name, SourceType: req.SourceType, SourceID: req.SourceID,
		OperationType: req.OperationType, TargetType: req.TargetType, TargetID: req.TargetID,
		TargetName: req.TargetName, Environment: req.Environment,
		Parameters: req.Parameters, RollbackPlan: req.RollbackPlan, RiskLevel: req.RiskLevel,
		RunbookTemplateID: req.RunbookTemplateID, DryRun: req.DryRun,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

type taskListQuery struct {
	pagination.Query
	Status     string `form:"status"`
	SourceType string `form:"source_type"`
	SourceID   string `form:"source_id"`
}

// ListTasks GET /api/executions/tasks
func (h *Handler) ListTasks(c *gin.Context) {
	if h.tasks == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "execution service is not enabled")
		return
	}
	var q taskListQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	items, total, err := h.tasks.List(c.Request.Context(), execapp.TaskListQuery{
		Page: q.Page, PageSize: q.PageSize, Status: q.Status,
		SourceType: q.SourceType, SourceID: q.SourceID, Keyword: q.Keyword,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pagination.NewResult(items, total, q.Query))
}

// GetTask GET /api/executions/tasks/:task_id
func (h *Handler) GetTask(c *gin.Context) {
	if h.tasks == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "execution service is not enabled")
		return
	}
	out, err := h.tasks.GetDetail(c.Request.Context(), c.Param("task_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// ConfirmTask POST /api/executions/tasks/:task_id/confirm
func (h *Handler) ConfirmTask(c *gin.Context) {
	if h.tasks == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "execution service is not enabled")
		return
	}
	var req confirmTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid confirm request")
		return
	}
	out, err := h.tasks.Confirm(c.Request.Context(), c.Param("task_id"), actorFromContext(c), execapp.ConfirmTaskInput{
		Confirm: req.Confirm, ConfirmText: req.ConfirmText,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// ExecuteTask POST /api/executions/tasks/:task_id/execute
func (h *Handler) ExecuteTask(c *gin.Context) {
	if h.tasks == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "execution service is not enabled")
		return
	}
	out, err := h.tasks.Execute(c.Request.Context(), c.Param("task_id"), actorFromContext(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
