package http

import (
	"strconv"

	assetapp "github.com/734965549/aiops/internal/asset/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

type createResourceRelationRequest struct {
	SourceResourceID string `json:"source_resource_id" binding:"required"`
	TargetResourceID string `json:"target_resource_id" binding:"required"`
	RelationType     string `json:"relation_type" binding:"required"`
}

func (h *Handler) GetTopology(c *gin.Context) {
	if h.topology == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset topology service is not enabled")
		return
	}
	depth := 0
	if rawDepth := c.Query("depth"); rawDepth != "" {
		parsed, err := strconv.Atoi(rawDepth)
		if err != nil {
			httpx.FailWith(c, apperr.CodeInvalidArgument, "depth must be an integer")
			return
		}
		depth = parsed
	}
	out, err := h.topology.GetTopology(c.Request.Context(), assetapp.TopologyQuery{
		ApplicationID:     c.Query("application_id"),
		RootResourceID:    c.Query("root_resource_id"),
		Depth:             depth,
		Direction:         c.Query("direction"),
		Region:            c.Query("region"),
		CloudResourceType: c.Query("cloud_resource_type"),
		RelationType:      c.Query("relation_type"),
		Keyword:           c.Query("keyword"),
		IncludeStale:      c.Query("include_stale") == "true",
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ListResourceRelations(c *gin.Context) {
	if h.topology == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset topology service is not enabled")
		return
	}
	items, err := h.topology.ListRelations(c.Request.Context(), c.Query("application_id"), c.Query("relation_type"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items, "total": len(items)})
}

func (h *Handler) CreateResourceRelation(c *gin.Context) {
	if h.topology == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset topology service is not enabled")
		return
	}
	var req createResourceRelationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "source_resource_id, target_resource_id and relation_type are required")
		return
	}
	out, err := h.topology.CreateRelation(c.Request.Context(), actorFromContext(c), assetapp.CreateResourceRelationInput{
		SourceResourceID: req.SourceResourceID,
		TargetResourceID: req.TargetResourceID,
		RelationType:     req.RelationType,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) DeleteResourceRelation(c *gin.Context) {
	if h.topology == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset topology service is not enabled")
		return
	}
	if err := h.topology.DeleteRelation(c.Request.Context(), c.Param("relation_id"), actorFromContext(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}
