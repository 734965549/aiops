package domain

import (
	"strconv"
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
	MetricName   string
	Condition    *AlarmCondition
	ResourceID   string
	ResourceName string
	ResourceType string
	Dimensions   map[string]string
	// RuleName / RuleDescription / RuleType 来自 ListAlarmRules 富化；
	// RuleLookup 记录富化结果（not_found / error），成功时为空。
	RuleName        string
	RuleDescription string
	RuleType        string
	RuleLookup      string
	FirstSeenAt     *time.Time
	LastAlarmAt     *time.Time
	RecoveredAt     *time.Time
}

// AlarmCondition 告警触发条件（华为 CES AlarmHistoryItemV2.condition，契约 §6.2）。
type AlarmCondition struct {
	Filter             string  // average / max / min / sum / variance
	ComparisonOperator string  // > / >= / < / <= / = / != / cycle_*
	Threshold          float64 // 阈值
	Unit               string  // % / count / ms ...
	PeriodSeconds      int64   // 指标聚合周期（秒）
	TriggerCount       int     // 连续触发周期数
	SuppressDurationS  int64   // 通知抑制间隔（秒）
}

// Describe 输出人类可读的触发条件，供 summary/annotations 与 AI 分析使用。
// 与 internal/alert/application.AlarmCondition.Describe 保持同一输出格式（端口自包含）。
func (c *AlarmCondition) Describe() string {
	if c == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(c.ComparisonOperator)
	if c.ComparisonOperator != "" {
		b.WriteString(" ")
	}
	b.WriteString(strconv.FormatFloat(c.Threshold, 'f', -1, 64))
	if c.Unit != "" {
		b.WriteString(c.Unit)
	}
	if c.Filter != "" {
		b.WriteString(" (" + c.Filter)
		if c.PeriodSeconds > 0 {
			b.WriteString(", " + strconv.FormatInt(c.PeriodSeconds, 10) + "s")
		}
		if c.TriggerCount > 0 {
			b.WriteString(" x" + strconv.Itoa(c.TriggerCount))
		}
		b.WriteString(")")
	}
	return b.String()
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
