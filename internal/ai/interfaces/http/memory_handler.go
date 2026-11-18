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

// MemoryUseCase 是记忆管理用例端口（*application.MemoryService 实现）。
type MemoryUseCase interface {
	ListCases(ctx context.Context, in aiapp.MemoryCaseListInput) ([]domain.MemoryCase, error)
	UpdateCaseStatus(ctx context.Context, actorID, caseID, status string) (*domain.MemoryCase, error)
}

// MemoryHandler 是记忆条目的管理侧 HTTP 层（鉴权 app:ai:memory:manage）。
type MemoryHandler struct {
	memory MemoryUseCase
}

func NewMemoryHandler(memory MemoryUseCase) *MemoryHandler {
	return &MemoryHandler{memory: memory}
}

type memoryCaseResponse struct {
	CaseID          string   `json:"case_id"`
	SourceType      string   `json:"source_type"`
	SessionID       string   `json:"session_id,omitempty"`
	UserID          string   `json:"user_id"`
	ApplicationID   string   `json:"application_id"`
	ResourceID      string   `json:"resource_id"`
	ResourceType    string   `json:"resource_type"`
	AlertSignature  string   `json:"alert_signature,omitempty"`
	Keywords        string   `json:"keywords,omitempty"`
	Summary         string   `json:"summary"`
	Recommendations []string `json:"recommendations"`
	RiskLevel       string   `json:"risk_level"`
	QualityScore    int      `json:"quality_score"`
	Status          string   `json:"status"`
	CreatedAt       int64    `json:"created_at"`
	UpdatedAt       int64    `json:"updated_at"`
}

func toMemoryCaseResponse(item domain.MemoryCase) memoryCaseResponse {
	return memoryCaseResponse{
		CaseID: item.CaseID, SourceType: string(item.SourceType), SessionID: item.SessionID,
		UserID: item.UserID, ApplicationID: item.ApplicationID, ResourceID: item.ResourceID,
		ResourceType: item.ResourceType, AlertSignature: item.AlertSignature, Keywords: item.Keywords,
		Summary: item.Summary, Recommendations: item.Recommendations, RiskLevel: item.RiskLevel,
		QualityScore: item.QualityScore, Status: string(item.Status),
		CreatedAt: item.CreatedAt.Unix(), UpdatedAt: item.UpdatedAt.Unix(),
	}
}

func (h *MemoryHandler) service() (MemoryUseCase, bool) {
	if h == nil || h.memory == nil {
		return nil, false
	}
	return h.memory, true
}

// List GET /api/ai/memory/cases?status=&user_id=&application_id=&resource_id=&keyword=&limit=&offset=
func (h *MemoryHandler) List(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai memory service is not enabled")
		return
	}
	in := aiapp.MemoryCaseListInput{
		UserID:        c.Query("user_id"),
		ApplicationID: c.Query("application_id"),
		ResourceID:    c.Query("resource_id"),
		Status:        c.Query("status"),
		Keyword:       c.Query("keyword"),
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > aiapp.MemoryCaseListMaxLimit {
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
	cases, err := svc.ListCases(c.Request.Context(), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := make([]memoryCaseResponse, 0, len(cases))
	for _, item := range cases {
		out = append(out, toMemoryCaseResponse(item))
	}
	httpx.OK(c, gin.H{"items": out})
}

// UpdateStatus POST /api/ai/memory/cases/:case_id/status  body: { "status": "active" | "retired" }
func (h *MemoryHandler) UpdateStatus(c *gin.Context) {
	svc, ok := h.service()
	if !ok {
		httpx.FailWith(c, apperr.CodeUnavailable, "ai memory service is not enabled")
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := httpx.BindJSON(c, &req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "status is required")
		return
	}
	updated, err := svc.UpdateCaseStatus(
		c.Request.Context(), c.GetString(middleware.CtxKeyUserID), c.Param("case_id"), req.Status,
	)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, toMemoryCaseResponse(*updated))
}
