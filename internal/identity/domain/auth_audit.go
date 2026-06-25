package domain

import "time"

// AuthAuditEvent 表示一条认证审计属于边个入口，例如登录、刷新 token 或登出。
type AuthAuditEvent string

const (
	AuthAuditEventLogin   AuthAuditEvent = "login"
	AuthAuditEventRefresh AuthAuditEvent = "refresh"
	AuthAuditEventLogout  AuthAuditEvent = "logout"
)

// AuthAuditMethod 表示本次认证动作使用哪种方式触发。
type AuthAuditMethod string

const (
	AuthAuditMethodLocal    AuthAuditMethod = "local"
	AuthAuditMethodExternal AuthAuditMethod = "external"
	AuthAuditMethodOAuth    AuthAuditMethod = "oauth"
	AuthAuditMethodRefresh  AuthAuditMethod = "refresh"
)

// AuthAuditResult 标记认证动作最终成功或失败，方便后台筛选和排查。
type AuthAuditResult string

const (
	AuthAuditResultSuccess AuthAuditResult = "success"
	AuthAuditResultFailure AuthAuditResult = "failure"
)

// AuthAudit 保存一次认证链路的审计资料，包含入口、结果、IP、UA 和失败原因。
type AuthAudit struct {
	ID         string
	UserID     string
	Username   string
	ProviderID string
	Event      AuthAuditEvent
	Method     AuthAuditMethod
	Result     AuthAuditResult
	IP         string
	UserAgent  string
	Reason     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// AuthAuditFilter 是管理员查询认证审计时使用的筛选条件和分页参数。
type AuthAuditFilter struct {
	UserID     string
	Username   string
	ProviderID string
	Event      AuthAuditEvent
	Result     AuthAuditResult
	Limit      int
	Offset     int
}
