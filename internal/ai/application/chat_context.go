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
	Resource     *AssetContext
	Evidence     *MetricEvidenceResult
	Postgres     *PostgresEvidenceResult
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
	// 账号发现不依赖页面上下文：无告警/资源时也执行，让模型第 0 轮就能拿到可见 account_id 清单。
	s.gatherAccounts(ctx, userID, session, userMessageID, &data)
	alertID := strings.TrimSpace(session.Context.AlertID)
	if alertID == "" || s.alerts == nil {
		// 无告警上下文时尝试按资源上下文推导账号/区域/命名空间/维度。
		s.gatherResourceContext(ctx, userID, session, userMessageID, &data)
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
		appendMetricEvidenceReferences(&data, data.Evidence)
	}
	postgresObs, activity := s.collectPostgresObservation(ctx, userID, alert)
	if postgresObs != nil {
		s.appendToolResult(ctx, userID, session.SessionID, userMessageID, &data, *postgresObs)
	}
	data.Postgres = activity
	appendPostgresEvidenceReference(&data, activity)
	return data
}

// gatherAccounts 执行 accounts.list 工具并把可见账号写入观测；返回是否真正执行。
// 清单为空也是合法结果：模型据此提示用户到云账号接入页确认。
func (s *ChatService) gatherAccounts(ctx context.Context, userID string, session *domain.Session, userMessageID string, data *chatContextData) bool {
	if s.accounts == nil {
		return false
	}
	obs := s.checkTool(ctx, userID, chatToolAccountsList)
	if !obs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, userMessageID, data, obs)
		return false
	}
	return s.loadAccountObservation(ctx, userID, session, userMessageID, data, obs)
}

// loadAccountObservation 在工具权限已通过后执行账号发现并写入观测。
func (s *ChatService) loadAccountObservation(ctx context.Context, userID string, session *domain.Session, userMessageID string, data *chatContextData, obs ChatObservation) bool {
	result, err := s.accounts.List(ctx, userID)
	if err != nil {
		logger.From(ctx).Warn("ai chat account discovery failed", logger.Error(err))
		obs.Status = EvidenceStatusError
		obs.Reason = "account list failed"
		s.appendToolResult(ctx, userID, session.SessionID, userMessageID, data, obs)
		return false
	}
	if result == nil {
		obs.Status = EvidenceStatusError
		obs.Reason = "empty account list result"
		s.appendToolResult(ctx, userID, session.SessionID, userMessageID, data, obs)
		return false
	}
	obs.Status = result.Status
	if obs.Status == "" {
		obs.Status = EvidenceStatusOK
	}
	obs.Reason = result.Reason
	if obs.Status != EvidenceStatusOK {
		obs.Summary = "账号发现 " + obs.Status
		s.appendToolResult(ctx, userID, session.SessionID, userMessageID, data, obs)
		return false
	}
	obs.Summary = fmt.Sprintf("可见集成账号 %d 个（共 %d 个）", len(result.Accounts), result.Total)
	obs.Data = map[string]any{"accounts": result.Accounts, "total": result.Total}
	s.appendToolResult(ctx, userID, session.SessionID, userMessageID, data, obs)
	return true
}

// gatherResourceContext 读取会话资源上下文并注入账号/区域/命名空间/维度推导结果。
// 仅在没有告警上下文时执行；读取失败按 denied/error 观测降级，不阻断对话。
func (s *ChatService) gatherResourceContext(ctx context.Context, userID string, session *domain.Session, userMessageID string, data *chatContextData) bool {
	resourceID := strings.TrimSpace(session.Context.ResourceID)
	if resourceID == "" || s.assets == nil {
		return false
	}
	obs := s.checkTool(ctx, userID, chatToolResourceAnalyze)
	if !obs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, userMessageID, data, obs)
		return false
	}
	return s.loadResourceObservation(ctx, userID, session, userMessageID, data, resourceID, obs)
}

