package identityprovider

import (
	"fmt"
	"strings"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/config"
)

// BuildLDAPProviderFromConnection 根据管理员填写的连接参数构造临时 LDAP/AD 客户端。
func BuildLDAPProviderFromConnection(env, providerID, providerType string, cfg config.LDAPProviderConfig) (*LDAPProvider, error) {
	providerID = strings.TrimSpace(providerID)
	providerType = strings.TrimSpace(providerType)
	if providerID == "" {
		return nil, fmt.Errorf("provider_id is required")
	}
	if providerType == "" {
		providerType = string(domain.ProviderTypeLDAP)
	}
	info := domain.ProviderInfo{
		ID:      providerID,
		Type:    domain.ProviderType(providerType),
		Name:    providerID,
		Enabled: true,
	}
	if err := validateLDAPTransport(env, cfg); err != nil {
		return nil, err
	}
	return NewLDAPProvider(info, cfg)
}
