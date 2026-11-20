// Package application 实现 AI 告警分析用例（Alert §9.2 POST /api/ai/analyze-alert）。
package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
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
	alerts                  AlertReader
	tools                   ToolInvoker
	providerID              string // 静态兜底 provider；空且无 resolver 时仅返回本地摘要
	resolver                AnalysisProviderResolver
	evidence                MetricEvidencePort
	evidenceSnapshots       EvidenceSnapshotStore
	evidencePayloadMaxBytes int
	audit                   AuditRecorder
	// memory 为可选的相似案例记忆能力；nil 表示关闭。
	memory MemoryCaseStore
}

// NewAnalyzeService 构造分析服务；providerID 为空时跳过工具网关调用。
func NewAnalyzeService(alerts AlertReader, tools ToolInvoker, providerID string, audit AuditRecorder) *AnalyzeService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &AnalyzeService{alerts: alerts, tools: tools, providerID: strings.TrimSpace(providerID), audit: audit}
}

// SetProviderResolver 注入动态 provider 解析器；设置后每次分析实时解析，
// 前端启停或更换模型无需重启。
func (s *AnalyzeService) SetProviderResolver(resolver AnalysisProviderResolver) {
	if s == nil {
		return
	}
	s.resolver = resolver
}

func (s *AnalyzeService) analysisProviderID() string {
	if s != nil && s.resolver != nil {
		if id, ok := s.resolver.AnalysisProviderID(); ok && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id)
		}
	}
	if s == nil {
		return ""
	}
	return s.providerID
}

// SetMetricEvidence 注入只读指标证据端口；nil 表示关闭指标证据（灰度开关）。
func (s *AnalyzeService) SetMetricEvidence(port MetricEvidencePort) {
	if s == nil {
		return
	}
	s.evidence = port
}

func (s *AnalyzeService) SetEvidenceSnapshots(store EvidenceSnapshotStore, maxBytes int) {
	if s == nil {
		return
	}
	s.evidenceSnapshots = store
	s.evidencePayloadMaxBytes = maxBytes
}

