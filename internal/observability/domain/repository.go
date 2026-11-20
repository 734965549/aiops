package domain

import "context"

// EvidenceRepository 证据引用持久化。
type EvidenceRepository interface {
	Create(ctx context.Context, ref *EvidenceRef) error
	GetByID(ctx context.Context, evidenceID string) (*EvidenceRef, error)
}

// EvidenceSnapshotRepository 保存已定稿且将用于模型输入的证据快照。
type EvidenceSnapshotRepository interface {
	SaveSnapshot(ctx context.Context, ref *EvidenceRef) error
}
