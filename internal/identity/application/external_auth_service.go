package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/internal/identity/infrastructure/identityprovider"
	"github.com/734965549/aiops/internal/identity/infrastructure/oauthstate"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"go.uber.org/zap"
)

// ExternalLoginInput 是企业身份源登录参数。
type ExternalLoginInput struct {
	ProviderID string
	Username   string
	Password   string
}

// OAuthCallbackInput 係 OAuth2/OIDC 回调参数，包含服务端校验 state 时需要嘅客户端上下文。
type OAuthCallbackInput struct {
	ProviderID string
	Code       string
	State      string
	ClientIP   string
	UserAgent  string
}

// OAuthAuthorizeInput 用嚟签发授权地址，并将 state 绑定到发起授权嘅客户端。
type OAuthAuthorizeInput struct {
	ProviderID string
	ClientIP   string
	UserAgent  string
}

// ListIdentityProviders 返回已启用的企业身份源摘要。
func (s *AuthService) ListIdentityProviders() []domain.ProviderInfo {
	if s == nil || s.providers == nil {
		return nil
	}
	return s.providers.ListProviders()
}

// LoginExternal 通过 LDAP/AD 等企业身份源校验用户名密码并签发平台 token。
func (s *AuthService) LoginExternal(ctx context.Context, in ExternalLoginInput) (*TokenPair, error) {
	if s == nil || s.jwt == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "authentication service is not configured")
	}
	if s.providers == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "external identity providers are not configured")
	}
	providerID := strings.TrimSpace(in.ProviderID)
	if providerID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "provider_id is required")
	}
	username := strings.TrimSpace(in.Username)
	if username == "" || in.Password == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "username and password are required")
	}

	provider, ok := s.providers.PasswordProvider(providerID)
	if !ok {
		return nil, apperr.New(apperr.CodeInvalidArgument, "identity provider not found or not password-based")
	}

	extUser, err := provider.Authenticate(ctx, username, in.Password)
	if err != nil {
		logger.From(ctx).Warn("external login failed",
			zap.String("provider_id", providerID),
			zap.String("username", username),
			zap.Error(err),
		)
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid username or password")
	}

	u, err := s.provisionExternalUser(ctx, extUser, identityprovider.ProvisioningForPasswordProvider(provider))
	if err != nil {
		return nil, err
	}
	if !u.IsActive() {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid username or password")
	}
	return s.issueTokenPair(ctx, u)
}

// OAuthAuthorizeURL 返回 OAuth2/OIDC 授权跳转地址；保留旧签名方便内部调用兼容。
func (s *AuthService) OAuthAuthorizeURL(ctx context.Context, providerID string) (string, string, error) {
	return s.OAuthAuthorizeURLWithContext(ctx, OAuthAuthorizeInput{ProviderID: providerID})
}

// OAuthAuthorizeURLWithContext 返回 OAuth2/OIDC 授权地址，并将 state 同 provider、IP/UA 指纹绑定。
func (s *AuthService) OAuthAuthorizeURLWithContext(ctx context.Context, in OAuthAuthorizeInput) (string, string, error) {
	if s == nil || s.providers == nil {
		return "", "", apperr.New(apperr.CodeUnavailable, "external identity providers are not configured")
	}
	if s.oauthStateStore == nil {
		return "", "", apperr.New(apperr.CodeUnavailable, "oauth state store is not configured")
	}
	providerID := strings.TrimSpace(in.ProviderID)
	provider, ok := s.providers.OAuthProvider(providerID)
	if !ok {
		return "", "", apperr.New(apperr.CodeInvalidArgument, "identity provider not found or not oauth-based")
	}
	state, err := s.oauthStateStore.Issue(ctx, providerID, oauthStateBinding(in.ClientIP, in.UserAgent), oauthstate.DefaultTTL)
	if err != nil {
		return "", "", apperr.Wrap(err, apperr.CodeInternal, "generate oauth state failed")
	}
	url, err := provider.AuthorizationURL(state)
	if err != nil {
		return "", "", apperr.Wrap(err, apperr.CodeInternal, "build authorization url failed")
	}
	return url, state, nil
}

// LoginOAuthCallback 处理 OAuth2/OIDC 回调；state 校验通过后先绑定外部身份，再签发平台 token。
func (s *AuthService) LoginOAuthCallback(ctx context.Context, in OAuthCallbackInput) (*TokenPair, error) {
	if s == nil || s.jwt == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "authentication service is not configured")
	}
	if s.providers == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "external identity providers are not configured")
	}
	providerID := strings.TrimSpace(in.ProviderID)
	code := strings.TrimSpace(in.Code)
	state := strings.TrimSpace(in.State)
	if providerID == "" || code == "" || state == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "provider_id, code and state are required")
	}
	if s.oauthStateStore == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "oauth state store is not configured")
	}
	provider, ok := s.providers.OAuthProvider(providerID)
	if !ok {
		return nil, apperr.New(apperr.CodeInvalidArgument, "identity provider not found or not oauth-based")
	}
	if err := s.oauthStateStore.Consume(ctx, state, providerID, oauthStateBinding(in.ClientIP, in.UserAgent)); err != nil {
		logger.From(ctx).Warn("oauth state validation failed", zap.String("provider_id", providerID), zap.Error(err))
		return nil, apperr.New(apperr.CodeUnauthenticated, "oauth authentication failed")
	}
	extUser, err := provider.ExchangeCode(ctx, code)
	if err != nil {
		logger.From(ctx).Warn("oauth callback failed", zap.String("provider_id", providerID), zap.Error(err))
		return nil, apperr.New(apperr.CodeUnauthenticated, "oauth authentication failed")
	}
	u, err := s.provisionExternalUser(ctx, extUser, identityprovider.ProvisioningForOAuthProvider(provider))
	if err != nil {
		return nil, err
	}
	if !u.IsActive() {
		return nil, apperr.New(apperr.CodeUnauthenticated, "user disabled or not allowed")
	}
	return s.issueTokenPair(ctx, u)
}

