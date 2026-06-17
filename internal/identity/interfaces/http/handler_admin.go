package http

import (
	"github.com/734965549/aiops/internal/identity/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

type createLocalUserRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type provisionExternalIdentityRequest struct {
	ProviderID       string `json:"provider_id" binding:"required"`
	ExternalSubject  string `json:"external_subject" binding:"required"`
	ExternalUsername string `json:"external_username"`
	DisplayName      string `json:"display_name"`
	Email            string `json:"email"`
	PlatformUsername string `json:"platform_username"`
	UserID           string `json:"user_id"`
}

// AdminCreateLocalUser 管理员创建本地平台账号（预留注册路径，不对外公开）。
func (h *Handler) AdminCreateLocalUser(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	var req createLocalUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "username and password are required")
		return
	}
	dto, err := h.auth.CreateLocalUser(c.Request.Context(), application.CreateLocalUserInput{
		Username:    req.Username,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		Email:       req.Email,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, dto)
}

// AdminProvisionExternalIdentity 管理员预置域账号 / 外部身份绑定。
func (h *Handler) AdminProvisionExternalIdentity(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	var req provisionExternalIdentityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "provider_id and external_subject are required")
		return
	}
	dto, err := h.auth.ProvisionExternalIdentity(c.Request.Context(), application.ProvisionExternalIdentityInput{
		ProviderID:       req.ProviderID,
		ExternalSubject:  req.ExternalSubject,
		ExternalUsername: req.ExternalUsername,
		DisplayName:      req.DisplayName,
		Email:            req.Email,
		PlatformUsername: req.PlatformUsername,
		UserID:           req.UserID,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, dto)
}

// AdminBindExternalIdentity 将外部身份绑定到已有平台用户（路径参数 user_id）。
func (h *Handler) AdminBindExternalIdentity(c *gin.Context) {
	if h.auth == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "authentication is not enabled")
		return
	}
	userID := c.Param("user_id")
	var req provisionExternalIdentityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "provider_id and external_subject are required")
		return
	}
	dto, err := h.auth.ProvisionExternalIdentity(c.Request.Context(), application.ProvisionExternalIdentityInput{
		ProviderID:       req.ProviderID,
		ExternalSubject:  req.ExternalSubject,
		ExternalUsername: req.ExternalUsername,
		DisplayName:      req.DisplayName,
		Email:            req.Email,
		UserID:           userID,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, dto)
}
