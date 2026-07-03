package domain

import (
	"context"
	"time"
)

// ResourceMatchQuery 资源匹配查询条件（§9.1：namespace/pod/node/instance/resource_name）。
type ResourceMatchQuery struct {
	ApplicationID string
	Name          string
	ResourceType  string
	Namespace     string
	Pod           string
	Node          string
	Instance      string
}

// ApplicationFilter 应用注册表列表分页过滤（对应 §5.5 标准分页查询）。
type ApplicationFilter struct {
	Limit  int
	Offset int
}

// ApplicationRepository 应用注册表持久化接口。
type ApplicationRepository interface {
	Create(ctx context.Context, app *Application) error
	List(ctx context.Context) ([]Application, error)
	// ListPaged 按分页过滤返回应用列表与总数（page 从 1 开始，由调用方换算 Offset）。
	ListPaged(ctx context.Context, filter ApplicationFilter) ([]Application, int64, error)
	GetByID(ctx context.Context, id string) (*Application, error)
	Update(ctx context.Context, app *Application) error
	Delete(ctx context.Context, id string) error
	FindByNameEnv(ctx context.Context, name, environment string) (*Application, error)
	ExistsByID(ctx context.Context, id string) (bool, error)
	Count(ctx context.Context) (int64, error)
}

// ResourceFilter 资源注册表列表分页过滤（对应 §5.5 标准分页查询）。
type ResourceFilter struct {
	CloudResourceType string
	Region            string
	SyncStatus        string
	Limit             int
	Offset            int
}

// ResourceRepository 资源注册表持久化接口。
type ResourceRepository interface {
	Create(ctx context.Context, res *Resource) error
	ListByApplicationID(ctx context.Context, applicationID string) ([]Resource, error)
	// ListByApplicationIDPaged 按应用 + 分页过滤返回资源列表与总数。
	ListByApplicationIDPaged(ctx context.Context, applicationID string, filter ResourceFilter) ([]Resource, int64, error)
	GetByID(ctx context.Context, id string) (*Resource, error)
	Update(ctx context.Context, res *Resource) error
	Delete(ctx context.Context, id string) error
	CountByApplicationID(ctx context.Context, applicationID string) (int64, error)
	FindBestMatch(ctx context.Context, q ResourceMatchQuery) (*Resource, error)
	Count(ctx context.Context) (int64, error)
	FindByCloudKey(ctx context.Context, key CloudResourceKey) (*Resource, error)
	// UpsertCloudSyncWithLease 在同一事务内校验 batch_id + fencing_token 的 running 未过期租约后写入云同步资源。
	UpsertCloudSyncWithLease(ctx context.Context, res *Resource, batchID, fencingToken string) (created bool, err error)
	// MarkStaleByAccountScopeExceptBatchWithLease 在同一事务内校验租约后执行逐类型 stale 标记。
	MarkStaleByAccountScopeExceptBatchWithLease(ctx context.Context, accountID, region, cloudResourceType, batchID, fencingToken string) (int64, error)
	// MarkStaleByAccountRegionExceptTypesWithLease 在同一事务内校验租约后执行反向 stale 标记。
	MarkStaleByAccountRegionExceptTypesWithLease(ctx context.Context, accountID, region string, exceptTypes []string, batchID, fencingToken string) (int64, error)
}

// SyncBatchRepository 同步批次持久化接口。
type SyncBatchRepository interface {
	Create(ctx context.Context, batch *SyncBatch) error
	Update(ctx context.Context, batch *SyncBatch) error
	// FinalizeSuccess 在同一事务内完成：
	// 1) 按 batch_id + fencing_token 校验未过期 running 租约；
	// 2) 将批次标记为 success 并清空租约；
	// 3) 将本轮已写入的 active 资源 sync_batch_id 提升为本批次。
	// 返回被提升的资源数。若租约丢失返回 ErrLeaseLost。
	FinalizeSuccess(ctx context.Context, batch *SyncBatch, accountID string, syncedSince time.Time) (int64, error)
	GetByID(ctx context.Context, batchID string) (*SyncBatch, error)
	List(ctx context.Context, filter SyncBatchFilter) ([]SyncBatch, int64, error)
	// ReapExpiredRunning 将指定账号下租约已过期的 running 批次标记为 failed，
	// 释放账号级 running 槽位，避免崩溃批次导致后续同步永久 409。
	// now 为判定基准时间（UTC）。返回被 reap 的行数。
	ReapExpiredRunning(ctx context.Context, accountID string, now time.Time) (int64, error)
	// RenewLease 续租 running 批次：按 batch_id + fencing_token 校验租约所有权，
	// 成功时把 lease_expires_at 置为 now+ttl，updated_at=now。
	// 若批次已终态、被 reap 或 token 不匹配，返回 ErrLeaseLost。ttl 为续租有效期。
	RenewLease(ctx context.Context, batchID, fencingToken string, now time.Time, ttl time.Duration) error
	// CheckLeaseOwned 校验当前后台任务仍持有 running 租约；写操作前调用。
	CheckLeaseOwned(ctx context.Context, batchID, fencingToken string, now time.Time) error
}

// SyncBatchFilter 同步批次列表过滤。
type SyncBatchFilter struct {
	IntegrationAccountID string
	Limit                int
	Offset               int
}

// MatchRuleFilter 匹配规则列表分页过滤（对应 §5.5 标准分页查询）。
type MatchRuleFilter struct {
	Limit  int
	Offset int
}

// MatchRuleRepository 匹配规则持久化接口。
type MatchRuleRepository interface {
	Create(ctx context.Context, rule *MatchRule) error
	List(ctx context.Context) ([]MatchRule, error)
	// ListPaged 按分页过滤返回匹配规则列表与总数（保持 priority DESC, created_at ASC 排序）。
	ListPaged(ctx context.Context, filter MatchRuleFilter) ([]MatchRule, int64, error)
	ListEnabledByPriority(ctx context.Context) ([]MatchRule, error)
	GetByID(ctx context.Context, id string) (*MatchRule, error)
	Update(ctx context.Context, rule *MatchRule) error
	Delete(ctx context.Context, id string) error
	CountByApplicationID(ctx context.Context, applicationID string) (int64, error)
	CountByResourceID(ctx context.Context, resourceID string) (int64, error)
}
