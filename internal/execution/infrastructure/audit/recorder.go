// Package audit 将 Execution AuditRecorder 适配到 Audit 模块。
package audit

import (
	"context"

	auditapp "github.com/734965549/aiops/internal/audit/application"
	execapp "github.com/734965549/aiops/internal/execution/application"
)

// Recorder 实现 execution AuditRecorder。
type Recorder struct {
	svc *auditapp.OperationAuditService
}

// NewRecorder 构造适配器。
func NewRecorder(svc *auditapp.OperationAuditService) *Recorder {
	return &Recorder{svc: svc}
}

func (r *Recorder) Record(ctx context.Context, rec execapp.AuditRecord) error {
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
