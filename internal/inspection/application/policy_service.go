package application

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/inspection/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// PolicyService 巡检策略 CRUD。
type PolicyService struct {
	policies domain.PolicyRepository
	appCatalog ApplicationCatalogPort
	audit    AuditRecorder
}

func NewPolicyService(policies domain.PolicyRepository, appCatalog ApplicationCatalogPort, audit AuditRecorder) *PolicyService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &PolicyService{policies: policies, appCatalog: appCatalog, audit: audit}
}

type CreatePolicyInput struct {
	PolicyID             string
	Name                 string
	Enabled              *bool
	Schedule             string
	Scope                PolicyScopeDTO
	Checks               []string
	AgentProfile         string
	NotificationPolicyID string
}

type UpdatePolicyInput struct {
	Name                 *string
	Enabled              *bool
	Schedule             *string
	Scope                *PolicyScopeDTO
	Checks               []string
	ChecksSet            bool
	AgentProfile         *string
	NotificationPolicyID *string
}

type ListPoliciesQuery struct {
	Page     int
	PageSize int
	Enabled  *bool
	Keyword  string
}

func (s *PolicyService) Create(ctx context.Context, actor Actor, in CreatePolicyInput) (*PolicyDTO, error) {
	if s == nil || s.policies == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "inspection service is not enabled")
	}
	policyID := strings.TrimSpace(in.PolicyID)
	if policyID == "" {
		policyID = "pol-" + uuid.NewString()
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	agentProfile := strings.TrimSpace(in.AgentProfile)
	if agentProfile == "" {
		agentProfile = "sre_default"
	}
	p := &domain.InspectionPolicy{
		PolicyID: policyID, Name: strings.TrimSpace(in.Name), Enabled: enabled,
		Schedule: strings.TrimSpace(in.Schedule), Scope: scopeFromDTO(in.Scope),
		Checks: in.Checks, AgentProfile: agentProfile, NotificationPolicyID: strings.TrimSpace(in.NotificationPolicyID),
	}
	if err := p.Validate(); err != nil {
		return nil, mapDomainErr(err)
	}
	if err := s.validateScopeApplicationIDs(ctx, p.Scope.ApplicationIDs); err != nil {
		return nil, err
	}
	if err := s.policies.Create(ctx, p); err != nil {
		return nil, mapDomainErr(err)
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "inspection_policy", ResourceID: policyID, Action: AuditPolicyCreate, UserID: actor.UserID,
		Payload: map[string]any{"name": p.Name, "checks_count": len(p.Checks), "account_id": p.Scope.AccountID},
	})
	dto := toPolicyDTO(p)
	return &dto, nil
}

func (s *PolicyService) Update(ctx context.Context, actor Actor, policyID string, in UpdatePolicyInput) (*PolicyDTO, error) {
	if s == nil || s.policies == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "inspection service is not enabled")
	}
	p, err := s.policies.GetByID(ctx, policyID)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	if in.Name != nil {
		p.Name = strings.TrimSpace(*in.Name)
	}
	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}
	if in.Schedule != nil {
		p.Schedule = strings.TrimSpace(*in.Schedule)
	}
	if in.Scope != nil {
		p.Scope = scopeFromDTO(*in.Scope)
	}
	if in.ChecksSet {
		p.Checks = in.Checks
	}
	if in.AgentProfile != nil {
		p.AgentProfile = strings.TrimSpace(*in.AgentProfile)
	}
	if in.NotificationPolicyID != nil {
		p.NotificationPolicyID = strings.TrimSpace(*in.NotificationPolicyID)
	}
	if err := p.Validate(); err != nil {
		return nil, mapDomainErr(err)
	}
	if err := s.validateScopeApplicationIDs(ctx, p.Scope.ApplicationIDs); err != nil {
		return nil, err
	}
	if err := s.policies.Update(ctx, p); err != nil {
		return nil, mapDomainErr(err)
	}
	action := AuditPolicyUpdate
	if in.Enabled != nil {
		if *in.Enabled {
			action = AuditPolicyEnable
		} else {
			action = AuditPolicyDisable
		}
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "inspection_policy", ResourceID: policyID, Action: action, UserID: actor.UserID,
		Payload: map[string]any{"enabled": p.Enabled},
	})
	dto := toPolicyDTO(p)
	return &dto, nil
}

func (s *PolicyService) Get(ctx context.Context, policyID string) (*PolicyDTO, error) {
	if s == nil || s.policies == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "inspection service is not enabled")
	}
	p, err := s.policies.GetByID(ctx, policyID)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	dto := toPolicyDTO(p)
	return &dto, nil
}

func (s *PolicyService) List(ctx context.Context, q ListPoliciesQuery) ([]PolicyDTO, int64, error) {
	if s == nil || s.policies == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "inspection service is not enabled")
	}
	filter := domain.PolicyFilter{Enabled: q.Enabled, Keyword: q.Keyword, Limit: q.PageSize, Offset: (q.Page - 1) * q.PageSize}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	items, err := s.policies.List(ctx, filter)
	if err != nil {
		return nil, 0, mapDomainErr(err)
	}
	total, err := s.policies.Count(ctx, filter)
	if err != nil {
		return nil, 0, mapDomainErr(err)
	}
	out := make([]PolicyDTO, 0, len(items))
	for i := range items {
		out = append(out, toPolicyDTO(&items[i]))
	}
	return out, total, nil
}

func (s *PolicyService) Delete(ctx context.Context, actor Actor, policyID string) error {
	if s == nil || s.policies == nil {
		return apperr.New(apperr.CodeUnavailable, "inspection service is not enabled")
	}
	if err := s.policies.SoftDelete(ctx, policyID); err != nil {
		return mapDomainErr(err)
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "inspection_policy", ResourceID: policyID, Action: AuditPolicyDelete, UserID: actor.UserID,
	})
	return nil
}

func scopeFromDTO(d PolicyScopeDTO) domain.PolicyScope {
	return domain.PolicyScope{
		Environment: d.Environment, AccountID: strings.TrimSpace(d.AccountID),
		Provider: strings.TrimSpace(d.Provider), ApplicationIDs: normalizeApplicationIDs(d.ApplicationIDs), ResourceTypes: d.ResourceTypes,
	}
}

func normalizeApplicationIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (s *PolicyService) validateScopeApplicationIDs(ctx context.Context, ids []string) error {
	if s == nil || s.appCatalog == nil || len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		exists, err := s.appCatalog.ExistsByID(ctx, id)
		if err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "validate application_id failed")
		}
		if !exists {
			return apperr.Newf(apperr.CodeInvalidArgument, "application_id %q not found", id)
		}
	}
	return nil
}

func mapDomainErr(err error) error {
	if err == nil {
		return nil
	}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apperr.New(apperr.CodeNotFound, err.Error())
	case errors.Is(err, domain.ErrAlreadyExists):
		return apperr.New(apperr.CodeAlreadyExists, err.Error())
	case errors.Is(err, domain.ErrInvalidArgument), errors.Is(err, domain.ErrScopeIncomplete), errors.Is(err, domain.ErrUnsupportedCheck):
		return apperr.New(apperr.CodeInvalidArgument, err.Error())
	case errors.Is(err, domain.ErrPolicyDisabled):
		return apperr.New(apperr.CodeFailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		return apperr.New(apperr.CodeFailedPrecondition, err.Error())
	default:
		return apperr.Wrap(err, apperr.CodeInternal, "inspection operation failed")
	}
}
