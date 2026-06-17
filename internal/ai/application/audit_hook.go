package application

import "context"

// AuditAction AI 审计动作。
type AuditAction string

const (
	AuditAnalyzeAlert AuditAction = "analyze_alert"
	AuditToolInvoke   AuditAction = "tool_invoke"
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
