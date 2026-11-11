package application

import (
	"context"
	"time"
)

// AlertPullPort 是告警拉取端口（alert/application 定义，自包含类型）：
// 入参只包含账号与 scope，出参为 CES 告警实例与逐 scope 失败，不暴露 observability 内部类型。
// 由 internal/alert/infrastructure/observability 适配 observability 应用服务。
type AlertPullPort interface {
	PullAlarms(ctx context.Context, req AlertPullRequest) (*AlertPullResult, error)
}

// AlertPullRequest 单个接入源的一轮拉取请求。
type AlertPullRequest struct {
	SourceID  string
	AccountID string
	// Regions 为空时由适配器按账号配置解析全部 region；region_filter 由调用方收窄后传入。
	Regions []string
	// Since 为增量下界（上轮完成时间减重叠窗口）；nil 表示首次拉取使用默认回看窗口。
	Since *time.Time
}

// PulledAlarm 是归一化前的 CES 告警实例快照。
type PulledAlarm struct {
	RecordID     string
	AlarmID      string
	Name         string
	Status       string // alarm / ok / ok_manual / invalid / insufficient_data
	SeverityName string // critical / major / minor / info
	ResourceType string
	ResourceName string
	Summary      string
	Description  string
	Region       string
	ProjectID    string
	Labels       map[string]string
	FirstSeenAt  *time.Time
	LastSeenAt   *time.Time
	RecoveredAt  *time.Time
}

// AlertPullScopeFailure 描述单个 region/project scope 的失败，用于 failed_scopes 与摘要。
type AlertPullScopeFailure struct {
	Region    string
	ProjectID string
	Code      string
	Message   string
}

// AlertPullResult 一轮拉取结果；Instances 可能来自部分成功的 scope。
type AlertPullResult struct {
	Regions      []string
	Instances    []PulledAlarm
	FailedScopes []AlertPullScopeFailure
}
