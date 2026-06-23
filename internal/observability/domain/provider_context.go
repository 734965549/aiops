package domain

// AccountSnapshot 系 Provider 调用所需嘅脱敏账号摘要，唔承载明文凭据。
type AccountSnapshot struct {
	AccountID       string
	Provider        string
	AuthType        string
	Regions         []string
	ProjectID       string
	CredentialRefID string
	OwnerTeam       string
	Capabilities    []string
}

// ProviderContext 系 Provider Adapter 调用上下文，后续真实 adapter 都经呢度取账号摘要。
type ProviderContext struct {
	Account AccountSnapshot
}
