package application

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
)

// chatContextData 是对话上下文与只读观测的聚合结果。
type chatContextData struct {
	Alert        *AlertContext
	Evidence     *MetricEvidenceResult
	Observations []ChatObservation
	ToolCalls    []domain.ToolCall
	References   []domain.Reference
	// AlertDenied 表示告警上下文因数据范围被拒绝；此时不展示告警引用与跳转入口。
	AlertDenied bool
	// Memories 是命中并注入 Prompt 的历史案例；MemoryStatus 记录检索引擎状态。
	Memories     []domain.MemoryCase
	MemoryStatus string
	// KnowledgeItems 是命中并注入的知识条目；Appendix 进 system prompt，Status 记审计。
	KnowledgeItems    []domain.KnowledgeItem
	KnowledgeAppendix string
	KnowledgeStatus   string
	// LoopRounds 是回合内追加只读取数的轮数（阶段 3 审计字段）。
	LoopRounds int
}

// gatherContext 按会话页面上下文执行只读工具权限校验与取数（流程图 §16 的 J1/J4）。
// 任何工具失败都只记录观测状态，不阻断对话。
//
// 顺序：先校验 alarm.analyze 的 RBAC 与工具权限（不读对象），再由告警读取端口按
// 对象数据范围守卫；对象级数据范围只在读取端口收口，避免两处口径分叉，也避免
// 无权用户通过拒绝原因探测告警是否存在。
func (s *ChatService) gatherContext(ctx context.Context, userID string, session *domain.Session, userMessageID string) chatContextData {
	var data chatContextData
	alertID := strings.TrimSpace(session.Context.AlertID)
	if alertID == "" || s.alerts == nil {
		return data
	}
	alertObs := s.checkTool(ctx, userID, chatToolAlarmAnalyze)
	if !alertObs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, userMessageID, &data, alertObs)
		return data
	}
	alert, err := s.alerts.GetForAnalysis(ctx, userID, alertID)
	if err != nil {
		s.appendAlertLoadFailure(ctx, userID, session.SessionID, userMessageID, &data, alertID, err)
		return data
	}
	data.Alert = alert
	alertObs.Status = "ok"
	alertObs.Summary = fmt.Sprintf("已读取告警 %s（%s）", alert.Name, alert.Severity)
	alertObs.Data = map[string]any{
		"alert_id": alert.ID, "name": alert.Name, "severity": alert.Severity,
		"status": alert.Status, "application_name": alert.ApplicationName,
		"resource_name": alert.ResourceName, "occurrence_count": alert.OccurrenceCount,
	}
	s.appendToolResult(ctx, userID, session.SessionID, userMessageID, &data, alertObs)
	data.References = append(data.References, domain.Reference{
		Type: "alert", ID: alert.ID, Label: alert.Name,
		URL: "/alerts?alert_id=" + url.QueryEscape(alert.ID),
	})

	metricObs, evidence := s.collectMetricObservation(ctx, userID, alert)
	if metricObs != nil {
		s.appendToolResult(ctx, userID, session.SessionID, userMessageID, &data, *metricObs)
	}
	data.Evidence = evidence
	if data.Evidence != nil {
		ids := evidenceIDs(data.Evidence)
		for _, id := range ids {
			data.References = append(data.References, domain.Reference{
				Type: "observability_evidence", ID: id,
				Label: "指标证据 " + id, URL: "/observability",
			})
		}
	}
	return data
}

// appendAlertLoadFailure 把告警读取失败降级为一次工具观测：数据范围拒绝标记 denied，
// 其它错误标记 error；两种情况都不返回告警内容与引用。
func (s *ChatService) appendAlertLoadFailure(ctx context.Context, userID, sessionID, messageID string, data *chatContextData, alertID string, err error) {
	obs := ChatObservation{ToolCode: chatToolAlarmAnalyze, Status: "error", Reason: "alert_load_failed"}
	switch code := apperr.CodeOf(err); code {
	case apperr.CodePermissionDenied:
		obs.Status = "denied"
		obs.Reason = "data_scope_denied"
		data.AlertDenied = true
	case apperr.CodeNotFound:
		obs.Reason = "alert_not_found"
	}
	logger.From(ctx).Warn("ai chat alert context load failed",
		logger.String("alert_id", alertID),
		logger.String("status", obs.Status),
		logger.String("reason", obs.Reason),
		logger.Error(err))
	s.appendToolResult(ctx, userID, sessionID, messageID, data, obs)
}

