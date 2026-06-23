package domain

import "context"

// AccountFilter 账号列表筛选。
type AccountFilter struct {
	Provider string
	Enabled  *bool
	Keyword  string
	Limit    int
	Offset   int
}

// TransactionRepositories 同一数据库事务内可用的仓储集合。
type TransactionRepositories struct {
	Accounts     AccountRepository
	Credentials  CredentialRepository
	Capabilities CapabilityRepository
	Checks       CheckResultRepository
}

// UnitOfWork 保证账号、凭据引用与能力声明的原子提交。
type UnitOfWork interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context, repos TransactionRepositories) error) error
}

// AccountRepository 接入账号持久化。
type AccountRepository interface {
	Create(ctx context.Context, account *IntegrationAccount) error
	Update(ctx context.Context, account *IntegrationAccount) error
	GetByID(ctx context.Context, accountID string) (*IntegrationAccount, error)
	List(ctx context.Context, filter AccountFilter) ([]IntegrationAccount, error)
	Count(ctx context.Context, filter AccountFilter) (int64, error)
	SoftDelete(ctx context.Context, accountID string) error
}

// CredentialRepository 凭据引用持久化。
type CredentialRepository interface {
	Create(ctx context.Context, ref *CredentialRef) error
	Update(ctx context.Context, ref *CredentialRef) error
	GetByAccountID(ctx context.Context, accountID string) (*CredentialRef, error)
	DeleteByAccountID(ctx context.Context, accountID string) error
}

// CapabilityRepository Provider 能力声明持久化。
type CapabilityRepository interface {
	ReplaceForAccount(ctx context.Context, accountID string, caps []Capability) error
	ListByAccountID(ctx context.Context, accountID string) ([]Capability, error)
}

// CheckResultRepository 连通性检查历史。
type CheckResultRepository interface {
	Create(ctx context.Context, check *ConnectivityCheck) error
	LatestByAccountID(ctx context.Context, accountID string) (*ConnectivityCheck, error)
}

// CredentialVault 凭据加解密端口。
type CredentialVault interface {
	Encrypt(material CredentialMaterial) ([]byte, string, error)
	Decrypt(ciphertext []byte) (CredentialMaterial, error)
	Fingerprint(material CredentialMaterial) string
}

// ProviderChecker Provider 连通性探测端口。
type ProviderChecker interface {
	Provider() ProviderType
	CheckConnectivity(ctx context.Context, account IntegrationAccount, material CredentialMaterial) (*ConnectivityCheck, error)
}
