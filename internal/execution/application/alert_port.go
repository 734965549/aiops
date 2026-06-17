package application

import "context"

// AlertContext 创建任务时从告警读取的快照。
type AlertContext struct {
	ID              string
	Name            string
	Status          string
	Environment     string
	ApplicationID   string
	ApplicationName string
	ResourceID      string
	ResourceType    string
	ResourceName    string
	Labels          map[string]string
	Annotations     map[string]string
}

// AlertReader 读取告警上下文。
type AlertReader interface {
	GetForExecution(ctx context.Context, alertID string) (*AlertContext, error)
}

// AlertTimelineWriter 回写告警时间线（§7）。
type AlertTimelineWriter interface {
	RecordExecutionCreated(ctx context.Context, alertID string, actor Actor, taskID string, payload map[string]any) error
	RecordExecutionStarted(ctx context.Context, alertID string, actor Actor, taskID string, payload map[string]any) error
	RecordExecutionFinished(ctx context.Context, alertID string, actor Actor, taskID string, payload map[string]any) error
}
