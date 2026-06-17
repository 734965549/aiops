package http

import (
	dashapp "github.com/734965549/aiops/internal/dashboard/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

// Handler Dashboard HTTP 层。
type Handler struct {
	summary *dashapp.SummaryService
}

// NewHandler 构造 Handler。
func NewHandler(summary *dashapp.SummaryService) *Handler {
	return &Handler{summary: summary}
}

// GetSummary GET /api/dashboard/summary
func (h *Handler) GetSummary(c *gin.Context) {
	if h.summary == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "dashboard summary is not enabled")
		return
	}
	out, err := h.summary.GetSummary(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
