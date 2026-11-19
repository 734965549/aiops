package application

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/734965549/aiops/internal/ai/domain"
)

// chatGeneration 是一次回答生成结果。
type chatGeneration struct {
	Answer     ChatAnswer
	Status     string
	ProviderID string
	Tokens     int
	// Aborted 表示流式回调失败（通常是客户端断开），调用方不得落 assistant 消息。
	Aborted bool
	// FallbackNote 非空表示所选模型不可用、已回退默认模型；最终写入 answer.uncertainties。
	FallbackNote string
}

// generateAnswer 解析本次使用的模型并调用受限 Agent Loop。
// 所选模型不可用/禁用时回退默认模型，并在回答不确定性中说明，不直接让请求失败。
func (s *ChatService) generateAnswer(ctx context.Context, userID, question string, session *domain.Session, history []ChatHistoryMessage, data *chatContextData, requestedProviderID string) chatGeneration {
	providerID, fallbackNote, ok := s.resolveChatProvider(requestedProviderID)
	if !ok {
		var evidence []string
		if data != nil {
			evidence = observationEvidenceLines(*data)
		}
		return chatGeneration{
			Answer: degradedChatAnswer(chatDegradedModelUnavailable, evidence),
			Status: chatStatusDegraded, FallbackNote: fallbackNote,
		}
	}
	generation := s.generateWithLoop(ctx, providerID, userID, question, session, history, data)
	generation.FallbackNote = fallbackNote
	return generation
}

// finalizeAnswer 由服务端补齐动作、证据与确认语义（模型输出的 actions 一律忽略）。
func (s *ChatService) finalizeAnswer(ctx context.Context, userID string, session *domain.Session, userMessageID string, data *chatContextData, gen *chatGeneration) {
	answer := &gen.Answer
	if answer.Evidence == nil {
		answer.Evidence = []string{}
	}
	if len(answer.Evidence) == 0 {
		answer.Evidence = observationEvidenceLines(*data)
	}
	if len(answer.Evidence) == 0 {
		answer.Evidence = []string{}
	}
	if answer.Impact == nil {
		answer.Impact = []string{}
	}
	if answer.Recommendations == nil {
		answer.Recommendations = []string{}
	}
	if answer.Uncertainties == nil {
		answer.Uncertainties = []string{}
	}
	// 模型回退说明放在最前，确保用户/审计能看到“所选模型不可用”的事实。
	if note := strings.TrimSpace(gen.FallbackNote); note != "" {
		answer.Uncertainties = append([]string{note}, answer.Uncertainties...)
	}

	actions := make([]ChatAction, 0, 3)
	if s.tools != nil {
		proposeObs := s.checkTool(ctx, userID, chatToolExecutionPropose)
		// 记录提议动作的权限校验，便于审计追溯；不进入本轮模型观测。
		data.ToolCalls = append(data.ToolCalls, domain.ToolCall{
			ToolCode: proposeObs.ToolCode, Allowed: proposeObs.Allowed, Mode: proposeObs.Mode,
			Reason: proposeObs.Reason, Status: proposeObs.Status,
			Summary: "生成执行计划的权限校验（真实执行仍须人工确认）",
		})
		s.recordAudit(ctx, userID, "ai", session.SessionID, AuditToolInvoke, map[string]any{
			"session_id": session.SessionID, "message_id": userMessageID,
			"tool_code": proposeObs.ToolCode, "allowed": proposeObs.Allowed,
			"mode": proposeObs.Mode, "status": proposeObs.Status, "reason": proposeObs.Reason,
			"result": toolAuditResult(proposeObs),
		})
		action := ChatAction{
			Code: "execution.propose", Label: "生成执行计划",
			Allowed: proposeObs.Allowed, Target: "/executions",
		}
		if !proposeObs.Allowed {
			action.Reason = proposeObs.Reason
		}
		actions = append(actions, action)
	}
	// 告警上下文被数据范围拒绝时不给跳转入口，避免把用户引到无权查看的详情页。
	if alertID := strings.TrimSpace(session.Context.AlertID); alertID != "" && !data.AlertDenied {
		actions = append(actions, ChatAction{
			Code: "view.alert", Label: "查看告警详情", Allowed: true,
			Target: "/alerts?alert_id=" + url.QueryEscape(alertID),
		})
	}
	if data.Evidence != nil && len(data.Evidence.Metrics) > 0 {
		actions = append(actions, ChatAction{
			Code: "view.metrics", Label: "查看指标", Allowed: true, Target: "/observability",
		})
	}
	answer.Actions = actions
	// 中高风险回答或模型显式要求时提示人工确认；任何真实执行都还要走 Execution 状态机。
	if chatRiskRank(answer.RiskLevel) >= 2 {
		answer.RequireConfirm = true
	}
}

func observationEvidenceLines(data chatContextData) []string {
	out := make([]string, 0, 4)
	if data.Alert != nil {
		out = append(out, fmt.Sprintf(
			"告警 %s（级别 %s，状态 %s，应用 %s，资源 %s）",
			data.Alert.Name, data.Alert.Severity, data.Alert.Status,
			data.Alert.ApplicationName, data.Alert.ResourceName,
		))
	}
	for _, id := range evidenceIDs(data.Evidence) {
		out = append(out, "observability evidence "+id)
	}
	return out
}
