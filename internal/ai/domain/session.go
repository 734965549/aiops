// Package domain 定义 AI 运维助手会话上下文与仓储契约。
//
// 会话只保存用户可见的对话内容、页面上下文、工具调用结果摘要与证据引用；
// 不保存模型密钥、Prompt 明文或任何凭据。
package domain

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrSessionNotFound 表示会话不存在或不属于当前用户。
var ErrSessionNotFound = errors.New("ai chat session not found")

// ErrMessageNotFound 表示消息不存在或不属于该会话。
var ErrMessageNotFound = errors.New("ai chat message not found")

// MaxTitleRunes 是会话标题最大字符数（按 rune 截断，避免截断 UTF-8）。
const MaxTitleRunes = 60

// SessionStatus 表示会话状态。
type SessionStatus string

const (
	SessionStatusActive   SessionStatus = "active"
	SessionStatusArchived SessionStatus = "archived"
)

func (s SessionStatus) IsValid() bool {
	switch s {
	case SessionStatusActive, SessionStatusArchived:
		return true
	default:
		return false
	}
}

// MessageRole 表示消息角色。
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

func (r MessageRole) IsValid() bool {
	switch r {
	case RoleUser, RoleAssistant:
		return true
	default:
		return false
	}
}

// Feedback 表示用户对 AI 回答的反馈。
type Feedback string

const (
	FeedbackNone    Feedback = ""
	FeedbackValid   Feedback = "valid"
	FeedbackInvalid Feedback = "invalid"
)

func (f Feedback) IsValid() bool {
	switch f {
	case FeedbackNone, FeedbackValid, FeedbackInvalid:
		return true
	default:
		return false
	}
}

// SessionContext 是会话关联的页面上下文；只存业务标识，不存凭据。
type SessionContext struct {
	Page          string `json:"page"`
	PageTitle     string `json:"page_title"`
	AlertID       string `json:"alert_id"`
	ApplicationID string `json:"application_id"`
	ResourceID    string `json:"resource_id"`
	ResourceType  string `json:"resource_type"`
	TimeRange     string `json:"time_range"`
}

// IsEmpty 表示没有可用于 AI 推理的上下文。
func (c SessionContext) IsEmpty() bool {
	return strings.TrimSpace(c.AlertID) == "" &&
		strings.TrimSpace(c.ApplicationID) == "" &&
		strings.TrimSpace(c.ResourceID) == ""
}

// Normalize 去除首尾空白并约束长度。
func (c SessionContext) Normalize() SessionContext {
	return SessionContext{
		Page:          truncate(strings.TrimSpace(c.Page), 64),
		PageTitle:     truncate(strings.TrimSpace(c.PageTitle), 128),
		AlertID:       truncate(strings.TrimSpace(c.AlertID), 64),
		ApplicationID: truncate(strings.TrimSpace(c.ApplicationID), 64),
		ResourceID:    truncate(strings.TrimSpace(c.ResourceID), 128),
		ResourceType:  truncate(strings.TrimSpace(c.ResourceType), 64),
		TimeRange:     truncate(strings.TrimSpace(c.TimeRange), 32),
	}
}

// Session 是一次 AI 助手会话。
type Session struct {
	SessionID string
	UserID    string
	Title     string
	Context   SessionContext
	Status    SessionStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ToolCall 记录一次工具权限校验与调用结果（不含 Prompt 与凭据）。
type ToolCall struct {
	ToolCode string `json:"tool_code"`
	Allowed  bool   `json:"allowed"`
	Mode     string `json:"mode,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Status   string `json:"status"`
	Summary  string `json:"summary,omitempty"`
}

// Reference 是回答引用的数据来源，可跳转到对应页面。
type Reference struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Label string `json:"label"`
	URL   string `json:"url,omitempty"`
}

// Message 是会话中的一条消息。
// assistant 消息的 Content 保存结构化回答 JSON，读取时由 application 解析。
type Message struct {
	MessageID  string
	SessionID  string
	Role       MessageRole
	Content    string
	ToolCalls  []ToolCall
	References []Reference
	Feedback   Feedback
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SessionTouch 描述与消息落库同一事务内的会话更新。
type SessionTouch struct {
	// Title 非空时更新会话标题（首条用户消息写入）。
	Title string
	// Context 非 nil 时更新会话页面上下文（浮层从不同页面带入新上下文）。
	Context *SessionContext
	// UpdatedAt 用于推进会话排序时间。
	UpdatedAt time.Time
}

// SessionRepository 是 AI 会话的持久化端口。
type SessionRepository interface {
	Create(ctx context.Context, session Session) error
	Get(ctx context.Context, sessionID string) (*Session, error)
	ListByUser(ctx context.Context, userID string, limit int) ([]Session, error)
	// AppendMessage 在同一事务内写入消息并更新会话（标题/上下文/updated_at）。
	AppendMessage(ctx context.Context, message Message, touch SessionTouch) error
	ListMessages(ctx context.Context, sessionID string, limit int) ([]Message, error)
	// ListRecentMessages 返回最近 limit 条消息，按时间升序返回，用于对话与模型历史。
	ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]Message, error)
	GetMessage(ctx context.Context, sessionID, messageID string) (*Message, error)
	SetFeedback(ctx context.Context, sessionID, messageID string, feedback Feedback, updatedAt time.Time) error
}

// NewSessionTitle 从首条用户消息生成会话标题。
func NewSessionTitle(content string) string {
	title := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if title == "" {
		return "新会话"
	}
	return truncate(title, MaxTitleRunes)
}

func truncate(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
