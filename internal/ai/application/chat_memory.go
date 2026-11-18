package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/pkg/logger"
)

// chatMemoryCase 是注入 Prompt 的单条记忆案例（脱敏摘要 + 建议）。
type chatMemoryCase struct {
	CaseID          string   `json:"case_id"`
	Summary         string   `json:"summary"`
	Recommendations []string `json:"recommendations,omitempty"`
	RiskLevel       string   `json:"risk_level,omitempty"`
}

// SetMemory 注入相似案例记忆能力；nil 表示关闭（回答与未接入记忆时一致）。
//
// 记忆不是安全边界：关闭或检索失败只影响证据质量，不改变权限判断与回答主链路。
func (s *ChatService) SetMemory(memory MemoryCaseStore) {
	if s == nil {
		return
	}
	s.memory = memory
}

// injectMemory 在调用模型前检索相似案例：命中则写入 Prompt 输入与 references，
// 失败只记录 memory_status，不阻断回答。
func (s *ChatService) injectMemory(ctx context.Context, userID string, session *domain.Session, question string, data *chatContextData) {
	if s.memory == nil {
		data.MemoryStatus = MemoryStatusSkipped
		return
	}
	result := s.memory.SearchSimilar(ctx, MemorySearchInput{
		UserID:   userID,
		Alert:    data.Alert,
		Evidence: data.Evidence,
		Context:  session.Context,
		Keywords: question,
	})
	data.MemoryStatus = result.Status
	if len(result.Cases) == 0 {
		return
	}
	data.Memories = result.Cases
	for _, memoryCase := range result.Cases {
		data.References = append(data.References, MemoryCaseReference(memoryCase))
	}
	logger.From(ctx).Info("ai chat memory injected",
		logger.String("session_id", session.SessionID),
		logger.Int("memory_used", len(result.Cases)))
}

// memoryPromptCases 把领域案例转成 Prompt 输入（摘要截断，建议数有上限）。
func memoryPromptCases(cases []domain.MemoryCase) []chatMemoryCase {
	if len(cases) == 0 {
		return nil
	}
	out := make([]chatMemoryCase, 0, len(cases))
	for _, c := range cases {
		summary := strings.TrimSpace(c.Summary)
		if summary == "" {
			continue
		}
		recommendations := c.Recommendations
		if len(recommendations) > domain.MemoryCaseMaxRecommendations {
			recommendations = recommendations[:domain.MemoryCaseMaxRecommendations]
		}
		out = append(out, chatMemoryCase{
			CaseID:          c.CaseID,
			Summary:         truncateRunes(summary, domain.MemoryCaseSummaryRunes),
			Recommendations: recommendations,
			RiskLevel:       c.RiskLevel,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// syncMemoryFeedback 把反馈回流到记忆：有效反馈沉淀案例，无效反馈降低质量分。
// 失败只记 Warn，不影响反馈接口结果。
func (s *ChatService) syncMemoryFeedback(ctx context.Context, session *domain.Session, message *domain.Message, feedback domain.Feedback) {
	if s.memory == nil || session == nil || message == nil {
		return
	}
	var err error
	switch feedback {
	case domain.FeedbackValid:
		err = s.memory.RecordValidFeedback(ctx, session, message)
	case domain.FeedbackInvalid:
		err = s.memory.RecordInvalidFeedback(ctx, session, message)
	default:
		return
	}
	if err != nil {
		logger.From(ctx).Warn("ai memory feedback sync failed",
			logger.String("message_id", message.MessageID),
			logger.String("feedback", string(feedback)),
			logger.Error(err))
	}
}
