package domain

// ProviderType 描述企业身份源类型。
type ProviderType string

const (
	ProviderTypeLocal  ProviderType = "local"
	ProviderTypeLDAP   ProviderType = "ldap"
	ProviderTypeAD     ProviderType = "ad"
	ProviderTypeOAuth2 ProviderType = "oauth2"
	ProviderTypeOIDC   ProviderType = "oidc"
	ProviderTypeSSO    ProviderType = "sso"
)

// IsPasswordBased 判断该类型是否走用户名密码认证（LDAP/AD）。
func (t ProviderType) IsPasswordBased() bool {
	return t == ProviderTypeLDAP || t == ProviderTypeAD
}

// IsOAuthBased 判断该类型是否走 OAuth2/OIDC 授权码流程。
func (t ProviderType) IsOAuthBased() bool {
	return t == ProviderTypeOAuth2 || t == ProviderTypeOIDC || t == ProviderTypeSSO
}

// ProviderInfo 是对外暴露的身份源摘要（不含敏感配置）。
type ProviderInfo struct {
	ID       string       `json:"id"`
	Type     ProviderType `json:"type"`
	Name     string       `json:"name"`
	Enabled  bool         `json:"enabled"`
	Priority int          `json:"priority"`
}
