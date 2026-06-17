package application

import "context"

// AuditAction Runbook 审计动作。
type AuditAction string

const (
	AuditCreate    AuditAction = "runbook_create"
	AuditUpdate    AuditAction = "runbook_update"
	AuditDelete    AuditAction = "runbook_delete"
	AuditEnable    AuditAction = "runbook_enable"
	AuditDisable   AuditAction = "runbook_disable"
	AuditRecommend AuditAction = "runbook_recommend"
)

// AuditRecord 审计载荷。
type AuditRecord struct {
	ResourceType string
	ResourceID   string
	Action       AuditAction
	UserID       string
	Payload      map[string]any
}

// AuditRecorder 审计接口。
type AuditRecorder interface {
	Record(ctx context.Context, rec AuditRecord) error
}

// NoopAuditRecorder 空实现。
type NoopAuditRecorder struct{}

func (NoopAuditRecorder) Record(context.Context, AuditRecord) error { return nil }
