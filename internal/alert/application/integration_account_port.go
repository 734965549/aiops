package application

import "context"

// IntegrationAccountReader 是拉取式接入源校验账号的端口（alert/application 自包含类型，
// 不 import integration/observability 的实现）。由 infrastructure 适配 integration 账号服务。
type IntegrationAccountReader interface {
	GetAccount(ctx context.Context, accountID string) (*IntegrationAccountInfo, error)
}

// IntegrationAccountInfo 是告警拉取所需的账号摘要，不含任何凭据。
type IntegrationAccountInfo struct {
	AccountID      string
	Provider       string
	AuthType       string
	Regions        []string
	ProjectID      string
	RegionProjects map[string]string
	OwnerTeam      string
	Capabilities   []string
	Enabled        bool
}

// AccountAccessChecker 校验当前用户对绑定账号的数据范围权限。
type AccountAccessChecker interface {
	CheckAccountAccess(ctx context.Context, userID string, account *IntegrationAccountInfo) error
}
