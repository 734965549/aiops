package domain

import "time"

// ExternalIdentity 表示外部身份源与平台用户的绑定关系。
type ExternalIdentity struct {
	ID               string
	UserID           string
	ProviderID       string
	ExternalSubject  string
	ExternalUsername string
	ExternalEmail    string
	ExternalGroups   []string
	LastLoginAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AuthenticatedExternalUser 是身份源认证成功后返回的用户摘要。
type AuthenticatedExternalUser struct {
	ProviderID      string
	ExternalSubject string
	Username        string
	DisplayName     string
	Email           string
	Groups          []string
}
