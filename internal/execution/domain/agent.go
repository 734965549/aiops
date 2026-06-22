package domain

import "time"

// AgentStatus 执行代理状态。
type AgentStatus string

const (
	AgentRegistered AgentStatus = "registered"
	AgentOnline     AgentStatus = "online"
	AgentDraining   AgentStatus = "draining"
	AgentOffline    AgentStatus = "offline"
	AgentUnhealthy  AgentStatus = "unhealthy"
	AgentDisabled   AgentStatus = "disabled"
)

func (s AgentStatus) IsValid() bool {
	switch s {
	case AgentRegistered, AgentOnline, AgentDraining, AgentOffline, AgentUnhealthy, AgentDisabled:
		return true
	default:
		return false
	}
}

func (s AgentStatus) CanLease() bool {
	return s == AgentOnline
}

// ExecutionAgent 部署在介体上的执行代理。
type ExecutionAgent struct {
	AgentID        string
	MediumID       string
	Status         AgentStatus
	PublicKey      string
	TokenHash      string
	Version        string
	Capabilities   []string
	RunningTasks   int
	FreeSlots      int
	LastHeartbeat  *time.Time
	Disabled       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
