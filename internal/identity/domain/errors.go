package domain

import "errors"

// ErrReferenceNotFound 表示关联写入时引用的主记录不存在（替代 DB 外键约束的仓储层校验）。
var ErrReferenceNotFound = errors.New("reference not found")

// ErrAlreadyExists 表示唯一约束冲突（如用户名 / 业务 ID 重复）。
var ErrAlreadyExists = errors.New("already exists")

// ErrSessionNotFound 表示 LDAP 浏览会话不存在或已过期。
var ErrSessionNotFound = errors.New("ldap browse session not found")

// ErrOAuthStateNotFound 表示 OAuth state 已过期、不存在或已被消费。
var ErrOAuthStateNotFound = errors.New("oauth state not found")

// ErrOAuthStateInvalid 表示 OAuth state 与 provider 或客户端指纹不匹配。
var ErrOAuthStateInvalid = errors.New("oauth state invalid")

// ErrInvalidCredentials 表示企业身份源用户名/密码或 OAuth token 校验失败。
var ErrInvalidCredentials = errors.New("invalid credentials")
