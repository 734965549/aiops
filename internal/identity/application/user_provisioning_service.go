package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/auth"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CreateLocalUserInput 是管理员创建本地账号（预留注册路径，不对外公开）的入参。
type CreateLocalUserInput struct {
	Username    string
	Password    string
	DisplayName string
	Email       string
}

// ProvisionExternalIdentityInput 是管理员预置域账号 / 外部身份绑定的入参。
//
// 外部用户首次登录前必须存在绑定；平台不会按用户名自动关联已有账号。
type ProvisionExternalIdentityInput struct {
	ProviderID       string
	ExternalSubject  string
	ExternalUsername string
	DisplayName      string
	Email            string
	// PlatformUsername 指定平台用户名；为空时按 provider 命名空间自动生成，避免与本地账号冲突。
	PlatformUsername string
	// UserID 非空时将绑定到已有平台用户，不再新建用户。
	UserID string
}

// CreateLocalUser 创建带密码的本地平台账号。用户名全局唯一（与域账号、外部账号共用命名空间）。
func (s *AuthService) CreateLocalUser(ctx context.Context, in CreateLocalUserInput) (*CurrentUserDTO, error) {
	if s == nil || s.users == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "authentication service is not configured")
	}
	username, err := validatePlatformUsername(in.Username)
	if err != nil {
		return nil, err
	}
	password := in.Password
	if password == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "password is required")
	}
	if len(password) < 8 || len(password) > 256 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "password length must be between 8 and 256")
	}
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		displayName = username
	}
	existing, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "check username failed")
	}
	if existing != nil {
		return nil, apperr.New(apperr.CodeAlreadyExists, "username already exists")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "hash password failed")
	}
	u := &domain.User{
		ID:           uuid.NewString(),
		Username:     username,
		DisplayName:  displayName,
		Email:        strings.TrimSpace(in.Email),
		PasswordHash: hash,
		Status:       domain.UserStatusActive,
	}
	if err := s.users.Create(ctx, u); err != nil {
		if mapped := mapAlreadyExists(err, "username already exists"); mapped != err {
			return nil, mapped
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "create user failed")
	}
	logger.From(ctx).Info("local user provisioned by admin", zap.String("username", username), zap.String("user_id", u.ID))
	return toCurrentUserDTO(u), nil
}

// ProvisionExternalIdentity 预置外部身份绑定，使域账号 / SSO 用户可在平台登录。
func (s *AuthService) ProvisionExternalIdentity(ctx context.Context, in ProvisionExternalIdentityInput) (*CurrentUserDTO, error) {
	if s == nil || s.users == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "authentication service is not configured")
	}
	if s.externalIDs == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "external identity store is not configured")
	}
	providerID := strings.TrimSpace(in.ProviderID)
	externalSubject := strings.TrimSpace(in.ExternalSubject)
	if providerID == "" || externalSubject == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "provider_id and external_subject are required")
	}
	existingBinding, err := s.externalIDs.FindByProviderSubject(ctx, providerID, externalSubject)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load external identity failed")
	}
	if existingBinding != nil {
		u, err := s.users.FindByID(ctx, existingBinding.UserID)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "load user failed")
		}
		if u == nil {
			return nil, apperr.New(apperr.CodeInternal, "external identity references missing user")
		}
		return toCurrentUserDTO(u), nil
	}

	var u *domain.User
	createdUser := false
	userID := strings.TrimSpace(in.UserID)
	if userID != "" {
		u, err = s.users.FindByID(ctx, userID)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "load user failed")
		}
		if u == nil {
			return nil, apperr.New(apperr.CodeNotFound, "user not found")
		}
		linked, err := s.externalIDs.FindByUserAndProvider(ctx, u.ID, providerID)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "load external identity failed")
		}
		if linked != nil {
			return nil, apperr.New(apperr.CodeAlreadyExists, "user already bound to this provider")
		}
	} else {
		username := strings.TrimSpace(in.PlatformUsername)
		if username == "" {
			username = namespacedExternalUsername(providerID, in.ExternalUsername)
		}
		username, err = validatePlatformUsername(username)
		if err != nil {
			return nil, err
		}
		existing, err := s.users.FindByUsername(ctx, username)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "check username failed")
		}
		if existing != nil {
			return nil, apperr.New(apperr.CodeAlreadyExists, "username already exists")
		}
		displayName := strings.TrimSpace(in.DisplayName)
		if displayName == "" {
			displayName = strings.TrimSpace(in.ExternalUsername)
		}
		if displayName == "" {
			displayName = username
		}
		u = &domain.User{
			ID:          uuid.NewString(),
			Username:    username,
			DisplayName: displayName,
			Email:       strings.TrimSpace(in.Email),
			Status:      domain.UserStatusActive,
		}
		if err := s.users.Create(ctx, u); err != nil {
			if mapped := mapAlreadyExists(err, "username already exists"); mapped != err {
				return nil, mapped
			}
			return nil, apperr.Wrap(err, apperr.CodeInternal, "create user failed")
		}
		createdUser = true
	}

	binding := &domain.ExternalIdentity{
		ID:               uuid.NewString(),
		UserID:           u.ID,
		ProviderID:       providerID,
		ExternalSubject:  externalSubject,
		ExternalUsername: strings.TrimSpace(in.ExternalUsername),
		ExternalEmail:    strings.TrimSpace(in.Email),
	}
	if err := s.externalIDs.Create(ctx, binding); err != nil {
		if createdUser {
			s.rollbackCreatedUser(ctx, u.ID)
		}
		if mapped := mapAlreadyExists(err, "external identity already bound"); mapped != err {
			return nil, mapped
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "create external identity failed")
	}
	logger.From(ctx).Info("external identity provisioned by admin",
		zap.String("provider_id", providerID),
		zap.String("external_subject", externalSubject),
		zap.String("user_id", u.ID),
		zap.String("username", u.Username),
	)
	return toCurrentUserDTO(u), nil
}

func (s *AuthService) rollbackCreatedUser(ctx context.Context, userID string) {
	if s == nil || s.users == nil || strings.TrimSpace(userID) == "" {
		return
	}
	if err := s.users.DeleteByID(ctx, userID); err != nil {
		logger.From(ctx).Warn("rollback provisioned user failed", zap.String("user_id", userID), zap.Error(err))
	}
}

func validatePlatformUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", apperr.New(apperr.CodeInvalidArgument, "username is required")
	}
	if len(username) > 64 {
		return "", apperr.New(apperr.CodeInvalidArgument, "username is too long")
	}
	return username, nil
}

// namespacedExternalUsername 为外部账号生成带 provider 前缀的平台用户名，保留完整外部标识以避免跨域碰撞。
func namespacedExternalUsername(providerID, externalUsername string) string {
	providerID = strings.TrimSpace(providerID)
	ext := strings.TrimSpace(externalUsername)
	if ext == "" {
		ext = "unknown"
	}
	prefix := providerID + ":"
	candidate := prefix + ext
	if len(candidate) <= 64 {
		return candidate
	}
	maxExtLen := 64 - len(prefix)
	if maxExtLen < 1 {
		maxExtLen = 1
	}
	return prefix + ext[:maxExtLen]
}

func toCurrentUserDTO(u *domain.User) *CurrentUserDTO {
	if u == nil {
		return nil
	}
	return &CurrentUserDTO{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		Status:      string(u.Status),
	}
}
