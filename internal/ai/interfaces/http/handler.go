package http

import (
	"context"
	"strings"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	"github.com/734965549/aiops/internal/ai/toolgateway"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type Gateway interface {
	Validate(ctx context.Context, req toolgateway.ToolRequest) (*toolgateway.ToolResponse, error)
	Invoke(ctx context.Context, providerID string, req toolgateway.ToolRequest) (*toolgateway.ToolResponse, error)
}

type ProviderRegistry interface {
	ListProviders() []toolgateway.ProviderPublic
	UpsertProvider(cfg toolgateway.ProviderConfig) error
	DeleteProvider(id string)
	GetProvider(id string) (toolgateway.ProviderConfig, bool)
}

type Authorizer interface {
	Authorize(ctx context.Context, input identityapp.AuthorizationInput) (*identityapp.AuthorizationResult, error)
}

type Handler struct {
	gateway    Gateway
	registry   ProviderRegistry
	authorizer Authorizer
	analyze    *aiapp.AnalyzeService
	audit      aiapp.AuditRecorder
}

func NewHandler(gateway Gateway, registry ProviderRegistry, authorizer Authorizer, analyze *aiapp.AnalyzeService, audit aiapp.AuditRecorder) *Handler {
	if audit == nil {
		audit = aiapp.NoopAuditRecorder{}
	}
	return &Handler{gateway: gateway, registry: registry, authorizer: authorizer, analyze: analyze, audit: audit}
}

type providerRequest struct {
	ID          string            `json:"id" binding:"required"`
	Name        string            `json:"name" binding:"required"`
	Type        string            `json:"type" binding:"required"`
	BaseURL     string            `json:"base_url" binding:"required"`
	APIKey      string            `json:"api_key"`
	TimeoutMS   int64             `json:"timeout_ms"`
	Headers     map[string]string `json:"headers"`
	Enabled     bool              `json:"enabled"`
	Description string            `json:"description"`
}

type providerListItem struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	BaseURL     string            `json:"base_url"`
	HasAPIKey   bool              `json:"has_api_key"`
	TimeoutMS   int64             `json:"timeout_ms"`
	Headers     map[string]string `json:"headers"`
	Enabled     bool              `json:"enabled"`
	Description string            `json:"description"`
}

func maskProviderForList(p toolgateway.ProviderPublic) providerListItem {
	return providerListItem{
		ID:          p.ID,
		Name:        p.Name,
		Type:        string(p.Type),
		BaseURL:     p.BaseURL,
		HasAPIKey:   p.HasAPIKey,
		TimeoutMS:   p.TimeoutMS,
		Headers:     p.Headers,
		Enabled:     p.Enabled,
		Description: p.Description,
	}
}

func (h *Handler) ListProviders(c *gin.Context) {
	if h.registry == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "provider registry is not configured")
		return
	}
	providers := h.registry.ListProviders()
	out := make([]providerListItem, 0, len(providers))
	for _, p := range providers {
		out = append(out, maskProviderForList(p))
	}
	httpx.OK(c, out)
}

func (h *Handler) UpsertProvider(c *gin.Context) {
	if h.registry == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "provider registry is not configured")
		return
	}
	var req providerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid provider request")
		return
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		if existing, ok := h.registry.GetProvider(req.ID); ok {
			apiKey = existing.APIKey
		} else {
			httpx.FailWith(c, apperr.CodeInvalidArgument, "api_key is required for new provider")
			return
		}
	}
	cfg := toolgateway.ProviderConfig{ID: req.ID, Name: req.Name, Type: toolgateway.ProviderType(req.Type), BaseURL: req.BaseURL, APIKey: apiKey, TimeoutMS: req.TimeoutMS, Headers: req.Headers, Enabled: req.Enabled, Description: req.Description}
	if err := h.registry.UpsertProvider(cfg); err != nil {
		httpx.Fail(c, err)
		return
	}
	logger.From(c.Request.Context()).Info("ai provider upserted",
		logger.String("actor_user_id", c.GetString(middleware.CtxKeyUserID)),
		logger.Any("provider", cfg.AuditFields()),
	)
	httpx.OK(c, gin.H{"updated": true})
}

func (h *Handler) DeleteProvider(c *gin.Context) {
	if h.registry == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "provider registry is not configured")
		return
	}
	id := c.Param("id")
	h.registry.DeleteProvider(id)
	logger.From(c.Request.Context()).Info("ai provider deleted",
		logger.String("actor_user_id", c.GetString(middleware.CtxKeyUserID)),
		logger.String("provider_id", id),
	)
	httpx.OK(c, gin.H{"deleted": true})
}

type invokeRequest struct {
	ProviderID string         `json:"provider_id" binding:"required"`
	ToolCode   string         `json:"tool_code" binding:"required"`
	Resource   string         `json:"resource"`
	Action     string         `json:"action"`
	OwnerID    string         `json:"owner_id"`
	Dept       string         `json:"dept"`
	Team       string         `json:"team"`
	Region     string         `json:"region"`
	Tags       []string       `json:"tags"`
	Confirmed  bool           `json:"confirmed"`
	Payload    map[string]any `json:"payload"`
}

func (h *Handler) Invoke(c *gin.Context) {
	if h.gateway == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai gateway is not configured")
		return
	}
	var req invokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid invoke request")
		return
	}
	userID := c.GetString(middleware.CtxKeyUserID)
	resp, err := h.gateway.Invoke(c.Request.Context(), req.ProviderID, toolgateway.ToolRequest{UserID: userID, ToolCode: req.ToolCode, Resource: req.Resource, Action: req.Action, OwnerID: req.OwnerID, Dept: req.Dept, Team: req.Team, Region: req.Region, Tags: req.Tags, Confirmed: req.Confirmed, Payload: req.Payload})
	if err != nil {
		h.recordToolInvokeAudit(c, req, userID, "failure", err.Error())
		httpx.Fail(c, err)
		return
	}
	h.recordToolInvokeAudit(c, req, userID, "success", "")
	httpx.OK(c, resp)
}

func (h *Handler) recordToolInvokeAudit(c *gin.Context, req invokeRequest, userID, result, errMsg string) {
	if h == nil || h.audit == nil {
		return
	}
	payload := map[string]any{
		"provider_id": req.ProviderID,
		"tool_code":   req.ToolCode,
		"resource":    req.Resource,
		"action":      req.Action,
		"result":      result,
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	_ = h.audit.Record(c.Request.Context(), aiapp.AuditRecord{
		ResourceType: "ai",
		ResourceID:   req.ToolCode,
		Action:       aiapp.AuditToolInvoke,
		UserID:       userID,
		Payload:      payload,
	})
}

type analyzeAlertRequest struct {
	AlertID        string `json:"alert_id" binding:"required"`
	TimeRange      string `json:"time_range"`
	IncludeLogs    bool   `json:"include_logs"`
	IncludeMetrics bool   `json:"include_metrics"`
	IncludeChanges bool   `json:"include_changes"`
}

// AnalyzeAlert POST /api/ai/analyze-alert（Alert §9.2）。
func (h *Handler) AnalyzeAlert(c *gin.Context) {
	if h.analyze == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai analyze service is not enabled")
		return
	}
	var req analyzeAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "alert_id is required")
		return
	}
	out, err := h.analyze.AnalyzeAlert(c.Request.Context(), c.GetString(middleware.CtxKeyUserID), aiapp.AnalyzeAlertInput{
		AlertID: req.AlertID, TimeRange: req.TimeRange,
		IncludeLogs: req.IncludeLogs, IncludeMetrics: req.IncludeMetrics, IncludeChanges: req.IncludeChanges,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
