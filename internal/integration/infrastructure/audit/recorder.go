package audit

import (
	"context"

	auditapp "github.com/734965549/aiops/internal/audit/application"
	integapp "github.com/734965549/aiops/internal/integration/application"
)

type Recorder struct {
	svc *auditapp.OperationAuditService
}

func NewRecorder(svc *auditapp.OperationAuditService) *Recorder {
	return &Recorder{svc: svc}
}

func (r *Recorder) Record(ctx context.Context, rec integapp.AuditRecord) error {
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
