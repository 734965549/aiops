// Package application 实现 AI 告警分析用例（Alert §9.2 POST /api/ai/analyze-alert）。
package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// AnalyzeAlertInput 分析请求（ops/alert-contract.md §9.2）。
type AnalyzeAlertInput struct {
	AlertID        string
	TimeRange      string
	IncludeLogs    bool
	IncludeMetrics bool
	IncludeChanges bool
}

// AnalyzeAlertResult 分析响应（技术架构设计 §6.3.1）。
type AnalyzeAlertResult struct {
	ConversationID  string   `json:"conversation_id"`
	Summary         string   `json:"summary"`
	RiskLevel       string   `json:"risk_level"`
	Recommendations []string `json:"recommendations"`
	References      []string `json:"references"`
}

// ToolInvoker AI 工具网关调用 port。
type ToolInvoker interface {
	Invoke(ctx context.Context, providerID string, req toolgateway.ToolRequest) (*toolgateway.ToolResponse, error)
}

// AnalyzeService 告警 AI 分析服务。
type AnalyzeService struct {
	alerts     AlertReader
	tools      ToolInvoker
	providerID string // 默认工具 provider；空则仅返回本地摘要
	evidence   MetricEvidencePort
	audit      AuditRecorder
}

// NewAnalyzeService 构造分析服务；providerID 为空时跳过工具网关调用。
func NewAnalyzeService(alerts AlertReader, tools ToolInvoker, providerID string, audit AuditRecorder) *AnalyzeService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &AnalyzeService{alerts: alerts, tools: tools, providerID: strings.TrimSpace(providerID), audit: audit}
}

// SetMetricEvidence 注入只读指标证据端口；nil 表示关闭指标证据（灰度开关）。
func (s *AnalyzeService) SetMetricEvidence(port MetricEvidencePort) {
	if s == nil {
		return
	}
	s.evidence = port
}

// AnalyzeAlert 加载告警上下文并尝试经 toolgateway 调用 alarm.analyze；失败时回落本地摘要。
func (s *AnalyzeService) AnalyzeAlert(ctx context.Context, userID string, in AnalyzeAlertInput) (*AnalyzeAlertResult, error) {
	if s == nil || s.alerts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai analyze service is not enabled")
	}
	alertID := strings.TrimSpace(in.AlertID)
	if alertID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "alert_id is required")
	}
	alert, err := s.alerts.GetForAnalysis(ctx, alertID)
	if err != nil {
		return nil, err
	}
	timeRange := strings.TrimSpace(in.TimeRange)
	if timeRange == "" {
		timeRange = "30m"
	}
	convID := uuid.NewString()
	result := &AnalyzeAlertResult{
		ConversationID:  convID,
		Summary:         buildLocalSummary(alert),
		RiskLevel:       severityToRisk(alert.Severity),
		Recommendations: []string{},
		References:      []string{},
	}
	toolFailed := false
	var toolError string
	var evidence *MetricEvidenceResult
	if s.tools != nil && s.providerID != "" {
		payload := map[string]any{
			"alert_id":        alertID,
			"time_range":      timeRange,
			"include_logs":    in.IncludeLogs,
			"include_metrics": in.IncludeMetrics,
			"include_changes": in.IncludeChanges,
			// alert 为完整告警快照（含 description/annotations/状态/时间/来源）；
			// 其余平铺字段保留给既有 provider 兼容。
			"alert":            alertSnapshot(alert),
			"alert_name":       alert.Name,
			"alert_summary":    alert.Summary,
			"alert_severity":   alert.Severity,
			"application_name": alert.ApplicationName,
			"resource_name":    alert.ResourceName,
			"labels":           alert.Labels,
		}
		if in.IncludeMetrics {
			evidence = s.collectMetricEvidence(ctx, userID, alert)
			payload["evidence"] = evidence
		}
		resp, invokeErr := s.tools.Invoke(ctx, s.providerID, toolgateway.ToolRequest{
			UserID:   userID,
			ToolCode: "alarm.analyze",
			Resource: "alerts",
			Action:   "analyze",
			Payload:  payload,
		})
		if invokeErr != nil {
			toolFailed = true
			toolError = invokeErr.Error()
		} else if resp != nil && resp.Data != nil {
			mergeToolResponse(result, resp.Data)
		}
		if len(result.References) == 0 {
			if refs := evidenceReferences(evidence); len(refs) > 0 {
				result.References = refs
			}
		}
	}
	s.recordAnalyzeAudit(ctx, alertID, userID, convID, toolFailed, toolError, result.RiskLevel, evidence)
	return result, nil
}

