package application

import "context"

// AuditAction 资产审计动作。
type AuditAction string

const (
	AuditCreateApplication AuditAction = "create"
	AuditUpdateApplication AuditAction = "update"
	AuditDeleteApplication AuditAction = "delete"
	AuditCreateResource    AuditAction = "create"
	AuditUpdateResource    AuditAction = "update"
	AuditDeleteResource    AuditAction = "delete"
	AuditCreateMatchRule   AuditAction = "create"
	AuditUpdateMatchRule   AuditAction = "update"
	AuditDeleteMatchRule   AuditAction = "delete"
	AuditAssetSync         AuditAction = "sync"
)

// AuditRecord 审计载荷。
type AuditRecord struct {
	ResourceType string
	ResourceID   string
	Action       AuditAction
	UserID       string
	Payload      map[string]any
}

// AuditRecorder 审计写入接口。
type AuditRecorder interface {
	Record(ctx context.Context, rec AuditRecord) error
}

// NoopAuditRecorder 空实现。
type NoopAuditRecorder struct{}

func (NoopAuditRecorder) Record(context.Context, AuditRecord) error { return nil }
