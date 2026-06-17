package http

import (
	rbapp "github.com/734965549/aiops/internal/runbook/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/pagination"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

// Handler Runbook HTTP 层。
type Handler struct {
	templates *rbapp.TemplateService
}

// NewHandler 构造 Handler。
func NewHandler(templates *rbapp.TemplateService) *Handler {
	return &Handler{templates: templates}
}

type templateListQuery struct {
	pagination.Query
	Enabled *bool `form:"enabled"`
}

type createStepRequest struct {
	StepOrder         int            `json:"step_order" binding:"required"`
	Name              string         `json:"name" binding:"required"`
	ActionType        string         `json:"action_type" binding:"required"`
	RiskLevel         string         `json:"risk_level" binding:"required"`
	DryRunSupported   bool           `json:"dry_run_supported"`
	DefaultDryRun     bool           `json:"default_dry_run"`
	ParameterSchema   map[string]any `json:"parameter_schema"`
	DefaultParameters map[string]any `json:"default_parameters"`
	RollbackPlan      map[string]any `json:"rollback_plan"`
	TimeoutSeconds    int            `json:"timeout_seconds"`
}

type createTemplateRequest struct {
	Name              string              `json:"name" binding:"required"`
	Description       string              `json:"description"`
	Enabled           bool                `json:"enabled"`
	OperationType     string              `json:"operation_type" binding:"required"`
	RiskLevel         string              `json:"risk_level" binding:"required"`
	MatchAlertName    string              `json:"match_alert_name"`
	MatchResourceType string              `json:"match_resource_type"`
	MatchEnvironment  string              `json:"match_environment"`
	ParameterSchema   map[string]any      `json:"parameter_schema"`
	RollbackPlan      map[string]any      `json:"rollback_plan"`
	Steps             []createStepRequest `json:"steps" binding:"required"`
}

type updateTemplateRequest = createTemplateRequest

// ListTemplates GET /api/runbooks/templates
func (h *Handler) ListTemplates(c *gin.Context) {
	if h.templates == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "runbook service is not enabled")
		return
	}
	var q templateListQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	items, total, err := h.templates.List(c.Request.Context(), rbapp.TemplateListQuery{
		Page: q.Page, PageSize: q.PageSize, Keyword: q.Keyword, Enabled: q.Enabled,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pagination.NewResult(items, total, q.Query))
}

// GetTemplate GET /api/runbooks/templates/:template_id
func (h *Handler) GetTemplate(c *gin.Context) {
	if h.templates == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "runbook service is not enabled")
		return
	}
	out, err := h.templates.GetDetail(c.Request.Context(), c.Param("template_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// CreateTemplate POST /api/runbooks/templates
func (h *Handler) CreateTemplate(c *gin.Context) {
	if h.templates == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "runbook service is not enabled")
		return
	}
	var req createTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid runbook template request")
		return
	}
	out, err := h.templates.Create(c.Request.Context(), actorFromContext(c), toCreateInput(req))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// UpdateTemplate PUT /api/runbooks/templates/:template_id
func (h *Handler) UpdateTemplate(c *gin.Context) {
	if h.templates == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "runbook service is not enabled")
		return
	}
	var req updateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid runbook template request")
		return
	}
	out, err := h.templates.Update(c.Request.Context(), c.Param("template_id"), actorFromContext(c), toUpdateInput(req))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// DeleteTemplate DELETE /api/runbooks/templates/:template_id
func (h *Handler) DeleteTemplate(c *gin.Context) {
	if h.templates == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "runbook service is not enabled")
		return
	}
	if err := h.templates.Delete(c.Request.Context(), c.Param("template_id"), actorFromContext(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, map[string]any{"deleted": true})
}

// Recommend GET /api/runbooks/recommendations
func (h *Handler) Recommend(c *gin.Context) {
	if h.templates == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "runbook service is not enabled")
		return
	}
	alertID := c.Query("alert_id")
	if alertID == "" {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "alert_id is required")
		return
	}
	items, err := h.templates.Recommend(c.Request.Context(), actorFromContext(c), alertID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, map[string]any{"items": items})
}

func toCreateInput(req createTemplateRequest) rbapp.CreateTemplateInput {
	return rbapp.CreateTemplateInput{
		Name: req.Name, Description: req.Description, Enabled: req.Enabled,
		OperationType: req.OperationType, RiskLevel: req.RiskLevel,
		MatchAlertName: req.MatchAlertName, MatchResourceType: req.MatchResourceType,
		MatchEnvironment: req.MatchEnvironment, ParameterSchema: req.ParameterSchema,
		RollbackPlan: req.RollbackPlan, Steps: toStepInputs(req.Steps),
	}
}

func toUpdateInput(req updateTemplateRequest) rbapp.UpdateTemplateInput {
	return rbapp.UpdateTemplateInput{
		Name: req.Name, Description: req.Description, Enabled: req.Enabled,
		OperationType: req.OperationType, RiskLevel: req.RiskLevel,
		MatchAlertName: req.MatchAlertName, MatchResourceType: req.MatchResourceType,
		MatchEnvironment: req.MatchEnvironment, ParameterSchema: req.ParameterSchema,
		RollbackPlan: req.RollbackPlan, Steps: toStepInputs(req.Steps),
	}
}

func toStepInputs(steps []createStepRequest) []rbapp.CreateStepInput {
	out := make([]rbapp.CreateStepInput, 0, len(steps))
	for _, s := range steps {
		out = append(out, rbapp.CreateStepInput{
			StepOrder: s.StepOrder, Name: s.Name, ActionType: s.ActionType, RiskLevel: s.RiskLevel,
			DryRunSupported: s.DryRunSupported, DefaultDryRun: s.DefaultDryRun,
			ParameterSchema: s.ParameterSchema, DefaultParameters: s.DefaultParameters,
			RollbackPlan: s.RollbackPlan, TimeoutSeconds: s.TimeoutSeconds,
		})
	}
	return out
}
