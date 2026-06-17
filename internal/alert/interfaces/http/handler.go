// Package http 是 Alert 上下文的 HTTP 适配层。
//
// 只做参数解析与 httpx 响应封装；业务规则在 application 层。
//
// 第一阶段范围（ops/alert-contract.md §1）由 routes.go 注册；本包 Handler 覆盖
// 管理端列表/详情/状态动作与接入源 CRUD，Webhook 见 handler_ingest.go。
//
// 统一响应格式（ops/alert-contract.md §2）：
//   - 成功：httpx.OK → code="OK"、message="ok"、trace_id、data
//   - 失败：httpx.Fail / httpx.FailWith → code 为业务错误码、message 为原因、trace_id
//   - 分页列表：data 为 PageData<T>（pagination.Result），含 items/total/page/page_size
//
// 鉴权（ops/alert-contract.md §3.1）：管理端路由经 Authed 组 + AuthorizeStatic，
// 未登录/无效 token → 401 UNAUTHENTICATED，无权限 → 403 PERMISSION_DENIED；Webhook 见 handler_ingest.go。
package http

import (
	alertapp "github.com/734965549/aiops/internal/alert/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/pagination"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// Handler 持有告警管理与接入源用例依赖。
type Handler struct {
	alerts  *alertapp.AlertService
	sources *alertapp.SourceService
}

// NewHandler 构造管理端 Handler。
func NewHandler(alerts *alertapp.AlertService, sources *alertapp.SourceService) *Handler {
	return &Handler{alerts: alerts, sources: sources}
}

// alertListQuery 告警列表查询参数，嵌入 pagination.Query 以复用 §2 分页字段 page/page_size/keyword。
type alertListQuery struct {
	pagination.Query
	Status         string `form:"status"`
	Severity       string `form:"severity"`
	Source         string `form:"source"`
	SourceID       string `form:"source_id"`
	BusinessLine   string `form:"business_line"`
	Environment    string `form:"environment"`
	ApplicationID  string `form:"application_id"`
	ResourceID     string `form:"resource_id"`
	AssigneeUserID string `form:"assignee_user_id"`
	ActiveOnly     bool   `form:"active_only"`
	From           int64  `form:"from"`
	To             int64  `form:"to"`
}

// ListAlerts GET /api/alerts（§8.1）。
// 成功时 data 为 PageData<AlertDTO>（§2）：items、total、page（从 1 起）、page_size（最大 100）。
func (h *Handler) ListAlerts(c *gin.Context) {
	if h.alerts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert service is not enabled")
		return
	}
	var q alertListQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize() // 默认 page=1、page_size=20，page_size 上限 100（§2）
	items, total, err := h.alerts.List(c.Request.Context(), alertapp.AlertListQuery{
		Page:           q.Page,
		PageSize:       q.PageSize,
		Status:         q.Status,
		Severity:       q.Severity,
		Source:         q.Source,
		SourceID:       q.SourceID,
		BusinessLine:   q.BusinessLine,
		Environment:    q.Environment,
		ApplicationID:  q.ApplicationID,
		ResourceID:     q.ResourceID,
		AssigneeUserID: q.AssigneeUserID,
		Keyword:        q.Keyword,
		ActiveOnly:     q.ActiveOnly,
		From:           q.From,
		To:             q.To,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pagination.NewResult(items, total, q.Query)) // §2 PageData 封装
}

// GetAlert GET /api/alerts/:alert_id（§8.2）。
// 成功时 data 为 AlertDetailDTO（alert + events + related），经 httpx.OK 输出（§2）。
func (h *Handler) GetAlert(c *gin.Context) {
	if h.alerts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert service is not enabled")
		return
	}
	detail, err := h.alerts.GetDetail(c.Request.Context(), c.Param("alert_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, detail)
}

type messageRequest struct {
	Message string `json:"message"`
}

type assignRequest struct {
	AssigneeUserID string `json:"assignee_user_id" binding:"required"`
	Message        string `json:"message"`
}

type closeRequest struct {
	Resolution string `json:"resolution" binding:"required"`
}

type silenceRequest struct {
	Reason    string `json:"reason" binding:"required"`
	DurationS int64  `json:"duration_s" binding:"required"`
}

type commentRequest struct {
	Message string `json:"message" binding:"required"`
}

type aiAnalysisRequest struct {
	TimeRange      string `json:"time_range"`
	IncludeLogs    bool   `json:"include_logs"`
	IncludeMetrics bool   `json:"include_metrics"`
	IncludeChanges bool   `json:"include_changes"`
}

func actorFromContext(c *gin.Context) alertapp.Actor {
	return alertapp.Actor{
		UserID:      c.GetString(middleware.CtxKeyUserID),
		DisplayName: c.GetString(middleware.CtxKeyUserID),
	}
}

// 以下 Handler 为 §1 状态流转动作（§8.3–§8.10）；成功 data 为更新后 AlertDTO 或事件 DTO，经 httpx.OK 输出（§2）。

// Acknowledge POST /api/alerts/:alert_id/acknowledge（§8.3）：new → acknowledged。
func (h *Handler) Acknowledge(c *gin.Context) {
	if h.alerts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert service is not enabled")
		return
	}
	var req messageRequest
	_ = c.ShouldBindJSON(&req)
	out, err := h.alerts.Acknowledge(c.Request.Context(), c.Param("alert_id"), actorFromContext(c), req.Message)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// Assign POST /api/alerts/:alert_id/assign（§8.4）。
func (h *Handler) Assign(c *gin.Context) {
	if h.alerts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert service is not enabled")
		return
	}
	var req assignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "assignee_user_id is required")
		return
	}
	out, err := h.alerts.Assign(c.Request.Context(), c.Param("alert_id"), actorFromContext(c), req.AssigneeUserID, req.Message)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// StartProcessing POST /api/alerts/:alert_id/start-processing（§8.5）：acknowledged → processing。
