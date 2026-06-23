package domain

import "time"

// CredentialStoreType 凭据存储方式。
type CredentialStoreType string

const (
	StoreLocalEncrypted CredentialStoreType = "local_encrypted"
	StoreExternalRef    CredentialStoreType = "external_ref"
)

// CredentialRef 凭据引用，仅存密文或外部 Secret 引用，不暴露明文。
//
// Ciphertext 为 AES-GCM 加密后的 JSON；ExternalRef 预留外部密钥管理引用。
type CredentialRef struct {
	CredentialRefID string
	AccountID       string
	StoreType       CredentialStoreType
	Ciphertext      []byte
	ExternalRef     string
	Fingerprint     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CredentialMaterial 解密后的凭据字段，仅在 application/infrastructure 内部流转。
type CredentialMaterial map[string]string