func (s *ChatService) collectMetricObservation(ctx context.Context, userID string, alert *AlertContext) (*ChatObservation, *MetricEvidenceResult) {
	if s.evidence == nil || alert == nil {
		return nil, nil
	}
	query, skip := deriveMetricEvidenceQuery(alert)
	if skip != "" {
		return nil, nil
	}
	obs := s.checkTool(ctx, userID, chatToolMetricsQuery)
	if !obs.Allowed {
		return &obs, nil
	}
	result, err := s.evidence.Collect(ctx, userID, query)
	if err != nil {
		obs.Status = "error"
		obs.Reason = "evidence_collect_failed"
		logger.From(ctx).Warn("ai chat metric evidence failed", logger.Error(err))
		return &obs, nil
	}
	result = capEvidencePayload(result)
	if result == nil {
		obs.Status = "error"
		obs.Reason = "empty evidence result"
		return &obs, nil
	}
	obs.Status = result.Status
	obs.Reason = result.Reason
	obs.Summary = fmt.Sprintf("指标证据 %s（%s）", result.Status, query.Metric)
	obs.Data = map[string]any{
		"status": result.Status, "reason": result.Reason,
		"metric": query.Metric, "metric_count": len(result.Metrics),
		"evidence_ids": evidenceIDs(result),
	}
	return &obs, result
}

// checkTool 校验工具 RBAC 与工具权限。
//
// 语义：
//   - 指标取数：数据范围由 MetricEvidencePort 按账号二次校验；
//   - 告警快照：RBAC/工具权限在此校验，对象级数据范围由 AlertReader 读取端口统一收口
//     （读取发生在权限校验之后，避免无权用户探测告警是否存在）；
//   - 无对象上下文的调用（如 execution.tasks.propose）跳过数据范围，避免被误判为缺少对象上下文。
func (s *ChatService) checkTool(ctx context.Context, userID, toolCode string) ChatObservation {
	obs := ChatObservation{ToolCode: toolCode}
	if s.tools == nil {
		obs.Status = "denied"
		obs.Reason = "tool gateway is not configured"
		return obs
	}
	resp, err := s.tools.Validate(ctx, toolgateway.ToolRequest{
		UserID: userID, ToolCode: toolCode, RequiredPermission: chatPermission, SkipDataScope: true,
	})
	if err != nil {
		obs.Status = "denied"
		obs.Reason = "authorization failed"
		if ae := apperr.FromError(err); ae != nil && strings.TrimSpace(ae.Message) != "" {
			obs.Reason = ae.Message
		}
		return obs
	}
	if resp == nil || !resp.Allowed {
		obs.Status = "denied"
		obs.Reason = "tool permission denied"
		if resp != nil {
			obs.Mode = resp.Mode
			if strings.TrimSpace(resp.Reason) != "" {
				obs.Reason = resp.Reason
			}
		}
		return obs
	}
	obs.Allowed = true
	obs.Mode = resp.Mode
	obs.Status = "ok"
	return obs
}

func (s *ChatService) appendToolResult(ctx context.Context, userID, sessionID, messageID string, data *chatContextData, obs ChatObservation) {
	if strings.TrimSpace(obs.Status) == "" {
		obs.Status = "ok"
	}
	data.Observations = append(data.Observations, obs)
	// 流式通道存在时同步推送工具事件；推送失败不阻断主链路。
	_ = emitChatEvent(ctx, toolEventFromObservation(obs))
	data.ToolCalls = append(data.ToolCalls, domain.ToolCall{
		ToolCode: obs.ToolCode, Allowed: obs.Allowed, Mode: obs.Mode,
		Reason: obs.Reason, Status: obs.Status, Summary: obs.Summary,
	})
	s.recordAudit(ctx, userID, "ai", sessionID, AuditToolInvoke, map[string]any{
		"session_id": sessionID,
		"message_id": messageID,
		"tool_code":  obs.ToolCode,
		"allowed":    obs.Allowed,
		"mode":       obs.Mode,
		"status":     obs.Status,
		"reason":     obs.Reason,
		"result":     toolAuditResult(obs),
	})
}
