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
}

// ProviderContext 是 Provider Adapter 调用上下文，后续真实 adapter 都通过这里获取账号摘要。
type ProviderContext struct {
	Account AccountSnapshot
}
