// Package http 是 Identity 上下文的 HTTP 适配层。
//
// 这里只做参数解析与响应封装，不写业务规则；具体用例在 application 层。
package http

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/auth"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/734965549/aiops/pkg/pagination"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UserReader 定义当前用户查询能力。
type UserReader interface {
	GetCurrentUser(ctx context.Context, userID string) (*application.CurrentUserDTO, error)
}

// AccessControlReader 定义权限域只读查询能力。
type AccessControlReader interface {
	ListUsers(ctx context.Context, filter domain.UserFilter) ([]application.UserDTO, int64, error)
	ListRoles(ctx context.Context, filter domain.RoleFilter) ([]domain.Role, error)
	CountRoles(ctx context.Context, filter domain.RoleFilter) (int64, error)
	ListPermissions(ctx context.Context, filter domain.PermissionFilter) ([]domain.Permission, error)
	CountPermissions(ctx context.Context, filter domain.PermissionFilter) (int64, error)
	ListUserRoles(ctx context.Context, userID string) ([]domain.Role, error)
	ListUserRoleBindings(ctx context.Context, userID string) ([]application.UserRoleBindingDTO, error)
	ReplaceUserManualRoles(ctx context.Context, actor application.Actor, userID string, roleIDs []string) ([]application.UserRoleBindingDTO, error)
	ListRolePermissions(ctx context.Context, roleID string) ([]domain.Permission, error)
	ReplaceRolePermissions(ctx context.Context, actor application.Actor, roleID string, permissionIDs []string) ([]domain.Permission, error)
	ListDataScopes(ctx context.Context, filter domain.DataScopeFilter) ([]domain.DataScope, error)
	ListRoleDataScopes(ctx context.Context, roleID string) ([]domain.DataScope, error)
	ReplaceRoleDataScopes(ctx context.Context, actor application.Actor, roleID string, dataScopeIDs []string) ([]domain.DataScope, error)
	ListAIToolPermissions(ctx context.Context, filter domain.AIToolPermissionFilter) ([]domain.AIToolPermission, error)
	ListRoleAIToolPermissions(ctx context.Context, roleID string) ([]domain.AIToolPermission, error)
	ReplaceRoleAIToolPermissions(ctx context.Context, actor application.Actor, roleID string, toolPermissionIDs []string) ([]domain.AIToolPermission, error)
}

// Handler 持有用例依赖，提供 Gin handler 方法。
type AuthorizationReader interface {
	Authorize(ctx context.Context, input application.AuthorizationInput) (*application.AuthorizationResult, error)
}

// AuthAuditService 係 HTTP 层依赖嘅认证审计服务，只暴露记录同管理员查询能力。
type AuthAuditService interface {
	Record(ctx context.Context, audit domain.AuthAudit) error
	List(ctx context.Context, filter domain.AuthAuditFilter) ([]domain.AuthAudit, error)
	Count(ctx context.Context, filter domain.AuthAuditFilter) (int64, error)
}

// LoginIPAllowlist 负责判断登录相关入口係咪允许当前客户端 IP 访问。
type LoginIPAllowlist interface {
	Enabled() bool
	Allows(ip string) bool
}

type Handler struct {
	users        UserReader
	auth         *application.AuthService
	access       AccessControlReader
	authorizer   AuthorizationReader
	loginLimiter auth.LoginAttemptLimiter
	authAudit    AuthAuditService
	loginIPs     LoginIPAllowlist
}

// NewHandler 构造 Handler；auth / access / authorizer / loginLimiter / authAudit / loginIPs 可以为 nil，表示对应能力未启用。
func NewHandler(users UserReader, auth *application.AuthService, access AccessControlReader, authorizer AuthorizationReader, loginLimiter auth.LoginAttemptLimiter, authAudit AuthAuditService, loginIPs LoginIPAllowlist) *Handler {
	return &Handler{users: users, auth: auth, access: access, authorizer: authorizer, loginLimiter: loginLimiter, authAudit: authAudit, loginIPs: loginIPs}
}

// loginRequest 是 /api/identity/login 的入参。
type loginRequest struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	ProviderID string `json:"provider_id"`
}

