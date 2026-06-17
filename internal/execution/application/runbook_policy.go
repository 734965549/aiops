package application

import (
	"strings"

	"github.com/734965549/aiops/internal/execution/domain"
)

func buildRunbookSnapshot(tpl *ExecutableRunbook) map[string]any {
	if tpl == nil {
		return map[string]any{}
	}
	stepItems := make([]map[string]any, 0, len(tpl.Steps))
	for _, st := range tpl.Steps {
		stepItems = append(stepItems, map[string]any{
			"step_id":            st.StepID,
			"step_order":         st.StepOrder,
			"name":               st.Name,
			"action_type":        st.ActionType,
			"risk_level":         st.RiskLevel,
			"dry_run_supported":  st.DryRunSupported,
			"default_dry_run":    st.DefaultDryRun,
			"default_parameters": cloneMap(st.DefaultParameters),
			"rollback_plan":      cloneMap(st.RollbackPlan),
			"timeout_seconds":    st.TimeoutSeconds,
		})
	}
	return map[string]any{
		"template_id":    tpl.TemplateID,
		"name":           tpl.Name,
		"description":    tpl.Description,
		"operation_type": tpl.OperationType,
		"risk_level":     tpl.RiskLevel,
		"rollback_plan":  cloneMap(tpl.RollbackPlan),
		"steps":          stepItems,
	}
}

func mergeStepParameters(defaults, user map[string]any) map[string]any {
	out := cloneMap(defaults)
	for k, v := range user {
		out[k] = v
	}
	return out
}

func resolveRunbookTaskRisk(
	op domain.OperationType,
	env string,
	templateRisk string,
	steps []ExecutableRunbookStep,
	userOverride string,
) (domain.RiskLevel, error) {
	base := domain.DefaultRiskForOperation(op, env)
	risks := []domain.RiskLevel{base}
	if templateRisk != "" {
		risk := domain.RiskLevel(strings.ToLower(strings.TrimSpace(templateRisk)))
		if !risk.IsValid() {
			return "", domain.ErrInvalidArgument
		}
		risks = append(risks, risk)
	}
	for _, st := range steps {
		stepRisk := domain.RiskLevel(strings.ToLower(strings.TrimSpace(st.RiskLevel)))
		if !stepRisk.IsValid() {
			return "", domain.ErrInvalidArgument
		}
		risks = append(risks, stepRisk)
		if requiresMediumInProd(st.ActionType, env) {
			risks = append(risks, domain.RiskMedium)
		}
	}
	maxRisk := maxRisk(risks...)
	userOverride = strings.ToLower(strings.TrimSpace(userOverride))
	if userOverride == "" {
		return maxRisk, nil
	}
	requested := domain.RiskLevel(userOverride)
	if !requested.IsValid() {
		return "", domain.ErrInvalidArgument
	}
	if domain.RiskRank(requested) < domain.RiskRank(maxRisk) {
		return "", domain.ErrInvalidArgument
	}
	return requested, nil
}

func maxRisk(levels ...domain.RiskLevel) domain.RiskLevel {
	max := domain.RiskLow
	for _, level := range levels {
		if !level.IsValid() {
			continue
		}
		if domain.RiskRank(level) > domain.RiskRank(max) {
			max = level
		}
	}
	return max
}

func requiresMediumInProd(action, env string) bool {
	if !isProdEnvironment(env) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "script", "command", "custom":
		return true
	default:
		return false
	}
}

func isProdEnvironment(env string) bool {
	e := strings.ToLower(strings.TrimSpace(env))
	return e == "prod" || e == "production"
}
