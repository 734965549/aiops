package domain

import (
	"strings"
	"time"
)

// AlarmPullScope 单个 region/project 拉取 scope（CES 告警记录）。
type AlarmPullScope struct {
	Region    string
	ProjectID string
}

// AlarmPullQuery 一轮按 scope 的告警记录拉取请求。
type AlarmPullQuery struct {
	Scopes      []AlarmPullScope
	Cursors     map[string]int64 // scopeKey -> unix 秒，0/缺失表示新 scope
	RoundCutoff int64            // 本轮固定截止时间（unix 秒）
	Deadline    time.Time        // 本轮整体截止时间
}

// AlarmRecord CES 告警记录归一化结果（未做平台去重/状态机）。
type AlarmRecord struct {
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

// AlarmScopeResult 单个 scope 的拉取结果。
// Complete=true 表示本轮窗口完整处理，可推进游标；否则保持旧游标。
type AlarmScopeResult struct {
	Region        string
	ProjectID     string
	Records       []AlarmRecord
	WindowEnd     int64
	Complete      bool
	FailureCode   string
	FailureReason string
}

// AlarmScopeFailure 非 scope 级（区域不可用、缺 project 等）失败。
type AlarmScopeFailure struct {
	Region    string
	ProjectID string
	Reason    string
}

// AlarmPullResult 一轮多 scope 拉取结果。
type AlarmPullResult struct {
	Scopes         []AlarmScopeResult
	PlannedRegions []string
	Failures       []AlarmScopeFailure
}

// AlarmScopeKey 返回 scope 游标键，与 alert 模块保持一致。
func AlarmScopeKey(region, projectID string) string {
	return strings.ToLower(strings.TrimSpace(region)) + "/" + strings.TrimSpace(projectID)
}
