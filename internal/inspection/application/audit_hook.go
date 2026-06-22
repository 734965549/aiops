package application

import "context"

type AuditAction string

const (
	AuditPolicyCreate  AuditAction = "create"
	AuditPolicyUpdate  AuditAction = "update"
	AuditPolicyDelete  AuditAction = "delete"
	AuditPolicyEnable  AuditAction = "enable"
	AuditPolicyDisable AuditAction = "disable"
	AuditRunCreate     AuditAction = "create"
	AuditRunStart      AuditAction = "start"
	AuditRunFinish     AuditAction = "finish"
	AuditRunCancel     AuditAction = "cancel"
	AuditRecCreate     AuditAction = "create"
)

type AuditRecord struct {
	ResourceType string
	ResourceID   string
	Action       AuditAction
	UserID       string
	Payload      map[string]any
}

type AuditRecorder interface {
	Record(ctx context.Context, rec AuditRecord) error
}

type NoopAuditRecorder struct{}

func (NoopAuditRecorder) Record(context.Context, AuditRecord) error { return nil }

type Actor struct {
	UserID      string
	DisplayName string
}
