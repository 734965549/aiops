package application

import "context"

// AuditAction AI 审计动作。
type AuditAction string

const (
	AuditAnalyzeAlert AuditAction = "analyze_alert"
	AuditToolInvoke   AuditAction = "tool_invoke"
	AuditChatSession  AuditAction = "chat_session"
	AuditChatMessage  AuditAction = "chat_message"
	AuditChatFeedback AuditAction = "chat_feedback"
	// AuditChatSessionStatus 覆盖会话归档与恢复（`status` 字段记录目标状态）。
	AuditChatSessionStatus AuditAction = "chat_session_status"
	// AuditChatSessionDelete 覆盖会话删除（会话与其消息一并删除）。
	AuditChatSessionDelete AuditAction = "chat_session_delete"
	// AuditKnowledgeEdit 覆盖知识条目的新建与编辑。
	AuditKnowledgeEdit AuditAction = "knowledge_edit"
	// AuditKnowledgeReview 覆盖知识条目的审核激活/停用/删除与自动停用。
	AuditKnowledgeReview AuditAction = "knowledge_review"
	// AuditMemoryReview 覆盖记忆条目的手动停用与启用。
	AuditMemoryReview AuditAction = "memory_review"
	// AuditChatCanceled 覆盖流式对话被客户端取消（不落 assistant 消息）。
	AuditChatCanceled AuditAction = "chat_canceled"
	// AuditChatShareCreate 覆盖会话只读分享链接的创建（payload 不含 token）。
	AuditChatShareCreate AuditAction = "chat_share_create"
	// AuditChatShareRevoke 覆盖分享链接的手动撤销。
	AuditChatShareRevoke AuditAction = "chat_share_revoke"
	// AuditChatShareView 覆盖通过分享链接读取会话（按访问者记录）。
	AuditChatShareView AuditAction = "chat_share_view"
)

// AuditRecord 审计载荷。
type AuditRecord struct {
	ResourceType string
	ResourceID   string
	Action       AuditAction
	UserID       string
	Payload      map[string]any
}

// AuditRecorder 审计接口。
type AuditRecorder interface {
	Record(ctx context.Context, rec AuditRecord) error
}

// NoopAuditRecorder 空实现。
type NoopAuditRecorder struct{}

func (NoopAuditRecorder) Record(context.Context, AuditRecord) error { return nil }
