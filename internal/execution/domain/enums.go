package domain

import "strings"

// SourceType 任务来源。
type SourceType string

const (
	SourceAlert          SourceType = "alert"
	SourceManual         SourceType = "manual"
	SourceAIConversation SourceType = "ai_conversation"
)

func (s SourceType) IsValid() bool {
	switch s {
	case SourceAlert, SourceManual, SourceAIConversation:
		return true
	default:
		return false
	}
}

// OperationType 操作类型。
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

// DefaultRiskForOperation 按操作类型与环境映射默认风险。
func DefaultRiskForOperation(op OperationType, environment string) RiskLevel {
	env := strings.ToLower(strings.TrimSpace(environment))
	isProd := env == "prod" || env == "production"
	switch op {
	case OpScript, OpRunbook:
		if isProd {
			return RiskMedium
		}
		return RiskLow
	case OpRestart, OpScale, OpCustom:
		return RiskMedium
	default:
		return RiskMedium
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

// ResolveRiskLevel 计算任务风险：默认规则 + 可选覆盖（不可低于默认）。
func ResolveRiskLevel(op OperationType, environment, override string) (RiskLevel, error) {
	base := DefaultRiskForOperation(op, environment)
	override = strings.ToLower(strings.TrimSpace(override))
	if override == "" {
		return base, nil
	}
	requested := RiskLevel(override)
	if !requested.IsValid() {
		return "", ErrInvalidArgument
	}
	if RiskRank(requested) < RiskRank(base) {
		return "", ErrInvalidArgument
	}
	return requested, nil
}

// InitialStatusForRisk 按风险决定创建后初始状态。
func InitialStatusForRisk(risk RiskLevel) TaskStatus {
	if risk == RiskLow {
		return StatusPendingExecute
	}
	return StatusPendingConfirm
}
