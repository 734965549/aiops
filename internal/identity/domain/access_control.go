// Package domain 描述 Identity 限界上下文的领域模型与仓储接口。

package domain

import (
	"strings"
	"time"
)

// RoleStatus 角色状态。
type RoleStatus string

const (
	RoleStatusActive   RoleStatus = "active"
	RoleStatusDisabled RoleStatus = "disabled"
)

// Permission 表示平台权限定义。
type Permission struct {
	ID          string
	Code        string
	Name        string
	Resource    string
	Action      string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Role 表示平台角色。
type Role struct {
	ID          string
	Code        string
	Name        string
	Description string
	Status      RoleStatus
	IsSystem    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UserRoleSource 表示用户角色绑定的来源，用于区分可自动回收的托管角色与手工授权。
type UserRoleSource string

const (
	UserRoleSourceManual        UserRoleSource = "manual"
	UserRoleSourceExternalGroup UserRoleSource = "external_group"
	UserRoleSourceLDAPImport    UserRoleSource = "ldap_import"
)

// NormalizeUserRoleSource 将空来源视为 manual。
func NormalizeUserRoleSource(source UserRoleSource) UserRoleSource {
	if strings.TrimSpace(string(source)) == "" {
		return UserRoleSourceManual
	}
	return source
}

// PreserveUserRoleSource 在重复绑定时保留 manual / ldap_import，避免登录同步覆盖手工授权。
func PreserveUserRoleSource(current, incoming UserRoleSource) UserRoleSource {
	current = NormalizeUserRoleSource(current)
	incoming = NormalizeUserRoleSource(incoming)
	if current == UserRoleSourceManual || current == UserRoleSourceLDAPImport {
		return current
	}
	return incoming
}

// UserRole 表示用户与角色的关联。
type UserRole struct {
	ID        int64
	UserID    string
	RoleID    string
	Source    UserRoleSource
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DataScopeType 表示数据范围类型。
type DataScopeType string

const (
	DataScopeAll        DataScopeType = "all"
	DataScopeDepartment DataScopeType = "department"
	DataScopeTeam       DataScopeType = "team"
	DataScopeRegion     DataScopeType = "region"
	DataScopeTag        DataScopeType = "tag"
	DataScopeCustom     DataScopeType = "custom"
)

// DataScope 表示数据权限范围定义。
type DataScope struct {
	ID          string
	Code        string
	Name        string
	ScopeType   DataScopeType
	ScopeConfig map[string]any
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AI tool permission mode。
type AIToolPermissionMode string

const (
	AIToolPermissionReadOnly       AIToolPermissionMode = "read_only"
	AIToolPermissionRequireConfirm AIToolPermissionMode = "require_confirm"
	AIToolPermissionDeny           AIToolPermissionMode = "deny"
)

// AIToolPermission 表示 AI 工具权限定义。
type AIToolPermission struct {
	ID             string
	ToolCode       string
	ToolName       string
	PermissionMode AIToolPermissionMode
	// PermitsUnconfirmedInvoke 为 true 时，require_confirm 类工具可在未携带 UserConfirmed 时直接放行
	// （通常授予可跳过人工确认的高权限角色）；为 false 时必须携带 UserConfirmed 才允许执行。
	PermitsUnconfirmedInvoke bool
	Description              string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}
