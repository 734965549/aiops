package application

import "context"

// AlertRuleQuery 是一次只读告警规则查询请求（provider 无关）。
type AlertRuleQuery struct {
	AccountID string
	Region    string
	Namespace string
	Keyword   string
	Limit     int
}

// AlertRuleSummary 是注入对话观测的规则摘要；阈值字段由 provider 可选提供。
type AlertRuleSummary struct {
	RuleID      string `json:"rule_id"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
	Metric      string `json:"metric,omitempty"`
	// MetricLabel 是指标中文名；空表示未映射，模型必须保留原始指标代码并注明未映射。
	MetricLabel string   `json:"metric_label,omitempty"`
	Operator    string   `json:"operator,omitempty"`
	Threshold   *float64 `json:"threshold,omitempty"`
	// Unit 是规则阈值的量纲（如 % / ms / MiB）；空表示 provider 未提供，不得臆造。
	Unit          string            `json:"unit,omitempty"`
	WindowSeconds int               `json:"window_seconds,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	// ResourceCount 是规则关联资源总数；Resources 是其维度样本（服务端已截断）。
	ResourceCount int                 `json:"resource_count,omitempty"`
	Resources     []map[string]string `json:"resources,omitempty"`
	// ResourceGroupCount 大于 0 表示规则按资源分组关联，资源范围由分组展开得到。
	ResourceGroupCount int `json:"resource_group_count,omitempty"`
}

// AlertRuleListResult 查询结果；status 非 ok 时 Rules 为空且 Reason 说明原因。
type AlertRuleListResult struct {
	Status     string             `json:"status"`
	Reason     string             `json:"reason,omitempty"`
	EvidenceID string             `json:"evidence_id,omitempty"`
	Rules      []AlertRuleSummary `json:"rules,omitempty"`
}

// AlertRuleListPort 只读规则查询端口，由 infrastructure/observability 适配。
// 与 MetricEvidencePort 相同：业务失败以 Result.Status 表达，端口不可用才返回 error。
type AlertRuleListPort interface {
	List(ctx context.Context, userID string, q AlertRuleQuery) (*AlertRuleListResult, error)
}

// SetAlertRules 注入规则查询能力；nil 表示关闭规则体检工具。
func (s *ChatService) SetAlertRules(rules AlertRuleListPort) {
	if s == nil {
		return
	}
	s.alertRules = rules
}
