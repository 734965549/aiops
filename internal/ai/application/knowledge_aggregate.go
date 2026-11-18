package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
)

// 聚合动作（写入审计与响应，便于运维判断幂等行为）。
const (
	AggregateActionCreated           = "created"
	AggregateActionUpdated           = "updated"
	AggregateActionSkippedActive     = "skipped_active"
	AggregateActionInsufficientCases = "insufficient_cases"
)

// 聚合默认参数。
const (
	aggregateDefaultMinCases = 3
	aggregateMaxCases        = 20
	aggregateSummaryRunes    = 300
)

// SetMemorySource 注入记忆案例来源；nil 表示关闭反馈聚合。
func (s *KnowledgeService) SetMemorySource(source MemoryAggregationSource) {
	if s == nil {
		return
	}
	s.memory = source
}

// AggregateFromFeedback 把某应用下已确认有效的历史案例聚合成一条 draft 知识条目。
//
// 幂等与安全：
//   - 同一 (application, feedback_aggregate) 只保留一条条目，重复聚合更新其内容；
//   - 已 active 的条目不覆盖（避免机器改写线上生效知识），返回 skipped_active；
//   - 案例不足 minCases 时不生成 draft，避免单例噪声变成「知识」。
func (s *KnowledgeService) AggregateFromFeedback(ctx context.Context, actor KnowledgeActor, applicationID string, minCases int) (*AggregateResult, error) {
	if s == nil || s.repo == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai knowledge service is not enabled")
	}
	if s.memory == nil {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "ai memory source is not configured")
	}
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "application_id is required")
	}
	if minCases <= 0 {
		minCases = aggregateDefaultMinCases
	}
	userID := strings.TrimSpace(actor.UserID)
	cases, err := s.memory.ListForAggregation(ctx, userID, applicationID, aggregateMaxCases)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load memory cases failed")
	}
	if len(cases) < minCases {
		return &AggregateResult{SourceCases: len(cases), Action: AggregateActionInsufficientCases}, nil
	}
	title, content := composeAggregateDraft(applicationID, cases)
	existing, err := s.repo.FindByScope(ctx, domain.KnowledgeScopeApplication, applicationID, domain.KnowledgeOriginFeedbackAggregate)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load aggregated knowledge item failed")
	}
	now := s.now()
	if existing != nil {
		if existing.Status == domain.KnowledgeStatusActive {
			// 已经审核生效的版本不被机器覆盖。
			return &AggregateResult{Item: existing, SourceCases: len(cases), Action: AggregateActionSkippedActive}, nil
		}
		existing.Title = title
		existing.Content = content
		existing.Status = domain.KnowledgeStatusDraft
		existing.UpdatedAt = now
		if err := s.repo.Update(ctx, *existing); err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "update aggregated knowledge item failed")
		}
		s.recordAudit(ctx, userID, existing.ItemID, AuditKnowledgeEdit, map[string]any{
			"item_id": existing.ItemID, "action": "aggregate", "result": "success",
			"source_cases": len(cases), "status": string(existing.Status),
		})
		return &AggregateResult{Item: existing, SourceCases: len(cases), Action: AggregateActionUpdated}, nil
	}
	item := domain.KnowledgeItem{
		ItemID:    newAggregateItemID(applicationID),
		Scope:     domain.KnowledgeScopeApplication,
		ScopeRef:  applicationID,
		Title:     title,
		Content:   content,
		Origin:    domain.KnowledgeOriginFeedbackAggregate,
		Status:    domain.KnowledgeStatusDraft,
		CreatedBy: userID,
		CreatedAt: now,
		UpdatedAt: now,
	}.Normalize()
	if err := item.Validate(); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "aggregated knowledge item is invalid")
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "create aggregated knowledge item failed")
	}
	s.recordAudit(ctx, userID, item.ItemID, AuditKnowledgeEdit, map[string]any{
		"item_id": item.ItemID, "action": "aggregate", "result": "success",
		"source_cases": len(cases), "status": string(item.Status),
	})
	logger.From(ctx).Info("ai knowledge aggregated",
		logger.String("item_id", item.ItemID), logger.Int("source_cases", len(cases)))
	return &AggregateResult{Item: &item, SourceCases: len(cases), Action: AggregateActionCreated}, nil
}

// composeAggregateDraft 由案例摘要与建议拼装 draft 标题与内容（纯函数，便于断言）。
//
// 标题固定为「应用 <id> 历史有效经验」，内容按案例质量顺序列出摘要与建议摘要。
func composeAggregateDraft(applicationID string, cases []domain.MemoryCase) (title, content string) {
	title = "应用 " + applicationID + " 历史有效经验"
	var b strings.Builder
	b.WriteString("由 ")
	b.WriteString(fmt.Sprintf("%d", len(cases)))
	b.WriteString(" 条已确认有效的历史案例聚合（来源：AI 助手反馈），仅供排查参考：\n")
	for index, c := range cases {
		summary := truncateRunes(strings.TrimSpace(c.Summary), aggregateSummaryRunes)
		if summary == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("%d. ", index+1))
		b.WriteString(summary)
		if recommendations := joinLimited(c.Recommendations, 2); recommendations != "" {
			b.WriteString("（建议：")
			b.WriteString(truncateRunes(recommendations, aggregateSummaryRunes))
			b.WriteString("）")
		}
		b.WriteString("\n")
	}
	return title, strings.TrimSpace(b.String())
}

func joinLimited(items []string, limit int) string {
	out := make([]string, 0, limit)
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return strings.Join(out, "；")
}

func newAggregateItemID(applicationID string) string {
	// 固定前缀 + 应用 ID 便于人工识别来源；不含随机段，保证同应用重复聚合落到同一条目。
	return "agg-" + applicationID
}
