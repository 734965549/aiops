package application

import "context"

// AuditAction 执行任务审计动作。
type AuditAction string

const (
	AuditCreate            AuditAction = "create"
	AuditConfirm           AuditAction = "confirm"
	AuditReject            AuditAction = "reject"
	AuditExecute           AuditAction = "execute"
	AuditAlertCreate       AuditAction = "execution_create"
	AuditCreateFromRunbook AuditAction = "create_from_runbook"
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
