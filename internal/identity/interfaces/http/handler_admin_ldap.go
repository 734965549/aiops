package http

import (
	"strconv"

	"github.com/734965549/aiops/internal/identity/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/734965549/aiops/pkg/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type ldapConnectionRequest struct {
	ProviderID         string `json:"provider_id" binding:"required"`
	Type               string `json:"type"`
	ServerURL          string `json:"server_url" binding:"required"`
	BindDN             string `json:"bind_dn"`
	BindPassword       string `json:"bind_password"`
	BaseDN             string `json:"base_dn" binding:"required"`
	StartTLS           bool   `json:"start_tls"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	BrowseOrgFilter    string `json:"browse_org_filter"`
	BrowseUserFilter   string `json:"browse_user_filter"`
	AttrSubject        string `json:"attr_subject"`
}

type importLDAPUsersRequest struct {
	OrgDN            string   `json:"org_dn"`
	ExternalSubjects []string `json:"external_subjects"`
	ImportAll        bool     `json:"import_all"`
	RoleCodes        []string `json:"role_codes"`
}

// AdminConnectLDAPSession 使用前端填写的 LDAP/AD 连接建立短期浏览会话。
func (h *Handler) AdminConnectLDAPSession(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	var req ldapConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "provider_id, server_url and base_dn are required")
		return
	}
	adminUserID := c.GetString(middleware.CtxKeyUserID)
	result, err := h.auth.ConnectLDAPSession(c.Request.Context(), adminUserID, application.LDAPConnectionInput{
		ProviderID:         req.ProviderID,
		Type:               req.Type,
		ServerURL:          req.ServerURL,
		BindDN:             req.BindDN,
		BindPassword:       req.BindPassword,
		BaseDN:             req.BaseDN,
		StartTLS:           req.StartTLS,
		InsecureSkipVerify: req.InsecureSkipVerify,
		BrowseOrgFilter:    req.BrowseOrgFilter,
		BrowseUserFilter:   req.BrowseUserFilter,
		AttrSubject:        req.AttrSubject,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// AdminCloseLDAPSession 关闭 LDAP 浏览会话。
func (h *Handler) AdminCloseLDAPSession(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	adminUserID := c.GetString(middleware.CtxKeyUserID)
	if err := h.auth.CloseLDAPSession(c.Request.Context(), adminUserID, c.Param("session_id")); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"closed": true})
}

// AdminBrowseLDAPSessionOrganizations 浏览会话下组织单元。
func (h *Handler) AdminBrowseLDAPSessionOrganizations(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	adminUserID := c.GetString(middleware.CtxKeyUserID)
	rows, err := h.auth.BrowseLDAPSessionOrganizations(
		c.Request.Context(), adminUserID, c.Param("session_id"), c.Query("parent_dn"),
	)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"organizations": rows})
}

// AdminPreviewLDAPSessionUsers 预览会话下目录用户。
func (h *Handler) AdminPreviewLDAPSessionUsers(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	limit := 100
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	adminUserID := c.GetString(middleware.CtxKeyUserID)
	rows, err := h.auth.PreviewLDAPSessionUsers(
		c.Request.Context(), adminUserID, c.Param("session_id"), c.Query("org_dn"), limit,
	)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"users": rows})
}

// AdminImportLDAPSessionUsers 从会话连接批量导入用户并绑定角色。
func (h *Handler) AdminImportLDAPSessionUsers(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	var req importLDAPUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid import request")
		return
	}
	if !req.ImportAll && len(req.ExternalSubjects) == 0 {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "external_subjects or import_all is required")
		return
	}
	adminUserID := c.GetString(middleware.CtxKeyUserID)
	result, err := h.auth.ImportLDAPSessionUsers(c.Request.Context(), adminUserID, c.Param("session_id"), application.ImportLDAPUsersInput{
		OrgDN:            req.OrgDN,
		ExternalSubjects: req.ExternalSubjects,
		ImportAll:        req.ImportAll,
		RoleCodes:        req.RoleCodes,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}

// AdminTestLDAPConnection 测试已配置 LDAP/AD 连接。
func (h *Handler) AdminTestLDAPConnection(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	providerID := c.Param("provider_id")
	if err := h.auth.TestLDAPConnection(c.Request.Context(), providerID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"connected": true})
}

// AdminBrowseLDAPOrganizations 浏览 LDAP/AD 组织单元树。
func (h *Handler) AdminBrowseLDAPOrganizations(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	providerID := c.Param("provider_id")
	parentDN := c.Query("parent_dn")
	rows, err := h.auth.BrowseLDAPOrganizations(c.Request.Context(), providerID, parentDN)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"organizations": rows})
}

// AdminPreviewLDAPUsers 预览组织下可导入的目录用户。
func (h *Handler) AdminPreviewLDAPUsers(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	providerID := c.Param("provider_id")
	orgDN := c.Query("org_dn")
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	rows, err := h.auth.PreviewLDAPUsers(c.Request.Context(), providerID, orgDN, limit)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"users": rows})
}

// AdminImportLDAPUsers 从已配置 LDAP 身份源批量导入域账号绑定。
func (h *Handler) AdminImportLDAPUsers(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	providerID := c.Param("provider_id")
	var req importLDAPUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid import request")
		return
	}
	if !req.ImportAll && len(req.ExternalSubjects) == 0 {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "external_subjects or import_all is required")
		return
	}
	result, err := h.auth.ImportLDAPUsers(c.Request.Context(), application.ImportLDAPUsersInput{
		ProviderID:       providerID,
		OrgDN:            req.OrgDN,
		ExternalSubjects: req.ExternalSubjects,
		ImportAll:        req.ImportAll,
		RoleCodes:        req.RoleCodes,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, result)
}
