package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/google/uuid"
)

// KnowledgeService 实现阶段 2「反馈闭环与知识沉淀」的管理侧用例：
// 人工录入 / 审核激活 / 停用 / 注入取数与命中统计。
//
// 安全边界：只有 active 条目会进入回答；审核动作全部写审计；条目内容不含告警正文与凭据。
type KnowledgeService struct {
	repo  KnowledgeItemRepository
	audit AuditRecorder
	// memory 为可选的记忆案例来源，用于反馈聚合生成 draft；nil 表示关闭聚合。
	memory MemoryAggregationSource
	now    func() time.Time
}

func NewKnowledgeService(repo KnowledgeItemRepository, audit AuditRecorder) *KnowledgeService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &KnowledgeService{repo: repo, audit: audit, now: time.Now}
}

var _ KnowledgeStore = (*KnowledgeService)(nil)

// List 返回知识条目列表（默认 20 条）。
func (s *KnowledgeService) List(ctx context.Context, in KnowledgeListInput) ([]domain.KnowledgeItem, error) {
	if s == nil || s.repo == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai knowledge service is not enabled")
	}
	filter := domain.KnowledgeItemFilter{Limit: in.Limit, Offset: in.Offset}
	if status := strings.TrimSpace(in.Status); status != "" {
		value := domain.KnowledgeStatus(status)
		if !value.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid knowledge status")
		}
		filter.Status = &value
	}
	if scope := strings.TrimSpace(in.Scope); scope != "" {
		value := domain.KnowledgeScope(scope)
		if !value.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid knowledge scope")
		}
		filter.Scope = &value
	}
	items, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list ai knowledge items failed")
	}
	return items, nil
}

// Create 新建知识条目：人工录入默认 draft，必须审核激活后才注入。
func (s *KnowledgeService) Create(ctx context.Context, actor KnowledgeActor, in KnowledgeItemInput) (*domain.KnowledgeItem, error) {
	if s == nil || s.repo == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai knowledge service is not enabled")
	}
	item := domain.KnowledgeItem{
		ItemID:    uuid.NewString(),
		Scope:     domain.KnowledgeScope(strings.TrimSpace(in.Scope)),
		ScopeRef:  in.ScopeRef,
		Title:     strings.TrimSpace(in.Title),
		Content:   strings.TrimSpace(in.Content),
		Origin:    domain.KnowledgeOriginManual,
		Status:    domain.KnowledgeStatusDraft,
		CreatedBy: strings.TrimSpace(actor.UserID),
		CreatedAt: s.now(),
		UpdatedAt: s.now(),
	}.Normalize()
	if err := item.Validate(); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInvalidArgument, "invalid ai knowledge item")
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "create ai knowledge item failed")
	}
	s.recordAudit(ctx, actor.UserID, item.ItemID, AuditKnowledgeEdit, map[string]any{
		"item_id": item.ItemID, "scope": string(item.Scope), "scope_ref": item.ScopeRef,
		"status": string(item.Status), "origin": string(item.Origin), "result": "success",
	})
	return &item, nil
}

