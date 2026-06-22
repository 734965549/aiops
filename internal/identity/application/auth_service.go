package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/internal/identity/infrastructure/identityprovider"
	"github.com/734965549/aiops/internal/identity/infrastructure/ldapsession"
	"github.com/734965549/aiops/internal/identity/infrastructure/oauthstate"
	"github.com/734965549/aiops/pkg/auth"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/google/uuid"
)

// LoginInput 是 /api/identity/login 的参数。
type LoginInput struct {
	Username string
	Password string
}

// TokenPair 是登录与刷新接口的统一出参。
//
// ExpiresAt 用 Unix 秒返回，避免前后端时区差异。
type TokenPair struct {
	AccessToken     string          `json:"access_token"`
	RefreshToken    string          `json:"refresh_token"`
	AccessExpiresAt int64           `json:"access_expires_at"`
	RefreshExpires  int64           `json:"refresh_expires_at"`
	TokenType       string          `json:"token_type"`
	User            *CurrentUserDTO `json:"user"`
}

// AuthService 聚合登录 / 刷新令牌 / 注册等用例。
type AuthService struct {
	users           domain.UserRepository
	externalIDs     domain.ExternalIdentityRepository
	ac              domain.AccessControlRepository
	jwt             *auth.JWTManager
	refreshStore    auth.RefreshTokenStore
	providers       *identityprovider.Registry
	ldapBrowseStore ldapsession.Store
	oauthStateStore oauthstate.Store
	appEnv          string
}

// NewAuthService 构造 AuthService。refreshStore 为 nil 时使用 NoopRefreshTokenStore（不强制轮换）。
// ac 可选；提供时登录/刷新会在 JWT 中写入用户角色编码列表。
// externalIDs / providers 可选；提供时启用企业身份源登录。
// ldapBrowseStore 可选；提供时启用管理员临时 LDAP 浏览会话。
// oauthStateStore 可选；未提供时使用进程内存储（单实例开发可用，生产应接 Redis）。
func NewAuthService(
	users domain.UserRepository,
	jwt *auth.JWTManager,
	refreshStore auth.RefreshTokenStore,
	ac domain.AccessControlRepository,
	externalIDs domain.ExternalIdentityRepository,
	providers *identityprovider.Registry,
	ldapBrowseStore ldapsession.Store,
	oauthStateStore oauthstate.Store,
	appEnv string,
) *AuthService {
	if refreshStore == nil {
		refreshStore = auth.NoopRefreshTokenStore{}
	}
	if oauthStateStore == nil {
		oauthStateStore = oauthstate.NewMemoryStore()
	}
	return &AuthService{
		users:           users,
		externalIDs:     externalIDs,
		ac:              ac,
		jwt:             jwt,
		refreshStore:    refreshStore,
		providers:       providers,
		ldapBrowseStore: ldapBrowseStore,
		oauthStateStore: oauthStateStore,
		appEnv:          strings.TrimSpace(appEnv),
	}
}

// Login 校验用户名密码并签发一对 access/refresh token。
//
// 错误语义：用户不存在 / 密码错误 / 用户被禁用 统一返回 UNAUTHENTICATED，
// 避免给爆破攻击留下「用户存在 vs 不存在」的信息差。
func (s *AuthService) Login(ctx context.Context, in LoginInput) (*TokenPair, error) {
	if s == nil || s.users == nil || s.jwt == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "authentication service is not configured")
	}
	username := strings.TrimSpace(in.Username)
	if username == "" || in.Password == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "username and password are required")
	}
	if len(username) > 64 || len(in.Password) > 256 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "username or password is too long")
	}

	u, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "find user failed")
	}
	if u == nil {
		logger.From(ctx).Warn("login: user not found", logger.String("username", username))
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid username or password")
	}
	if !u.IsActive() {
		logger.From(ctx).Warn("login: user not active", logger.String("user_id", u.ID), logger.String("status", string(u.Status)))
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid username or password")
	}
	if err := auth.VerifyPassword(u.PasswordHash, in.Password); err != nil {
		if mapped := mapAuthError(err); mapped != err {
			return nil, mapped
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "verify password failed")
	}

	return s.issueTokenPair(ctx, u)
}

// Logout 吊销 refresh token，使会话在过期前失效。
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	if s == nil || s.jwt == nil {
		return apperr.New(apperr.CodeUnavailable, "authentication service is not configured")
	}
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return apperr.New(apperr.CodeInvalidArgument, "refresh token is required")
	}
	claims, err := s.jwt.Verify(refreshToken, auth.TokenTypeRefresh)
	if err != nil {
		return apperr.New(apperr.CodeUnauthenticated, "invalid or expired refresh token")
	}
	if s.refreshRotationEnabled() {
		if strings.TrimSpace(claims.ID) == "" {
			return apperr.New(apperr.CodeUnauthenticated, "invalid or expired refresh token")
		}
		if err := s.refreshStore.Revoke(ctx, claims.Subject, claims.ID); err != nil {
			return apperr.Wrap(err, apperr.CodeInternal, "revoke refresh token failed")
		}
	}
	return nil
}

