package application

import "context"

// AccountSummary 是注入对话观测的集成账号摘要。
// 脱敏边界：只含账号标识、名称、provider 与区域，不含 auth_type、extra_config、凭据或 project_id。
type AccountSummary struct {
	AccountID string   `json:"account_id"`
	Name      string   `json:"name,omitempty"`
	Provider  string   `json:"provider,omitempty"`
	Regions   []string `json:"regions,omitempty"`
}

// AccountListResult 是一次账号发现结果；status 非 ok 时 Accounts 为空且 Reason 说明原因。
type AccountListResult struct {
	Status   string           `json:"status"`
	Reason   string           `json:"reason,omitempty"`
	Total    int              `json:"total,omitempty"`
	Accounts []AccountSummary `json:"accounts,omitempty"`
}

// AccountListPort 发现当前用户可见的集成账号，由 infrastructure/integration 适配。
// 实现方负责数据范围过滤：清单与 metrics.query 取数保持同一口径。
type AccountListPort interface {
	List(ctx context.Context, userID string) (*AccountListResult, error)
}

// SetAccounts 注入账号发现能力；nil 表示关闭 accounts.list 工具。
func (s *ChatService) SetAccounts(accounts AccountListPort) {
	if s == nil {
		return
	}
	s.accounts = accounts
}
