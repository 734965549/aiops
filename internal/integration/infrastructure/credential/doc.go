// Package credential 提供凭据本地加密存储，密文写入 integration_credential_ref，不暴露明文。
//
// 加密密钥由启动配置注入（复用 auth.jwt_secret 派生 AES-256 密钥）；生产环境应使用独立密钥源。
package credential
