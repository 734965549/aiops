package application

import "context"

// AlertContext 推荐 Runbook 时从告警读取的快照。
type AlertContext struct {
	ID           string
	Name         string
	Status       string
	Environment  string
	ResourceID   string
	ResourceType string
	ResourceName string
	Labels       map[string]string
	Annotations  map[string]string
}

// AlertReader 读取告警上下文。
type AlertReader interface {
	GetForExecution(ctx context.Context, alertID string) (*AlertContext, error)
}
