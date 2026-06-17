package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenType 区分 access / refresh，便于在 Authenticator 中拒绝错用。
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// Claims 是平台 JWT payload。
//
//   - Subject 存业务用户 UUID（user_id），与领域模型 User.ID 一致；
//   - Roles 仅供前端展示与简单网关判断，敏感权限校验仍需服务端再查；
//   - Type 区分 access / refresh，防止 refresh token 被当成 access 使用。
type Claims struct {
	jwt.RegisteredClaims
	Username string    `json:"uname,omitempty"`
	Roles    []string  `json:"roles,omitempty"`
	Type     TokenType `json:"typ,omitempty"`
}

// IssueOptions 是签发 token 所需的入参。
type IssueOptions struct {
	UserID   string
	Username string
	Roles    []string
}

// JWTManager 同时承担签发与校验。
//
// 一个 JWTManager 实例对应一份 (secret, issuer, ttl) 配置，
// 平台目前只用 HS256 对称密钥；如需轮换密钥，应在 manager 外部包一层并保留 kid。
type JWTManager struct {
	secret     []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
	signMethod jwt.SigningMethod
	clockSkew  time.Duration
}

// Options 控制 JWTManager 行为。
type Options struct {
	Secret     string
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	// ClockSkew 容忍签发/校验机器之间的时间偏差，默认 30s。
	ClockSkew time.Duration
}

// NewJWTManager 构造 JWTManager；secret 必须非空，否则返回错误。
func NewJWTManager(opt Options) (*JWTManager, error) {
	if opt.Secret == "" {
		return nil, fmt.Errorf("jwt secret must not be empty")
	}
	if opt.AccessTTL <= 0 {
		opt.AccessTTL = 2 * time.Hour
	}
	if opt.RefreshTTL <= 0 {
		opt.RefreshTTL = 7 * 24 * time.Hour
	}
	if opt.ClockSkew <= 0 {
		opt.ClockSkew = 30 * time.Second
	}
	return &JWTManager{
		secret:     []byte(opt.Secret),
		issuer:     opt.Issuer,
		accessTTL:  opt.AccessTTL,
		refreshTTL: opt.RefreshTTL,
		signMethod: jwt.SigningMethodHS256,
		clockSkew:  opt.ClockSkew,
	}, nil
}

// IssueAccess 签发 access token，回传 token 字符串与过期时间（便于前端控制刷新）。
func (m *JWTManager) IssueAccess(opt IssueOptions) (string, time.Time, error) {
	token, exp, err := m.issue(opt, TokenTypeAccess, m.accessTTL, "")
	return token, exp, err
}

// IssueRefresh 签发 refresh token，并返回 jti 供会话存储使用。
func (m *JWTManager) IssueRefresh(opt IssueOptions) (token string, exp time.Time, jti string, err error) {
	jti = NewRefreshTokenJTI()
	token, exp, err = m.issue(opt, TokenTypeRefresh, m.refreshTTL, jti)
	return token, exp, jti, err
}

func (m *JWTManager) issue(opt IssueOptions, typ TokenType, ttl time.Duration, jti string) (string, time.Time, error) {
	if opt.UserID == "" {
		return "", time.Time{}, fmt.Errorf("UserID is required")
	}
	now := time.Now()
	exp := now.Add(ttl)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   opt.UserID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-m.clockSkew)),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		Username: opt.Username,
		Roles:    opt.Roles,
		Type:     typ,
	}
	token := jwt.NewWithClaims(m.signMethod, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign jwt: %w", err)
	}
	return signed, exp, nil
}

// ErrInvalidToken 是统一的 token 校验失败错误（不区分签名错 / 过期 / 类型错），
// 上层应将其映射为 UNAUTHENTICATED，避免向调用方泄漏细节。
var ErrInvalidToken = errors.New("invalid token")

// RefreshTTL 返回 refresh token 过期时长。
func (m *JWTManager) RefreshTTL() time.Duration {
	if m == nil {
		return 7 * 24 * time.Hour
	}
	return m.refreshTTL
}

// Verify 解析并校验 token；expectType 不为空时会同时校验 typ。
func (m *JWTManager) Verify(tokenStr string, expectType TokenType) (*Claims, error) {
	if tokenStr == "" {
		return nil, ErrInvalidToken
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{m.signMethod.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithLeeway(m.clockSkew),
		jwt.WithExpirationRequired(),
	)
	tok, err := parser.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		return m.secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, ErrInvalidToken
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}
	if expectType != "" && claims.Type != expectType {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
