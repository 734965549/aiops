// Package domain 定义 AI 助手知识条目（阶段 2）的领域模型与注入规则。
//
// 知识条目是「经人工审核后生效」的经验沉淀：draft 不进入回答，只有 active 才会
// 按 scope 注入 system prompt 附加段；disabled 立即失效。条目只存摘要与业务 ID，
// 不保存告警正文、Prompt 或任何凭据。
package domain

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// 知识条目状态。
type KnowledgeStatus string

const (
	KnowledgeStatusDraft    KnowledgeStatus = "draft"
	KnowledgeStatusActive   KnowledgeStatus = "active"
	KnowledgeStatusDisabled KnowledgeStatus = "disabled"
)

func (s KnowledgeStatus) IsValid() bool {
	switch s {
	case KnowledgeStatusDraft, KnowledgeStatusActive, KnowledgeStatusDisabled:
		return true
	default:
		return false
	}
}

// 知识条目生效范围。
type KnowledgeScope string

const (
	KnowledgeScopeGlobal       KnowledgeScope = "global"
	KnowledgeScopeApplication  KnowledgeScope = "application"
	KnowledgeScopeResourceType KnowledgeScope = "resource_type"
)

func (s KnowledgeScope) IsValid() bool {
	switch s {
	case KnowledgeScopeGlobal, KnowledgeScopeApplication, KnowledgeScopeResourceType:
		return true
	default:
		return false
	}
}

// 知识条目来源。
type KnowledgeOrigin string

const (
	KnowledgeOriginFeedbackAggregate KnowledgeOrigin = "feedback_aggregate"
	KnowledgeOriginManual            KnowledgeOrigin = "manual"
)

func (o KnowledgeOrigin) IsValid() bool {
	switch o {
	case KnowledgeOriginFeedbackAggregate, KnowledgeOriginManual:
		return true
	default:
		return false
	}
}

const (
	// KnowledgeMaxInjectedItems 单次回答注入的条目数上限。
	KnowledgeMaxInjectedItems = 5
	// KnowledgeMaxInjectedRunes 附加段总长度上限（含标题与内容）。
	KnowledgeMaxInjectedRunes = 3000
	// KnowledgeMaxTitleRunes 条目标题上限。
	KnowledgeMaxTitleRunes = 120
	// KnowledgeMaxContentRunes 条目内容上限。
	KnowledgeMaxContentRunes = 2000
	// KnowledgeAutoDisableMinHits 自动停用前要求的最小注入次数。
	KnowledgeAutoDisableMinHits = 5
)

// ErrKnowledgeItemNotFound 表示知识条目不存在。
var ErrKnowledgeItemNotFound = errors.New("ai knowledge item not found")

// ErrKnowledgeInvalid 表示知识条目字段非法。
var ErrKnowledgeInvalid = errors.New("invalid ai knowledge item")

// KnowledgeItemFilter 是列表查询条件（status/scope 为空表示不过滤）。
type KnowledgeItemFilter struct {
	Status *KnowledgeStatus
	Scope  *KnowledgeScope
	Limit  int
	Offset int
}

