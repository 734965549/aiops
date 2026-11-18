package application

import (
	"context"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
)

// 知识检索状态（写入 chat_message 审计，不含条目正文）。
const (
	KnowledgeStatusOK      = "ok"
	KnowledgeStatusEmpty   = "empty"
	KnowledgeStatusError   = "error"
	KnowledgeStatusSkipped = "skipped"
)

// KnowledgeItemRepository 是知识条目的持久化端口（由 persistence.KnowledgeItemRepository 实现）。
type KnowledgeItemRepository interface {
	Create(ctx context.Context, item domain.KnowledgeItem) error
	Get(ctx context.Context, itemID string) (*domain.KnowledgeItem, error)
	Update(ctx context.Context, item domain.KnowledgeItem) error
	Delete(ctx context.Context, itemID string) error
	List(ctx context.Context, filter domain.KnowledgeItemFilter) ([]domain.KnowledgeItem, error)
	// ListActiveForTarget 返回对给定应用/资源类型生效的 active 条目（含 global）。
	ListActiveForTarget(ctx context.Context, applicationID, resourceType string) ([]domain.KnowledgeItem, error)
	AddHit(ctx context.Context, itemIDs []string, updatedAt time.Time) error
	AddUseful(ctx context.Context, itemIDs []string, updatedAt time.Time) error
	// FindByScope 按 scope + scope_ref + origin 查找条目（聚合幂等用）；不存在返回 (nil, nil)。
	FindByScope(ctx context.Context, scope domain.KnowledgeScope, scopeRef string, origin domain.KnowledgeOrigin) (*domain.KnowledgeItem, error)
}

// MemoryAggregationSource 为知识聚合提供记忆案例来源（由 persistence.MemoryCaseRepository 实现）。
type MemoryAggregationSource interface {
	ListForAggregation(ctx context.Context, userID, applicationID string, limit int) ([]domain.MemoryCase, error)
}

// AggregateResult 是一次聚合的结果摘要。
type AggregateResult struct {
	Item        *domain.KnowledgeItem
	SourceCases int
	// Action 为 created / updated / skipped_active / insufficient_cases。
	Action string
}

// KnowledgeActor 是执行审核动作的操作者。
type KnowledgeActor struct {
	UserID string
}

// KnowledgeItemInput 是新建/编辑知识条目的入参。
type KnowledgeItemInput struct {
	Scope    string
	ScopeRef string
	Title    string
	Content  string
}

// KnowledgeListInput 是列表查询入参。
type KnowledgeListInput struct {
	Status string
	Scope  string
	Limit  int
	Offset int
}

// KnowledgeInjection 是一次回答可注入的知识（已按 scope 匹配并做上限截断）。
type KnowledgeInjection struct {
	Items    []domain.KnowledgeItem
	Appendix string
	Status   string
}

// KnowledgeStore 是聊天编排依赖的最小知识端口。
type KnowledgeStore interface {
	// Injection 按上下文取生效知识并生成 system prompt 附加段；失败只降级。
	Injection(ctx context.Context, applicationID, resourceType string) KnowledgeInjection
	// RecordInjection 累加注入次数（命中率评估），失败只记日志。
	RecordInjection(ctx context.Context, itemIDs []string)
	// RecordUseful 把「有效反馈」回写到注入过的条目，并按其表现自动停用低效条目。
	RecordUseful(ctx context.Context, itemIDs []string)
	// RecordUsageByFeedback 依据单条回答的反馈更新知识质量（valid → useful，invalid → 只评估）。
	RecordUsageByFeedback(ctx context.Context, itemIDs []string, useful bool)
}