// collectMetricEvidence 在预算内收集指标证据；任何失败都降级为 evidence status，不中断分析。
func (s *AnalyzeService) collectMetricEvidence(ctx context.Context, userID string, alert *AlertContext) *MetricEvidenceResult {
	if s == nil || s.evidence == nil {
		return &MetricEvidenceResult{Status: EvidenceStatusSkipped, Reason: "collector_disabled"}
	}
	query, skip := deriveMetricEvidenceQuery(alert)
	if skip != "" {
		return &MetricEvidenceResult{Status: EvidenceStatusSkipped, Reason: skip}
	}
	result, err := s.evidence.Collect(ctx, userID, query)
	if err != nil {
		return &MetricEvidenceResult{Status: EvidenceStatusError, Reason: "evidence collector failed"}
	}
	if result == nil {
		return &MetricEvidenceResult{Status: EvidenceStatusError, Reason: "empty evidence result"}
	}
	return capEvidencePayload(result)
}

func (s *AnalyzeService) recordAnalyzeAudit(ctx context.Context, alertID, userID, convID string, toolFailed bool, toolError, riskLevel string, evidence *MetricEvidenceResult) {
	if s == nil || s.audit == nil {
		return
	}
	result := "success"
	payload := map[string]any{
		"alert_id": alertID, "conversation_id": convID,
		"risk_level": riskLevel, "result": result,
	}
	if toolFailed {
		payload["result"] = "partial"
		payload["tool_error"] = toolError
	}
	if evidence != nil {
		payload["evidence_status"] = evidence.Status
		if ids := evidenceIDs(evidence); len(ids) > 0 {
			payload["evidence_ids"] = ids
		}
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "ai",
		ResourceID:   convID,
		Action:       AuditAnalyzeAlert,
		UserID:       userID,
		Payload:      payload,
	})
}

// alertSnapshot 将告警上下文整体序列化为 provider 可读快照，避免逐字段挑选再次漏掉新字段。
func alertSnapshot(alert *AlertContext) map[string]any {
	if alert == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(alert)
	if err != nil {
		return map[string]any{"id": alert.ID, "name": alert.Name}
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"id": alert.ID, "name": alert.Name}
	}
	return out
}

// buildLocalSummary 无 provider 时也给出资源/指标/阈值级别摘要，优先使用更完整的 description。
func buildLocalSummary(alert *AlertContext) string {
	if alert == nil {
		return ""
	}
	for _, detail := range []string{alert.Description, alert.Summary} {
		if detail = strings.TrimSpace(detail); detail != "" {
			return fmt.Sprintf("%s: %s", alert.Name, detail)
		}
	}
	return fmt.Sprintf("告警 %s（级别 %s）待进一步分析", alert.Name, alert.Severity)
}

func severityToRisk(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "p0", "p1":
		return "high"
	case "p2":
		return "medium"
	case "p3":
		return "low"
	default:
		return "info"
	}
}

func mergeToolResponse(out *AnalyzeAlertResult, data map[string]any) {
	if v, ok := data["summary"].(string); ok && strings.TrimSpace(v) != "" {
		out.Summary = v
	}
	if v, ok := data["risk_level"].(string); ok && strings.TrimSpace(v) != "" {
		out.RiskLevel = v
	}
	if v, ok := data["conversation_id"].(string); ok && strings.TrimSpace(v) != "" {
		out.ConversationID = v
	}
	out.Recommendations = appendStrings(data["recommendations"], out.Recommendations)
	out.References = appendStrings(data["references"], out.References)
}

func appendStrings(raw any, fallback []string) []string {
	switch v := raw.(type) {
	case []string:
		if len(v) > 0 {
			return v
		}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if fallback == nil {
		return []string{}
	}
	return fallback
}