// loadResourceObservation 在工具权限已通过后读取资源并写入观测。
func (s *ChatService) loadResourceObservation(ctx context.Context, userID string, session *domain.Session, userMessageID string, data *chatContextData, resourceID string, obs ChatObservation) bool {
	resource, err := s.assets.GetForAnalysis(ctx, userID, resourceID)
	if err != nil {
		obs.Status = "error"
		obs.Reason = "resource_load_failed"
		switch apperr.CodeOf(err) {
		case apperr.CodePermissionDenied:
			obs.Status = "denied"
			obs.Reason = "data_scope_denied"
		case apperr.CodeNotFound:
			obs.Reason = "resource_not_found"
		case apperr.CodeFailedPrecondition:
			obs.Reason = "resource_not_cloud_linked"
		}
		logger.From(ctx).Warn("ai chat resource context load failed",
			logger.String("resource_id", resourceID),
			logger.String("status", obs.Status),
			logger.String("reason", obs.Reason),
			logger.Error(err))
		s.appendToolResult(ctx, userID, session.SessionID, userMessageID, data, obs)
		return false
	}
	if resource == nil {
		obs.Status = "error"
		obs.Reason = "empty resource context"
		s.appendToolResult(ctx, userID, session.SessionID, userMessageID, data, obs)
		return false
	}
	data.Resource = resource
	selector := deriveResourceCloudSelector(resource)
	obs.Status = EvidenceStatusOK
	obs.Summary = fmt.Sprintf("已读取资源 %s（%s）",
		firstNonEmptyValue(resource.Name, resource.CloudResourceID, resource.ResourceID),
		firstNonEmptyValue(resource.CloudResourceType, resource.ResourceType))
	obs.Data = map[string]any{
		"resource_id": resource.ResourceID, "name": resource.Name,
		"application_id":      resource.ApplicationID,
		"resource_type":       resource.ResourceType,
		"cloud_resource_type": resource.CloudResourceType,
		"cloud_resource_id":   resource.CloudResourceID,
		"account_id":          resource.AccountID, "region": resource.Region,
		"namespace": selector.Namespace,
	}
	if selector.DimensionName != "" && selector.DimensionValue != "" {
		obs.Data["dimensions"] = map[string]string{selector.DimensionName: selector.DimensionValue}
	}
	s.appendToolResult(ctx, userID, session.SessionID, userMessageID, data, obs)
	return true
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

// collectMetricObservation 按告警推导参数采集指标证据（首轮常规取数）。
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
	metricObs, evidence := s.executeMetricObservation(ctx, userID, query)
	if metricObs == nil {
		obs.Status = "skipped"
		obs.Reason = "metric evidence collector is not configured"
		return &obs, nil
	}
	metricObs.Mode = obs.Mode
	return metricObs, evidence
}

