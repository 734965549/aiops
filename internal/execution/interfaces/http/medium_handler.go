package http

import (
	execapp "github.com/734965549/aiops/internal/execution/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/pagination"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

type createMediumRequest struct {
	MediumID          string   `json:"medium_id"`
	Name              string   `json:"name" binding:"required"`
	MediumType        string   `json:"medium_type" binding:"required"`
	Environment       string   `json:"environment"`
	Region            string   `json:"region"`
	NetworkZone       string   `json:"network_zone"`
	Capabilities      []string `json:"capabilities"`
	AllowedCommandIDs []string `json:"allowed_command_ids"`
	MaxRiskLevel      string   `json:"max_risk_level"`
	Enabled           *bool    `json:"enabled"`
	Description       string   `json:"description"`
}

type mediumListQuery struct {
	pagination.Query
	Enabled     *bool  `form:"enabled"`
	Environment string `form:"environment"`
	MediumType  string `form:"medium_type"`
}

func (h *Handler) CreateMedium(c *gin.Context) {
	if h.media == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "medium service is not enabled")
		return
	}
	var req createMediumRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid create medium request")
		return
	}
	out, err := h.media.Create(c.Request.Context(), actorFromContext(c), execapp.CreateMediumInput{
		MediumID: req.MediumID, Name: req.Name, MediumType: req.MediumType,
		Environment: req.Environment, Region: req.Region, NetworkZone: req.NetworkZone,
		Capabilities: req.Capabilities, AllowedCommandIDs: req.AllowedCommandIDs,
		MaxRiskLevel: req.MaxRiskLevel, Enabled: req.Enabled, Description: req.Description,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ListMedia(c *gin.Context) {
	if h.media == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "medium service is not enabled")
		return
	}
	var q mediumListQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	items, total, err := h.media.List(c.Request.Context(), execapp.ListMediaQuery{
		Page: q.Page, PageSize: q.PageSize, Enabled: q.Enabled,
		Environment: q.Environment, MediumType: q.MediumType, Keyword: q.Keyword,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pagination.NewResult(items, total, q.Query))
}

func (h *Handler) GetMedium(c *gin.Context) {
	if h.media == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "medium service is not enabled")
		return
	}
	out, err := h.media.Get(c.Request.Context(), c.Param("medium_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ListCommandSpecs(c *gin.Context) {
	if h.specs == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "command spec service is not enabled")
		return
	}
	var q pagination.Query
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	var enabled *bool
	if raw := c.Query("enabled"); raw != "" {
		v := raw == "true" || raw == "1"
		enabled = &v
	}
	items, total, err := h.specs.List(c.Request.Context(), enabled, q.Page, q.PageSize)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pagination.NewResult(items, total, q))
}

func (h *Handler) GetCommandSpec(c *gin.Context) {
	if h.specs == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "command spec service is not enabled")
		return
	}
	out, err := h.specs.Get(c.Request.Context(), c.Param("command_spec_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ListTaskLogs(c *gin.Context) {
	if h.dispatch == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "dispatch service is not enabled")
		return
	}
	stepID := c.Query("step_id")
	if stepID == "" {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "step_id is required")
		return
	}
	items, err := h.dispatch.ListLogs(c.Request.Context(), c.Param("task_id"), stepID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}