// Update 编辑条目标题/内容/范围；编辑已生效条目会回到 draft，必须重新审核。
func (s *KnowledgeService) Update(ctx context.Context, actor KnowledgeActor, itemID string, in KnowledgeItemInput) (*domain.KnowledgeItem, error) {
	if s == nil || s.repo == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai knowledge service is not enabled")
	}
	current, err := s.load(ctx, itemID)
	if err != nil {
		return nil, err
	}
	previousStatus := current.Status
	updated := domain.KnowledgeItem{
		ItemID:     current.ItemID,
		Scope:      domain.KnowledgeScope(strings.TrimSpace(in.Scope)),
		ScopeRef:   in.ScopeRef,
		Title:      strings.TrimSpace(in.Title),
		Content:    strings.TrimSpace(in.Content),
		Origin:     current.Origin,
		Status:     domain.KnowledgeStatusDraft,
		CreatedBy:  current.CreatedBy,
		CreatedAt:  current.CreatedAt,
		ReviewedBy: current.ReviewedBy,
		ReviewedAt: current.ReviewedAt,
		UpdatedAt:  s.now(),
	}.Normalize()
	if err := updated.Validate(); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInvalidArgument, "invalid ai knowledge item")
	}
	if previousStatus == domain.KnowledgeStatusActive {
		// 生效内容被改动，必须重新审核，避免未经审核的内容继续注入。
		updated.ReviewedBy = ""
		updated.ReviewedAt = nil
	}
	if err := s.repo.Update(ctx, updated); err != nil {
		if errors.Is(err, domain.ErrKnowledgeItemNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "knowledge item not found")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "update ai knowledge item failed")
	}
	s.recordAudit(ctx, actor.UserID, updated.ItemID, AuditKnowledgeEdit, map[string]any{
		"item_id": updated.ItemID, "previous_status": string(previousStatus),
		"status": string(updated.Status), "result": "success",
	})
	return &updated, nil
}

// Review 审核流转：activate（draft/disabled → active）或 disable（→ disabled）。
func (s *KnowledgeService) Review(ctx context.Context, actor KnowledgeActor, itemID, action string) (*domain.KnowledgeItem, error) {
	if s == nil || s.repo == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai knowledge service is not enabled")
	}
	current, err := s.load(ctx, itemID)
	if err != nil {
		return nil, err
	}
	var target domain.KnowledgeStatus
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "activate":
		target = domain.KnowledgeStatusActive
	case "disable":
		target = domain.KnowledgeStatusDisabled
	default:
		return nil, apperr.New(apperr.CodeInvalidArgument, "action must be activate or disable")
	}
	now := s.now()
	current.Status = target
	current.ReviewedBy = strings.TrimSpace(actor.UserID)
	current.ReviewedAt = &now
	current.UpdatedAt = now
	if err := s.repo.Update(ctx, *current); err != nil {
		if errors.Is(err, domain.ErrKnowledgeItemNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "knowledge item not found")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "review ai knowledge item failed")
	}
	s.recordAudit(ctx, actor.UserID, current.ItemID, AuditKnowledgeReview, map[string]any{
		"item_id": current.ItemID, "action": strings.ToLower(strings.TrimSpace(action)),
		"status": string(current.Status), "result": "success",
	})
	return current, nil
}

// Delete 删除知识条目（审计保留删除事实）。
func (s *KnowledgeService) Delete(ctx context.Context, actor KnowledgeActor, itemID string) error {
	if s == nil || s.repo == nil {
		return apperr.New(apperr.CodeUnavailable, "ai knowledge service is not enabled")
	}
	if _, err := s.load(ctx, itemID); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, itemID); err != nil {
		if errors.Is(err, domain.ErrKnowledgeItemNotFound) {
			return apperr.New(apperr.CodeNotFound, "knowledge item not found")
		}
		return apperr.Wrap(err, apperr.CodeInternal, "delete ai knowledge item failed")
	}
	s.recordAudit(ctx, actor.UserID, itemID, AuditKnowledgeReview, map[string]any{
		"item_id": itemID, "action": "delete", "result": "success",
	})
	return nil
}

// Injection 取生效知识并生成附加段；任何失败都降级为空注入。
func (s *KnowledgeService) Injection(ctx context.Context, applicationID, resourceType string) KnowledgeInjection {
	if s == nil || s.repo == nil {
		return KnowledgeInjection{Status: KnowledgeStatusSkipped}
	}
	items, err := s.repo.ListActiveForTarget(ctx, applicationID, resourceType)
	if err != nil {
		logger.From(ctx).Warn("ai knowledge lookup failed", logger.Error(err))
		return KnowledgeInjection{Status: KnowledgeStatusError}
	}
	selected := domain.SelectKnowledgeForInjection(items, applicationID, resourceType)
	if len(selected) == 0 {
		return KnowledgeInjection{Status: KnowledgeStatusEmpty}
	}
	return KnowledgeInjection{Items: selected, Appendix: domain.KnowledgeAppendix(selected), Status: KnowledgeStatusOK}
}

