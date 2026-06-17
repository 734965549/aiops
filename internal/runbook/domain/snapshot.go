package domain

import "strings"

// BuildSnapshot 构建任务创建时的模板快照。
func BuildSnapshot(tpl Template, steps []Step) map[string]any {
	stepItems := make([]map[string]any, 0, len(steps))
	for _, st := range steps {
		stepItems = append(stepItems, map[string]any{
			"step_id":            st.ID,
			"step_order":         st.StepOrder,
			"name":               st.Name,
			"action_type":        string(st.ActionType),
			"risk_level":         string(st.RiskLevel),
			"dry_run_supported":  st.DryRunSupported,
			"default_dry_run":    st.DefaultDryRun,
			"default_parameters": cloneAnyMap(st.DefaultParameters),
			"rollback_plan":      cloneAnyMap(st.RollbackPlan),
			"timeout_seconds":    st.TimeoutSeconds,
		})
	}
	return map[string]any{
		"template_id":    tpl.ID,
		"name":           tpl.Name,
		"description":    tpl.Description,
		"operation_type": string(tpl.OperationType),
		"risk_level":     string(tpl.RiskLevel),
		"rollback_plan":  cloneAnyMap(tpl.RollbackPlan),
		"steps":          stepItems,
	}
}

// MergeStepParameters 合并步骤默认参数与用户参数。
func MergeStepParameters(defaults, user map[string]any) map[string]any {
	out := cloneAnyMap(defaults)
	for k, v := range user {
		out[k] = v
	}
	return out
}

// ResolveTaskRisk 计算 Runbook 任务最终风险。
func ResolveTaskRisk(
	op OperationType,
	env string,
	templateRisk RiskLevel,
	steps []Step,
	userOverride string,
) (RiskLevel, error) {
	base := defaultRiskForOperation(op, env)
	risks := []RiskLevel{base, templateRisk}
	for _, st := range steps {
		risks = append(risks, st.RiskLevel)
		if RequiresMediumInProd(st.ActionType, env) {
			risks = append(risks, RiskMedium)
		}
	}
	maxRisk := MaxRisk(risks...)
	userOverride = strings.ToLower(strings.TrimSpace(userOverride))
	if userOverride == "" {
		return maxRisk, nil
	}
	requested := RiskLevel(userOverride)
	if !requested.IsValid() {
		return "", ErrInvalidArgument
	}
	if RiskRank(requested) < RiskRank(maxRisk) {
		return "", ErrInvalidArgument
	}
	return requested, nil
}

func defaultRiskForOperation(op OperationType, env string) RiskLevel {
	switch op {
	case OpScript, OpRunbook:
		if IsProdEnvironment(env) {
			return RiskMedium
		}
		return RiskLow
	case OpRestart, OpScale, OpCustom:
		return RiskMedium
	default:
		return RiskMedium
	}
}

func cloneAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
