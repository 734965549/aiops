// Package audit 将 Alert application.AuditRecorder 适配到 Audit 模块（§9.4）。
package audit

import (
	"context"

	alertapp "github.com/734965549/aiops/internal/alert/application"
	auditapp "github.com/734965549/aiops/internal/audit/application"
)

// Recorder 实现 alert AuditRecorder，写入 audit_operation 表。
type Recorder struct {
	svc *auditapp.OperationAuditService
}

// NewRecorder 构造 Alert 审计适配器。
func NewRecorder(svc *auditapp.OperationAuditService) *Recorder {
	return &Recorder{svc: svc}
}

func (r *Recorder) Record(ctx context.Context, rec alertapp.AuditRecord) error {
	if r == nil || r.svc == nil {
		return nil
	}
	return r.svc.Record(ctx, auditapp.RecordInput{
		UserID:       rec.UserID,
		ResourceType: rec.ResourceType,
		ResourceID:   rec.ResourceID,
		Action:       string(rec.Action),
		Payload:      rec.Payload,
	})
}
