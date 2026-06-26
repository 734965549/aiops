package domain

import "time"

// ProviderType 外部接入 Provider 类型，见 ops/cloud-observability-contract.md §4.1。
type ProviderType string

const (
	ProviderHuaweiCloud ProviderType = "huawei_cloud"
	ProviderSigNoz      ProviderType = "signoz"
	ProviderPrometheus  ProviderType = "prometheus"
)

func (p ProviderType) IsValid() bool {
	switch p {
	case ProviderHuaweiCloud, ProviderSigNoz, ProviderPrometheus:
		return true
	default:
		return false
	}
}

// AuthType 凭据认证方式。
type AuthType string

const (
	AuthAKSK     AuthType = "ak_sk"
	AuthAgency   AuthType = "agency"
	AuthAPIToken AuthType = "api_token"
	AuthNone     AuthType = "none"
)

func (a AuthType) IsValid() bool {
	switch a {
	case AuthAKSK, AuthAgency, AuthAPIToken, AuthNone:
		return true
	default:
		return false
	}
}

// SupportedAuthTypes 返回 provider 支持的 auth_type 列表（与 infrastructure/provider checker 一致）。
func SupportedAuthTypes(provider ProviderType) []AuthType {
	switch provider {
	case ProviderHuaweiCloud:
		return []AuthType{AuthNone, AuthAKSK, AuthAgency}
	case ProviderSigNoz:
		return []AuthType{AuthNone, AuthAPIToken}
	case ProviderPrometheus:
		return []AuthType{AuthNone, AuthAPIToken, AuthAKSK}
	default:
		return nil
	}
}

// SupportsAuthType 判断 provider 是否支持给定 auth_type。
func (p ProviderType) SupportsAuthType(auth AuthType) bool {
	for _, supported := range SupportedAuthTypes(p) {
		if supported == auth {
			return true
		}
	}
	return false
}

// IntegrationAccount 云账号或可观测平台账号配置。
//
// CredentialRefID 仅保存凭据引用 ID，明文凭据不落库到本表。
// ExtraConfig 保存 provider 专属扩展配置（JSON 原始字节，由各 provider 自行解析），
// 例如华为云 sync_mode/resource_group_name/max_resources；禁止存放任何密钥或凭据。
type IntegrationAccount struct {
	AccountID       string
	Name            string
	Provider        ProviderType
	AuthType        AuthType
	Regions         []string
	ProjectID       string
	CredentialRefID string
	Enabled         bool
	Deleted         bool
	OwnerTeam       string
	Description     string
	ExtraConfig     []byte
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
