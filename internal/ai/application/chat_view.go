package application

import (
	"context"

	"github.com/734965549/aiops/internal/ai/domain"
)

func toSessionView(session domain.Session) ChatSessionView {
	return ChatSessionView{
		SessionID: session.SessionID, Title: session.Title, Status: string(session.Status),
		Context: session.Context, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt,
	}
}

func toMessageView(message domain.Message) ChatMessageView {
	view := ChatMessageView{
		MessageID: message.MessageID, SessionID: message.SessionID, Role: string(message.Role),
		Content: message.Content, Images: message.Images, ToolCalls: message.ToolCalls, References: message.References,
		Feedback: string(message.Feedback), CreatedAt: message.CreatedAt,
	}
	if view.ToolCalls == nil {
		view.ToolCalls = []domain.ToolCall{}
	}
	if view.References == nil {
		view.References = []domain.Reference{}
	}
	if message.Role == domain.RoleAssistant {
		if answer, err := parseChatAnswer(message.Content); err == nil {
			view.Answer = &answer
		}
	}
	return view
}

func (s *ChatService) recordAudit(ctx context.Context, userID, resourceType, resourceID string, action AuditAction, payload map[string]any) {
	if s == nil || s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: resourceType, ResourceID: resourceID,
		Action: action, UserID: userID, Payload: payload,
	})
}

func auditResult(status string) string {
	if status == chatStatusOK || status == chatStatusInsufficientContext {
		return "success"
	}
	return "partial"
}

func toolAuditResult(obs ChatObservation) string {
	if !obs.Allowed {
		return "denied"
	}
	if obs.Status == "ok" {
		return "success"
	}
	return "partial"
}

func toolCodes(calls []domain.ToolCall) []string {
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		out = append(out, call.ToolCode)
	}
	return out
}

func deniedToolCodes(calls []domain.ToolCall) []string {
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		if !call.Allowed {
			out = append(out, call.ToolCode)
		}
	}
	return out
}
