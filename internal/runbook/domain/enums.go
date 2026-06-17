package domain

import "strings"

// OperationType 预案操作类型。
type OperationType string

const (
	OpRestart OperationType = "restart"
	OpScale   OperationType = "scale"
	OpScript  OperationType = "script"
	OpRunbook OperationType = "runbook"
	OpCustom  OperationType = "custom"
)

func (o OperationType) IsValid() bool {
	switch o {
	case OpRestart, OpScale, OpScript, OpRunbook, OpCustom:
		return true
	default:
		return false
	}
}

// ActionType 步骤动作类型。
type ActionType string

const (
	ActionRestart ActionType = "restart"
	ActionScale   ActionType = "scale"
	ActionScript  ActionType = "script"
	ActionCommand ActionType = "command"
	ActionHTTP    ActionType = "http"
	ActionManual  ActionType = "manual"
	ActionCustom  ActionType = "custom"
)

func (a ActionType) IsValid() bool {
	switch a {
	case ActionRestart, ActionScale, ActionScript, ActionCommand, ActionHTTP, ActionManual, ActionCustom:
		return true
	default:
		return false
	}
}

// RiskLevel 风险等级。
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

func (r RiskLevel) IsValid() bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
		return true
	default:
		return false
	}
}

// RiskRank 风险等级序（越大越高）。
func RiskRank(r RiskLevel) int {
	switch r {
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	case RiskCritical:
		return 4
	default:
		return 0
	}
}

// MaxRisk 取最高风险。
func MaxRisk(levels ...RiskLevel) RiskLevel {
	max := RiskLow
	for _, l := range levels {
		if !l.IsValid() {
			continue
		}
		if RiskRank(l) > RiskRank(max) {
			max = l
		}
	}
	return max
}

// IsProdEnvironment 是否生产环境。
func IsProdEnvironment(env string) bool {
	e := strings.ToLower(strings.TrimSpace(env))
	return e == "prod" || e == "production"
}

// RequiresMediumInProd prod 下 script/command/custom 默认至少 medium。
func RequiresMediumInProd(action ActionType, env string) bool {
	if !IsProdEnvironment(env) {
		return false
	}
	switch action {
	case ActionScript, ActionCommand, ActionCustom:
		return true
	default:
		return false
	}
}
