package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/google/uuid"
)

// MemoryService 实现相似案例记忆的阶段 1 闭环（docs/ai-assistant-memory-roadmap.md §4）：
// 有效反馈沉淀案例、无效反馈降分淘汰、发送消息前检索注入。
//
// 所有失败都只降级并记日志：记忆不是回答主链路的前置条件。
type MemoryService struct {
	repo   MemoryCaseRepository
	alerts AlertReader
	now    func() time.Time
}

func NewMemoryService(repo MemoryCaseRepository, alerts AlertReader) *MemoryService {
	return &MemoryService{repo: repo, alerts: alerts, now: time.Now}
}

var _ MemoryCaseStore = (*MemoryService)(nil)

// SearchSimilar 按告警指纹 / 应用 / 资源 / 关键词检索当前用户的相似案例。
func (s *MemoryService) SearchSimilar(ctx context.Context, in MemorySearchInput) MemorySearchResult {
	if s == nil || s.repo == nil {
		return MemorySearchResult{Status: MemoryStatusSkipped}
	}
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return MemorySearchResult{Status: MemoryStatusSkipped}
	}
	metric, dimensions := memorySignatureParts(in.Evidence)
	alertNameValue := ""
	if in.Alert != nil {
		alertNameValue = in.Alert.Name
		alertMetric, alertDimensions := alertSignatureParts(in.Alert)
		if metric == "" {
			metric = alertMetric
		}
		if len(dimensions) == 0 {
			dimensions = alertDimensions
		}
	}
	filter := domain.MemoryCaseFilter{
		UserID:         userID,
		AlertSignature: domain.BuildAlertSignature(alertNameValue, metric, dimensions),
		ApplicationID:  strings.TrimSpace(in.Context.ApplicationID),
		ResourceID:     strings.TrimSpace(in.Context.ResourceID),
		Keywords:       s.searchKeywords(in),
		Limit:          domain.MemoryCaseMaxMatches,
	}
	if filter.ApplicationID == "" && in.Alert != nil {
		filter.ApplicationID = strings.TrimSpace(in.Alert.ApplicationID)
	}
	if filter.ResourceID == "" && in.Alert != nil {
		filter.ResourceID = strings.TrimSpace(in.Alert.ResourceID)
	}
	if filter.AlertSignature == "" && filter.ApplicationID == "" && filter.ResourceID == "" && filter.Keywords == "" {
		return MemorySearchResult{Status: MemoryStatusEmpty}
	}
	cases, err := s.repo.Search(ctx, filter)
	if err != nil {
		logger.From(ctx).Warn("ai memory search failed", logger.Error(err))
		return MemorySearchResult{Status: MemoryStatusError}
	}
	if len(cases) == 0 {
		return MemorySearchResult{Status: MemoryStatusEmpty}
	}
	if len(cases) > domain.MemoryCaseMaxMatches {
		cases = cases[:domain.MemoryCaseMaxMatches]
	}
	return MemorySearchResult{Cases: cases, Status: MemoryStatusOK}
}

// RecordValidFeedback 把「有效反馈 + 非降级回答」沉淀为案例。
func (s *MemoryService) RecordValidFeedback(ctx context.Context, session *domain.Session, message *domain.Message) error {
	if s == nil || s.repo == nil || session == nil || message == nil {
		return nil
	}
	answer, err := parseChatAnswer(message.Content)
	if err != nil {
		logger.From(ctx).Warn("ai memory skip invalid answer json", logger.Error(err))
		return nil
	}
	// 降级回答不进入记忆，避免把「模型不可用」当成经验。
	if answer.Degraded || strings.TrimSpace(answer.Summary) == "" {
		return nil
	}
	alert := s.loadAlertForSignature(ctx, session)
	metric, dimensions := memorySignatureParts(nil)
	if alert != nil {
		alertMetric, alertDimensions := alertSignatureParts(alert)
		if metric == "" {
			metric = alertMetric
		}
		if len(dimensions) == 0 {
			dimensions = alertDimensions
		}
	}
	signature := domain.BuildAlertSignature(alertName(alert), metric, dimensions)
	caseID := uuid.NewString()
	now := s.now()
	c := domain.MemoryCase{
		CaseID:          caseID,
		SourceType:      domain.MemoryCaseSourceChatMessage,
		SourceMessageID: message.MessageID,
		SessionID:       session.SessionID,
		UserID:          session.UserID,
		ApplicationID:   session.Context.ApplicationID,
		ResourceID:      session.Context.ResourceID,
		ResourceType:    session.Context.ResourceType,
		AlertSignature:  signature,
		Keywords:        memoryKeywords(answer.Summary, strings.Join(answer.Recommendations, " "), alertName(alert), session.Context.ResourceType),
		Summary:         truncateRunes(answer.Summary, domain.MemoryCaseSummaryRunes),
		Recommendations: sanitizeList(answer.Recommendations),
		RiskLevel:       answer.RiskLevel,
		QualityScore:    domain.MemoryCaseValidDelta,
		Status:          domain.MemoryCaseStatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if len(c.Recommendations) > domain.MemoryCaseMaxRecommendations {
		c.Recommendations = c.Recommendations[:domain.MemoryCaseMaxRecommendations]
	}
	created, createErr := s.repo.Create(ctx, c)
	if createErr != nil {
		return createErr
	}
	if !created {
		// 幂等：同一条消息重复反馈不重复沉淀。
		logger.From(ctx).Info("ai memory case already exists", logger.String("message_id", message.MessageID))
		return nil
	}
	logger.From(ctx).Info("ai memory case created",
		logger.String("case_id", caseID), logger.String("session_id", session.SessionID))
	return nil
}

// RecordInvalidFeedback 降低已有案例的质量分，达到阈值时转 retired。
func (s *MemoryService) RecordInvalidFeedback(ctx context.Context, session *domain.Session, message *domain.Message) error {
	if s == nil || s.repo == nil || session == nil || message == nil {
		return nil
	}
	existing, err := s.repo.FindBySourceMessage(ctx, session.UserID, message.MessageID)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}
	score, status := existing.ApplyFeedback(false)
	existing.QualityScore = score
	existing.Status = status
	existing.UpdatedAt = s.now()
	if err := s.repo.UpdateQuality(ctx, *existing); err != nil {
		return err
	}
	logger.From(ctx).Info("ai memory case quality updated",
		logger.String("case_id", existing.CaseID), logger.Int("quality_score", score), logger.String("status", string(status)))
	return nil
}

