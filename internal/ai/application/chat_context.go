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
}

// gatherContext 按会话页面上下文执行只读工具权限校验与取数（流程图 §16 的 J1/J4）。
// 任何工具失败都只记录观测状态，不阻断对话。
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
	alert, err := s.alerts.GetForAnalysis(ctx, alertID)
	if err != nil {
		alertObs.Status = "error"
		alertObs.Reason = "alert_load_failed"
		logger.From(ctx).Warn("ai chat alert context load failed",
			logger.String("alert_id", alertID), logger.Error(err))
		s.appendToolResult(ctx, userID, session.SessionID, userMessageID, &data, alertObs)
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

func (s *ChatService) checkTool(ctx context.Context, userID, toolCode string) ChatObservation {
	obs := ChatObservation{ToolCode: toolCode}
	if s.tools == nil {
		obs.Status = "denied"
		obs.Reason = "tool gateway is not configured"
		return obs
	}
	// 工具权限校验只做 RBAC 与工具权限，不做对象级数据范围判断：
	//   - 指标取数：由 MetricEvidencePort 按账号数据权限二次校验；
	//   - 告警快照：当前与 /api/ai/analyze-alert 一致，仅校验告警读取/工具权限，
	//     尚未按用户数据范围过滤（P1 待补，见 ops/ai-contract.md §10.11）；
	// 若在此强制数据范围，无对象上下文的对话会被误判为“缺少对象上下文”。
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
