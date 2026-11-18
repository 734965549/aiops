package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/ai/domain"
)

// 记忆检索状态（写入 chat_message / analyze_alert 审计，不含案例原文）。
const (
	MemoryStatusOK      = "ok"
	MemoryStatusEmpty   = "empty"
	MemoryStatusError   = "error"
	MemoryStatusSkipped = "skipped"
)

// MemoryCaseRepository 是记忆条目的持久化端口（由 persistence.MemoryCaseRepository 实现）。
type MemoryCaseRepository interface {
	// Create 幂等沉淀一条案例；同一用户同一来源消息已存在时返回 created=false。
	Create(ctx context.Context, c domain.MemoryCase) (created bool, err error)
	// FindBySourceMessage 按用户与来源消息查找案例；不存在返回 (nil, nil)。
	FindBySourceMessage(ctx context.Context, userID, sourceMessageID string) (*domain.MemoryCase, error)
	// UpdateQuality 只更新质量分、状态与更新时间。
	UpdateQuality(ctx context.Context, c domain.MemoryCase) error
	// Search 仅返回该用户 status=active 的案例，结果按匹配优先级排序。
	Search(ctx context.Context, filter domain.MemoryCaseFilter) ([]domain.MemoryCase, error)
	// ListForAggregation 返回某应用下该用户的高质量案例，供知识聚合使用。
	ListForAggregation(ctx context.Context, userID, applicationID string, limit int) ([]domain.MemoryCase, error)
}

// MemorySearchInput 是一次相似案例检索的输入。
type MemorySearchInput struct {
	UserID string
	// Alert 非 nil 时参与指纹与关键词提取（告警页 / 分析接口）。
	Alert *AlertContext
	// Evidence 非 nil 时把指标名并入指纹（分析接口的指标证据）。
	Evidence *MetricEvidenceResult
	// Context 是会话页面上下文（应用/资源匹配键）。
	Context domain.SessionContext
	// Keywords 是额外的检索词（用户问题等）。
	Keywords string
}

// MemorySearchResult 是一次检索的结果与状态。
type MemorySearchResult struct {
	Cases  []domain.MemoryCase
	Status string
}

// MemoryCaseStore 是记忆能力对上层暴露的应用端口。
type MemoryCaseStore interface {
	// SearchSimilar 检索相似案例；失败只返回 status=error，不阻断回答主链路。
	SearchSimilar(ctx context.Context, in MemorySearchInput) MemorySearchResult
	// RecordValidFeedback 依据有效反馈沉淀案例（按来源消息幂等）。
	RecordValidFeedback(ctx context.Context, session *domain.Session, message *domain.Message) error
	// RecordInvalidFeedback 依据无效反馈降低已有案例质量分，必要时转 retired。
	RecordInvalidFeedback(ctx context.Context, session *domain.Session, message *domain.Message) error
}

// memoryKeywords 拼接检索词，保持顺序以便单测与日志可读。
func memoryKeywords(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return domain.BuildMemoryKeywords(cleaned...)
}
