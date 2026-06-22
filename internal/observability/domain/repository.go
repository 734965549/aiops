package domain

import "context"

// EvidenceRepository 证据引用持久化（第一阶段仅保存摘要，不存原始查询结果）。
type EvidenceRepository interface {
	Create(ctx context.Context, ref *EvidenceRef) error
	GetByID(ctx context.Context, evidenceID string) (*EvidenceRef, error)
}
