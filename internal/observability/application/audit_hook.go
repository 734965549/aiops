package application

import "context"

type AuditAction string

const (
	AuditMetricsQuery  AuditAction = "metrics_query"
	AuditLogsSearch    AuditAction = "logs_search"
	AuditTracesQuery   AuditAction = "traces_query"
	AuditTopologyGet   AuditAction = "topology_get"
	AuditResourcesList AuditAction = "resources_list"
	AuditAlertsList    AuditAction = "alerts_list"
)

type AuditRecord struct {
	ResourceType string
	ResourceID   string
	Action       AuditAction
	UserID       string
	Payload      map[string]any
}

// AuditRecorder 观测查询审计；resource_type 固定 observability_query。
type AuditRecorder interface {
	Record(ctx context.Context, rec AuditRecord) error
}

type NoopAuditRecorder struct{}

func (NoopAuditRecorder) Record(context.Context, AuditRecord) error { return nil }

type Actor struct {
	UserID      string
	DisplayName string
}
