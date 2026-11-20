package http

import (
	assetapp "github.com/734965549/aiops/internal/asset/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ResolveResource(c *gin.Context) {
	if h.topology == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "asset resource resolution service is not enabled")
		return
	}
	out, err := h.topology.ResolveResource(c.Request.Context(), assetapp.ResolveResourceQuery{
		CloudResourceID:   c.Query("cloud_resource_id"),
		AccountID:         c.Query("account_id"),
		Region:            c.Query("region"),
		CloudResourceType: c.Query("cloud_resource_type"),
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
