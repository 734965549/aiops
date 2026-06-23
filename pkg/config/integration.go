package config

import (
	"fmt"
	"strings"
)

const (
	// DefaultCredentialEncryptionKeyPlaceholder dev 环境占位凭据加密密钥，与 JWT 密钥独立。
	DefaultCredentialEncryptionKeyPlaceholder = "dev-integration-credential-key-change-me"
)

// IntegrationConfig 描述 Integration 限界上下文运行参数。
type IntegrationConfig struct {
	// CredentialEncryptionKey 凭据 AES-GCM 加密密钥，与 auth.jwt_secret 独立配置与轮换。
	CredentialEncryptionKey string `mapstructure:"credential_encryption_key"`
	// CredentialEncryptionKeyVersion 当前密钥版本号（1–255），写入密文首字节便于后续轮换。
	CredentialEncryptionKeyVersion int `mapstructure:"credential_encryption_key_version"`
}

// ValidateCredentialEncryptionKey 校验凭据加密密钥；dev 允许占位值，非 dev 要求强密钥。
func ValidateCredentialEncryptionKey(key, env string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("integration.credential_encryption_key must not be empty")
	}
	if env == "dev" {
		return nil
	}
	if isWeakCredentialEncryptionKey(key) {
		return fmt.Errorf("integration.credential_encryption_key is weak or uses a dev placeholder")
	}
	if len(key) < minJWTSecretLenNonDev {
		return fmt.Errorf("integration.credential_encryption_key must be at least %d characters in non-dev", minJWTSecretLenNonDev)
	}
	if uniqueRunes(key) < minJWTSecretUnique {
		return fmt.Errorf("integration.credential_encryption_key lacks character diversity")
	}
	if maxRepeatRun(key) > maxJWTSecretRepeatRun {
		return fmt.Errorf("integration.credential_encryption_key has excessive repeated characters")
	}
	entropy := shannonEntropy(key)
	classes := countCharClasses(key)
	if classes < 3 && entropy < minJWTSecretEntropy+0.7 {
		return fmt.Errorf("integration.credential_encryption_key entropy is too low")
	}
	if entropy < minJWTSecretEntropy {
		return fmt.Errorf("integration.credential_encryption_key entropy is too low")
	}
	return nil
}

func isWeakCredentialEncryptionKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, weak := range []string{
		DefaultCredentialEncryptionKeyPlaceholder,
		DefaultJWTSecretPlaceholder,
		DevJWTSecretPlaceholder,
	} {
		if normalized == strings.ToLower(weak) {
			return true
		}
	}
	return false
}

func validateIntegrationConfig(cfg IntegrationConfig, env, jwtSecret string) error {
	if err := ValidateCredentialEncryptionKey(cfg.CredentialEncryptionKey, env); err != nil {
		return err
	}
	if cfg.CredentialEncryptionKeyVersion <= 0 || cfg.CredentialEncryptionKeyVersion > 255 {
		return fmt.Errorf("integration.credential_encryption_key_version must be between 1 and 255")
	}
	if env != "dev" && strings.TrimSpace(cfg.CredentialEncryptionKey) == strings.TrimSpace(jwtSecret) {
		return fmt.Errorf("integration.credential_encryption_key must differ from auth.jwt_secret")
	}
	return nil
}
