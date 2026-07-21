package application

import (
	"context"
	"strings"
	"time"

	rbdomain "github.com/734965549/aiops/internal/runbook/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// Create 创建模板与步骤。
func (s *TemplateService) Create(ctx context.Context, actor Actor, in CreateTemplateInput) (*TemplateDetailDTO, error) {
	if s == nil || s.templates == nil || s.steps == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "runbook service is not enabled")
	}
	op, err := validateOperationType(in.OperationType)
	if err != nil {
		return nil, err
	}
	risk, err := validateRiskLevel(in.RiskLevel)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "name is required")
	}
	if len(in.Steps) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "at least one step is required")
	}
	now := s.now()
	templateID := uuid.NewString()
	tpl := &rbdomain.Template{
		ID:                templateID,
		Name:              name,
		Description:       strings.TrimSpace(in.Description),
		Enabled:           in.Enabled,
		OperationType:     op,
		RiskLevel:         risk,
		MatchAlertName:    strings.TrimSpace(in.MatchAlertName),
		MatchResourceType: strings.TrimSpace(in.MatchResourceType),
		MatchEnvironment:  strings.TrimSpace(in.MatchEnvironment),
		ParameterSchema:   cloneAnyMap(in.ParameterSchema),
		RollbackPlan:      cloneAnyMap(in.RollbackPlan),
		CreatedBy:         strings.TrimSpace(actor.UserID),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	steps, err := s.buildSteps(templateID, in.Steps, now)
	if err != nil {
		return nil, err
	}
	if err := s.templates.CreateWithSteps(ctx, tpl, steps); err != nil {
		return nil, wrapRBError(err, "create runbook template failed")
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "runbook",
		ResourceID:   templateID,
		Action:       AuditCreate,
		UserID:       actor.UserID,
		Payload:      map[string]any{"name": name},
	})
	return s.toDetailDTO(*tpl, steps), nil
}

// Update 更新模板（步骤全量替换）。
func (s *TemplateService) Update(ctx context.Context, templateID string, actor Actor, in UpdateTemplateInput) (*TemplateDetailDTO, error) {
	if s == nil || s.templates == nil || s.steps == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "runbook service is not enabled")
	}
	templateID = strings.TrimSpace(templateID)
	existing, err := s.templates.GetByID(ctx, templateID)
	if err != nil {
		return nil, wrapRBError(err, "load runbook template failed")
	}
	op, err := validateOperationType(in.OperationType)
	if err != nil {
		return nil, err
	}
	risk, err := validateRiskLevel(in.RiskLevel)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "name is required")
	}
	if len(in.Steps) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "at least one step is required")
	}
	now := s.now()
	enabledBefore := existing.Enabled
	existing.Name = name
	existing.Description = strings.TrimSpace(in.Description)
	existing.Enabled = in.Enabled
	existing.OperationType = op
	existing.RiskLevel = risk
	existing.MatchAlertName = strings.TrimSpace(in.MatchAlertName)
	existing.MatchResourceType = strings.TrimSpace(in.MatchResourceType)
	existing.MatchEnvironment = strings.TrimSpace(in.MatchEnvironment)
	existing.ParameterSchema = cloneAnyMap(in.ParameterSchema)
	existing.RollbackPlan = cloneAnyMap(in.RollbackPlan)
	existing.UpdatedAt = now
	steps, err := s.buildSteps(templateID, in.Steps, now)
	if err != nil {
		return nil, err
	}
	if err := s.templates.ReplaceWithSteps(ctx, existing, steps); err != nil {
		return nil, wrapRBError(err, "update runbook template failed")
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "runbook",
		ResourceID:   templateID,
		Action:       AuditUpdate,
		UserID:       actor.UserID,
		Payload:      map[string]any{"name": name},
	})
	if enabledBefore != existing.Enabled {
		action := AuditDisable
		if existing.Enabled {
			action = AuditEnable
		}
		_ = s.audit.Record(ctx, AuditRecord{
			ResourceType: "runbook",
			ResourceID:   templateID,
			Action:       action,
			UserID:       actor.UserID,
			Payload:      map[string]any{"enabled": existing.Enabled},
		})
	}
	return s.toDetailDTO(*existing, steps), nil
}

// Delete 删除模板。
func (s *TemplateService) Delete(ctx context.Context, templateID string, actor Actor) error {
	if s == nil || s.templates == nil || s.steps == nil {
		return apperr.New(apperr.CodeUnavailable, "runbook service is not enabled")
	}
	templateID = strings.TrimSpace(templateID)
	existing, err := s.templates.GetByID(ctx, templateID)
	if err != nil {
		return wrapRBError(err, "load runbook template failed")
	}
	existing.Enabled = false
	existing.UpdatedAt = s.now()
	if err := s.templates.Update(ctx, existing); err != nil {
		return wrapRBError(err, "disable runbook template failed")
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "runbook",
		ResourceID:   templateID,
		Action:       AuditDelete,
		UserID:       actor.UserID,
		Payload:      map[string]any{},
	})
	return nil
}

func (s *TemplateService) buildSteps(templateID string, inputs []CreateStepInput, now time.Time) ([]rbdomain.Step, error) {
	steps := make([]rbdomain.Step, 0, len(inputs))
	for _, in := range inputs {
		action := rbdomain.ActionType(strings.ToLower(strings.TrimSpace(in.ActionType)))
		if !action.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid action_type")
		}
		stepRisk, err := validateRiskLevel(in.RiskLevel)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "step name is required")
		}
		if in.StepOrder < 1 {
			return nil, apperr.New(apperr.CodeInvalidArgument, "step_order must be >= 1")
		}
		timeout := in.TimeoutSeconds
		if timeout <= 0 {
			timeout = 300
		}
		step := rbdomain.Step{
			ID:                uuid.NewString(),
			TemplateID:        templateID,
			StepOrder:         in.StepOrder,
			Name:              name,
			ActionType:        action,
			RiskLevel:         stepRisk,
			DryRunSupported:   in.DryRunSupported,
			DefaultDryRun:     in.DefaultDryRun,
			ParameterSchema:   cloneAnyMap(in.ParameterSchema),
			DefaultParameters: cloneAnyMap(in.DefaultParameters),
			RollbackPlan:      cloneAnyMap(in.RollbackPlan),
			TimeoutSeconds:    timeout,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func (s *TemplateService) toDetailDTO(tpl rbdomain.Template, steps []rbdomain.Step) *TemplateDetailDTO {
	stepDTOs := make([]StepDTO, 0, len(steps))
	for _, st := range steps {
		stepDTOs = append(stepDTOs, ToStepDTO(st))
	}
	return &TemplateDetailDTO{Template: ToTemplateDTO(tpl), Steps: stepDTOs}
}