// executeMetricObservation 在工具权限已通过后调用证据端口并组装观测。
func (s *ChatService) executeMetricObservation(ctx context.Context, userID string, query MetricEvidenceQuery) (*ChatObservation, *MetricEvidenceResult) {
	if s.evidence == nil {
		return nil, nil
	}
	result, err := s.evidence.Collect(ctx, userID, query)
	if err != nil {
		logger.From(ctx).Warn("ai chat metric evidence failed", logger.Error(err))
		result = &MetricEvidenceResult{Status: EvidenceStatusError, Reason: "evidence_collect_failed"}
	}
	result = finalizeMetricEvidence(ctx, s.evidenceSnapshots, query, result, evidenceSnapshotOptions{
		MaxPointsPerSeries: chatMetricPromptMaxPoints,
		MaxBytes:           s.evidencePayloadMaxBytes,
	})
	pointCount := result.OriginalCount
	status := result.Status
	if status == "" {
		status = EvidenceStatusOK
	}
	ids := evidenceIDs(result)
	display := chatMetricDisplay(query.Metric)
	summary := "指标证据 " + status + "（" + display + "）"
	if status == EvidenceStatusOK {
		if pointCount == 0 {
			summary = "指标证据 ok（" + display + "，无数据点）"
		} else {
			summary = fmt.Sprintf("指标证据 ok（%s，%d 点）", display, pointCount)
		}
	}
	obs := &ChatObservation{
		ToolCode: chatToolMetricsQuery, Allowed: true,
		Status: status, Reason: result.Reason,
		Summary: summary,
		Data: map[string]any{
			"status": status, "reason": result.Reason,
			"metric": query.Metric, "metric_label": chatMetricLabel(query.Metric),
			"metric_count": len(result.Metrics), "point_count": pointCount,
			"retained_point_count": result.RetainedCount, "truncated": result.Truncated,
			"evidence_ids": ids, "schema_version": result.SchemaVersion,
			"provider": result.Provider, "trace_id": result.TraceID,
			"project_id": result.ProjectID, "resource_id": result.ResourceID,
			"cloud_resource_id": result.CloudResourceID, "collected_at": result.CollectedAt,
			"window_start": result.WindowStart, "window_end": result.WindowEnd,
			"aggregator": result.Aggregator, "sampling": result.Sampling,
			"from": query.From, "to": query.To, "period": result.Period,
			"dimensions": query.Dimensions,
		},
	}
	if len(result.Metrics) > 0 {
		// 持久化快照与 Prompt 共用同一份定稿序列，禁止再次独立降采样。
		obs.Data["metrics"] = result.Metrics
	}
	if len(ids) > 0 {
		obs.EvidenceID = ids[0]
		obs.Data["evidence_id"] = ids[0]
	}
	return obs, result
}

// appendMetricEvidenceReferences 追加指标证据引用（按 evidence_id 去重）。
func appendMetricEvidenceReferences(data *chatContextData, evidence *MetricEvidenceResult) {
	if data == nil {
		return
	}
	if evidence == nil || evidence.Status != EvidenceStatusOK {
		return
	}
	for _, id := range evidenceIDs(evidence) {
		appendObservabilityEvidenceReference(data, id, "指标证据 "+id)
	}
}

// appendRuleEvidenceReference 追加告警规则证据引用。
func appendRuleEvidenceReference(data *chatContextData, evidenceID string) {
	appendObservabilityEvidenceReference(data, evidenceID, "告警规则证据")
}

func appendObservabilityEvidenceReference(data *chatContextData, evidenceID, label string) {
	if data == nil {
		return
	}
	evidenceID = strings.TrimSpace(evidenceID)
	if evidenceID == "" {
		return
	}
	for _, ref := range data.References {
		if ref.Type == "observability_evidence" && ref.ID == evidenceID {
			return
		}
	}
	data.References = append(data.References, domain.Reference{
		Type: "observability_evidence", ID: evidenceID, Label: label, URL: "/observability",
	})
}

// checkTool 校验工具 RBAC 与工具权限。
//
// 语义：
//   - 指标取数：数据范围由 MetricEvidencePort 按账号二次校验；
//   - 规则体检：数据范围由 RuleHealthPort 在账号解析后二次校验；
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

func firstNonEmptyValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
		"session_id":  sessionID,
		"message_id":  messageID,
		"tool_code":   obs.ToolCode,
		"allowed":     obs.Allowed,
		"mode":        obs.Mode,
		"status":      obs.Status,
		"reason":      obs.Reason,
		"evidence_id": obs.EvidenceID,
		"result":      toolAuditResult(obs),
		"query":       metricAuditQuery(obs),
	})
}

// metricAuditQuery 提取指标查询参数供审计排查（不含凭据）；非指标工具返回 nil。
func metricAuditQuery(obs ChatObservation) map[string]any {
	if obs.ToolCode != chatToolMetricsQuery || obs.Data == nil {
		return nil
	}
	return map[string]any{
		"metric": obs.Data["metric"], "dimensions": obs.Data["dimensions"],
		"from": obs.Data["from"], "to": obs.Data["to"], "period": obs.Data["period"],
		"point_count": obs.Data["point_count"],
	}
}