// KnowledgeItem 是一条待审核或已生效的知识。
type KnowledgeItem struct {
	ItemID      string
	Scope       KnowledgeScope
	ScopeRef    string
	Title       string
	Content     string
	Origin      KnowledgeOrigin
	Status      KnowledgeStatus
	HitCount    int
	UsefulCount int
	ReviewedBy  string
	ReviewedAt  *time.Time
	CreatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Normalize 去除首尾空白并约束长度。
func (i KnowledgeItem) Normalize() KnowledgeItem {
	i.ScopeRef = strings.TrimSpace(i.ScopeRef)
	i.Title = truncateRunes(strings.TrimSpace(i.Title), KnowledgeMaxTitleRunes)
	i.Content = truncateRunes(strings.TrimSpace(i.Content), KnowledgeMaxContentRunes)
	i.CreatedBy = strings.TrimSpace(i.CreatedBy)
	i.ReviewedBy = strings.TrimSpace(i.ReviewedBy)
	return i
}

// Validate 校验状态/范围/来源与必填字段。
//
// 规则：global 范围必须没有 scope_ref；application / resource_type 必须指定 scope_ref。
func (i KnowledgeItem) Validate() error {
	if !i.Scope.IsValid() {
		return wrapKnowledgeInvalid("unknown scope")
	}
	if !i.Origin.IsValid() {
		return wrapKnowledgeInvalid("unknown origin")
	}
	if i.Status != "" && !i.Status.IsValid() {
		return wrapKnowledgeInvalid("unknown status")
	}
	if i.Title == "" {
		return wrapKnowledgeInvalid("title is required")
	}
	if i.Content == "" {
		return wrapKnowledgeInvalid("content is required")
	}
	if i.Scope == KnowledgeScopeGlobal {
		if i.ScopeRef != "" {
			return wrapKnowledgeInvalid("global scope must not carry scope_ref")
		}
		return nil
	}
	if i.ScopeRef == "" {
		return wrapKnowledgeInvalid("scope_ref is required for non-global scope")
	}
	return nil
}

// ShouldAutoDisable 判断低效条目是否应自动停用：
// 已被注入达到阈值次数却从未被标记有效。已停用/草稿条目不参与。
func (i KnowledgeItem) ShouldAutoDisable() bool {
	if i.Status != KnowledgeStatusActive {
		return false
	}
	return i.HitCount >= KnowledgeAutoDisableMinHits && i.UsefulCount == 0
}

// MatchesTarget 判断条目是否对给定应用/资源类型生效。
func (i KnowledgeItem) MatchesTarget(applicationID, resourceType string) bool {
	switch i.Scope {
	case KnowledgeScopeGlobal:
		return true
	case KnowledgeScopeApplication:
		return strings.EqualFold(strings.TrimSpace(i.ScopeRef), strings.TrimSpace(applicationID)) &&
			strings.TrimSpace(applicationID) != ""
	case KnowledgeScopeResourceType:
		return strings.EqualFold(strings.TrimSpace(i.ScopeRef), strings.TrimSpace(resourceType)) &&
			strings.TrimSpace(resourceType) != ""
	default:
		return false
	}
}

// SelectKnowledgeForInjection 从候选条目中挑选注入项：
// 仅 active、按 scope 匹配；命中次数高者优先，其次最近更新；受条数与总长双重上限约束。
func SelectKnowledgeForInjection(items []KnowledgeItem, applicationID, resourceType string) []KnowledgeItem {
	candidates := make([]KnowledgeItem, 0, len(items))
	for _, item := range items {
		if item.Status != KnowledgeStatusActive {
			continue
		}
		if !item.MatchesTarget(applicationID, resourceType) {
			continue
		}
		candidates = append(candidates, item)
	}
	sort.SliceStable(candidates, func(a, b int) bool {
		if candidates[a].HitCount != candidates[b].HitCount {
			return candidates[a].HitCount > candidates[b].HitCount
		}
		return candidates[a].UpdatedAt.After(candidates[b].UpdatedAt)
	})
	out := make([]KnowledgeItem, 0, KnowledgeMaxInjectedItems)
	total := 0
	for _, item := range candidates {
		cost := len([]rune(item.Title)) + len([]rune(item.Content))
		if len(out) >= KnowledgeMaxInjectedItems {
			break
		}
		if total+cost > KnowledgeMaxInjectedRunes {
			continue
		}
		total += cost
		out = append(out, item)
	}
	return out
}

// KnowledgeAppendix 生成 system prompt 附加段（服务端构建，固定前缀，便于审计与断言）。
func KnowledgeAppendix(items []KnowledgeItem) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("知识沉淀（管理员审核后生效，仅作参考；引用时标注 item_id，不得照搬结论）：\n")
	for _, item := range items {
		b.WriteString("- [")
		b.WriteString(item.ItemID)
		b.WriteString("] ")
		b.WriteString(item.Title)
		b.WriteString("：")
		b.WriteString(item.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func wrapKnowledgeInvalid(reason string) error {
	return errors.Join(ErrKnowledgeInvalid, errors.New(reason))
}
