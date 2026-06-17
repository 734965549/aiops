package http

import (
	"strings"

	"github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/identity/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/pagination"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type userListQuery struct {
	pagination.Query
	Status  string `form:"status"`
	Keyword string `form:"keyword"`
}

type replaceIDsRequest struct {
	RoleIDs           []string `json:"role_ids"`
	PermissionIDs     []string `json:"permission_ids"`
	DataScopeIDs      []string `json:"data_scope_ids"`
	ToolPermissionIDs []string `json:"tool_permission_ids"`
}

type dataScopeResponse struct {
	ID          string         `json:"id"`
	Code        string         `json:"code"`
	Name        string         `json:"name"`
	ScopeType   string         `json:"scope_type"`
	ScopeConfig map[string]any `json:"scope_config"`
	Description string         `json:"description"`
}

type aiToolPermissionResponse struct {
	ID                       string `json:"id"`
	ToolCode                 string `json:"tool_code"`
	ToolName                 string `json:"tool_name"`
	PermissionMode           string `json:"permission_mode"`
	PermitsUnconfirmedInvoke bool   `json:"permits_unconfirmed_invoke"`
	Description              string `json:"description"`
}

func (h *Handler) AdminUsers(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	var q userListQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	var st *domain.UserStatus
	if strings.TrimSpace(q.Status) != "" {
		t := domain.UserStatus(strings.TrimSpace(q.Status))
		st = &t
	}
	items, total, err := h.access.ListUsers(c.Request.Context(), domain.UserFilter{
		Status:  st,
		Keyword: q.Keyword,
		Limit:   q.Limit(),
		Offset:  q.Offset(),
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pagination.NewResult(items, total, q.Query))
}

func (h *Handler) AdminUserRoles(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	items, err := h.access.ListUserRoleBindings(c.Request.Context(), c.Param("user_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) AdminReplaceUserRoles(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	var req replaceIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid role binding request")
		return
	}
	items, err := h.access.ReplaceUserManualRoles(c.Request.Context(), accessActorFromContext(c), c.Param("user_id"), req.RoleIDs)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) DataScopes(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	var scopeType *domain.DataScopeType
	if raw := strings.TrimSpace(c.Query("scope_type")); raw != "" {
		t := domain.DataScopeType(raw)
		scopeType = &t
	}
	rows, err := h.access.ListDataScopes(c.Request.Context(), domain.DataScopeFilter{ScopeType: scopeType})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": toDataScopeResponses(rows)})
}

func (h *Handler) AIToolPermissions(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	var mode *domain.AIToolPermissionMode
	if raw := strings.TrimSpace(c.Query("permission_mode")); raw != "" {
		t := domain.AIToolPermissionMode(raw)
		mode = &t
	}
	rows, err := h.access.ListAIToolPermissions(c.Request.Context(), domain.AIToolPermissionFilter{PermissionMode: mode})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": toAIToolPermissionResponses(rows)})
}

func (h *Handler) AdminRolePermissions(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	rows, err := h.access.ListRolePermissions(c.Request.Context(), c.Param("role_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": toPermissionResponses(rows)})
}

func (h *Handler) AdminReplaceRolePermissions(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	var req replaceIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid permission binding request")
		return
	}
	rows, err := h.access.ReplaceRolePermissions(c.Request.Context(), accessActorFromContext(c), c.Param("role_id"), req.PermissionIDs)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": toPermissionResponses(rows)})
}

func (h *Handler) AdminRoleDataScopes(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	rows, err := h.access.ListRoleDataScopes(c.Request.Context(), c.Param("role_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": toDataScopeResponses(rows)})
}

func (h *Handler) AdminReplaceRoleDataScopes(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	var req replaceIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid data scope binding request")
		return
	}
	rows, err := h.access.ReplaceRoleDataScopes(c.Request.Context(), accessActorFromContext(c), c.Param("role_id"), req.DataScopeIDs)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": toDataScopeResponses(rows)})
}

func (h *Handler) AdminRoleAIToolPermissions(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	rows, err := h.access.ListRoleAIToolPermissions(c.Request.Context(), c.Param("role_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": toAIToolPermissionResponses(rows)})
}

func (h *Handler) AdminReplaceRoleAIToolPermissions(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	var req replaceIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid ai tool permission binding request")
		return
	}
	rows, err := h.access.ReplaceRoleAIToolPermissions(c.Request.Context(), accessActorFromContext(c), c.Param("role_id"), req.ToolPermissionIDs)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": toAIToolPermissionResponses(rows)})
}

func accessActorFromContext(c *gin.Context) application.Actor {
	return application.Actor{
		UserID:      c.GetString(middleware.CtxKeyUserID),
		DisplayName: c.GetString(middleware.CtxKeyUsername),
	}
}

func toPermissionResponses(rows []domain.Permission) []permissionResponse {
	out := make([]permissionResponse, 0, len(rows))
	for _, p := range rows {
		out = append(out, permissionResponse{ID: p.ID, Code: p.Code, Name: p.Name, Resource: p.Resource, Action: p.Action, Description: p.Description})
	}
	return out
}

func toDataScopeResponses(rows []domain.DataScope) []dataScopeResponse {
	out := make([]dataScopeResponse, 0, len(rows))
	for _, sc := range rows {
		cfg := sc.ScopeConfig
		if cfg == nil {
			cfg = map[string]any{}
		}
		out = append(out, dataScopeResponse{
			ID: sc.ID, Code: sc.Code, Name: sc.Name,
			ScopeType: string(sc.ScopeType), ScopeConfig: cfg,
			Description: sc.Description,
		})
	}
	return out
}

func toAIToolPermissionResponses(rows []domain.AIToolPermission) []aiToolPermissionResponse {
	out := make([]aiToolPermissionResponse, 0, len(rows))
	for _, tp := range rows {
		out = append(out, aiToolPermissionResponse{
			ID: tp.ID, ToolCode: tp.ToolCode, ToolName: tp.ToolName,
			PermissionMode:           string(tp.PermissionMode),
			PermitsUnconfirmedInvoke: tp.PermitsUnconfirmedInvoke,
			Description:              tp.Description,
		})
	}
	return out
}
