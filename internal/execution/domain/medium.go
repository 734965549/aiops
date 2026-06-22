package domain

import "time"

// MediumType 执行介体类型。
type MediumType string

const (
	MediumJumpbox     MediumType = "jumpbox"
	MediumTargetHost  MediumType = "target_host"
	MediumK8sPod      MediumType = "kubernetes_pod"
	MediumCloudRunCmd MediumType = "cloud_run_command"
	MediumDBReadonly  MediumType = "database_readonly"
)

func (m MediumType) IsValid() bool {
	switch m {
	case MediumJumpbox, MediumTargetHost, MediumK8sPod, MediumCloudRunCmd, MediumDBReadonly:
		return true
	default:
		return false
	}
}

// MediumHealthStatus 介体健康状态。
type MediumHealthStatus string

const (
	MediumHealthUnknown MediumHealthStatus = "unknown"
	MediumHealthOnline  MediumHealthStatus = "online"
	MediumHealthOffline MediumHealthStatus = "offline"
)

// ExecutionMedium 执行介体。
type ExecutionMedium struct {
	MediumID          string
	Name              string
	MediumType        MediumType
	Environment       string
	Region            string
	NetworkZone       string
	Capabilities      []string
	AllowedCommandIDs []string
	MaxRiskLevel      RiskLevel
	Enabled           bool
	HealthStatus      MediumHealthStatus
	Description       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
