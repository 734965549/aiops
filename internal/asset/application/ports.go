package application

import (
	"context"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
)

// CloudDiscoveryPort 云资源发现端口（复用 Observability QueryService）。
type CloudDiscoveryPort interface {
	ListResources(ctx context.Context, actor obsapp.Actor, q obsdomain.AssetDiscoveryQuery) (*obsapp.AssetDiscoveryResult, error)
}

// SyncAccountSnapshot 同步所需的接入账号摘要。
type SyncAccountSnapshot struct {
	AccountID string
	Provider  string
	Regions   []string
	Enabled   bool
}

// IntegrationAccountPort 读取 Integration 账号配置（不暴露凭据）。
type IntegrationAccountPort interface {
	ResolveSyncAccount(ctx context.Context, accountID string) (*SyncAccountSnapshot, error)
}
