# internal/identity 模块说明（粤语版）

## 呢个模块做咩

`identity` 系平台嘅「门禁同会员中心」。边个可以入、入咗之后系边个、账号有冇被停用，主要都由呢个模块处理。

当前已完成：

- 用户登录：本地用户名 + 密码换 token，并支持 LDAP/AD 同 OAuth2/OIDC 登录入口。
- 密码校验：使用 bcrypt 哈希存储同校验。
- 刷新 token：用 refresh token 换新 token。
- 当前用户：根据 access token 查用户资料。
- 登录安全：登录限流、认证审计、OAuth state 服务端校验、登录 IP 白名单、登出同 refresh token 轮换。
- 路由鉴权：受保护接口统一校验 Bearer access token。
- 启动时按配置创建默认管理员账号。
- Identity 核心数据表已落地：用户、角色、权限、用户角色、角色权限、数据范围、角色数据范围、AI 工具权限、角色 AI 工具权限。
- 领域模型已补齐：`User`、`Role`、`Permission`、`UserRole`、`DataScope`、`AIToolPermission`。

已验证：

- Identity 数据模型补充工作已完成，并已通过 `go test ./...` 验证。
- 已落地表结构：`iam_user`、`iam_role`、`iam_permission`、`iam_user_role`、`iam_role_permission`、`iam_data_scope`、`iam_role_data_scope`、`iam_ai_tool_permission`、`iam_role_ai_tool_permission`。
- 已补齐领域模型与仓储查询：角色、权限、用户角色、角色权限、数据范围、角色数据范围、AI 工具权限、角色 AI 工具权限。
- 已提供角色同权限只读分页接口，以及当前用户角色查询接口。
- 统一权限校验链路已接入：路由层授权中间件、`AuthorizationService` 聚合 RBAC + 数据权限 + AI 工具权限，AI 工具网关在调用 provider 前二次校验。
- 认证审计会记录本地登录、LDAP/AD 登录、OAuth 回调、refresh 同 logout 嘅成功/失败事件，并提供管理员查询接口。
- OAuth2/OIDC `state` 已绑定 provider、客户端 IP/User-Agent 指纹，并喺回调时一次性消费。
- `auth.login_ip_allowlist` 可限制登录相关入口来源 IP；空列表表示唔限制。

下一步要完善：

- RBAC 增强：角色继承、菜单权限、操作权限聚合查询。
- 数据权限运行时应用：按业务线、环境、资源类型、标签控制可见范围。
- 权限管理写接口：角色/权限/数据范围/AI 工具权限配置同变更审计。
- 登录安全增强：MFA、账号锁定策略、token 吊销列表。
- 企业身份源：LDAP / AD 登录、OAuth2/OIDC 已具备基础能力；组织映射同企业 SSO 体验仲可以继续补。

## 分层结构

```text
internal/identity
  -> domain              领域层：User 实体、Repository 接口
  -> application         应用层：AuthService、UserService
  -> infrastructure      基础设施层：GORM 数据库实现
  -> interfaces/http     HTTP 层：Handler、Routes
```

## 调用关系

本地账号登录流程：

```text
POST /api/identity/login
  -> interfaces/http.Handler.Login
  -> requireLoginIP / loginLimiter
  -> application.AuthService.Login
  -> domain.UserRepository.FindByUsername
  -> infrastructure/persistence.UserRepository
  -> PostgreSQL iam_user 表
  -> pkg/auth.VerifyPassword
  -> pkg/auth.JWTManager.IssueAccess / IssueRefresh
  -> AuthAuditService.Record
  -> 返回 TokenPair
```

LDAP / AD 域账号登录流程（须管理员预置绑定）：

```text
管理员：POST /api/identity/admin/ldap/connect（或 Admin API 预置）
  -> 浏览 OU / 勾选用户 / 导入并绑定 role_codes
  -> iam_user + iam_external_identity

用户：POST /api/identity/login/external
  -> requireLoginIP / loginLimiter
  -> LDAPProvider.Authenticate
  -> provisionExternalUser：仅当 iam_external_identity 已有 binding 才签发 token
  -> 同步 profile / 组角色映射
  -> AuthAuditService.Record
  -> 返回 TokenPair
```

OAuth2/OIDC 登录流程（须已配置 provider）：

```text
GET /api/identity/oauth/:provider_id/authorize
  -> requireLoginIP
  -> OAuthStateStore.Issue：绑定 provider + 客户端 IP/User-Agent 指纹
  -> 跳转企业身份源授权页

GET/POST /api/identity/oauth/:provider_id/callback
  -> requireLoginIP
  -> OAuthStateStore.Consume：校验并一次性消费 state
  -> provisionExternalUser：仅当 iam_external_identity 已有 binding 才签发 token
  -> AuthAuditService.Record
  -> 返回 TokenPair
```

注意：外部登录**不会**按用户名自动绑定本地账号；`auto_create_user` 已关闭。契约见 `ops/auth-contract.md` §8、`ops/identity-api-contract.md` §8。

当前用户流程：

```text
GET /api/identity/me
  -> server Auth 中间件验证 Bearer Token
  -> 将 user_id 放入 Gin Context
  -> Handler.GetCurrentUser
  -> application.UserService.GetCurrentUser
  -> UserRepository.FindByID
  -> 返回 CurrentUserDTO
```

## 入参

| 接口 / 方法 | 入参 | 说明 |
| --- | --- | --- |
| `Login` | `username`、`password` | 用户名同密码，JSON 提交 |
| `Refresh` | `refresh_token` | 刷新令牌，JSON 提交 |
| `GetCurrentUser` | Bearer access token | 从请求头 `Authorization` 传入 |
| `EnsureBootstrapUser` | 默认用户名、密码、显示名 | 启动时从配置读取 |

## 出参

| 输出 | 字段 | 说明 |
| --- | --- | --- |
| `TokenPair` | `access_token` | 访问接口用嘅短期 token |
| `TokenPair` | `refresh_token` | 换新 token 用嘅长期 token |
| `TokenPair` | `access_expires_at`、`refresh_expires_at` | 过期时间，Unix 秒 |
| `CurrentUserDTO` | `id/username/display_name/email/status` | 当前用户资料 |
| 业务错误 | `UNAUTHENTICATED/INVALID_ARGUMENT/...` | 登录失败、参数错、服务未配置等 |

## 通俗比喻

`identity` 就似写字楼门禁：你用工牌或者身份证明入闸，保安会查你系咪员工、工牌过期未、账号有冇被冻结。通过之后，先可以入去其他楼层办事。
