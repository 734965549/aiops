// Package credential 提供 Integration 凭据本地加密保存能力。
//
// 加密密钥来自 integration.credential_encryption_key，必须同 auth.jwt_secret
// 分开，避免 JWT 轮换时导致已保存的接入账号凭据失效。
package credential
