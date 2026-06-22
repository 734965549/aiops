package domain

// AccountSnapshot Provider 调用所需的脱敏账号摘要，不承载明文凭据。
type AccountSnapshot struct {
	AccountID    string
	Provider     string
	AuthType     string
	Regions      []string
	ProjectID    string
	OwnerTeam    string
	Capabilities []string
}

// ProviderContext Provider Adapter 调用上下文。
type ProviderContext struct {
	Account AccountSnapshot
}
