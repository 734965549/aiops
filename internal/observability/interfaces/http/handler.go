package http

import (
	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	queries *obsapp.QueryService
}

func NewHandler(queries *obsapp.QueryService) *Handler {
	return &Handler{queries: queries}
}

type metricQueryRequest struct {
	AccountID  string            `json:"account_id" binding:"required"`
	Provider   string            `json:"provider"`
	Region     string            `json:"region"`
	Namespace  string            `json:"namespace"`
	Metric     string            `json:"metric" binding:"required"`
	Dimensions map[string]string `json:"dimensions"`
	From       int64             `json:"from" binding:"required"`
	To         int64             `json:"to" binding:"required"`
	Period     int               `json:"period"`
	Aggregator string            `json:"aggregator"`
}

type logSearchRequest struct {
	AccountID  string `json:"account_id" binding:"required"`
	Provider   string `json:"provider"`
	Service    string `json:"service"`
	ResourceID string `json:"resource_id"`
	Keyword    string `json:"keyword"`
	TraceID    string `json:"trace_id"`
	From       int64  `json:"from" binding:"required"`
	To         int64  `json:"to" binding:"required"`
	Limit      int    `json:"limit"`
}

type traceQueryRequest struct {
	AccountID    string `json:"account_id" binding:"required"`
	Provider     string `json:"provider"`
	Service      string `json:"service"`
	Operation    string `json:"operation"`
	TraceID      string `json:"trace_id"`
	ErrorOnly    bool   `json:"error_only"`
	MinLatencyMS int    `json:"min_latency_ms"`
	From         int64  `json:"from" binding:"required"`
	To           int64  `json:"to" binding:"required"`
	Limit        int    `json:"limit"`
}

type topologyQuery struct {
	AccountID     string `form:"account_id" binding:"required"`
	Provider      string `form:"provider"`
	ApplicationID string `form:"application_id"`
	From          int64  `form:"from" binding:"required"`
	To            int64  `form:"to" binding:"required"`
}

func (h *Handler) QueryMetrics(c *gin.Context) {
	if h.queries == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "observability service is not enabled")
		return
	}
	var req metricQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid request body")
		return
	}
	out, err := h.queries.QueryMetrics(c.Request.Context(), actorFromContext(c), domain.MetricQuery{
		AccountID: req.AccountID, Provider: req.Provider, Region: req.Region, Namespace: req.Namespace,
		Metric: req.Metric, Dimensions: req.Dimensions, From: req.From, To: req.To,
		Period: req.Period, Aggregator: req.Aggregator,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) SearchLogs(c *gin.Context) {
	if h.queries == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "observability service is not enabled")
		return
	}
	var req logSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid request body")
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	out, err := h.queries.SearchLogs(c.Request.Context(), actorFromContext(c), domain.LogQuery{
		AccountID: req.AccountID, Provider: req.Provider, Service: req.Service, ResourceID: req.ResourceID,
		Keyword: req.Keyword, TraceID: req.TraceID, From: req.From, To: req.To, Limit: limit,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) QueryTraces(c *gin.Context) {
	if h.queries == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "observability service is not enabled")
		return
	}
	var req traceQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid request body")
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	out, err := h.queries.QueryTraces(c.Request.Context(), actorFromContext(c), domain.TraceQuery{
		AccountID: req.AccountID, Provider: req.Provider, Service: req.Service, Operation: req.Operation,
		TraceID: req.TraceID, ErrorOnly: req.ErrorOnly, MinLatencyMS: req.MinLatencyMS,
		From: req.From, To: req.To, Limit: limit,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) QueryTopology(c *gin.Context) {
	if h.queries == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "observability service is not enabled")
		return
	}
	var q topologyQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid query parameters")
		return
	}
	out, err := h.queries.QueryTopology(c.Request.Context(), actorFromContext(c), domain.TopologyQuery{
		AccountID: q.AccountID, Provider: q.Provider, ApplicationID: q.ApplicationID, From: q.From, To: q.To,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
