// Package http 是 Audit 上下文的 HTTP 适配层。
package http

import (
	auditapp "github.com/734965549/aiops/internal/audit/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/pagination"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

// Handler 审计查询 HTTP 层。
type Handler struct {
	audits *auditapp.OperationAuditService
}

// NewHandler 构造 Handler。
func NewHandler(audits *auditapp.OperationAuditService) *Handler {
	return &Handler{audits: audits}
}

type auditListQuery struct {
	pagination.Query
	ResourceType string `form:"resource_type"`
	ResourceID   string `form:"resource_id"`
	UserID       string `form:"user_id"`
	Action       string `form:"action"`
}

// ListAudits GET /api/audits
func (h *Handler) ListAudits(c *gin.Context) {
	if h.audits == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "audit service is not enabled")
		return
	}
	var q auditListQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	items, total, err := h.audits.List(c.Request.Context(), auditapp.ListQuery{
		Page: q.Page, PageSize: q.PageSize,
		ResourceType: q.ResourceType, ResourceID: q.ResourceID,
		UserID: q.UserID, Action: q.Action,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pagination.NewResult(items, total, q.Query))
}
