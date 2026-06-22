package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/734965549/aiops/internal/integration/domain"
)

// Vault 使用 AES-GCM 加密凭据 JSON，密文首字节为密钥版本号便于轮换。
type Vault struct {
	key     []byte
	version byte
}

func NewVault(encryptKey string, keyVersion int) (*Vault, error) {
	key := deriveKey(encryptKey)
	if len(key) != 32 {
		return nil, errors.New("invalid encryption key length")
	}
	if keyVersion <= 0 || keyVersion > 255 {
		return nil, errors.New("invalid key version")
	}
	return &Vault{key: key, version: byte(keyVersion)}, nil
}

func deriveKey(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func (v *Vault) Encrypt(material domain.CredentialMaterial) ([]byte, string, error) {
	if v == nil {
		return nil, "", errors.New("credential vault is not configured")
	}
	if len(material) == 0 {
		return nil, "", domain.ErrCredentialRequired
	}
	plain, err := json.Marshal(material)
	if err != nil {
		return nil, "", err
	}
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, "", err
	}
	payload := gcm.Seal(nonce, nonce, plain, nil)
	out := append([]byte{v.version}, payload...)
	return out, v.Fingerprint(material), nil
}

func (v *Vault) Decrypt(ciphertext []byte) (domain.CredentialMaterial, error) {
	if v == nil {
		return nil, errors.New("credential vault is not configured")
	}
	if len(ciphertext) == 0 {
		return nil, domain.ErrCredentialRequired
	}
	version := ciphertext[0]
	if version != v.version {
		return nil, fmt.Errorf("unsupported credential key version %d", version)
	}
	return v.decryptPayload(ciphertext[1:])
}

func (v *Vault) decryptPayload(payload []byte) (domain.CredentialMaterial, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(payload) < nonceSize {
		return nil, errors.New("invalid ciphertext")
	}
	plain, err := gcm.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return nil, err
	}
	out := domain.CredentialMaterial{}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (v *Vault) Fingerprint(material domain.CredentialMaterial) string {
	raw, _ := json.Marshal(material)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}
