package http

import (
	"context"
	"strconv"
	"strings"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// KnowledgeUseCase 是知识沉淀管理用例端口（*application.KnowledgeService 实现）。
type KnowledgeUseCase interface {
	List(ctx context.Context, in aiapp.KnowledgeListInput) ([]domain.KnowledgeItem, error)
	Create(ctx context.Context, actor aiapp.KnowledgeActor, in aiapp.KnowledgeItemInput) (*domain.KnowledgeItem, error)
	Update(ctx context.Context, actor aiapp.KnowledgeActor, itemID string, in aiapp.KnowledgeItemInput) (*domain.KnowledgeItem, error)
	Review(ctx context.Context, actor aiapp.KnowledgeActor, itemID, action string) (*domain.KnowledgeItem, error)
	Delete(ctx context.Context, actor aiapp.KnowledgeActor, itemID string) error
	AggregateFromFeedback(ctx context.Context, actor aiapp.KnowledgeActor, applicationID string, minCases int) (*aiapp.AggregateResult, error)
}

// KnowledgeHandler 是知识条目的管理侧 HTTP 层（鉴权 app:ai:knowledge:manage）。
type KnowledgeHandler struct {
	knowledge KnowledgeUseCase
}

func NewKnowledgeHandler(knowledge KnowledgeUseCase) *KnowledgeHandler {
	return &KnowledgeHandler{knowledge: knowledge}
}

type knowledgeItemRequest struct {
	Scope    string `json:"scope"`
	ScopeRef string `json:"scope_ref"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

func (r knowledgeItemRequest) toInput() aiapp.KnowledgeItemInput {
	return aiapp.KnowledgeItemInput{Scope: r.Scope, ScopeRef: r.ScopeRef, Title: r.Title, Content: r.Content}
}

type knowledgeItemResponse struct {
	ItemID      string `json:"item_id"`
	Scope       string `json:"scope"`
	ScopeRef    string `json:"scope_ref"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	Origin      string `json:"origin"`
	Status      string `json:"status"`
	HitCount    int    `json:"hit_count"`
	UsefulCount int    `json:"useful_count"`
	ReviewedBy  string `json:"reviewed_by,omitempty"`
	ReviewedAt  int64  `json:"reviewed_at,omitempty"`
	CreatedBy   string `json:"created_by,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

func toKnowledgeResponse(item domain.KnowledgeItem) knowledgeItemResponse {
	out := knowledgeItemResponse{
		ItemID: item.ItemID, Scope: string(item.Scope), ScopeRef: item.ScopeRef,
		Title: item.Title, Content: item.Content, Origin: string(item.Origin),
		Status: string(item.Status), HitCount: item.HitCount, UsefulCount: item.UsefulCount,
		ReviewedBy: item.ReviewedBy, CreatedBy: item.CreatedBy,
		CreatedAt: item.CreatedAt.Unix(), UpdatedAt: item.UpdatedAt.Unix(),
	}
	if item.ReviewedAt != nil {
		out.ReviewedAt = item.ReviewedAt.Unix()
	}
	return out
}

func (h *KnowledgeHandler) service() (KnowledgeUseCase, bool) {
	if h == nil || h.knowledge == nil {
		return nil, false
	}
	return h.knowledge, true
}

func knowledgeActor(c *gin.Context) aiapp.KnowledgeActor {
	return aiapp.KnowledgeActor{UserID: c.GetString(middleware.CtxKeyUserID)}
}

// List GET /api/ai/knowledge/items?status=&scope=&limit=&offset=
func (h *KnowledgeHandler) List(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai knowledge service is not enabled")
		return
	}
	in := aiapp.KnowledgeListInput{Status: c.Query("status"), Scope: c.Query("scope")}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > 100 {
			httpx.FailWith(c, apperr.CodeInvalidArgument, "limit must be between 1 and 100")
			return
		}
		in.Limit = parsed
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			httpx.FailWith(c, apperr.CodeInvalidArgument, "offset must be non-negative")
			return
		}
		in.Offset = parsed
	}
	items, err := svc.List(c.Request.Context(), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := make([]knowledgeItemResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toKnowledgeResponse(item))
	}
	httpx.OK(c, gin.H{"items": out})
}

// Create POST /api/ai/knowledge/items
func (h *KnowledgeHandler) Create(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai knowledge service is not enabled")
		return
	}
	var req knowledgeItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid knowledge item request")
		return
	}
	item, err := svc.Create(c.Request.Context(), knowledgeActor(c), req.toInput())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, toKnowledgeResponse(*item))
}

// Update PUT /api/ai/knowledge/items/:item_id
func (h *KnowledgeHandler) Update(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai knowledge service is not enabled")
		return
	}
	var req knowledgeItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid knowledge item request")
		return
	}
	item, err := svc.Update(c.Request.Context(), knowledgeActor(c), c.Param("item_id"), req.toInput())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, toKnowledgeResponse(*item))
}

// Review POST /api/ai/knowledge/items/:item_id/review  body: { "action": "activate" | "disable" }
func (h *KnowledgeHandler) Review(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai knowledge service is not enabled")
		return
	}
	var req struct {
		Action string `json:"action" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "action is required")
		return
	}
	item, err := svc.Review(c.Request.Context(), knowledgeActor(c), c.Param("item_id"), req.Action)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, toKnowledgeResponse(*item))
}

// Delete DELETE /api/ai/knowledge/items/:item_id
func (h *KnowledgeHandler) Delete(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai knowledge service is not enabled")
		return
	}
	if err := svc.Delete(c.Request.Context(), knowledgeActor(c), c.Param("item_id")); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}

// Aggregate POST /api/ai/knowledge/aggregate  body: { "application_id": "...", "min_cases": 3 }
//
// 把某应用下已确认有效的记忆案例聚合成一条 draft；已 active 的条目不覆盖。
func (h *KnowledgeHandler) Aggregate(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai knowledge service is not enabled")
		return
	}
	var req struct {
		ApplicationID string `json:"application_id" binding:"required"`
		MinCases      int    `json:"min_cases"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "application_id is required")
		return
	}
	result, err := svc.AggregateFromFeedback(c.Request.Context(), knowledgeActor(c), req.ApplicationID, req.MinCases)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	payload := gin.H{"action": result.Action, "source_cases": result.SourceCases}
	if result.Item != nil {
		payload["item"] = toKnowledgeResponse(*result.Item)
	}
	httpx.OK(c, payload)
}