func (s *AuthService) provisionExternalUser(
	ctx context.Context,
	ext *domain.AuthenticatedExternalUser,
	opts identityprovider.ProvisioningOptions,
) (*domain.User, error) {
	if s == nil || s.users == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "authentication service is not configured")
	}
	if ext == nil || strings.TrimSpace(ext.ExternalSubject) == "" {
		return nil, apperr.New(apperr.CodeInternal, "external identity is invalid")
	}

	now := time.Now()
	if s.externalIDs != nil {
		binding, err := s.externalIDs.FindByProviderSubject(ctx, ext.ProviderID, ext.ExternalSubject)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "load external identity failed")
		}
		if binding != nil {
			u, err := s.users.FindByID(ctx, binding.UserID)
			if err != nil {
				return nil, apperr.Wrap(err, apperr.CodeInternal, "load user failed")
			}
			if u == nil {
				return nil, apperr.New(apperr.CodeInternal, "external identity references missing user")
			}
			s.syncUserProfile(ctx, u, ext)
			binding.ExternalUsername = ext.Username
			binding.ExternalEmail = ext.Email
			binding.ExternalGroups = ext.Groups
			binding.LastLoginAt = &now
			if err := s.externalIDs.Update(ctx, binding); err != nil {
				return nil, apperr.Wrap(err, apperr.CodeInternal, "update external identity failed")
			}
			if err := s.syncMappedRoles(ctx, u.ID, ext.Groups, opts); err != nil {
				return nil, err
			}
			return u, nil
		}
	}

	// 未预置绑定的外部身份一律拒绝登录，避免按用户名自动关联已有本地账号造成接管风险。
	logger.From(ctx).Warn("external login rejected: identity not provisioned",
		zap.String("provider_id", ext.ProviderID),
		zap.String("external_subject", ext.ExternalSubject),
	)
	return nil, apperr.New(apperr.CodeUnauthenticated, "invalid username or password")
}

func (s *AuthService) syncUserProfile(ctx context.Context, u *domain.User, ext *domain.AuthenticatedExternalUser) {
	if u == nil || ext == nil {
		return
	}
	changed := false
	if v := strings.TrimSpace(ext.DisplayName); v != "" && u.DisplayName != v {
		u.DisplayName = v
		changed = true
	}
	if v := strings.TrimSpace(ext.Email); v != "" && u.Email != v {
		u.Email = v
		changed = true
	}
	if !changed {
		return
	}
	if err := s.users.Update(ctx, u); err != nil {
		logger.From(ctx).Warn("sync external user profile failed",
			zap.String("user_id", u.ID),
			zap.Error(err),
		)
	}
}

func (s *AuthService) syncMappedRoles(
	ctx context.Context,
	userID string,
	groups []string,
	opts identityprovider.ProvisioningOptions,
) error {
	if s == nil || s.ac == nil {
		return nil
	}
	roleCodes := mapRoleCodesFromGroups(groups, opts.GroupRoleMap)
	if len(roleCodes) == 0 && opts.DefaultRoleCode != "" {
		roleCodes = []string{opts.DefaultRoleCode}
	}

	desiredRoleIDs := make(map[string]struct{}, len(roleCodes))
	for _, code := range roleCodes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		role, err := s.ac.FindRoleByCode(ctx, code)
		if err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "find mapped role failed")
		}
		if role == nil {
			logger.From(ctx).Warn("mapped role not found", zap.String("role_code", code))
			continue
		}
		desiredRoleIDs[role.ID] = struct{}{}
	}

	bindings, err := s.ac.ListUserRoleBindings(ctx, userID)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "list user role bindings failed")
	}
	for _, binding := range bindings {
		if binding.Source != domain.UserRoleSourceExternalGroup {
			continue
		}
		if _, keep := desiredRoleIDs[binding.RoleID]; keep {
			continue
		}
		if err := s.ac.UnbindUserRole(ctx, userID, binding.RoleID); err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "unbind stale mapped role failed")
		}
	}

	for roleID := range desiredRoleIDs {
		if err := s.ac.BindUserRole(ctx, userID, roleID, domain.UserRoleSourceExternalGroup); err != nil {
			if errors.Is(err, domain.ErrReferenceNotFound) {
				continue
			}
			return apperr.Wrap(err, apperr.CodeInternal, "bind mapped role failed")
		}
	}
	return nil
}

func mapRoleCodesFromGroups(groups []string, mapping map[string]string) []string {
	if len(mapping) == 0 || len(groups) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if code, ok := mapping[group]; ok {
			addRoleCode(&out, seen, code)
			continue
		}
		for pattern, code := range mapping {
			if strings.EqualFold(strings.TrimSpace(pattern), group) {
				addRoleCode(&out, seen, code)
			}
		}
	}
	return out
}

func addRoleCode(out *[]string, seen map[string]struct{}, code string) {
	code = strings.TrimSpace(code)
	if code == "" {
		return
	}
	key := strings.ToLower(code)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*out = append(*out, code)
}

func oauthStateBinding(clientIP, userAgent string) oauthstate.Binding {
	return oauthstate.Binding{ClientIP: clientIP, UserAgent: userAgent}
}
