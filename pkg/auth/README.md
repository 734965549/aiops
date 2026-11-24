# pkg/auth 模块说明

`auth` 提供与业务无关的认证原语：密码哈希、JWT 签发/校验、refresh token 会话存储、登录限流与 IP 白名单。Identity 上下文消费这些原语实现登录与鉴权；接入 OIDC/SSO 时在 Identity 内适配，不改本包接口。

## JWT（`jwt.go`）

- HS256 对称密钥；`Claims` 在标准 RegisteredClaims 上扩展 `uname`（用户名）、`roles`（仅展示与粗判，权限校验仍需服务端再查）、`typ`（access/refresh 区分，防止 refresh 被当 access 使用）。
- `NewJWTManager`：secret 必填；TTL 缺省 access 2h / refresh 7 天；clock skew 缺省 30s。
- 一个实例绑定一份 (secret, issuer, ttl)；密钥轮换需在实例外再包一层并保留 kid。

## 密码（`password.go`）

- bcrypt cost 12（`HashPassword/VerifyPassword`）；空密码拒绝。
- `ErrPasswordMismatch` 是校验失败统一错误；调用方转为 `UNAUTHENTICATED` 等业务码，不回显内部细节。

## Refresh token 会话（`refresh_store.go`）

- 接口 `RefreshTokenStore`：`Register/Validate/Revoke/RevokeAll`（强制下线）。
- `RedisRefreshTokenStore`：键 `auth:refresh:<user_id>:<jti>`，TTL 与 token 过期一致；`RevokeAll` 按用户前缀扫描。无 Redis 时使用 `NoopRefreshTokenStore`（dev-only，禁轮换）。
- `NewRefreshTokenJTI` 生成唯一标识。

## 登录限流与 IP 白名单（`login_limiter.go`/`ip_allowlist.go`）

- `LoginAttemptLimiter`：按 IP 与用户名两个维度的窗口计数、失败锁定与锁定时长，配置经 `pkg/config` 注入；无 Redis 时降级为进程内计数（多副本不共享）。
- `NewIPAllowlist`：解析登录白名单网段，配置非法即报错（`config.Validate` 前置校验）。

## 中间件适配（`middleware_adapter.go`）

`JWTAuthenticator` 实现 `pkg/transport/http/middleware.Authenticator`，把 token 校验结果转换为 `Identity{UserID, Username, Roles}` 注入 Gin context（`user_id/username/roles`）。

## 测试

`jwt_test.go`/`password_test.go`/`login_limiter_test.go`/`ip_allowlist_test.go` 覆盖签发校验、哈希、限流与白名单语义。
