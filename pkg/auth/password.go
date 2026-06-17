// Package auth 提供平台共用的密码哈希与 JWT 签发/校验工具。
//
// 包级原则：
//   - 不依赖具体业务模块，只暴露原语（HashPassword / VerifyPassword / Issuer / Verifier）；
//   - Identity 上下文消费本包能力实现登录与中间件鉴权，
//     未来如果接入 OIDC / 飞书 SSO，也在 Identity 模块内适配，不改本包接口。
package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// 默认 bcrypt cost；10 在 2026 年的入门服务器上单次约 60-100ms，
// 比 bcrypt 默认 10 大一档可在 2-3 年内自然抗住硬件提升。
const defaultBcryptCost = 12

// ErrPasswordMismatch 是密码校验失败的统一错误，
// 调用方应将其转换为 UNAUTHENTICATED 等业务错误码，不要直接回显内部细节。
var ErrPasswordMismatch = errors.New("password mismatch")

// HashPassword 用 bcrypt 计算密码哈希。
func HashPassword(plain string) (string, error) {
	if plain == "" {
		return "", fmt.Errorf("empty password")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(plain), defaultBcryptCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt: %w", err)
	}
	return string(h), nil
}

// VerifyPassword 校验明文密码与已存哈希。
//
// 调用方收到 ErrPasswordMismatch 时不应再向外暴露具体原因（是否用户存在 / 是否密码错），
// 统一回 UNAUTHENTICATED，避免给爆破留下信息差。
func VerifyPassword(hash, plain string) error {
	if hash == "" || plain == "" {
		return ErrPasswordMismatch
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrPasswordMismatch
		}
		return fmt.Errorf("bcrypt verify: %w", err)
	}
	return nil
}
