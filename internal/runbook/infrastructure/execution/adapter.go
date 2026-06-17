package execution

import (
	"context"

	execapp "github.com/734965549/aiops/internal/execution/application"
	rbapp "github.com/734965549/aiops/internal/runbook/application"
	"github.com/734965549/aiops/internal/runbook/domain"
)

// Adapter maps the Runbook aggregate into Execution's port DTO.
type Adapter struct {
	templates *rbapp.TemplateService
}

func NewAdapter(templates *rbapp.TemplateService) *Adapter {
	return &Adapter{templates: templates}
}

func (a *Adapter) GetForExecution(ctx context.Context, templateID string) (*execapp.ExecutableRunbook, error) {
	tpl, err := a.templates.GetForExecution(ctx, templateID)
	if err != nil {
		return nil, err
	}
	return toExecutableRunbook(tpl), nil
}

func toExecutableRunbook(tpl *domain.TemplateWithSteps) *execapp.ExecutableRunbook {
	if tpl == nil {
		return nil
	}
	steps := make([]execapp.ExecutableRunbookStep, 0, len(tpl.Steps))
	for _, st := range tpl.Steps {
		steps = append(steps, execapp.ExecutableRunbookStep{
			StepID:            st.ID,
			StepOrder:         st.StepOrder,
			Name:              st.Name,
			ActionType:        string(st.ActionType),
			RiskLevel:         string(st.RiskLevel),
			DryRunSupported:   st.DryRunSupported,
			DefaultDryRun:     st.DefaultDryRun,
			ParameterSchema:   cloneMap(st.ParameterSchema),
			DefaultParameters: cloneMap(st.DefaultParameters),
			RollbackPlan:      cloneMap(st.RollbackPlan),
			TimeoutSeconds:    st.TimeoutSeconds,
		})
	}
	return &execapp.ExecutableRunbook{
		TemplateID:      tpl.Template.ID,
		Name:            tpl.Template.Name,
		Description:     tpl.Template.Description,
		OperationType:   string(tpl.Template.OperationType),
		RiskLevel:       string(tpl.Template.RiskLevel),
		RollbackPlan:    cloneMap(tpl.Template.RollbackPlan),
		ParameterSchema: cloneMap(tpl.Template.ParameterSchema),
		Steps:           steps,
	}
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
