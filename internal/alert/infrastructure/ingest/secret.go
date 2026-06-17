package ingest

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// HashWebhookSecret 将 §3.2 X-AIOPS-Webhook-Token 做 sha256 后 hex 落库，接口永不返回明文。
func HashWebhookSecret(secret string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(secret)))
	return hex.EncodeToString(sum[:])
}

// VerifyWebhookSecret 校验 §3.2 Webhook token；空 token 或 hash 视为无效。
// 摘要比对使用 subtle.ConstantTimeCompare，降低时序侧信道风险。
func VerifyWebhookSecret(secret, hash string) bool {
	secret = strings.TrimSpace(secret)
	hash = strings.TrimSpace(hash)
	if secret == "" || hash == "" {
		return false
	}
	expected, err := hex.DecodeString(hash)
	if err != nil || len(expected) != sha256.Size {
		return false
	}
	sum := sha256.Sum256([]byte(secret))
	return subtle.ConstantTimeCompare(sum[:], expected) == 1
}

// MaskSecret 生成密钥掩码展示。
func MaskSecret(secret string) string {
	s := strings.TrimSpace(secret)
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}
