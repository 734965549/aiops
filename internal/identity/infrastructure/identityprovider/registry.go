package identityprovider

import (
	"context"

	"github.com/734965549/aiops/internal/identity/domain"
)

// PasswordAuthenticator 支持用户名密码认证的身份源（LDAP / AD）。
type PasswordAuthenticator interface {
	Info() domain.ProviderInfo
	Authenticate(ctx context.Context, username, password string) (*domain.AuthenticatedExternalUser, error)
}

// OAuthAuthenticator 支持 OAuth2/OIDC 授权码流程的身份源。
type OAuthAuthenticator interface {
	Info() domain.ProviderInfo
	AuthorizationURL(state string) (string, error)
	ExchangeCode(ctx context.Context, code string) (*domain.AuthenticatedExternalUser, error)
}

// Registry 聚合已启用的企业身份源。
type Registry struct {
	password map[string]PasswordAuthenticator
	oauth    map[string]OAuthAuthenticator
	ordered  []domain.ProviderInfo
}

// NewRegistry 构造空注册表。
func NewRegistry() *Registry {
	return &Registry{
		password: make(map[string]PasswordAuthenticator),
		oauth:    make(map[string]OAuthAuthenticator),
	}
}

// RegisterPassword 注册 LDAP/AD 等密码类身份源。
func (r *Registry) RegisterPassword(p PasswordAuthenticator) {
	if r == nil || p == nil {
		return
	}
	info := p.Info()
	if info.ID == "" || !info.Enabled {
		return
	}
	r.password[info.ID] = p
	r.rebuildOrdered()
}

// RegisterOAuth 注册 OAuth2/OIDC/SSO 身份源。
func (r *Registry) RegisterOAuth(p OAuthAuthenticator) {
	if r == nil || p == nil {
		return
	}
	info := p.Info()
	if info.ID == "" || !info.Enabled {
		return
	}
	r.oauth[info.ID] = p
	r.rebuildOrdered()
}

// ListProviders 返回按 priority 排序的对外身份源摘要。
func (r *Registry) ListProviders() []domain.ProviderInfo {
	if r == nil {
		return nil
	}
	out := make([]domain.ProviderInfo, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// PasswordProvider 按 ID 获取密码类身份源。
func (r *Registry) PasswordProvider(id string) (PasswordAuthenticator, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.password[id]
	return p, ok
}

// OAuthProvider 按 ID 获取 OAuth 类身份源。
func (r *Registry) OAuthProvider(id string) (OAuthAuthenticator, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.oauth[id]
	return p, ok
}

// HasProvider 判断身份源是否已注册且启用。
func (r *Registry) HasProvider(id string) bool {
	if r == nil || id == "" {
		return false
	}
	if p, ok := r.password[id]; ok && p.Info().Enabled {
		return true
	}
	if p, ok := r.oauth[id]; ok && p.Info().Enabled {
		return true
	}
	return false
}

// LDAPProvider 按 ID 获取 LDAP/AD 身份源（用于管理员目录浏览与导入）。
func (r *Registry) LDAPProvider(id string) (*LDAPProvider, bool) {
	if r == nil || id == "" {
		return nil, false
	}
	p, ok := r.password[id]
	if !ok || !p.Info().Enabled {
		return nil, false
	}
	ldapProvider, ok := p.(*LDAPProvider)
	return ldapProvider, ok
}

func (r *Registry) rebuildOrdered() {
	seen := make(map[string]struct{})
	r.ordered = nil
	add := func(info domain.ProviderInfo) {
		if _, ok := seen[info.ID]; ok {
			return
		}
		seen[info.ID] = struct{}{}
		r.ordered = append(r.ordered, info)
	}
	for _, p := range r.password {
		add(p.Info())
	}
	for _, p := range r.oauth {
		add(p.Info())
	}
	for i := 0; i < len(r.ordered); i++ {
		for j := i + 1; j < len(r.ordered); j++ {
			a, b := r.ordered[i], r.ordered[j]
			if a.Priority > b.Priority || (a.Priority == b.Priority && a.ID > b.ID) {
				r.ordered[i], r.ordered[j] = r.ordered[j], r.ordered[i]
			}
		}
	}
}
