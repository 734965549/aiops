// Package application 聚合 Identity 上下文的用例（应用服务）。
//
// 应用层只负责编排领域模型与基础设施，禁止承载领域规则。
package application

import (
	"context"

	"github.com/734965549/aiops/internal/identity/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// CurrentUserDTO 给上层 interface 使用的用户视图（脱去敏感字段）。
type CurrentUserDTO struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Status      string `json:"status"`
}

// UserService 暴露给 interfaces/http 的应用服务。
type UserService struct {
	users domain.UserRepository
}

// NewUserService 构造 UserService。
func NewUserService(users domain.UserRepository) *UserService {
	return &UserService{users: users}
}

// GetCurrentUser 根据当前请求上下文返回用户信息。
//
// 调用方应已经过 AuthRequired 中间件，userID 不应为空；
// 此处把空 userID 作为编程错误返回 UNAUTHENTICATED，不再走任何 demo fallback。
func (s *UserService) GetCurrentUser(ctx context.Context, userID string) (*CurrentUserDTO, error) {
	if userID == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "missing user identity")
	}

	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load user failed")
	}
	if u == nil {
		return nil, apperr.New(apperr.CodeNotFound, "user not found")
	}
	return &CurrentUserDTO{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		Status:      string(u.Status),
	}, nil
}
