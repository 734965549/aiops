package application

import "context"

// AuthorizationInput 表示一次授权请求的最小必要字段。
type AuthorizationInput struct {
	UserID       string
	Resource     string
	Action       string
	ObjectOwner  string
	ObjectDept   string
	ObjectTeam   string
	ObjectRegion string
	ObjectTags   []string
}

// AuthorizationResult 表示一次授权校验结果。
type AuthorizationResult struct {
	Allowed bool
	Reason  string
}

// AuthorizationPort 适配统一授权服务。
type AuthorizationPort interface {
	Authorize(ctx context.Context, in AuthorizationInput) (*AuthorizationResult, error)
	ResolveAccessibleOwnerTeams(ctx context.Context, userID string) ([]string, bool, error)
}
