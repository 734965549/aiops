package domain

import (
	"context"
	"time"
)

// 仓储接口定义在 domain 层，由 infrastructure/persistence 用 GORM 实现（DDD 依赖倒置）。
// 表结构与索引建议见 ops/alert-contract.md §7.3、§11。

// AlertFilter 告警列表筛选条件，由 application 层从 HTTP §8.1 查询参数组装。
type AlertFilter struct {
	Statuses       []AlertStatus
	Severities     []AlertSeverity
	Source         string
	SourceID       string
	BusinessLine   string
	Environment    string
	ApplicationID  string
	ResourceID     string
	AssigneeUserID string
	Keyword        string // 搜索 name / summary / resource_name
	ActiveOnly     bool   // 仅未关闭
	FromFirstSeen  *time.Time
	ToFirstSeen    *time.Time
	Limit          int
	Offset         int
}

// AlertRepository 告警主记录持久化接口。
type AlertRepository interface {
	Create(ctx context.Context, alert *Alert) error
	Update(ctx context.Context, alert *Alert) error
	GetByID(ctx context.Context, alertID string) (*Alert, error)
	// FindActiveByDedupKey 查找同一接入源下未 closed 的 active 告警（用于去重更新）。
	FindActiveByDedupKey(ctx context.Context, sourceID, dedupKey string) (*Alert, error)
	// MaxLifecycleSeq 返回 dedup_key 下已有最大 lifecycle_seq，closed 后重开时 +1。
	MaxLifecycleSeq(ctx context.Context, dedupKey string) (int, error)
	List(ctx context.Context, filter AlertFilter) ([]Alert, error)
	Count(ctx context.Context, filter AlertFilter) (int64, error)
}

// AlertEventRepository 告警时间线持久化接口。
type AlertEventRepository interface {
	Create(ctx context.Context, event *AlertEvent) error
	ListByAlertID(ctx context.Context, alertID string) ([]AlertEvent, error)
}

// AlertSourceRepository 接入源持久化接口。
type AlertSourceRepository interface {
	Create(ctx context.Context, source *AlertSource) error
	Update(ctx context.Context, source *AlertSource) error
	GetByID(ctx context.Context, sourceID string) (*AlertSource, error)
	List(ctx context.Context) ([]AlertSource, error)
	Delete(ctx context.Context, sourceID string) error
}

// AlertSilenceRepository 静默记录持久化接口。
type AlertSilenceRepository interface {
	Create(ctx context.Context, silence *AlertSilence) error
}
