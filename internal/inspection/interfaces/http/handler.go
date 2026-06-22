package http

import (
	inspectionapp "github.com/734965549/aiops/internal/inspection/application"
	"github.com/734965549/aiops/internal/inspection/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/pagination"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	policies        *inspectionapp.PolicyService
	runs            *inspectionapp.RunService
	recommendations *inspectionapp.RecommendationService
}

func NewHandler(policies *inspectionapp.PolicyService, runs *inspectionapp.RunService, recommendations *inspectionapp.RecommendationService) *Handler {
	return &Handler{policies: policies, runs: runs, recommendations: recommendations}
}

type policyListQuery struct {
	pagination.Query
	Enabled *bool `form:"enabled"`
}

type createPolicyRequest struct {
	PolicyID             string                        `json:"policy_id"`
	Name                 string                        `json:"name" binding:"required"`
	Enabled              *bool                         `json:"enabled"`
	Schedule             string                        `json:"schedule"`
	Scope                inspectionapp.PolicyScopeDTO  `json:"scope" binding:"required"`
	Checks               []string                      `json:"checks" binding:"required"`
	AgentProfile         string                        `json:"agent_profile"`
	NotificationPolicyID string                        `json:"notification_policy_id"`
}

type updatePolicyRequest struct {
	Name                 *string                       `json:"name"`
	Enabled              *bool                         `json:"enabled"`
	Schedule             *string                       `json:"schedule"`
	Scope                *inspectionapp.PolicyScopeDTO `json:"scope"`
	Checks               []string                      `json:"checks"`
	AgentProfile         *string                       `json:"agent_profile"`
	NotificationPolicyID *string                       `json:"notification_policy_id"`
}

type findingListQuery struct {
	pagination.Query
	RunID     string `form:"run_id"`
	PolicyID  string `form:"policy_id"`
	RiskLevel string `form:"risk_level"`
}

func (h *Handler) ListPolicies(c *gin.Context) {
	var q policyListQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	items, total, err := h.policies.List(c.Request.Context(), inspectionapp.ListPoliciesQuery{
		Page: q.Page, PageSize: q.PageSize, Enabled: q.Enabled, Keyword: q.Keyword,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pagination.NewResult(items, total, q.Query))
}

func (h *Handler) GetPolicy(c *gin.Context) {
	out, err := h.policies.Get(c.Request.Context(), c.Param("policy_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) CreatePolicy(c *gin.Context) {
	var req createPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid request body")
		return
	}
	out, err := h.policies.Create(c.Request.Context(), actorFromContext(c), inspectionapp.CreatePolicyInput{
		PolicyID: req.PolicyID, Name: req.Name, Enabled: req.Enabled, Schedule: req.Schedule,
		Scope: req.Scope, Checks: req.Checks, AgentProfile: req.AgentProfile,
		NotificationPolicyID: req.NotificationPolicyID,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) UpdatePolicy(c *gin.Context) {
	var req updatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid request body")
		return
	}
	checksSet := c.Request.ContentLength > 0 && req.Checks != nil
	out, err := h.policies.Update(c.Request.Context(), actorFromContext(c), c.Param("policy_id"), inspectionapp.UpdatePolicyInput{
		Name: req.Name, Enabled: req.Enabled, Schedule: req.Schedule, Scope: req.Scope,
		Checks: req.Checks, ChecksSet: checksSet || len(req.Checks) > 0,
		AgentProfile: req.AgentProfile, NotificationPolicyID: req.NotificationPolicyID,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) DeletePolicy(c *gin.Context) {
	if err := h.policies.Delete(c.Request.Context(), actorFromContext(c), c.Param("policy_id")); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"policy_id": c.Param("policy_id")})
}

func (h *Handler) TriggerRun(c *gin.Context) {
	out, err := h.runs.TriggerRun(c.Request.Context(), actorFromContext(c), c.Param("policy_id"), domain.TriggerManual)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) GetRun(c *gin.Context) {
	out, err := h.runs.GetRun(c.Request.Context(), c.Param("run_id"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Handler) ListRuns(c *gin.Context) {
	var q pagination.Query
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	policyID := c.Query("policy_id")
	status := c.Query("status")
	items, total, err := h.runs.ListRuns(c.Request.Context(), inspectionapp.ListRunsQuery{
		Page: q.Page, PageSize: q.PageSize, PolicyID: policyID, Status: status,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pagination.NewResult(items, total, q))
}

func (h *Handler) ListFindings(c *gin.Context) {
	var q findingListQuery
	_ = c.ShouldBindQuery(&q)
	q.Normalize()
	items, total, err := h.runs.ListFindings(c.Request.Context(), inspectionapp.ListFindingsQuery{
		Page: q.Page, PageSize: q.PageSize, RunID: q.RunID, PolicyID: q.PolicyID, RiskLevel: q.RiskLevel,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, pagination.NewResult(items, total, q.Query))
}

type createExecutionFromRecommendationRequest struct {
	ExecutionMode string         `json:"execution_mode"`
	MediumID      string         `json:"medium_id" binding:"required"`
	CommandSpecID string         `json:"command_spec_id"`
	Arguments     map[string]any `json:"arguments"`
	ConfirmIntent string         `json:"confirm_intent"`
	TargetType    string         `json:"target_type"`
	TargetID      string         `json:"target_id"`
	TargetName    string         `json:"target_name"`
	Environment   string         `json:"environment"`
}

func (h *Handler) CreateExecutionFromRecommendation(c *gin.Context) {
	if h.recommendations == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "recommendation execution service is not enabled")
		return
	}
	var req createExecutionFromRecommendationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid request body")
		return
	}
	out, err := h.recommendations.CreateExecution(c.Request.Context(), actorFromContext(c), c.Param("recommendation_id"), inspectionapp.CreateExecutionFromRecommendationInput{
		ExecutionMode: req.ExecutionMode, MediumID: req.MediumID, CommandSpecID: req.CommandSpecID,
		Arguments: req.Arguments, ConfirmIntent: req.ConfirmIntent, TargetType: req.TargetType,
		TargetID: req.TargetID, TargetName: req.TargetName, Environment: req.Environment,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}
