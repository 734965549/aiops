// Package domain 定义 AI 助手记忆（相似案例）领域模型与规则。
//
// 记忆条目只保存脱敏摘要、建议与业务 ID，不保存告警正文、Prompt 或任何凭据；
// 检索必须按会话归属用户过滤，避免跨用户泄漏案例原文。
package domain

import (
	"crypto/sha1"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// MemoryCaseSource 表示记忆条目的来源。
type MemoryCaseSource string

const (
	MemoryCaseSourceChatMessage   MemoryCaseSource = "chat_message"
	MemoryCaseSourceAlertAnalysis MemoryCaseSource = "alert_analysis"
	MemoryCaseSourceKnowledgeItem MemoryCaseSource = "knowledge_item"
)

func (s MemoryCaseSource) IsValid() bool {
	switch s {
	case MemoryCaseSourceChatMessage, MemoryCaseSourceAlertAnalysis, MemoryCaseSourceKnowledgeItem:
		return true
	default:
		return false
	}
}

// MemoryCaseStatus 表示记忆条目状态（retired 不再参与检索）。
type MemoryCaseStatus string

const (
	MemoryCaseStatusActive  MemoryCaseStatus = "active"
	MemoryCaseStatusRetired MemoryCaseStatus = "retired"
)

func (s MemoryCaseStatus) IsValid() bool {
	switch s {
	case MemoryCaseStatusActive, MemoryCaseStatusRetired:
		return true
	default:
		return false
	}
}

const (
	// MemoryCaseMaxMatches 单次检索注入的案例条数上限（阶段 1 定为 3，控制 Prompt 体积）。
	MemoryCaseMaxMatches = 3
	// MemoryCaseSummaryRunes 单条案例摘要注入上限。
	MemoryCaseSummaryRunes = 500
	// MemoryCaseMaxRecommendations 单条案例保留的建议条数上限。
	MemoryCaseMaxRecommendations = 3
	// MemoryCaseValidDelta 有效反馈的质量分增量。
	MemoryCaseValidDelta = 1
	// MemoryCaseInvalidDelta 无效反馈的质量分增量。
	MemoryCaseInvalidDelta = -2
	// MemoryCaseRetireScore 质量分降到该值及以下时条目转 retired。
	MemoryCaseRetireScore = -2
	// MemoryCaseKeywordsRunes 关键词串长度上限。
	MemoryCaseKeywordsRunes = 512
)

// MemoryCase 是一条可复用的历史案例。
type MemoryCase struct {
	CaseID          string
	SourceType      MemoryCaseSource
	SourceMessageID string
	SessionID       string
	UserID          string
	ApplicationID   string
	ResourceID      string
	ResourceType    string
	AlertSignature  string
	Keywords        string
	Summary         string
	Recommendations []string
	RiskLevel       string
	QualityScore    int
	Status          MemoryCaseStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// MemoryCaseFilter 是检索条件；字段均为空时按关键词匹配兜底。
type MemoryCaseFilter struct {
	UserID         string
	AlertSignature string
	ApplicationID  string
	ResourceID     string
	Keywords       string
	Limit          int
}

// MatchPriority 返回检索优先级（越大越优先），与仓储 SQL 的 match_rank 一致。
func (f MemoryCaseFilter) MatchPriority(c MemoryCase) int {
	switch {
	case f.AlertSignature != "" && strings.EqualFold(c.AlertSignature, f.AlertSignature):
		return 3
	case f.ApplicationID != "" && strings.EqualFold(c.ApplicationID, f.ApplicationID):
		return 2
	case f.ResourceID != "" && strings.EqualFold(c.ResourceID, f.ResourceID):
		return 2
	case f.Keywords != "" && containsKeyword(c.Keywords, f.Keywords):
		return 1
	default:
		return 0
	}
}

// ApplyFeedback 应用一次反馈，返回新的质量分与目标状态。
// 有效反馈只提分；无效反馈降分并在达到阈值时转 retired，已 retired 的条目不再复活。
func (c MemoryCase) ApplyFeedback(valid bool) (score int, status MemoryCaseStatus) {
	score = c.QualityScore
	status = c.Status
	if status == "" {
		status = MemoryCaseStatusActive
	}
	if valid {
		return score + MemoryCaseValidDelta, status
	}
	score += MemoryCaseInvalidDelta
	if score <= MemoryCaseRetireScore {
		status = MemoryCaseStatusRetired
	}
	return score, status
}

// BuildAlertSignature 生成告警指纹：alert_name + metric + 维度哈希。
// 名称与指标归一化为小写，维度按 key 排序后取 sha1 前 8 字节，保证同类告警稳定命中。
func BuildAlertSignature(alertName, metric string, dimensions map[string]string) string {
	name := strings.ToLower(strings.TrimSpace(alertName))
	metric = strings.ToLower(strings.TrimSpace(metric))
	if name == "" && metric == "" && len(dimensions) == 0 {
		return ""
	}
	keys := make([]string, 0, len(dimensions))
	for key, value := range dimensions {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, strings.ToLower(strings.TrimSpace(key)))
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+strings.TrimSpace(dimensions[key]))
	}
	sum := sha1.Sum([]byte(name + "|" + metric + "|" + strings.Join(pairs, ",")))
	return fmt.Sprintf("%s|%s|%x", name, metric, sum[:8])
}

// BuildMemoryKeywords 从摘要、建议与上下文中提取检索关键词。
// 平台默认使用 PostgreSQL simple 分词，因此这里按标点/空白切词并去重限长。
func BuildMemoryKeywords(parts ...string) string {
	seen := make(map[string]struct{}, 32)
	tokens := make([]string, 0, 32)
	for _, part := range parts {
		for _, token := range strings.FieldsFunc(part, isKeywordSeparator) {
			token = strings.ToLower(strings.TrimSpace(token))
			if len([]rune(token)) < 2 {
				continue
			}
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			tokens = append(tokens, token)
			if len(tokens) >= 40 {
				return truncateRunes(strings.Join(tokens, " "), MemoryCaseKeywordsRunes)
			}
		}
	}
	return truncateRunes(strings.Join(tokens, " "), MemoryCaseKeywordsRunes)
}

// CaseRefLabels 渲染引用标签时使用的前缀。
const MemoryCaseReferenceLabel = "历史案例"

func isKeywordSeparator(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case ',', '.', ';', ':', '，', '。', '；', '：', '、', '!', '！', '?', '？',
		'(', ')', '（', '）', '[', ']', '【', '】', '"', '\'', '“', '”', '‘', '’',
		'/', '\\', '|', '-', '_', '+', '=', '<', '>', '·', '\n', '\t':
		return true
	default:
		return false
	}
}

func containsKeyword(haystack, needle string) bool {
	haystack = strings.ToLower(haystack)
	for _, token := range strings.Fields(strings.ToLower(needle)) {
		if len([]rune(token)) < 2 {
			continue
		}
		if strings.Contains(haystack, token) {
			return true
		}
	}
	return false
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
