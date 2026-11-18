package application

import "context"

// AlertContext 供 AI 分析读取告警快照（只读 port，避免 AI 依赖 Alert application）。
// 字段整体序列化为 provider payload.alert，新增字段应直接加在这里，避免逐字段挑选遗漏。
type AlertContext struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Summary         string            `json:"summary"`
	Description     string            `json:"description"`
	Severity        string            `json:"severity"`
	Status          string            `json:"status"`
	Source          string            `json:"source"`
	RuleName        string            `json:"rule_name"`
	BusinessLine    string            `json:"business_line"`
	Environment     string            `json:"environment"`
	ApplicationID   string            `json:"application_id"`
	ApplicationName string            `json:"application_name"`
	ResourceID      string            `json:"resource_id"`
	ResourceName    string            `json:"resource_name"`
	ResourceType    string            `json:"resource_type"`
	OccurrenceCount int               `json:"occurrence_count"`
	FirstSeenAt     int64             `json:"first_seen_at"`
	LastSeenAt      int64             `json:"last_seen_at"`
	RecoveredAt     int64             `json:"recovered_at,omitempty"`
	Labels          map[string]string `json:"labels"`
	Annotations     map[string]any    `json:"annotations"`
	// OwnerUserID 仅用于对象级数据范围校验（custom 范围的 owner_ids），
	// 不进入 provider payload，避免把内部用户标识写进模型输入。
	OwnerUserID string `json:"-"`
}

// AlertReader 只读告警详情，由 infrastructure/alert 适配 alert 仓储实现。
// 读取必须按当前用户的数据范围过滤，拒绝时返回 PERMISSION_DENIED。
type AlertReader interface {
	GetForAnalysis(ctx context.Context, userID, alertID string) (*AlertContext, error)
}
