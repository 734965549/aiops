// Package credential 提供 AI provider 密钥的加解密适配。
// 具体加密实现由装配层注入（复用 Integration credential vault），避免跨上下文直接依赖 infrastructure。
package credential

import "errors"

// EncryptFunc / DecryptFunc 由装配层提供，material 仅承载 provider api_key。
type EncryptFunc func(material map[string]string) ([]byte, error)
type DecryptFunc func(ciphertext []byte) (map[string]string, error)

// SecretCipher 将单值 api_key 包装为可注入的加解密端口。
type SecretCipher struct {
	encrypt EncryptFunc
	decrypt DecryptFunc
}

func NewSecretCipher(encrypt EncryptFunc, decrypt DecryptFunc) *SecretCipher {
	return &SecretCipher{encrypt: encrypt, decrypt: decrypt}
}

func (c *SecretCipher) Encrypt(plaintext string) ([]byte, error) {
	if c == nil || c.encrypt == nil {
		return nil, errors.New("ai provider secret cipher is not configured")
	}
	return c.encrypt(map[string]string{"api_key": plaintext})
}

func (c *SecretCipher) Decrypt(ciphertext []byte) (string, error) {
	if c == nil || c.decrypt == nil {
		return "", errors.New("ai provider secret cipher is not configured")
	}
	material, err := c.decrypt(ciphertext)
	if err != nil {
		return "", err
	}
	return material["api_key"], nil
}
