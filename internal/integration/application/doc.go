// Package application 编排 Integration 用例：账号 CRUD、凭据引用写入、连通性测试与审计。
//
// 凭据明文只在写入时经 CredentialVault 加密，API 响应仅返回 has_credential。
package application
