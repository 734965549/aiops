// Package application 编排 Integration 用例：账号 CRUD、凭据引用写入、连通性测试同审计。
//
// 凭据明文只可以喺写入时经 CredentialVault 加密，API 响应只返回 has_credential。
package application
