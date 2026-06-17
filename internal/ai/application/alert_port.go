package application

import "context"

// AlertContext 供 AI 分析读取嘅告警快照（只读 port，避免 AI 依赖 Alert application）。
type AlertContext struct {
	ID              string
	Name            string
	Summary         string
	Description     string
	Severity        string
	Status          string
	Environment     string
	ApplicationID   string
	ApplicationName string
	ResourceID      string
	ResourceName    string
	Labels          map[string]string
	Annotations     map[string]any
}

// AlertReader 只读告警详情，由 infrastructure/alert 适配 alert 仓储实现。
type AlertReader interface {
	GetForAnalysis(ctx context.Context, alertID string) (*AlertContext, error)
}