// Refresh 用 refresh token 换一对新的 access/refresh token；启用 Redis 会话时会轮换并作废旧 token。
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	if s == nil || s.users == nil || s.jwt == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "authentication service is not configured")
	}
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "refresh token is required")
	}
	if len(refreshToken) > 8192 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "refresh token is too long")
	}
	claims, err := s.jwt.Verify(refreshToken, auth.TokenTypeRefresh)
	if err != nil {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid or expired refresh token")
	}
	u, err := s.users.FindByID(ctx, claims.Subject)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load user failed")
	}
	if u == nil || !u.IsActive() {
		return nil, apperr.New(apperr.CodeUnauthenticated, "user disabled or removed")
	}
	if s.refreshRotationEnabled() {
		if strings.TrimSpace(claims.ID) == "" {
			return nil, apperr.New(apperr.CodeUnauthenticated, "invalid or expired refresh token")
		}
		ok, err := s.refreshStore.Validate(ctx, claims.Subject, claims.ID)
		if err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "validate refresh token failed")
		}
		if !ok {
			return nil, apperr.New(apperr.CodeUnauthenticated, "invalid or expired refresh token")
		}
		if err := s.refreshStore.Revoke(ctx, claims.Subject, claims.ID); err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "revoke refresh token failed")
		}
	}
	return s.issueTokenPair(ctx, u)
}

func (s *AuthService) issueTokenPair(ctx context.Context, u *domain.User) (*TokenPair, error) {
	if u == nil || strings.TrimSpace(u.ID) == "" {
		return nil, apperr.New(apperr.CodeInternal, "user identity is invalid")
	}
	roleCodes, err := s.loadUserRoleCodes(ctx, u.ID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load user roles failed")
	}
	issueOpts := auth.IssueOptions{
		UserID:   u.ID,
		Username: u.Username,
		Roles:    roleCodes,
	}
	access, accessExp, err := s.jwt.IssueAccess(issueOpts)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "issue access token failed")
	}
	refresh, refreshExp, jti, err := s.jwt.IssueRefresh(issueOpts)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "issue refresh token failed")
	}
	if s.refreshRotationEnabled() {
		ttl := time.Until(refreshExp)
		if ttl <= 0 {
			ttl = s.jwt.RefreshTTL()
		}
		if err := s.refreshStore.Register(ctx, u.ID, jti, ttl); err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "register refresh token failed")
		}
	}
	return &TokenPair{
		AccessToken:     access,
		RefreshToken:    refresh,
		AccessExpiresAt: accessExp.Unix(),
		RefreshExpires:  refreshExp.Unix(),
		TokenType:       "Bearer",
		User: &CurrentUserDTO{
			ID:          u.ID,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			Email:       u.Email,
			Status:      string(u.Status),
		},
	}, nil
}

func (s *AuthService) refreshRotationEnabled() bool {
	if s == nil || s.refreshStore == nil {
		return false
	}
	_, noop := s.refreshStore.(auth.NoopRefreshTokenStore)
	return !noop
}

// EnsureBootstrapUser 在 dev / 全新环境下尝试创建一个默认管理员账号，
// 仅当：
//   - 入参 username 不为空；
//   - 数据库中尚不存在同名用户。
//
// 该方法是「幂等的初始化助手」，可在 bootstrap 中调用，
// 生产部署通过配置关闭它（传入空 username），改由运维 SQL 注入种子。
func (s *AuthService) EnsureBootstrapUser(ctx context.Context, username, password, displayName string) error {
	if s == nil || s.users == nil {
		return apperr.New(apperr.CodeUnavailable, "authentication service is not configured")
	}
	username = strings.TrimSpace(username)
	if username == "" && password == "" {
		return nil
	}
	if username == "" || password == "" {
		return apperr.New(apperr.CodeInvalidArgument, "bootstrap username and password are required together")
	}
	if len(username) > 64 || len(password) < 8 || len(password) > 256 {
		return apperr.New(apperr.CodeInvalidArgument, "bootstrap username/password length is invalid")
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = username
	}
	existing, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "check bootstrap user failed")
	}
	if existing != nil {
		return nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "hash bootstrap password failed")
	}
	u := &domain.User{
		ID:           uuid.NewString(),
		Username:     username,
		DisplayName:  displayName,
		PasswordHash: hash,
		Status:       domain.UserStatusActive,
	}
	if err := s.users.Create(ctx, u); err != nil {
		// 并发场景下两实例可能同时尝试创建，第二者收到唯一冲突就当成功处理。
		if errors.Is(err, domain.ErrAlreadyExists) {
			return nil
		}
		return apperr.Wrap(err, apperr.CodeInternal, "create bootstrap user failed")
	}
	logger.From(ctx).Info("bootstrap user created",
		logger.String("username", u.Username),
		logger.Time("at", time.Now()),
	)
	return nil
}

func (s *AuthService) loadUserRoleCodes(ctx context.Context, userID string) ([]string, error) {
	if s == nil || s.ac == nil {
		return nil, nil
	}
	roles, err := s.ac.ListUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		if code := strings.TrimSpace(role.Code); code != "" {
			out = append(out, code)
		}
	}
	return out, nil
}
