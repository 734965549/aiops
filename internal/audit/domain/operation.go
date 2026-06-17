// Package domain 定义平台操作审计领域模型（Alert §9.4 等模块写入）。
package domain

import (
	"time"
)

// OperationAudit 记录一次业务操作审计。
type OperationAudit struct {
	ID           string
	UserID       string
	ResourceType string // 如 alert
	ResourceID   string
	Action       string // 如 close / silence
	Payload      map[string]any
	IP           string
	UserAgent    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// OperationAuditFilter 审计列表筛选。
type OperationAuditFilter struct {
	ResourceType string
	ResourceID   string
	UserID       string
	Action       string
	Limit        int
	Offset       int
}
