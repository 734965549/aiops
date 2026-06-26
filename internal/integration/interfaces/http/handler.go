package http

import (
	integapp "github.com/734965549/aiops/internal/integration/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/pagination"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

// Handler 云账号/观测平台接入 HTTP 层。
type Handler struct {
	accounts *integapp.AccountService
}

func NewHandler(accounts *integapp.AccountService) *Handler {
	return &Handler{accounts: accounts}
}

type accountListQuery struct {
	pagination.Query
	Provider string `form:"provider"`
	Enabled  *bool  `form:"enabled"`
}

type credentialInput map[string]string

type createAccountRequest struct {
	AccountID   string          `json:"account_id"`
	Name        string          `json:"name" binding:"required"`
	Provider    string          `json:"provider" binding:"required"`
	AuthType    string          `json:"auth_type" binding:"required"`
	Regions     []string        `json:"regions"`
	ProjectID   string          `json:"project_id"`
	Credential  credentialInput `json:"credential"`
	Enabled     *bool           `json:"enabled"`
	OwnerTeam   string          `json:"owner_team"`
	Description string          `json:"description"`
	ExtraConfig map[string]any  `json:"extra_config"`
}

type updateAccountRequest struct {
	Name        *string         `json:"name"`
	Provider    *string         `json:"provider"`
	AuthType    *string         `json:"auth_type"`
	Regions     []string        `json:"regions"`
	ProjectID   *string         `json:"project_id"`
	Credential  credentialInput `json:"credential"`
	Enabled     *bool           `json:"enabled"`
	OwnerTeam   *string         `json:"owner_team"`
	Description *string         `json:"description"`
	ExtraConfig map[string]any  `json:"extra_config"`
}

// ListAccounts GET /api/integrations/accounts（§4.2 分页列表）。
func (h *Handler) ListAccounts(c *gin.Context) {
	if h.accounts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "integration service is not enabled")
		return
	}
	var q accountListQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	items, total, err := h.accounts.List(c.Request.Context(), integapp.ListAccountsQuery{
		Page: q.Page, PageSize: q.PageSize, Provider: q.Provider, Enabled: q.Enabled, Keyword: q.Keyword,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pagination.NewResult(items, total, q.Query))
}

func (h *Handler) GetAccount(c *gin.Context) {
	if h.accounts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "integration service is not enabled")
		return
	}
	out, err := h.accounts.Get(c.Request.Context(), c.Param("account_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// CreateAccount POST /api/integrations/accounts（§4.1）；credential 仅写入，不在响应中回显。
func (h *Handler) CreateAccount(c *gin.Context) {
	if h.accounts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "integration service is not enabled")
		return
	}
	var req createAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid request body")
		return
	}
	out, err := h.accounts.Create(c.Request.Context(), actorFromContext(c), integapp.CreateAccountInput{
		AccountID: req.AccountID, Name: req.Name, Provider: req.Provider, AuthType: req.AuthType,
		Regions: req.Regions, ProjectID: req.ProjectID, Credential: req.Credential,
		Enabled: req.Enabled, OwnerTeam: req.OwnerTeam, Description: req.Description,
		ExtraConfig: req.ExtraConfig,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) UpdateAccount(c *gin.Context) {
	if h.accounts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "integration service is not enabled")
		return
	}
	var req updateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid request body")
		return
	}
	in := integapp.UpdateAccountInput{
		Name: req.Name, Provider: req.Provider, AuthType: req.AuthType, ProjectID: req.ProjectID,
		Credential: req.Credential, Enabled: req.Enabled, OwnerTeam: req.OwnerTeam, Description: req.Description,
	}
	if req.ExtraConfig != nil {
		in.ExtraConfig = req.ExtraConfig
		in.ExtraConfigSet = true
	}
	if req.Regions != nil {
		in.Regions = req.Regions
		in.RegionsSet = true
	}
	out, err := h.accounts.Update(c.Request.Context(), c.Param("account_id"), actorFromContext(c), in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) DeleteAccount(c *gin.Context) {
	if h.accounts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "integration service is not enabled")
		return
	}
	if err := h.accounts.Delete(c.Request.Context(), c.Param("account_id"), actorFromContext(c)); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"account_id": c.Param("account_id"), "deleted": true})
}

// CheckConnectivity POST /api/integrations/accounts/:account_id/check（§4.5）。
func (h *Handler) CheckConnectivity(c *gin.Context) {
	if h.accounts == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "integration service is not enabled")
		return
	}
	out, err := h.accounts.CheckConnectivity(c.Request.Context(), c.Param("account_id"), actorFromContext(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
