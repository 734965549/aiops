package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
)

// 受限 Agent Loop（阶段 3）的预算与白名单。
const (
	// chatMaxLoopRounds 模型可追加取数的最大轮数（不含首轮）。
	chatMaxLoopRounds = 3
	// chatLoopTokenBudget 单次对话累计 token 上限，超限即终止循环。
	chatLoopTokenBudget = 20000
	// chatLoopMaxRequestsPerRound 单轮最多受理的工具申请数。
	chatLoopMaxRequestsPerRound = 3
	// chatDegradedLoopExhausted 轮数/预算耗尽仍未产出 answer 时的降级原因。
	chatDegradedLoopExhausted = "agent_loop_exhausted"
)

// loopAllowedTools 是模型只能在回合内申请的只读工具白名单（toolgateway 内置工具清单）。
//
// 执行类工具（execution.tasks.propose / dispatch / command.run）永不出现在白名单里：
// 它们只能作为服务端生成的 actions 存在，不能由模型在循环中直接触发。
var loopAllowedTools = toolgateway.BuiltinChatToolSet()

// chatToolRequest 是模型申请的追加只读取数。
type chatToolRequest struct {
	ToolCode string         `json:"tool_code"`
	Reason   string         `json:"reason,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
}

// chatToolRequestPayload 兼容「模型只申请工具、不产出 answer」的响应形态。
type chatToolRequestPayload struct {
	ToolRequests []chatToolRequest `json:"tool_requests"`
	Summary      string            `json:"summary"`
}

// parseChatToolRequests 解析模型申请的只读工具；无申请时返回空切片。
func parseChatToolRequests(content string) []chatToolRequest {
	var payload chatToolRequestPayload
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &payload); err != nil {
		return nil
	}
	out := make([]chatToolRequest, 0, len(payload.ToolRequests))
	for _, req := range payload.ToolRequests {
		code := strings.TrimSpace(req.ToolCode)
		if code == "" {
			continue
		}
		out = append(out, chatToolRequest{ToolCode: code, Reason: strings.TrimSpace(req.Reason), Params: req.Params})
	}
	return out
}

// generateWithLoop 在预算内允许模型追加只读取数：
//
//	第 0 轮：常规取数后请求模型
//	模型申请只读工具 -> 逐个过 RBAC + 工具权限并执行 -> observations 追加 -> 下一轮
//	轮数/token 预算耗尽且仍无 answer -> degraded(agent_loop_exhausted)
//
// 任何执行类工具都不会被执行；模型申请它们只会得到一条 denied 观测。
func (s *ChatService) generateWithLoop(ctx context.Context, userID, question string, session *domain.Session, history []ChatHistoryMessage, data *chatContextData) chatGeneration {
	if data == nil {
		return chatGeneration{}
	}
	evidence := observationEvidenceLines(*data)
	if s.chat == nil {
		return chatGeneration{Answer: degradedChatAnswer(chatDegradedModelUnavailable, evidence), Status: chatStatusDegraded}
	}
	providerID, ok := s.chat.ChatProviderID()
	if !ok {
		return chatGeneration{Answer: degradedChatAnswer(chatDegradedModelUnavailable, evidence), Status: chatStatusDegraded}
	}
	systemPrompt := chatSystemPrompt
	if appendix := strings.TrimSpace(data.KnowledgeAppendix); appendix != "" {
		systemPrompt = chatSystemPrompt + "\n\n" + appendix
	}

	budget := s.turnBudget
	if budget <= 0 {
		budget = chatDefaultTurnBudget
	}
	deadline := time.Now().Add(budget)
	tokens := 0
	budgetExceeded := func() chatGeneration {
		logger.From(ctx).Warn("ai chat turn budget exceeded",
			logger.String("provider_id", providerID),
			logger.Int("tokens", tokens),
			logger.Int("loop_rounds", data.LoopRounds))
		return chatGeneration{
			Answer: degradedChatAnswer(chatDegradedTurnBudgetExceeded, observationEvidenceLines(*data)),
			Status: chatStatusDegraded, ProviderID: providerID, Tokens: tokens,
		}
	}

	rounds := 0
	// deniedRounds 统计连续「申请了但一个都没执行」的轮数，用于给模型一次纠正机会。
	deniedRounds := 0
	for {
		if time.Until(deadline) <= 0 {
			return budgetExceeded()
		}
		prompt := buildChatUserPrompt(ChatPromptInput{
			Question: question, Context: session.Context, TimeRange: session.Context.TimeRange,
			Now:   s.now().Unix(),
			Alert: alertSnapshot(data.Alert), Observations: data.Observations, History: history,
			Memory: memoryPromptCases(data.Memories),
		})
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return budgetExceeded()
		}
		callCtx, cancel := context.WithTimeout(ctx, remaining)
		result, err := s.completeModel(callCtx, providerID, systemPrompt, prompt)
		cancel()
		if err != nil {
			logger.From(ctx).Warn("ai chat completion failed",
				logger.String("provider_id", providerID),
				logger.Int64("turn_remaining_ms", time.Until(deadline).Milliseconds()),
				logger.Error(err))
			if apperr.CodeOf(err) == apperr.CodeAborted {
				return chatGeneration{Aborted: true, ProviderID: providerID, Tokens: tokens}
			}
			if time.Until(deadline) <= 0 {
				return budgetExceeded()
			}
			return chatGeneration{
				Answer: degradedChatAnswer(chatDegradedModelFailed, observationEvidenceLines(*data)),
				Status: chatStatusDegraded, ProviderID: providerID, Tokens: tokens,
			}
		}
		tokens += result.Tokens
		if answer, parseErr := parseChatAnswer(result.Content); parseErr == nil {
			status := chatStatusOK
			if answer.InsufficientContext {
				status = chatStatusInsufficientContext
			}
			return chatGeneration{Answer: answer, Status: status, ProviderID: providerID, Tokens: tokens}
		}
		requests := parseChatToolRequests(result.Content)
		if len(requests) == 0 {
			logger.From(ctx).Warn("ai chat answer invalid",
				logger.String("provider_id", providerID))
			return chatGeneration{
				Answer: degradedChatAnswer(chatDegradedOutputInvalid, observationEvidenceLines(*data)),
				Status: chatStatusDegraded, ProviderID: providerID, Tokens: tokens,
			}
		}
		// 预算检查：轮数或 token 超限即终止，用已有 observations 组装降级回答。
		if rounds >= chatMaxLoopRounds || tokens >= chatLoopTokenBudget {
			logger.From(ctx).Warn("ai chat agent loop exhausted",
				logger.String("session_id", session.SessionID),
				logger.Int("rounds", rounds), logger.Int("tokens", tokens))
			evidence = observationEvidenceLines(*data)
			return chatGeneration{
				Answer: degradedChatAnswer(chatDegradedLoopExhausted, evidence),
				Status: chatStatusDegraded, ProviderID: providerID, Tokens: tokens,
			}
		}
		if time.Until(deadline) <= 0 {
			return budgetExceeded()
		}
		executed := s.runLoopToolRequests(ctx, userID, session, data, requests)
		data.LoopRounds = rounds + 1
		rounds++
		if executed == 0 {
			// 本轮没有任何工具真正执行：把拒绝原因反馈给模型后允许一次纠正机会，
			// 连续两轮都无实际取数才收敛，避免模型陷入无效申请的死循环。
			deniedRounds++
			if deniedRounds > 1 {
				logger.From(ctx).Warn("ai chat agent loop made no progress",
					logger.String("session_id", session.SessionID),
					logger.Int("rounds", rounds), logger.Int("denied_rounds", deniedRounds))
				evidence = observationEvidenceLines(*data)
				return chatGeneration{
					Answer: degradedChatAnswer(chatDegradedLoopExhausted, evidence),
					Status: chatStatusDegraded, ProviderID: providerID, Tokens: tokens,
				}
			}
		} else {
			deniedRounds = 0
		}
	}
}

// completeModel 在流式请求（存在 SSE sink）时走模型增量；同步路径保持原 Complete。
func (s *ChatService) completeModel(ctx context.Context, providerID, systemPrompt, prompt string) (*toolgateway.ChatCompletionResult, error) {
	if _, ok := ctx.Value(ChatEventSinkKey{}).(ChatEventSink); !ok {
		return s.chat.Complete(ctx, providerID, systemPrompt, prompt)
	}
	return s.chat.CompleteStream(ctx, providerID, systemPrompt, prompt, func(phase, text string) error {
		return emitChatModelDelta(ctx, phase, text)
	})
}

// runLoopToolRequests 逐个执行模型申请的只读工具，返回真正执行的数量。
//
// 每轮每个工具独立过 RBAC + 工具权限；拒绝原因反馈给模型（denied 观测），不中断循环。
func (s *ChatService) runLoopToolRequests(ctx context.Context, userID string, session *domain.Session, data *chatContextData, requests []chatToolRequest) int {
	executed := 0
	seen := make(map[string]struct{}, len(requests))
	for _, req := range requests {
		if len(seen) >= chatLoopMaxRequestsPerRound {
			break
		}
		code := strings.TrimSpace(req.ToolCode)
		if code == "" {
			continue
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		if _, ok := loopAllowedTools[code]; !ok {
			// 执行类或未登记工具：明确拒绝并把原因反馈给模型。
			s.appendToolResult(ctx, userID, session.SessionID, "", data, ChatObservation{
				ToolCode: code, Status: "denied", Reason: "tool is not allowed in agent loop",
			})
			continue
		}
		switch code {
		case chatToolAlarmAnalyze:
			if s.refreshAlertObservation(ctx, userID, session, data) {
				executed++
			}
		case chatToolMetricsQuery:
			if s.refreshMetricObservation(ctx, userID, session, data, req.Params) {
				executed++
			}
		case chatToolAlertRulesList:
			if s.refreshAlertRulesObservation(ctx, userID, session, data, req.Params) {
				executed++
			}
		case chatToolAccountsList:
			if s.refreshAccountsObservation(ctx, userID, session, data) {
				executed++
			}
		case chatToolResourceAnalyze:
			if s.refreshResourceObservation(ctx, userID, session, data) {
				executed++
			}
		case chatToolResourcesList:
			if s.refreshResourcesObservation(ctx, userID, session, data, req.Params) {
				executed++
			}
		}
	}
	return executed
}

// refreshAlertObservation 重新读取告警快照（数据范围在读取端口统一校验）。
func (s *ChatService) refreshAlertObservation(ctx context.Context, userID string, session *domain.Session, data *chatContextData) bool {
	obs := s.checkTool(ctx, userID, chatToolAlarmAnalyze)
	if !obs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	alertID := strings.TrimSpace(session.Context.AlertID)
	if alertID == "" || s.alerts == nil {
		obs.Status = "denied"
		obs.Reason = "no alert context"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	alert, err := s.alerts.GetForAnalysis(ctx, userID, alertID)
	if err != nil {
		obs.Status = "error"
		obs.Reason = "alert_load_failed"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	data.Alert = alert
	obs.Status = "ok"
	obs.Summary = "已重新读取告警快照"
	s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
	return true
}

// refreshMetricObservation 重新采集指标证据（内部含工具权限校验）。
// refreshMetricObservation 按模型参数（缺省回退告警推导）重新采集指标证据。
func (s *ChatService) refreshMetricObservation(ctx context.Context, userID string, session *domain.Session, data *chatContextData, params map[string]any) bool {
	obs := s.checkTool(ctx, userID, chatToolMetricsQuery)
	if !obs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	query, status, reason := buildMetricEvidenceQuery(data.Alert, data.Resource, params, s.now().Unix())
	if status != "" {
		obs.Status = status
		obs.Reason = reason
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	metricObs, evidence := s.executeMetricObservation(ctx, userID, query)
	if metricObs == nil {
		obs.Status = "skipped"
		obs.Reason = "metric evidence collector is not configured"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	metricObs.Mode = obs.Mode
	s.appendToolResult(ctx, userID, session.SessionID, "", data, *metricObs)
	if evidence == nil || metricObs.Status != EvidenceStatusOK {
		return false
	}
	data.Evidence = evidence
	appendMetricEvidenceReferences(data, evidence)
	return true
}

// refreshAlertRulesObservation 查询告警规则（规则体检）。
func (s *ChatService) refreshAlertRulesObservation(ctx context.Context, userID string, session *domain.Session, data *chatContextData, params map[string]any) bool {
	obs := s.checkTool(ctx, userID, chatToolAlertRulesList)
	if !obs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	if s.alertRules == nil {
		obs.Status = "skipped"
		obs.Reason = "alert rule query is not configured"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	query, status, reason := buildAlertRuleQuery(data.Alert, data.Resource, params)
	if status != "" {
		obs.Status = status
		obs.Reason = reason
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	result, err := s.alertRules.List(ctx, userID, query)
	if err != nil {
		logger.From(ctx).Warn("ai chat alert rules query failed", logger.Error(err))
		obs.Status = EvidenceStatusError
		obs.Reason = "alert rules query failed"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	if result == nil {
		obs.Status = EvidenceStatusError
		obs.Reason = "empty alert rule result"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	obs.Status = result.Status
	if obs.Status == "" {
		obs.Status = EvidenceStatusOK
	}
	obs.Reason = result.Reason
	obs.EvidenceID = result.EvidenceID
	if obs.Status == EvidenceStatusOK {
		rules := capAlertRulesForObservation(result.Rules)
		obs.Summary = fmt.Sprintf("告警规则 %d 条（%s）", len(rules), query.Namespace)
		obs.Data = map[string]any{
			"namespace": query.Namespace, "rule_count": len(rules),
			"evidence_id": result.EvidenceID, "rules": rules,
		}
		appendRuleEvidenceReference(data, result.EvidenceID)
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return true
	}
	obs.Summary = "告警规则查询 " + obs.Status
	s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
	return false
}

// refreshAccountsObservation 在循环内重新执行账号发现；未装配时返回 skipped 观测。
func (s *ChatService) refreshAccountsObservation(ctx context.Context, userID string, session *domain.Session, data *chatContextData) bool {
	obs := s.checkTool(ctx, userID, chatToolAccountsList)
	if !obs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	if s.accounts == nil {
		obs.Status = "skipped"
		obs.Reason = "account discovery is not configured"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	return s.loadAccountObservation(ctx, userID, session, "", data, obs)
}

// refreshResourceObservation 在循环内重新读取资源上下文；缺上下文/未装配时返回 skipped 观测。
func (s *ChatService) refreshResourceObservation(ctx context.Context, userID string, session *domain.Session, data *chatContextData) bool {
	obs := s.checkTool(ctx, userID, chatToolResourceAnalyze)
	if !obs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	resourceID := strings.TrimSpace(session.Context.ResourceID)
	if resourceID == "" {
		obs.Status = "skipped"
		obs.Reason = "no_resource"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	if s.assets == nil {
		obs.Status = "skipped"
		obs.Reason = "resource reader is not configured"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	return s.loadResourceObservation(ctx, userID, session, "", data, resourceID, obs)
}

// refreshResourcesObservation 按账号发现云同步资源，供模型获取实例维度。
func (s *ChatService) refreshResourcesObservation(ctx context.Context, userID string, session *domain.Session, data *chatContextData, params map[string]any) bool {
	obs := s.checkTool(ctx, userID, chatToolResourcesList)
	if !obs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	if s.resources == nil {
		obs.Status = "skipped"
		obs.Reason = "resource discovery is not configured"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	query, status, reason := buildResourceListQuery(data.Resource, params)
	if status != "" {
		obs.Status = status
		obs.Reason = reason
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	result, err := s.resources.List(ctx, userID, query)
	if err != nil {
		logger.From(ctx).Warn("ai chat resource discovery failed", logger.Error(err))
		obs.Status = EvidenceStatusError
		obs.Reason = "resource discovery failed"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	if result == nil {
		obs.Status = EvidenceStatusError
		obs.Reason = "empty resource list result"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	obs.Status = result.Status
	if obs.Status == "" {
		obs.Status = EvidenceStatusOK
	}
	obs.Reason = result.Reason
	if obs.Status != EvidenceStatusOK {
		obs.Summary = "资源发现 " + obs.Status
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	resources := make([]ResourceSummary, 0, len(result.Resources))
	for _, item := range result.Resources {
		resources = append(resources, enrichResourceSummary(item))
	}
	obs.Summary = fmt.Sprintf("云同步资源 %d 个（账号 %s）", len(resources), query.AccountID)
	obs.Data = map[string]any{
		"account_id": query.AccountID, "resource_type": query.ResourceType,
		"region": query.Region, "resource_count": len(resources), "resources": resources,
	}
	s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
	return true
}

// enrichResourceSummary 用服务端推导结果补齐 namespace/主维度，并清空仅供推导的标签。
func enrichResourceSummary(item ResourceSummary) ResourceSummary {
	selector := deriveResourceCloudSelector(&AssetContext{
		CloudResourceType: item.CloudResourceType,
		CloudResourceID:   item.CloudResourceID,
		Labels:            item.Labels,
	})
	if item.Namespace == "" {
		item.Namespace = selector.Namespace
	}
	if item.DimensionName == "" {
		item.DimensionName = selector.DimensionName
	}
	if item.DimensionValue == "" {
		item.DimensionValue = selector.DimensionValue
	}
	item.Labels = nil
	return item
}
