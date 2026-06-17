package application

import "context"

// AuditAction 告警关键操作审计动作，供后续 Audit 模块接入（§9.4）。
type AuditAction string

const (
	AuditAcknowledge     AuditAction = "acknowledge"
	AuditAssign          AuditAction = "assign"
	AuditStartProcessing AuditAction = "start_processing"
	AuditSilence         AuditAction = "silence"
	AuditUnsilence       AuditAction = "unsilence"
	AuditRecover         AuditAction = "recover"
	AuditClose           AuditAction = "close"
	AuditComment         AuditAction = "comment"
	AuditAIAnalysis      AuditAction = "ai_analysis_requested"
	AuditExecutionCreate AuditAction = "execution_create"
	AuditSourceCreate    AuditAction = "source_create"
	AuditSourceUpdate    AuditAction = "source_update"
	AuditSourceDelete    AuditAction = "source_delete"
	AuditIngest          AuditAction = "ingest"
)

// AuditRecord 审计写入载荷；resource_type 固定 alert。
type AuditRecord struct {
	ResourceType string
	ResourceID   string
	Action       AuditAction
	UserID       string
	Payload      map[string]any
}

// AuditRecorder 审计写入接口；第一阶段默认 NoopAuditRecorder。
type AuditRecorder interface {
	Record(ctx context.Context, rec AuditRecord) error
}

// NoopAuditRecorder 不写入审计，仅预留接入点。
type NoopAuditRecorder struct{}

func (NoopAuditRecorder) Record(context.Context, AuditRecord) error { return nil }