// externalLoginRequest 是 /api/identity/login/external 的入参。
type externalLoginRequest struct {
	ProviderID string `json:"provider_id" binding:"required"`
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

// oauthCallbackRequest 是 OAuth 回调 JSON 入参（前端代理回调时使用）。
type oauthCallbackRequest struct {
	ProviderID string `json:"provider_id" binding:"required"`
	Code       string `json:"code" binding:"required"`
	State      string `json:"state"`
}

// Login 接收用户名密码，返回 access/refresh token 对。
func (h *Handler) Login(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "username and password are required")
		return
	}
	ctx := c.Request.Context()
	if err := h.requireLoginIP(c); err != nil {
		h.recordAuthAudit(ctx, c, domain.AuthAudit{
			Username:   req.Username,
			ProviderID: req.ProviderID,
			Event:      domain.AuthAuditEventLogin,
			Method:     loginAuditMethod(req.ProviderID),
			Result:     domain.AuthAuditResultFailure,
			Reason:     authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	if err := h.allowLoginAttempt(ctx, c.ClientIP(), req.Username); err != nil {
		h.recordAuthAudit(ctx, c, domain.AuthAudit{
			Username:   req.Username,
			ProviderID: req.ProviderID,
			Event:      domain.AuthAuditEventLogin,
			Method:     loginAuditMethod(req.ProviderID),
			Result:     domain.AuthAuditResultFailure,
			Reason:     authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	var tokens *application.TokenPair
	var err error
	if strings.TrimSpace(req.ProviderID) != "" {
		tokens, err = h.auth.LoginExternal(ctx, application.ExternalLoginInput{
			ProviderID: req.ProviderID,
			Username:   req.Username,
			Password:   req.Password,
		})
	} else {
		tokens, err = h.auth.Login(ctx, application.LoginInput{
			Username: req.Username,
			Password: req.Password,
		})
	}
	if err != nil {
		if isAuthFailure(err) {
			_ = h.recordLoginFailure(ctx, c.ClientIP(), req.Username)
		}
		h.recordAuthAudit(ctx, c, domain.AuthAudit{
			Username:   req.Username,
			ProviderID: req.ProviderID,
			Event:      domain.AuthAuditEventLogin,
			Method:     loginAuditMethod(req.ProviderID),
			Result:     domain.AuthAuditResultFailure,
			Reason:     authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	_ = h.recordLoginSuccess(ctx, c.ClientIP(), req.Username)
	h.recordAuthAudit(ctx, c, domain.AuthAudit{
		UserID:     tokenUserID(tokens),
		Username:   tokenUsername(tokens, req.Username),
		ProviderID: req.ProviderID,
		Event:      domain.AuthAuditEventLogin,
		Method:     loginAuditMethod(req.ProviderID),
		Result:     domain.AuthAuditResultSuccess,
	})
	httpx.OK(c, tokens)
}

// LoginExternal LDAP/AD 等企业身份源登录。
func (h *Handler) LoginExternal(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	var req externalLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "provider_id, username and password are required")
		return
	}
	ctx := c.Request.Context()
	if err := h.requireLoginIP(c); err != nil {
		h.recordAuthAudit(ctx, c, domain.AuthAudit{
			Username:   req.Username,
			ProviderID: req.ProviderID,
			Event:      domain.AuthAuditEventLogin,
			Method:     domain.AuthAuditMethodExternal,
			Result:     domain.AuthAuditResultFailure,
			Reason:     authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	if err := h.allowLoginAttempt(ctx, c.ClientIP(), req.Username); err != nil {
		h.recordAuthAudit(ctx, c, domain.AuthAudit{
			Username:   req.Username,
			ProviderID: req.ProviderID,
			Event:      domain.AuthAuditEventLogin,
			Method:     domain.AuthAuditMethodExternal,
			Result:     domain.AuthAuditResultFailure,
			Reason:     authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	tokens, err := h.auth.LoginExternal(ctx, application.ExternalLoginInput{
		ProviderID: req.ProviderID,
		Username:   req.Username,
		Password:   req.Password,
	})
	if err != nil {
		if isAuthFailure(err) {
			_ = h.recordLoginFailure(ctx, c.ClientIP(), req.Username)
		}
		h.recordAuthAudit(ctx, c, domain.AuthAudit{
			Username:   req.Username,
			ProviderID: req.ProviderID,
			Event:      domain.AuthAuditEventLogin,
			Method:     domain.AuthAuditMethodExternal,
			Result:     domain.AuthAuditResultFailure,
			Reason:     authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	_ = h.recordLoginSuccess(ctx, c.ClientIP(), req.Username)
	h.recordAuthAudit(ctx, c, domain.AuthAudit{
		UserID:     tokenUserID(tokens),
		Username:   tokenUsername(tokens, req.Username),
		ProviderID: req.ProviderID,
		Event:      domain.AuthAuditEventLogin,
		Method:     domain.AuthAuditMethodExternal,
		Result:     domain.AuthAuditResultSuccess,
	})
	httpx.OK(c, tokens)
}

// ListLoginProviders 返回已启用的企业身份源列表。
func (h *Handler) ListLoginProviders(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	httpx.OK(c, gin.H{"providers": h.auth.ListIdentityProviders()})
}

// OAuthAuthorize 发起 OAuth2/OIDC 授权，返回跳转 URL。
func (h *Handler) OAuthAuthorize(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	providerID := c.Param("provider_id")
	if err := h.requireLoginIP(c); err != nil {
		httpx.Fail(c, err)
		return
	}
	url, state, err := h.auth.OAuthAuthorizeURLWithContext(c.Request.Context(), application.OAuthAuthorizeInput{
		ProviderID: providerID,
		ClientIP:   c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"authorization_url": url, "state": state})
}

// OAuthCallback 处理 OAuth2/OIDC 授权码回调。
func (h *Handler) OAuthCallback(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	providerID := c.Param("provider_id")
	code := c.Query("code")
	state := c.Query("state")
	if code == "" {
		var req oauthCallbackRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.FailWith(c, apperr.CodeInvalidArgument, "code is required")
			return
		}
		providerID = req.ProviderID
		code = req.Code
		state = req.State
	}
	if err := h.requireLoginIP(c); err != nil {
		h.recordAuthAudit(c.Request.Context(), c, domain.AuthAudit{
			ProviderID: providerID,
			Event:      domain.AuthAuditEventLogin,
			Method:     domain.AuthAuditMethodOAuth,
			Result:     domain.AuthAuditResultFailure,
			Reason:     authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	tokens, err := h.auth.LoginOAuthCallback(c.Request.Context(), application.OAuthCallbackInput{
		ProviderID: providerID,
		Code:       code,
		State:      state,
		ClientIP:   c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
	})
	if err != nil {
		h.recordAuthAudit(c.Request.Context(), c, domain.AuthAudit{
			ProviderID: providerID,
			Event:      domain.AuthAuditEventLogin,
			Method:     domain.AuthAuditMethodOAuth,
			Result:     domain.AuthAuditResultFailure,
			Reason:     authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	h.recordAuthAudit(c.Request.Context(), c, domain.AuthAudit{
		UserID:     tokenUserID(tokens),
		Username:   tokenUsername(tokens, ""),
		ProviderID: providerID,
		Event:      domain.AuthAuditEventLogin,
		Method:     domain.AuthAuditMethodOAuth,
		Result:     domain.AuthAuditResultSuccess,
	})
	httpx.OK(c, tokens)
}

// refreshRequest 是 /api/identity/refresh 的入参。
type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh 用 refresh token 换新的 token 对。
func (h *Handler) Refresh(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "refresh_token is required")
		return
	}
	ctx := c.Request.Context()
	if err := h.requireLoginIP(c); err != nil {
		h.recordAuthAudit(ctx, c, domain.AuthAudit{
			Event:  domain.AuthAuditEventRefresh,
			Method: domain.AuthAuditMethodRefresh,
			Result: domain.AuthAuditResultFailure,
			Reason: authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	if err := h.allowLoginAttempt(ctx, c.ClientIP(), ""); err != nil {
		h.recordAuthAudit(ctx, c, domain.AuthAudit{
			Event:  domain.AuthAuditEventRefresh,
			Method: domain.AuthAuditMethodRefresh,
			Result: domain.AuthAuditResultFailure,
			Reason: authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	tokens, err := h.auth.Refresh(ctx, req.RefreshToken)
	if err != nil {
		if isAuthFailure(err) {
			_ = h.recordLoginFailure(ctx, c.ClientIP(), "")
		}
		h.recordAuthAudit(ctx, c, domain.AuthAudit{
			Event:  domain.AuthAuditEventRefresh,
			Method: domain.AuthAuditMethodRefresh,
			Result: domain.AuthAuditResultFailure,
			Reason: authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	h.recordAuthAudit(ctx, c, domain.AuthAudit{
		UserID:   tokenUserID(tokens),
		Username: tokenUsername(tokens, ""),
		Event:    domain.AuthAuditEventRefresh,
		Method:   domain.AuthAuditMethodRefresh,
		Result:   domain.AuthAuditResultSuccess,
	})
	httpx.OK(c, tokens)
}

// Logout 吊销 refresh token。
func (h *Handler) Logout(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "refresh_token is required")
		return
	}
	if err := h.requireLoginIP(c); err != nil {
		h.recordAuthAudit(c.Request.Context(), c, domain.AuthAudit{
			Event:  domain.AuthAuditEventLogout,
			Method: domain.AuthAuditMethodRefresh,
			Result: domain.AuthAuditResultFailure,
			Reason: authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	if err := h.auth.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		h.recordAuthAudit(c.Request.Context(), c, domain.AuthAudit{
			Event:  domain.AuthAuditEventLogout,
			Method: domain.AuthAuditMethodRefresh,
			Result: domain.AuthAuditResultFailure,
			Reason: authAuditReason(err),
		})
		httpx.Fail(c, err)
		return
	}
	h.recordAuthAudit(c.Request.Context(), c, domain.AuthAudit{
		Event:  domain.AuthAuditEventLogout,
		Method: domain.AuthAuditMethodRefresh,
		Result: domain.AuthAuditResultSuccess,
	})
	httpx.OK(c, gin.H{"logged_out": true})
}

// GetCurrentUser 返回当前已认证用户的信息（user_id 来自 JWT 中间件）。
func (h *Handler) GetCurrentUser(c *gin.Context) {
	userID := c.GetString(middleware.CtxKeyUserID)
	dto, err := h.users.GetCurrentUser(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, dto)
}

type listQuery struct {
	pagination.Query
	Status   string `form:"status"`
	IsSystem *bool  `form:"is_system"`
	Resource string `form:"resource"`
	Action   string `form:"action"`
}

type authAuditQuery struct {
	pagination.Query
	UserID     string `form:"user_id"`
	Username   string `form:"username"`
	ProviderID string `form:"provider_id"`
	Event      string `form:"event"`
	Result     string `form:"result"`
}

type authAuditResponse struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Username   string `json:"username"`
	ProviderID string `json:"provider_id"`
	Event      string `json:"event"`
	Method     string `json:"method"`
	Result     string `json:"result"`
	IP         string `json:"ip"`
	UserAgent  string `json:"user_agent"`
	Reason     string `json:"reason"`
	CreatedAt  int64  `json:"created_at"`
}

// roleResponse 是角色列表响应。
type roleResponse struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	IsSystem    bool   `json:"is_system"`
}

// permissionResponse 是权限列表响应。
type permissionResponse struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Description string `json:"description"`
}

// Roles 返回角色列表。
func (h *Handler) Roles(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	var q listQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	var st *domain.RoleStatus
	if q.Status != "" {
		t := domain.RoleStatus(q.Status)
		st = &t
	}
	filter := domain.RoleFilter{Status: st, IsSystem: q.IsSystem, Limit: q.Limit(), Offset: q.Offset()}
	rows, err := h.access.ListRoles(c.Request.Context(), filter)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	total, err := h.access.CountRoles(c.Request.Context(), filter)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	resp := make([]roleResponse, 0, len(rows))
	for _, r := range rows {
		resp = append(resp, roleResponse{ID: r.ID, Code: r.Code, Name: r.Name, Description: r.Description, Status: string(r.Status), IsSystem: r.IsSystem})
	}
	httpx.OK(c, pagination.NewResult(resp, total, q.Query))
}

// Permissions 返回权限列表。
func (h *Handler) Permissions(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	var q listQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	filter := domain.PermissionFilter{Resource: q.Resource, Action: q.Action, Limit: q.Limit(), Offset: q.Offset()}
	rows, err := h.access.ListPermissions(c.Request.Context(), filter)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	total, err := h.access.CountPermissions(c.Request.Context(), filter)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	resp := make([]permissionResponse, 0, len(rows))
	for _, p := range rows {
		resp = append(resp, permissionResponse{ID: p.ID, Code: p.Code, Name: p.Name, Resource: p.Resource, Action: p.Action, Description: p.Description})
	}
	httpx.OK(c, pagination.NewResult(resp, total, q.Query))
}

// AuthAudits 返回认证审计列表俾管理员查询同排障。
func (h *Handler) AuthAudits(c *gin.Context) {
	if h.authAudit == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "auth audit is not enabled")
		return
	}
	var q authAuditQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	filter := domain.AuthAuditFilter{
		UserID:     q.UserID,
		Username:   q.Username,
		ProviderID: q.ProviderID,
		Event:      domain.AuthAuditEvent(q.Event),
		Result:     domain.AuthAuditResult(q.Result),
		Limit:      q.Limit(),
		Offset:     q.Offset(),
	}
	rows, err := h.authAudit.List(c.Request.Context(), filter)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	total, err := h.authAudit.Count(c.Request.Context(), filter)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	resp := make([]authAuditResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, authAuditResponse{
			ID:         row.ID,
			UserID:     row.UserID,
			Username:   row.Username,
			ProviderID: row.ProviderID,
			Event:      string(row.Event),
			Method:     string(row.Method),
			Result:     string(row.Result),
			IP:         row.IP,
			UserAgent:  row.UserAgent,
			Reason:     row.Reason,
			CreatedAt:  row.CreatedAt.Unix(),
		})
	}
	httpx.OK(c, pagination.NewResult(resp, total, q.Query))
}

// MeRoles 返回当前用户绑定的角色。
func (h *Handler) MeRoles(c *gin.Context) {
	if h.access == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "access control is not enabled")
		return
	}
	userID := c.GetString(middleware.CtxKeyUserID)
	rows, err := h.access.ListUserRoles(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	resp := make([]roleResponse, 0, len(rows))
	for _, r := range rows {
		resp = append(resp, roleResponse{ID: r.ID, Code: r.Code, Name: r.Name, Description: r.Description, Status: string(r.Status), IsSystem: r.IsSystem})
	}
	httpx.OK(c, pagination.Result[roleResponse]{
		Items: resp, Total: int64(len(resp)), Page: 1, PageSize: len(resp),
	})
}

// Authorize 返回一次统一授权判断结果，供前端或工具网关调试。
func (h *Handler) Authorize(c *gin.Context) {
	if h.authorizer == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authorization is not enabled")
		return
	}
	var in application.AuthorizationInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid authorization request")
		return
	}
	in.UserID = c.GetString(middleware.CtxKeyUserID)
	res, err := h.authorizer.Authorize(c.Request.Context(), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, res)
}

func (h *Handler) allowLoginAttempt(ctx context.Context, ip, username string) error {
	if h.loginLimiter == nil {
		return nil
	}
	return h.loginLimiter.Allow(ctx, ip, username)
}

func (h *Handler) recordLoginFailure(ctx context.Context, ip, username string) error {
	if h.loginLimiter == nil {
		return nil
	}
	return h.loginLimiter.RecordFailure(ctx, ip, username)
}

func (h *Handler) recordLoginSuccess(ctx context.Context, ip, username string) error {
	if h.loginLimiter == nil {
		return nil
	}
	return h.loginLimiter.RecordSuccess(ctx, ip, username)
}

func (h *Handler) requireLoginIP(c *gin.Context) error {
	if h.loginIPs == nil || !h.loginIPs.Enabled() {
		return nil
	}
	if h.loginIPs.Allows(c.ClientIP()) {
		return nil
	}
	return apperr.New(apperr.CodePermissionDenied, "client ip is not allowed")
}

func (h *Handler) recordAuthAudit(ctx context.Context, c *gin.Context, audit domain.AuthAudit) {
	if h.authAudit == nil {
		return
	}
	if c != nil {
		audit.IP = c.ClientIP()
		audit.UserAgent = c.Request.UserAgent()
	}
	if err := h.authAudit.Record(ctx, audit); err != nil {
		logger.From(ctx).Warn("record auth audit failed",
			zap.String("event", string(audit.Event)),
			zap.String("result", string(audit.Result)),
			zap.Error(err),
		)
	}
}

func loginAuditMethod(providerID string) domain.AuthAuditMethod {
	if strings.TrimSpace(providerID) != "" {
		return domain.AuthAuditMethodExternal
	}
	return domain.AuthAuditMethodLocal
}

func tokenUserID(tokens *application.TokenPair) string {
	if tokens == nil || tokens.User == nil {
		return ""
	}
	return tokens.User.ID
}

func tokenUsername(tokens *application.TokenPair, fallback string) string {
	if tokens != nil && tokens.User != nil && strings.TrimSpace(tokens.User.Username) != "" {
		return tokens.User.Username
	}
	return fallback
}

func authAuditReason(err error) string {
	app := apperr.FromError(err)
	if app == nil {
		return ""
	}
	if app.Code == apperr.CodeUnauthenticated {
		return "authentication failed"
	}
	return app.Message
}

func isAuthFailure(err error) bool {
	var app *apperr.Error
	if !errors.As(err, &app) {
		return false
	}
	return app.Code == apperr.CodeUnauthenticated
}
