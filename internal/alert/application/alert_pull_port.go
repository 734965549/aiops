package application

import (
	"context"
	"time"
)

// AlertPullPort 是告警拉取端口（alert/application 定义，自包含类型）：
// 入参只包含账号、scope 游标与轮次截止时间，出参为按 scope 的 CES 告警记录，
// 不暴露 observability 内部类型。由 internal/alert/infrastructure/observability 适配。
type AlertPullPort interface {
	PullAlarms(ctx context.Context, req AlertPullRequest) (*AlertPullResult, error)
}

// AlertPullRequest 单个接入源的一轮拉取请求。
type AlertPullRequest struct {
	SourceID  string
	AccountID string
	// Regions 为空时由适配器按账号配置解析全部 region；region_filter 由调用方收窄后传入。
	Regions []string
	// Cursors 为 region/project scope -> 已处理至的 Unix 秒；0/缺失表示新 scope。
	Cursors map[string]int64
	// RoundCutoff 是本轮固定截止时间（Unix 秒），用于窗口上界与游标推进。
	RoundCutoff int64
	// Deadline 是本轮整体截止时间（手动 20s）。
	Deadline time.Time
}

// PulledAlarm 是归一化前的 CES 告警记录快照。
type PulledAlarm struct {
	RecordID     string
	AlarmID      string
	Name         string
	Status       string // alarm / ok / ok_manual / invalid / insufficient_data
	Level        string // CES level "1".."4"
	Namespace    string
	ResourceID   string
	ResourceName string
	ResourceType string
	Dimensions   map[string]string
	FirstSeenAt  *time.Time
	LastAlarmAt  *time.Time
	RecoveredAt  *time.Time
}

// PulledScope 单个 region/project scope 的拉取结果。
// Complete=true 表示窗口完整处理且已提交记录可推进游标。
type PulledScope struct {
	Region        string
	ProjectID     string
	Records       []PulledAlarm
	WindowEnd     int64
	Complete      bool
	FailureCode   string
	FailureReason string
}

// AlertPullScopeFailure 描述区域不可用、缺 project 等非 scope 级失败。
type AlertPullScopeFailure struct {
	Region    string
	ProjectID string
	Reason    string
}

// AlertPullResult 一轮拉取结果。
type AlertPullResult struct {
	Regions  []string
	Scopes   []PulledScope
	Failures []AlertPullScopeFailure
}
