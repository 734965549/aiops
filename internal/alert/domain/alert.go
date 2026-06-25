package domain

import "time"

// Alert 是告警中心主记录，一条记录代表一轮可处理的告警生命周期。
// 字段与 alert_alert 表及 API Alert 对象对齐；application_id/resource_id 为 AI/Execution 预留关联。
type Alert struct {
	ID              string            // 告警业务 ID（UUID）
	ExternalID      string            // 外部告警 ID 或 fingerprint
	Source          string            // 来源类型，如 prometheus_alertmanager
	SourceID        string            // 平台接入源 ID
	SourceName      string            // 接入源显示名快照
	Fingerprint     string            // 外部指纹
	DedupKey        string            // 平台去重键，必须稳定
	LifecycleSeq    int               // 生命周期序号，closed 后重开递增
	Name            string            // 告警名称
	Summary         string            // 摘要
	Description     string            // 详细描述
	Severity        AlertSeverity     // 级别
	Status          AlertStatus       // 当前状态
	RuleID          string            // 外部规则 ID
	RuleName        string            // 外部规则名
	BusinessLine    string            // 业务线
	Environment     string            // 环境
	ApplicationID   string            // 关联应用 ID（Asset 匹配后写入）
	ApplicationName string            // 关联应用名快照
	ResourceID      string            // 关联资源 ID
	ResourceType    string            // 资源类型
	ResourceName    string            // 资源名称
	OwnerUserID     string            // 归属负责人
	AssigneeUserID  string            // 当前处理人
	Labels          map[string]string // 标准化标签
	Annotations     map[string]string // 标准化注解
	OccurrenceCount int               // 重复触发次数
	FirstSeenAt     time.Time         // 首次触发
	LastSeenAt      time.Time         // 最近触发/更新
	RecoveredAt     *time.Time        // 恢复时间
	AcknowledgedAt  *time.Time        // 认领时间
	ClosedAt        *time.Time        // 关闭时间
	SilencedUntil   *time.Time        // 静默截止
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// IsActive 返回告警是否仍处于未关闭状态（含 silenced，仍接收外部更新）。
func (a Alert) IsActive() bool {
	return a.Status.IsActive()
}
