package domain

import "time"

// AlertSilence 记录一次静默操作，对应 alert_silence 表（§5.4）。
// 第一阶段仅支持单条告警静默；Matcher 留作后续标签规则扩展（§8.8）。
type AlertSilence struct {
	ID        string            // 静默业务 ID
	AlertID   string            // 目标告警 ID
	Matcher   map[string]string // 标签 matcher（后续扩展）
	Reason    string            // 静默原因
	StartsAt  time.Time         // 开始时间
	EndsAt    time.Time         // 结束时间
	CreatedBy string            // 创建人 user_id
	CreatedAt time.Time
	UpdatedAt time.Time
}
