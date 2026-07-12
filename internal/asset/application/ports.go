package application

import (
	"context"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
)

// CloudDiscoveryPort 云资源发现端口（复用 Observability QueryService）。
type CloudDiscoveryPort interface {
	ListResources(ctx context.Context, actor obsapp.Actor, q obsdomain.AssetDiscoveryQuery) (*obsapp.AssetDiscoveryResult, error)
	// ListAllResources 全量同步发现，不受交互查询 limit<=500 限制，返回资源与同步摘要，见 §7.2。
	ListAllResources(ctx context.Context, actor obsapp.Actor, q obsapp.AssetFullSyncQuery) (*obsapp.AssetFullSyncResult, error)
}

// SyncAccountSnapshot 同步所需的接入账号摘要。
type SyncAccountSnapshot struct {
	AccountID string
	Provider  string
	Regions   []string
	Enabled   bool
	// ExtraConfig 透传 provider 专属扩展配置原始 JSON，由 provider adapter 解析；禁止存放密钥。
	ExtraConfig     []byte
	ProjectID       string
	AuthType        string
	CredentialRefID string
	Capabilities    []string
	OwnerTeam       string
}

// IntegrationAccountPort 读取 Integration 账号配置（不暴露凭据）。
type IntegrationAccountPort interface {
	ResolveSyncAccount(ctx context.Context, accountID string) (*SyncAccountSnapshot, error)
}

// ApplicationReferenceChecker 跨上下文应用引用检查器。
// DeleteApplication 使用它防止 alert_alert / inspection_policy 产生孤儿引用，
// 确保 v_asset_app_ref_integrity 视图返回 0 行（release-checklist §2.6）。
type ApplicationReferenceChecker interface {
	// CountAlertsByApplicationID 统计引用了指定 application_id 的未关闭告警数量。
	CountAlertsByApplicationID(ctx context.Context, applicationID string) (int64, error)
	// CountInspectionPoliciesByApplicationID 统计 scope.application_ids 包含指定 application_id 的巡检策略数量。
	CountInspectionPoliciesByApplicationID(ctx context.Context, applicationID string) (int64, error)
}
