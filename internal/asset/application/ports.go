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

// ApplicationDeleteExecutor 在单数据库事务内锁定应用、重新校验引用、
// 解除 closed 告警关联并删除应用，避免分步提交导致部分结果或并发绕过前置计数。
type ApplicationDeleteExecutor interface {
	DeleteApplicationAtomic(ctx context.Context, applicationID string) error
}

// ApplicationReferenceChecker 跨上下文应用引用检查器。
// DeleteApplication 在无 ApplicationDeleteExecutor 时回退使用（单测/兼容）；
// 生产路径应注入 ApplicationDeleteExecutor 以保证事务与并发完整性。
type ApplicationReferenceChecker interface {
	// CountAlertsByApplicationID 统计引用了指定 application_id 的未关闭告警数量。
	CountAlertsByApplicationID(ctx context.Context, applicationID string) (int64, error)
	// CountInspectionPoliciesByApplicationID 统计 scope.application_ids 包含指定 application_id 的巡检策略数量。
	CountInspectionPoliciesByApplicationID(ctx context.Context, applicationID string) (int64, error)
	// DetachClosedAlertReferences 删除应用前解除已关闭告警的应用关联，避免产生孤儿引用。
	DetachClosedAlertReferences(ctx context.Context, applicationID string) error
}
