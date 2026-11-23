// Package observability 将 Observability 查询能力适配为 AI 规则查询端口。
package observability

import (
	"context"
	"errors"
	"strings"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

const defaultRuleQueryTimeout = 5 * time.Second

// AlertRulesQuerier 是规则查询端口依赖的只读能力（由 obsapp.QueryService 实现）。
type AlertRulesQuerier interface {
	ListAlertRules(ctx context.Context, actor obsapp.Actor, q obsdomain.AlertRuleQuery) (*obsapp.AlertRuleQueryResult, error)
}

// AlertRulesCollector 实现 aiapp.AlertRuleListPort。
type AlertRulesCollector struct {
	queries       AlertRulesQuerier
	accounts      AccountResolver
	accountAccess aiapp.AccountAccessAuthorizer
	timeout       time.Duration
}

func NewAlertRulesCollector(
	queries AlertRulesQuerier,
	accounts AccountResolver,
	accountAccess aiapp.AccountAccessAuthorizer,
	timeout time.Duration,
) *AlertRulesCollector {
	if timeout <= 0 {
		timeout = defaultRuleQueryTimeout
	}
	return &AlertRulesCollector{
		queries: queries, accounts: accounts, accountAccess: accountAccess, timeout: timeout,
	}
}

var _ aiapp.AlertRuleListPort = (*AlertRulesCollector)(nil)

// List 执行账号/数据范围预检后调用 Observation QueryService。
// 业务失败以 Result.Status 降级返回，保证对话编排不中断。
func (a *AlertRulesCollector) List(ctx context.Context, userID string, q aiapp.AlertRuleQuery) (*aiapp.AlertRuleListResult, error) {
	if a == nil || a.queries == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai alert rule collector is not configured")
	}
	q.AccountID = strings.TrimSpace(q.AccountID)
	q.Region = strings.TrimSpace(q.Region)
	q.Namespace = strings.TrimSpace(q.Namespace)
	q.Keyword = strings.TrimSpace(q.Keyword)
	switch {
	case q.AccountID == "":
		return &aiapp.AlertRuleListResult{Status: aiapp.EvidenceStatusSkipped, Reason: "no_account"}, nil
	case q.Namespace == "":
		return &aiapp.AlertRuleListResult{Status: aiapp.EvidenceStatusSkipped, Reason: "no_namespace"}, nil
	}
	if a.accounts == nil {
		return &aiapp.AlertRuleListResult{Status: aiapp.EvidenceStatusError, Reason: "account_resolver_unavailable"}, nil
	}
	snapshot, err := a.accounts.ResolveAccount(ctx, q.AccountID)
	if err != nil {
		return &aiapp.AlertRuleListResult{
			Status: aiapp.EvidenceStatusError,
			Reason: evidenceErrorReason(err, "resolve integration account failed"),
		}, nil
	}
	if snapshot == nil {
		return &aiapp.AlertRuleListResult{Status: aiapp.EvidenceStatusError, Reason: "integration account not found"}, nil
	}
	if a.accountAccess != nil && strings.TrimSpace(userID) != "" {
		ref := aiapp.AccountRef{
			AccountID: snapshot.AccountID,
			OwnerTeam: snapshot.OwnerTeam,
			Regions:   append([]string(nil), snapshot.Regions...),
		}
		if err := a.accountAccess.Authorize(ctx, userID, ref); err != nil {
			return &aiapp.AlertRuleListResult{
				Status: aiapp.EvidenceStatusPermissionDenied,
				Reason: "no access to integration account for evidence",
			}, nil
		}
	}

	queryCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	result, err := a.queries.ListAlertRules(queryCtx, obsapp.Actor{UserID: userID}, obsdomain.AlertRuleQuery{
		AccountID: q.AccountID,
		Region:    q.Region,
		Namespace: q.Namespace,
		Keyword:   q.Keyword,
		Limit:     q.Limit,
	})
	if err != nil {
		return &aiapp.AlertRuleListResult{Status: ruleErrorStatus(err), Reason: ruleErrorReason(err)}, nil
	}
	if result == nil {
		return &aiapp.AlertRuleListResult{Status: aiapp.EvidenceStatusError, Reason: "empty alert rule result"}, nil
	}
	out := &aiapp.AlertRuleListResult{
		Status:     aiapp.EvidenceStatusOK,
		EvidenceID: result.EvidenceID,
		Rules:      make([]aiapp.AlertRuleSummary, 0, len(result.Rules)),
	}
	for _, rule := range result.Rules {
		out.Rules = append(out.Rules, aiapp.AlertRuleSummary{
			RuleID:        rule.RuleID,
			Name:          rule.Name,
			Namespace:     rule.Namespace,
			Severity:      rule.Severity,
			Enabled:       rule.Enabled,
			Description:   rule.Description,
			Metric:        rule.Metric,
			Operator:      rule.Operator,
			Threshold:     rule.Threshold,
			Unit:          rule.Unit,
			WindowSeconds: rule.WindowSeconds,
			Labels:        rule.Labels,
			ResourceCount: rule.ResourceCount,
			Resources:     rule.Resources,
		})
	}
	return out, nil
}

func ruleErrorStatus(err error) string {
	if errors.Is(err, obsdomain.ErrCapabilityUnsupported) || errors.Is(err, obsdomain.ErrUnsupportedProvider) {
		return aiapp.EvidenceStatusUnsupported
	}
	ae := apperr.FromError(err)
	if ae != nil && ae.Code == apperr.CodeFailedPrecondition && strings.Contains(strings.ToLower(ae.Message), "does not support alert rules") {
		return aiapp.EvidenceStatusUnsupported
	}
	return aiapp.EvidenceStatusError
}

func ruleErrorReason(err error) string {
	if err != nil {
		if errors.Is(err, obsdomain.ErrCapabilityUnsupported) || errors.Is(err, obsdomain.ErrUnsupportedProvider) {
			return "alert rules are not supported by the provider"
		}
		ae := apperr.FromError(err)
		if ae != nil && strings.Contains(strings.ToLower(ae.Message), "does not support alert rules") {
			return "alert rules are not supported by the provider"
		}
	}
	return evidenceErrorReason(err, "alert rules query failed")
}
