// Package domain 描述 Identity 限界上下文的领域模型与仓储接口。
//
// 当前阶段仅放置最小可用的 User 聚合根，后续扩展角色、权限、组织等。
package domain

import "time"

// UserStatus 用户状态。
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
	UserStatusLocked   UserStatus = "locked"
)

// User 表示平台用户聚合根。
//
// 注意：密码哈希等敏感字段只在 infrastructure 内部使用，
// 暴露到 application / interfaces 层时使用 DTO，避免泄漏。
type User struct {
	ID           string
	Username     string
	DisplayName  string
	Email        string
	PasswordHash string
	Status       UserStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type UserFilter struct {
	Status  *UserStatus
	Keyword string
	Limit   int
	Offset  int
}

// IsActive 判断用户是否可登录。
func (u *User) IsActive() bool {
	return u != nil && u.Status == UserStatusActive
}
