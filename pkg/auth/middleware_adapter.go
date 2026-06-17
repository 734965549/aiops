package auth

import (
	"strings"

	"github.com/734965549/aiops/pkg/transport/http/middleware"
)

// JWTAuthenticator 把 *JWTManager 适配为 middleware.Authenticator。
//
// 之所以单独放一层适配器，是为了让 pkg/transport/http/middleware 保持「不感知」
// 具体鉴权方案，未来切换到 OIDC / SSO 时也只需在此处提供另一个 Authenticator 实现。
type JWTAuthenticator struct {
	jwt *JWTManager
}

// NewJWTAuthenticator 构造 JWT 鉴权适配器。
func NewJWTAuthenticator(jwt *JWTManager) *JWTAuthenticator {
	return &JWTAuthenticator{jwt: jwt}
}

// Authenticate 实现 middleware.Authenticator。
// 仅校验 access token；refresh token 走单独的 /api/identity/refresh 端点。
func (a *JWTAuthenticator) Authenticate(token string) (middleware.Identity, error) {
	if a == nil || a.jwt == nil {
		return middleware.Identity{}, ErrInvalidToken
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 8192 {
		return middleware.Identity{}, ErrInvalidToken
	}
	claims, err := a.jwt.Verify(token, TokenTypeAccess)
	if err != nil {
		return middleware.Identity{}, err
	}
	return middleware.Identity{
		UserID:   claims.Subject,
		Username: claims.Username,
		Roles:    claims.Roles,
	}, nil
}
