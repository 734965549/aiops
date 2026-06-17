package domain

import "context"

// OperationAuditRepository 操作审计持久化接口。
type OperationAuditRepository interface {
	Create(ctx context.Context, audit *OperationAudit) error
	List(ctx context.Context, filter OperationAuditFilter) ([]OperationAudit, error)
	Count(ctx context.Context, filter OperationAuditFilter) (int64, error)
}
