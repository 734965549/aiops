package identityprovider

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/config"
)

// BuildRegistryFromConfig 从平台配置装配企业身份源注册表。
func BuildRegistryFromConfig(env string, providers []config.IdentityProviderConfig) (*Registry, error) {
	reg := NewRegistry()
	for _, p := range providers {
		if strings.TrimSpace(p.ID) == "" || !p.Enabled {
			continue
		}
		info := domain.ProviderInfo{
			ID:       strings.TrimSpace(p.ID),
			Type:     domain.ProviderType(strings.TrimSpace(p.Type)),
			Name:     strings.TrimSpace(p.Name),
			Enabled:  p.Enabled,
			Priority: p.Priority,
		}
		if info.Name == "" {
			info.Name = info.ID
		}
		switch info.Type {
		case domain.ProviderTypeLDAP, domain.ProviderTypeAD:
			if err := validateLDAPTransport(env, p.LDAP); err != nil {
				return nil, err
			}
			ldapProvider, err := NewLDAPProvider(info, p.LDAP)
			if err != nil {
				return nil, err
			}
			reg.RegisterPassword(ldapProvider)
		case domain.ProviderTypeOAuth2, domain.ProviderTypeSSO:
			oauthProvider, err := NewOAuth2Provider(info, p.OAuth2)
			if err != nil {
				return nil, err
			}
			reg.RegisterOAuth(oauthProvider)
		case domain.ProviderTypeOIDC:
			oidcProvider, err := NewOIDCProvider(info, p.OIDC)
			if err != nil {
				return nil, err
			}
			reg.RegisterOAuth(oidcProvider)
		default:
			return nil, fmt.Errorf("identity provider %q: unsupported type %q", info.ID, info.Type)
		}
	}
	return reg, nil
}

func validateLDAPTransport(env string, cfg config.LDAPProviderConfig) error {
	if env != "prod" {
		return nil
	}
	if cfg.InsecureSkipVerify {
		return fmt.Errorf("prod ldap provider must not set insecure_skip_verify")
	}
	u, err := url.Parse(strings.TrimSpace(cfg.ServerURL))
	if err != nil {
		return fmt.Errorf("ldap server_url invalid: %w", err)
	}
	if strings.EqualFold(u.Scheme, "ldaps") || cfg.StartTLS {
		return nil
	}
	return fmt.Errorf("prod ldap provider must use ldaps:// or start_tls")
}

// ProvisioningOptions 描述外部身份源的账号同步策略。
type ProvisioningOptions struct {
	AutoCreateUser  bool
	DefaultRoleCode string
	GroupRoleMap    map[string]string
}

// ProvisioningForPasswordProvider 读取 LDAP/AD 同步策略。
func ProvisioningForPasswordProvider(p PasswordAuthenticator) ProvisioningOptions {
	if p == nil {
		return ProvisioningOptions{}
	}
	if extra, ok := p.(interface{ ProvisioningOptions() ProvisioningOptions }); ok {
		return extra.ProvisioningOptions()
	}
	if ldapProvider, ok := p.(*LDAPProvider); ok {
		return ProvisioningOptions{
			AutoCreateUser:  ldapProvider.AutoCreateUser(),
			DefaultRoleCode: ldapProvider.DefaultRoleCode(),
			GroupRoleMap:    ldapProvider.GroupRoleMapping(),
		}
	}
	return ProvisioningOptions{}
}

// ProvisioningForOAuthProvider 读取 OAuth/OIDC 同步策略。
func ProvisioningForOAuthProvider(p OAuthAuthenticator) ProvisioningOptions {
	if p == nil {
		return ProvisioningOptions{}
	}
	switch v := p.(type) {
	case *OAuth2Provider:
		ex := v.Extras()
		return ProvisioningOptions{
			AutoCreateUser:  ex.AutoCreateUser,
			DefaultRoleCode: ex.DefaultRoleCode,
			GroupRoleMap:    ex.GroupRoleMap,
		}
	case *OIDCProvider:
		ex := v.Extras()
		return ProvisioningOptions{
			AutoCreateUser:  ex.AutoCreateUser,
			DefaultRoleCode: ex.DefaultRoleCode,
			GroupRoleMap:    ex.GroupRoleMap,
		}
	default:
		return ProvisioningOptions{}
	}
}
