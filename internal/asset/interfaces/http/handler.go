// Package http 是 Asset 上下文的 HTTP 适配层。
package http

import (
	assetapp "github.com/734965549/aiops/internal/asset/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

// Handler 资产管理 HTTP 层。
type Handler struct {
	assets     *assetapp.AssetService
	matchRules *assetapp.MatchRuleService
}

// NewHandler 构造 Handler。
func NewHandler(assets *assetapp.AssetService, matchRules *assetapp.MatchRuleService) *Handler {
	return &Handler{assets: assets, matchRules: matchRules}
}

type createApplicationRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name" binding:"required"`
	Environment string `json:"environment"`
	Namespace   string `json:"namespace"`
	Description string `json:"description"`
}

type createResourceRequest struct {
	ID            string `json:"id"`
	ApplicationID string `json:"application_id" binding:"required"`
	Name          string `json:"name"`
	ResourceType  string `json:"resource_type"`
	Namespace     string `json:"namespace"`
	Pod           string `json:"pod"`
	Node          string `json:"node"`
	Instance      string `json:"instance"`
}

type updateApplicationRequest struct {
	Name        string `json:"name" binding:"required"`
	Environment string `json:"environment"`
	Namespace   string `json:"namespace"`
	Description string `json:"description"`
}

type updateResourceRequest struct {
	Name         string `json:"name"`
	ResourceType string `json:"resource_type"`
	Namespace    string `json:"namespace"`
	Pod          string `json:"pod"`
	Node         string `json:"node"`
	Instance     string `json:"instance"`
}

// ListApplications GET /api/assets/applications
func (h *Handler) ListApplications(c *gin.Context) {
	if h.assets == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset service is not enabled")
		return
	}
	items, err := h.assets.ListApplications(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// CreateApplication POST /api/assets/applications
func (h *Handler) CreateApplication(c *gin.Context) {
	if h.assets == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset service is not enabled")
		return
	}
	var req createApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "name is required")
		return
	}
	out, err := h.assets.CreateApplication(c.Request.Context(), actorFromContext(c), assetapp.CreateApplicationInput{
		ID: req.ID, Name: req.Name, Environment: req.Environment,
		Namespace: req.Namespace, Description: req.Description,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// UpdateApplication PUT /api/assets/applications/:id
func (h *Handler) UpdateApplication(c *gin.Context) {
	if h.assets == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset service is not enabled")
		return
	}
	var req updateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "name is required")
		return
	}
	out, err := h.assets.UpdateApplication(c.Request.Context(), c.Param("id"), actorFromContext(c), assetapp.UpdateApplicationInput{
		Name: req.Name, Environment: req.Environment,
		Namespace: req.Namespace, Description: req.Description,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// DeleteApplication DELETE /api/assets/applications/:id
func (h *Handler) DeleteApplication(c *gin.Context) {
	if h.assets == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset service is not enabled")
		return
	}
	if err := h.assets.DeleteApplication(c.Request.Context(), c.Param("id"), actorFromContext(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}

// ListResources GET /api/assets/applications/:application_id/resources
func (h *Handler) ListResources(c *gin.Context) {
	if h.assets == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset service is not enabled")
		return
	}
	items, err := h.assets.ListResources(c.Request.Context(), c.Param("application_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// CreateResource POST /api/assets/resources
func (h *Handler) CreateResource(c *gin.Context) {
	if h.assets == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset service is not enabled")
		return
	}
	var req createResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "application_id is required")
		return
	}
	out, err := h.assets.CreateResource(c.Request.Context(), actorFromContext(c), assetapp.CreateResourceInput{
		ID: req.ID, ApplicationID: req.ApplicationID, Name: req.Name,
		ResourceType: req.ResourceType, Namespace: req.Namespace,
		Pod: req.Pod, Node: req.Node, Instance: req.Instance,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// UpdateResource PUT /api/assets/resources/:id
func (h *Handler) UpdateResource(c *gin.Context) {
	if h.assets == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset service is not enabled")
		return
	}
	var req updateResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid request body")
		return
	}
	out, err := h.assets.UpdateResource(c.Request.Context(), c.Param("id"), actorFromContext(c), assetapp.UpdateResourceInput{
		Name: req.Name, ResourceType: req.ResourceType, Namespace: req.Namespace,
		Pod: req.Pod, Node: req.Node, Instance: req.Instance,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// DeleteResource DELETE /api/assets/resources/:id
func (h *Handler) DeleteResource(c *gin.Context) {
	if h.assets == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset service is not enabled")
		return
	}
	if err := h.assets.DeleteResource(c.Request.Context(), c.Param("id"), actorFromContext(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}

type createMatchRuleRequest struct {
	ID                string `json:"id"`
	Name              string `json:"name" binding:"required"`
	Enabled           *bool  `json:"enabled"`
	Priority          int    `json:"priority"`
	TargetType        string `json:"target_type"`
	SourceType        string `json:"source_type"`
	LabelKey          string `json:"label_key" binding:"required"`
	LabelValuePattern string `json:"label_value_pattern" binding:"required"`
	ApplicationID     string `json:"application_id" binding:"required"`
	ResourceID        string `json:"resource_id"`
}

type updateMatchRuleRequest struct {
	Name              string `json:"name" binding:"required"`
	Enabled           *bool  `json:"enabled"`
	Priority          int    `json:"priority"`
	TargetType        string `json:"target_type"`
	SourceType        string `json:"source_type"`
	LabelKey          string `json:"label_key" binding:"required"`
	LabelValuePattern string `json:"label_value_pattern" binding:"required"`
	ApplicationID     string `json:"application_id" binding:"required"`
	ResourceID        string `json:"resource_id"`
}

// ListMatchRules GET /api/assets/match-rules
func (h *Handler) ListMatchRules(c *gin.Context) {
	if h.matchRules == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "match rule service is not enabled")
		return
	}
	items, err := h.matchRules.List(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// CreateMatchRule POST /api/assets/match-rules
func (h *Handler) CreateMatchRule(c *gin.Context) {
	if h.matchRules == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "match rule service is not enabled")
		return
	}
	var req createMatchRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid match rule request")
		return
	}
	out, err := h.matchRules.Create(c.Request.Context(), actorFromContext(c), assetapp.CreateMatchRuleInput{
		ID: req.ID, Name: req.Name, Enabled: req.Enabled, Priority: req.Priority,
		TargetType: req.TargetType, SourceType: req.SourceType,
		LabelKey: req.LabelKey, LabelValuePattern: req.LabelValuePattern,
		ApplicationID: req.ApplicationID, ResourceID: req.ResourceID,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// UpdateMatchRule PUT /api/assets/match-rules/:id
func (h *Handler) UpdateMatchRule(c *gin.Context) {
	if h.matchRules == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "match rule service is not enabled")
		return
	}
	var req updateMatchRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid match rule request")
		return
	}
	out, err := h.matchRules.Update(c.Request.Context(), c.Param("id"), actorFromContext(c), assetapp.UpdateMatchRuleInput{
		Name: req.Name, Enabled: req.Enabled, Priority: req.Priority,
		TargetType: req.TargetType, SourceType: req.SourceType,
		LabelKey: req.LabelKey, LabelValuePattern: req.LabelValuePattern,
		ApplicationID: req.ApplicationID, ResourceID: req.ResourceID,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// DeleteMatchRule DELETE /api/assets/match-rules/:id
func (h *Handler) DeleteMatchRule(c *gin.Context) {
	if h.matchRules == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "match rule service is not enabled")
		return
	}
	if err := h.matchRules.Delete(c.Request.Context(), c.Param("id"), actorFromContext(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}