// SetMemory 注入相似案例记忆能力；nil 表示关闭（payload 不含 similar_cases）。
func (s *AnalyzeService) SetMemory(memory MemoryCaseStore) {
	if s == nil {
		return
	}
	s.memory = memory
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
	// 告警读取端口按当前用户数据范围过滤；拒绝时不返回任何分析结果。
	alert, err := s.alerts.GetForAnalysis(ctx, userID, alertID)
	if err != nil {
		s.recordScopeDeniedAudit(ctx, alertID, userID, err)
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
	memoryStatus := MemoryStatusSkipped
	var similarCases []chatMemoryCase
	providerID := s.analysisProviderID()
	if s.tools != nil && providerID != "" {
		snapshot := alertSnapshot(alert)
		obj := AlertObjectContextOf(alert)
		if in.IncludeMetrics {
			evidence = s.collectMetricEvidence(ctx, userID, alert)
		}
		// 相似案例记忆（阶段 1）：命中则与 evidence 平级注入，失败降级不中断分析。
		similarCases, memoryStatus = s.searchSimilarCases(ctx, userID, alert, evidence)
		payload := map[string]any{
			"alert_id":        alertID,
			"time_range":      timeRange,
			"include_logs":    in.IncludeLogs,
			"include_metrics": in.IncludeMetrics,
			"include_changes": in.IncludeChanges,
			// alert 为完整告警快照（含 description/annotations/状态/时间/来源）；
			// 其余平铺字段保留给既有 provider 兼容。
			"alert":            snapshot,
			"alert_name":       alert.Name,
			"alert_summary":    alert.Summary,
			"alert_severity":   alert.Severity,
			"application_name": alert.ApplicationName,
			"resource_name":    alert.ResourceName,
			"labels":           alert.Labels,
		}
		if evidence != nil {
			payload["evidence"] = evidence
		}
		if len(similarCases) > 0 {
			payload["similar_cases"] = similarCases
		}
		resp, invokeErr := s.tools.Invoke(ctx, providerID, toolgateway.ToolRequest{
			UserID:       userID,
			ToolCode:     "alarm.analyze",
			Resource:     "alerts",
			Action:       "analyze",
			Payload:      payload,
			SystemPrompt: analysisSystemPrompt,
			UserPrompt:   buildAnalysisUserPrompt(snapshot, evidence),
			// 与 HTTP 入口 app:ai.analysis:analyze 保持一致，避免网关按 resource/action 拼出
			// 未种子的 app:alerts:analyze 导致静默拒绝。
			RequiredPermission: "app:ai.analysis:analyze",
			// 工具调用继承告警对象上下文，网关按同一套映射复核数据范围（纵深防御）。
			OwnerID: obj.Owner, Dept: obj.Dept, Team: obj.Team, Region: obj.Region, Tags: obj.Tags,
		})
		if invokeErr != nil {
			toolFailed = true
			toolError = invokeErr.Error()
		} else if resp != nil && !resp.Allowed {
			toolFailed = true
			reason := strings.TrimSpace(resp.Reason)
			if reason == "" {
				reason = "tool authorization denied"
			}
			toolError = "authorization_denied: " + reason
			logger.From(ctx).Warn("ai analyze tool call denied", logger.String("reason", reason))
		} else if resp != nil && resp.Data != nil {
			if degraded, _ := resp.Data["degraded"].(bool); degraded {
				toolFailed = true
				toolError = "provider_degraded"
			}
			mergeToolResponse(result, resp.Data)
		}
		result.References = filterAnalyzeEvidenceReferences(result.References, evidence)
		if len(result.References) == 0 {
			if refs := evidenceReferences(evidence); len(refs) > 0 {
				result.References = refs
			}
		}
	}
	s.recordAnalyzeAudit(ctx, alertID, userID, convID, toolFailed, toolError, result.RiskLevel, evidence, memoryStatus, len(similarCases))
	return result, nil
}

// searchSimilarCases 检索相似案例并转成 payload 结构；未装配或失败时返回 skipped/error。
func (s *AnalyzeService) searchSimilarCases(ctx context.Context, userID string, alert *AlertContext, evidence *MetricEvidenceResult) ([]chatMemoryCase, string) {
	if s.memory == nil {
		return nil, MemoryStatusSkipped
	}
	result := s.memory.SearchSimilar(ctx, MemorySearchInput{
		UserID:   userID,
		Alert:    alert,
		Evidence: evidence,
		Context:  domainContextOf(alert),
	})
	return memoryPromptCases(result.Cases), result.Status
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
		result = &MetricEvidenceResult{Status: EvidenceStatusError, Reason: "evidence collector failed"}
	}
	return finalizeMetricEvidence(ctx, s.evidenceSnapshots, query, result, evidenceSnapshotOptions{
		MaxBytes: s.evidencePayloadMaxBytes,
	})
}

func (s *AnalyzeService) recordAnalyzeAudit(ctx context.Context, alertID, userID, convID string, toolFailed bool, toolError, riskLevel string, evidence *MetricEvidenceResult, memoryStatus string, memoryUsed int) {
	if s == nil || s.audit == nil {
		return
	}
	result := "success"
	payload := map[string]any{
		"alert_id": alertID, "conversation_id": convID,
		"risk_level": riskLevel, "result": result,
		"memory_status": memoryStatus, "memory_used": memoryUsed,
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

// recordScopeDeniedAudit 记录按数据范围被拒绝的分析请求，便于审计追溯越权尝试。
// 仅在权限不足时写审计；其它错误（不存在、内部错误）由调用方按原语义返回。
func (s *AnalyzeService) recordScopeDeniedAudit(ctx context.Context, alertID, userID string, err error) {
	if s == nil || s.audit == nil || err == nil {
		return
	}
	appErr := apperr.FromError(err)
	if appErr == nil || appErr.Code != apperr.CodePermissionDenied {
		return
	}
	reason := strings.TrimSpace(appErr.Message)
	if reason == "" {
		reason = "data scope denied"
	}
	logger.From(ctx).Warn("ai alert analysis denied by data scope",
		logger.String("alert_id", alertID), logger.String("user_id", userID))
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "ai",
		ResourceID:   alertID,
		Action:       AuditAnalyzeAlert,
		UserID:       userID,
		Payload: map[string]any{
			"alert_id": alertID, "result": "denied", "scope_denied": true, "reason": reason,
		},
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
