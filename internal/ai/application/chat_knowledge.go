package application

import (
	"context"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/pkg/logger"
)

// SetKnowledge 注入知识沉淀能力；nil 表示关闭（回答与未接入知识时一致）。
func (s *ChatService) SetKnowledge(knowledge KnowledgeStore) {
	if s == nil {
		return
	}
	s.knowledge = knowledge
}

// injectKnowledge 取生效知识并写入上下文：附加段进 system prompt，条目进 references。
// 任何失败只记录状态，不阻断回答。
func (s *ChatService) injectKnowledge(ctx context.Context, session *domain.Session, data *chatContextData) {
	if s.knowledge == nil {
		data.KnowledgeStatus = KnowledgeStatusSkipped
		return
	}
	applicationID := session.Context.ApplicationID
	resourceType := session.Context.ResourceType
	if data.Alert != nil {
		if applicationID == "" {
			applicationID = data.Alert.ApplicationID
		}
		if resourceType == "" {
			resourceType = data.Alert.ResourceType
		}
	}
	injection := s.knowledge.Injection(ctx, applicationID, resourceType)
	data.KnowledgeStatus = injection.Status
	if len(injection.Items) == 0 {
		return
	}
	data.KnowledgeItems = injection.Items
	data.KnowledgeAppendix = injection.Appendix
	for _, item := range injection.Items {
		data.References = append(data.References, domain.Reference{
			Type: "knowledge_item", ID: item.ItemID, Label: item.Title,
		})
	}
	logger.From(ctx).Info("ai chat knowledge injected",
		logger.String("session_id", session.SessionID),
		logger.Int("knowledge_used", len(injection.Items)))
}

// knowledgeItemIDs 提取本轮注入的条目 ID（用于命中统计）。
func knowledgeItemIDs(items []domain.KnowledgeItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ItemID)
	}
	return out
}

// knowledgeIDsFromReferences 从已落库的 references 还原注入过的知识条目。
func knowledgeIDsFromReferences(refs []domain.Reference) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Type == "knowledge_item" {
			out = append(out, ref.ID)
		}
	}
	return out
}

// recordKnowledgeInjection 累加命中次数；失败只记日志。
func (s *ChatService) recordKnowledgeInjection(ctx context.Context, items []domain.KnowledgeItem) {
	if s == nil || s.knowledge == nil || len(items) == 0 {
		return
	}
	s.knowledge.RecordInjection(ctx, knowledgeItemIDs(items))
}

// syncKnowledgeFeedback 把反馈回写到注入过的知识条目（有效 → useful，无效 → 低效评估）。
func (s *ChatService) syncKnowledgeFeedback(ctx context.Context, message *domain.Message, feedback domain.Feedback) {
	if s == nil || s.knowledge == nil || message == nil {
		return
	}
	ids := knowledgeIDsFromReferences(message.References)
	if len(ids) == 0 {
		return
	}
	switch feedback {
	case domain.FeedbackValid:
		s.knowledge.RecordUsageByFeedback(ctx, ids, true)
	case domain.FeedbackInvalid:
		s.knowledge.RecordUsageByFeedback(ctx, ids, false)
	}
}