func (h *Handler) StartProcessing(c *gin.Context) {
	if h.alerts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert service is not enabled")
		return
	}
	out, err := h.alerts.StartProcessing(c.Request.Context(), c.Param("alert_id"), actorFromContext(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// Recover POST /api/alerts/:alert_id/recover（§8.6）：processing → recovered（人工 recover）。
func (h *Handler) Recover(c *gin.Context) {
	if h.alerts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert service is not enabled")
		return
	}
	var req messageRequest
	_ = c.ShouldBindJSON(&req)
	out, err := h.alerts.Recover(c.Request.Context(), c.Param("alert_id"), actorFromContext(c), req.Message)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// Close POST /api/alerts/:alert_id/close（§8.7）。
func (h *Handler) Close(c *gin.Context) {
	if h.alerts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert service is not enabled")
		return
	}
	var req closeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "resolution is required")
		return
	}
	out, err := h.alerts.Close(c.Request.Context(), c.Param("alert_id"), actorFromContext(c), req.Resolution)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// Silence POST /api/alerts/:alert_id/silence（§8.8）。
func (h *Handler) Silence(c *gin.Context) {
	if h.alerts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert service is not enabled")
		return
	}
	var req silenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "reason and duration_s are required")
		return
	}
	out, err := h.alerts.Silence(c.Request.Context(), c.Param("alert_id"), actorFromContext(c), req.Reason, req.DurationS)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// Unsilence POST /api/alerts/:alert_id/unsilence（§8.9）：silenced → new。
func (h *Handler) Unsilence(c *gin.Context) {
	if h.alerts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert service is not enabled")
		return
	}
	out, err := h.alerts.Unsilence(c.Request.Context(), c.Param("alert_id"), actorFromContext(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// Comment POST /api/alerts/:alert_id/comments（§8.10）。
func (h *Handler) Comment(c *gin.Context) {
	if h.alerts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert service is not enabled")
		return
	}
	var req commentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "message is required")
		return
	}
	out, err := h.alerts.Comment(c.Request.Context(), c.Param("alert_id"), actorFromContext(c), req.Message)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// RequestAIAnalysis POST /api/alerts/:alert_id/ai-analysis（§9.2）。
// 写入 ai_analysis_requested 时间线事件；实际分析由前端再调 AI 模块 POST /api/ai/analyze-alert。
func (h *Handler) RequestAIAnalysis(c *gin.Context) {
	if h.alerts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert service is not enabled")
		return
	}
	var req aiAnalysisRequest
	_ = c.ShouldBindJSON(&req)
	out, err := h.alerts.RequestAIAnalysis(c.Request.Context(), c.Param("alert_id"), actorFromContext(c), alertapp.AIAnalysisInput{
		TimeRange:      req.TimeRange,
		IncludeLogs:    req.IncludeLogs,
		IncludeMetrics: req.IncludeMetrics,
		IncludeChanges: req.IncludeChanges,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

type createSourceRequest struct {
	ID           string `json:"id" binding:"required"`
	Name         string `json:"name" binding:"required"`
	Type         string `json:"type"`
	Enabled      *bool  `json:"enabled"`
	Secret       string `json:"secret" binding:"required"`
	Environment  string `json:"environment"`
	BusinessLine string `json:"business_line"`
	Description  string `json:"description"`
}

type updateSourceRequest struct {
	Name         *string `json:"name"`
	Type         *string `json:"type"`
	Enabled      *bool   `json:"enabled"`
	Secret       string  `json:"secret"`
	Environment  *string `json:"environment"`
	BusinessLine *string `json:"business_line"`
	Description  *string `json:"description"`
}

// 接入源管理（§1 接入源配置）；ListSources 返回 data.items，其余返回单条 AlertSourceDTO（§2）。

func (h *Handler) ListSources(c *gin.Context) {
	if h.sources == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert source service is not enabled")
		return
	}
	items, err := h.sources.List(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) GetSource(c *gin.Context) {
	if h.sources == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert source service is not enabled")
		return
	}
	out, err := h.sources.Get(c.Request.Context(), c.Param("source_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) CreateSource(c *gin.Context) {
	if h.sources == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert source service is not enabled")
		return
	}
	var req createSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "id, name and secret are required")
		return
	}
	out, err := h.sources.Create(c.Request.Context(), actorFromContext(c), alertapp.CreateSourceInput{
		ID:           req.ID,
		Name:         req.Name,
		Type:         req.Type,
		Enabled:      req.Enabled,
		Secret:       req.Secret,
		Environment:  req.Environment,
		BusinessLine: req.BusinessLine,
		Description:  req.Description,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) UpdateSource(c *gin.Context) {
	if h.sources == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert source service is not enabled")
		return
	}
	var req updateSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid request body")
		return
	}
	out, err := h.sources.Update(c.Request.Context(), c.Param("source_id"), actorFromContext(c), alertapp.UpdateSourceInput{
		Name:         req.Name,
		Type:         req.Type,
		Enabled:      req.Enabled,
		Secret:       req.Secret,
		Environment:  req.Environment,
		BusinessLine: req.BusinessLine,
		Description:  req.Description,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) DeleteSource(c *gin.Context) {
	if h.sources == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert source service is not enabled")
		return
	}
	if err := h.sources.Delete(c.Request.Context(), c.Param("source_id"), actorFromContext(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}