// loadAlertForSignature 读取会话关联告警用于生成指纹；失败只记日志。
// 读取走 AlertReader 的数据范围守卫，用户已无权访问时自然拿不到。
func (s *MemoryService) loadAlertForSignature(ctx context.Context, session *domain.Session) *AlertContext {
	alertID := strings.TrimSpace(session.Context.AlertID)
	if alertID == "" || s.alerts == nil {
		return nil
	}
	alert, err := s.alerts.GetForAnalysis(ctx, session.UserID, alertID)
	if err != nil {
		logger.From(ctx).Warn("ai memory alert signature skipped",
			logger.String("alert_id", alertID), logger.Error(err))
		return nil
	}
	return alert
}

// searchKeywords 组合检索词：用户问题由调用方在 Keywords 传入，告警名与资源类型并入。
func (s *MemoryService) searchKeywords(in MemorySearchInput) string {
	parts := []string{in.Keywords, in.Context.ResourceType}
	if in.Alert != nil {
		parts = append(parts, in.Alert.Name, in.Alert.BusinessLine)
	}
	return memoryKeywords(parts...)
}

// memorySignatureParts 从指标证据提取签名用的指标名与维度。
func memorySignatureParts(evidence *MetricEvidenceResult) (string, map[string]string) {
	if evidence == nil || len(evidence.Metrics) == 0 {
		return "", nil
	}
	first := evidence.Metrics[0]
	dimensions := make(map[string]string, len(first.Labels))
	for key, value := range first.Labels {
		dimensions[key] = value
	}
	return strings.TrimSpace(first.Metric), dimensions
}

// alertSignatureDimensions 是参与告警指纹的标签键，与指标证据推导的维度键保持一致。
var alertSignatureDimensions = []string{"instance_id", "cloud_resource_id", "pod", "node"}

// alertSignatureParts 从告警快照提取指纹用的指标名与维度标签。
func alertSignatureParts(alert *AlertContext) (string, map[string]string) {
	if alert == nil {
		return "", nil
	}
	metric := strings.TrimSpace(alert.Labels["metric_name"])
	dimensions := make(map[string]string, len(alertSignatureDimensions))
	for _, key := range alertSignatureDimensions {
		if value := strings.TrimSpace(alert.Labels[key]); value != "" {
			dimensions[key] = value
		}
	}
	if len(dimensions) == 0 {
		return metric, nil
	}
	return metric, dimensions
}

func alertName(alert *AlertContext) string {
	if alert == nil {
		return ""
	}
	return strings.TrimSpace(alert.Name)
}

// MemoryCaseReference 把案例转成可回显的引用（url 为空，前端只展示标签）。
func MemoryCaseReference(c domain.MemoryCase) domain.Reference {
	label := domain.MemoryCaseReferenceLabel
	if summary := strings.TrimSpace(c.Summary); summary != "" {
		label = fmt.Sprintf("%s：%s", label, truncateRunes(summary, 80))
	}
	return domain.Reference{Type: "memory_case", ID: c.CaseID, Label: label}
}
