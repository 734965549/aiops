package domain

// AccountSnapshot 是 Provider 调用所需的脱敏账号摘要，不承载明文凭据。
type AccountSnapshot struct {
	AccountID       string
	Provider        string
	AuthType        string
	Regions         []string
	ProjectID       string
	CredentialRefID string
	OwnerTeam       string
	Capabilities    []string
	// ExtraConfig 透传 provider 专属扩展配置原始 JSON，由各 provider 自行解析；禁止存放密钥。
	ExtraConfig []byte
}

// ProviderContext 是 Provider Adapter 调用上下文，后续真实 adapter 都通过这里获取账号摘要。
type ProviderContext struct {
	Account AccountSnapshot
}
