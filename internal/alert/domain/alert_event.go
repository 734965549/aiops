package domain

import "time"

// AlertEvent 领域模型见 ops/alert-contract.md §5.2；event_type 枚举见 §4.3。

// ActorType 时间线操作者类型。
type ActorType string

const (
	ActorSystem      ActorType = "system"      // 平台内部
	ActorUser        ActorType = "user"        // 登录用户
	ActorIntegration ActorType = "integration" // 外部接入（Webhook）
)

// AlertEvent 记录告警时间线上嘅单条事件，对应 alert_event 表。
type AlertEvent struct {
	ID        string         // 事件业务 ID
	AlertID   string         // 所属告警 ID
	EventType AlertEventType // 事件类型
	ActorType ActorType      // 操作者类型
	ActorID   string         // 用户 ID 或接入源 ID
	ActorName string         // 展示名
	Message   string         // 时间线文案
	Payload   map[string]any // 扩展数据（execution_id、request_id 等）
	CreatedAt time.Time
	UpdatedAt time.Time
}
