// Package audit 将 Asset AuditRecorder 适配到 Audit 模块。
package audit

import (
	"context"

	assetapp "github.com/734965549/aiops/internal/asset/application"
	auditapp "github.com/734965549/aiops/internal/audit/application"
)

// Recorder 实现 asset AuditRecorder。
type Recorder struct {
	svc *auditapp.OperationAuditService
}

// NewRecorder 构造适配器。
func NewRecorder(svc *auditapp.OperationAuditService) *Recorder {
	return &Recorder{svc: svc}
}

func (r *Recorder) Record(ctx context.Context, rec assetapp.AuditRecord) error {
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
