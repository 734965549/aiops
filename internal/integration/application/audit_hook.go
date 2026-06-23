package application

import "context"

type AuditAction string

const (
	AuditAccountCreate AuditAction = "create"
	AuditAccountUpdate AuditAction = "update"
	AuditAccountDelete AuditAction = "delete"
	AuditAccountCheck  AuditAction = "check"
)

type AuditRecord struct {
	ResourceType string
	ResourceID   string
	Action       AuditAction
	UserID       string
	Payload      map[string]any
}

// AuditRecorder 审计写入接口；resource_type 固定 integration_account。
type AuditRecorder interface {
	Record(ctx context.Context, rec AuditRecord) error
}

type NoopAuditRecorder struct{}

func (NoopAuditRecorder) Record(context.Context, AuditRecord) error { return nil }

type Actor struct {
	UserID      string
	DisplayName string
}