// RecordInjection 累加命中次数。
func (s *KnowledgeService) RecordInjection(ctx context.Context, itemIDs []string) {
	ids := normalizeItemIDs(itemIDs)
	if s == nil || s.repo == nil || len(ids) == 0 {
		return
	}
	if err := s.repo.AddHit(ctx, ids, s.now()); err != nil {
		logger.From(ctx).Warn("ai knowledge hit record failed", logger.Error(err))
	}
}

// RecordUseful 标记条目有效并评估是否需要自动停用。
func (s *KnowledgeService) RecordUseful(ctx context.Context, itemIDs []string) {
	ids := normalizeItemIDs(itemIDs)
	if s == nil || s.repo == nil || len(ids) == 0 {
		return
	}
	now := s.now()
	if err := s.repo.AddUseful(ctx, ids, now); err != nil {
		logger.From(ctx).Warn("ai knowledge useful record failed", logger.Error(err))
		return
	}
	s.autoDisable(ctx, ids)
}

// RecordUsageByFeedback 依据反馈更新知识质量：
//   - valid：useful_count +1，并评估低效停用；
//   - invalid：只评估低效停用（命中多但从未有效）。
func (s *KnowledgeService) RecordUsageByFeedback(ctx context.Context, itemIDs []string, useful bool) {
	if useful {
		s.RecordUseful(ctx, itemIDs)
		return
	}
	s.autoDisable(ctx, normalizeItemIDs(itemIDs))
}

// autoDisable 对「注入多但从未被标记有效」的条目自动停用并写审计。
func (s *KnowledgeService) autoDisable(ctx context.Context, itemIDs []string) {
	if s == nil || s.repo == nil || len(itemIDs) == 0 {
		return
	}
	for _, itemID := range itemIDs {
		item, err := s.repo.Get(ctx, itemID)
		if err != nil || item == nil {
			continue
		}
		if !item.ShouldAutoDisable() {
			continue
		}
		item.Status = domain.KnowledgeStatusDisabled
		item.ReviewedBy = ""
		item.ReviewedAt = nil
		item.UpdatedAt = s.now()
		if err := s.repo.Update(ctx, *item); err != nil {
			logger.From(ctx).Warn("ai knowledge auto disable failed",
				logger.String("item_id", itemID), logger.Error(err))
			continue
		}
		s.recordAudit(ctx, "", item.ItemID, AuditKnowledgeReview, map[string]any{
			"item_id": item.ItemID, "action": "auto_disable",
			"status": string(item.Status), "hit_count": item.HitCount, "useful_count": item.UsefulCount,
			"result": "success",
		})
		logger.From(ctx).Info("ai knowledge auto disabled",
			logger.String("item_id", item.ItemID), logger.Int("hit_count", item.HitCount))
	}
}

func (s *KnowledgeService) load(ctx context.Context, itemID string) (*domain.KnowledgeItem, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "item_id is required")
	}
	item, err := s.repo.Get(ctx, itemID)
	if err != nil {
		if errors.Is(err, domain.ErrKnowledgeItemNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "knowledge item not found")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load ai knowledge item failed")
	}
	if item == nil {
		return nil, apperr.New(apperr.CodeNotFound, "knowledge item not found")
	}
	return item, nil
}

func (s *KnowledgeService) recordAudit(ctx context.Context, userID, itemID string, action AuditAction, payload map[string]any) {
	if s == nil || s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "ai", ResourceID: itemID, Action: action, UserID: userID, Payload: payload,
	})
}

func normalizeItemIDs(itemIDs []string) []string {
	out := make([]string, 0, len(itemIDs))
	seen := make(map[string]struct{}, len(itemIDs))
	for _, id := range itemIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
