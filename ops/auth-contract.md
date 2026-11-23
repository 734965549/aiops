# 认证契约（登录 / JWT / 刷新）

本文档定义 Identity 模块对外认证接口的稳定契约，供前端联调、网关接入与运维配置参考。

角色、权限、当前用户资料等 Identity 管理接口见 `identity-api-contract.md`。

## 1. 范围

- 本地用户名密码、LDAP/AD 和 OAuth2/OIDC 登录。
- TokenPair 签发、refresh 换发、logout 吊销及 JWT 请求头约定。
- 登录 IP 白名单、限流、OAuth state、外部身份预置和认证审计。
- 统一响应封装与认证相关错误码。

角色、权限管理和数据范围见 `identity-api-contract.md`。本文描述当前已落地行为；MFA、access token 吊销列表和按权限动态隐藏前端菜单不属于当前能力。

## 2. 统一响应格式

所有接口均通过 `httpx.OK` / `httpx.Fail` 输出；成功时 `code` 为字符串 `"OK"`（**不是**数字 `0`）。字段说明与健康检查示例见 `health-contract.md`。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | string | 成功为 `"OK"`；失败为业务错误码（如 `UNAUTHENTICATED`） |
| `message` | string | 成功为 `"ok"`；失败为可展示错误提示 |
| `trace_id` | string | 请求追踪 ID，成功与失败均可能返回 |
| `data` | object | 成功时的业务数据；失败时通常省略 |

成功示例：

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": { }
}
```

## 3. 接口列表

### 3.1 登录

- Method: `POST`
- Path: `/api/identity/login`
- 鉴权: 否（公开）

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `username` | string | 是 | 用户名，最长 64 字符 |
| `password` | string | 是 | 密码，最长 256 字符 |
| `provider_id` | string | 否 | 为空走本地账号；非空时改用对应的 LDAP/AD 密码身份源，等价于外部登录路径 |

#### 响应 `data`（TokenPair）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `access_token` | string | 访问令牌，用于后续 Bearer 鉴权 |
| `refresh_token` | string | 刷新令牌，仅用于 `/api/identity/refresh` |
| `access_expires_at` | number | access token 过期时间（Unix 秒） |
| `refresh_expires_at` | number | refresh token 过期时间（Unix 秒） |
| `token_type` | string | 固定为 `Bearer` |
| `user` | CurrentUser | 当前用户摘要（见 `identity-api-contract.md`） |

#### 响应示例

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "access_expires_at": 1710000000,
    "refresh_expires_at": 1710604800,
    "token_type": "Bearer",
    "user": {
      "id": "user-uuid",
      "username": "admin",
      "display_name": "Administrator",
      "email": "",
      "status": "active"
    }
  }
}
```

#### 错误语义

| code | HTTP | 典型 message |
| --- | --- | --- |
| `INVALID_ARGUMENT` | 400 | `username and password are required` |
| `UNAUTHENTICATED` | 401 | `invalid username or password`（用户不存在、密码错误、账号禁用统一返回，避免枚举） |
| `UNAVAILABLE` | 503 | `authentication is not enabled` |

### 3.2 刷新 token

- Method: `POST`
- Path: `/api/identity/refresh`
- 鉴权: 否（公开，凭 refresh token 换发）

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `refresh_token` | string | 是 | 登录时下发的 refresh token |

#### 响应

与登录相同，返回新的 `TokenPair`（含新的 access / refresh 对）。

#### 错误语义

| code | HTTP | 典型 message |
| --- | --- | --- |
| `INVALID_ARGUMENT` | 400 | `refresh_token is required` |
| `UNAUTHENTICATED` | 401 | `invalid or expired refresh token` / `user disabled or removed` |
| `UNAVAILABLE` | 503 | `authentication is not enabled` |

Redis 会话启用时，refresh 会校验并作废旧 JTI，再登记新 JTI；dev 无 Redis 时使用 Noop store，只保证 JWT 校验和换发，不保证旧 refresh token 被服务端吊销。prod 要求 `redis.required=true`，不能用 dev 降级结果替代生产会话验收。

### 3.3 登出

- Method: `POST`
- Path: `/api/identity/logout`
- 鉴权: 否（公开，凭 refresh token 吊销当前会话）

请求体：

```json
{ "refresh_token": "<refresh_token>" }
```

成功响应 `data`：

```json
{ "logged_out": true }
```

Logout 校验 refresh token 类型、签名和过期时间，并在 Redis 会话启用时吊销对应 JTI。它只吊销当前 refresh token，不是全账号强制下线；已经签发的 access token 仍按原 TTL 有效。无 Redis 的 dev 降级模式没有持久吊销能力。

常见错误：`INVALID_ARGUMENT`（400）、`UNAUTHENTICATED`（401）、`UNAVAILABLE`（503）。IP 白名单拒绝返回 `PERMISSION_DENIED`（403）。

## 4. JWT 约定

### 4.1 请求头

受保护接口统一使用：

```http
Authorization: Bearer <access_token>
```

`AuthRequired` 中间件解析 access token；缺失或无效时返回 `UNAUTHENTICATED`（HTTP 401）。

### 4.2 Token 类型

| 类型 | Claims `typ` | 用途 |
| --- | --- | --- |
| access | `access` | API 鉴权 |
| refresh | `refresh` | 仅用于 `/api/identity/refresh`，不得当作 access 使用 |

### 4.3 Claims 摘要

| 字段 | 说明 |
| --- | --- |
| `sub` | 业务用户 UUID（`user_id`） |
| `uname` | 用户名 |
| `roles` | 角色编码列表（仅供展示；敏感权限须服务端再查） |
| `typ` | `access` 或 `refresh` |
| `iss` | 签发方，默认配置为 `aiops` |

`roles` 只是签发时的身份摘要，不能作为敏感操作的最终授权依据。服务端通过 `AuthorizationService` 重新加载角色、权限、数据范围和工具权限；授权聚合可使用短期 Redis 缓存，并在绑定变更时执行失效。

当前没有 access token 吊销列表。logout 或禁用用户不会让既有 access token 本身立即失效；refresh 会检查用户 active 状态，生产应配合较短 access TTL 控制窗口。角色和权限变化不以 JWT roles 为准，而由服务端授权上下文及缓存失效机制控制。

### 4.4 TTL（可配置）

| 配置项 | dev 默认 | 生产 compose 推荐 | 说明 |
| --- | --- | --- | --- |
| `auth.access_ttl_m` | `120` | `60`（可调 **30–60**） | access token 有效期（分钟） |
| `auth.refresh_ttl_h` | `168` | `72`（可调 **24–72**） | refresh token 有效期（小时） |

生产环境可通过 `AIOPS_AUTH__ACCESS_TTL_M` / `AIOPS_AUTH__REFRESH_TTL_H` 覆盖；`deployments/docker-compose.prod.yml` 已采用较短默认值以降低 token 泄露窗口。

## 5. 配置与密钥约束

| 配置项 | 说明 |
| --- | --- |
| `auth.jwt_secret` | HS256 对称密钥；非 `dev` 环境拒绝占位值与弱密钥 |
| `auth.jwt_issuer` | JWT `iss` 声明 |
| `auth.bootstrap_username` / `bootstrap_password` | 仅 dev/test 可配置；prod 配置校验会直接拒绝。用于创建管理员或重新激活被 0044 锁定的账号 |
| `auth.grant_cache_ttl_s` | 授权聚合 Redis 缓存秒数，默认 60；0 禁用，绑定变更会主动失效缓存 |
| `auth.login_ip_allowlist` | 可选；登录安全 IP 白名单，支持单 IP 和 CIDR，空数组表示不限制 |
| `auth.login_rate_limit.*` | 登录/refresh 限流；默认 60 秒每 IP 30 次、IP 失败 20 次或用户名失败 5 次临时锁定 900 秒 |
| `redis.required` | prod 必须为 true；refresh JTI、OAuth state、LDAP 浏览会话和分布式限流依赖 Redis |

- `dev` 环境允许 `please-change-me-in-production` 等占位值。
- 非 `dev` 环境要求密钥长度 ≥ 32、具备足够熵与字符多样性。
- 迁移 `0016`/`0017` 会种子 `admin/admin123`；迁移 `0044` 将其锁定（`status=locked`、清空 `password_hash`），**生产默认不可登录**。生产须关闭 bootstrap，API 对外开放前由 `scripts/provision-prod-admin.ps1` 创建安全管理员；**不得**依赖 `admin/admin123`。
- 配置 `auth.login_ip_allowlist` 后，本地登录、LDAP/AD 登录、OAuth authorize/callback、refresh 和 logout 都会先校验客户端 IP；未命中返回 `PERMISSION_DENIED`。
- 登录、外部登录和 refresh 还会进入限流链路；触发窗口或临时锁定时返回 `RESOURCE_EXHAUSTED`（HTTP 429）。Redis 可用时使用共享计数，否则 dev 使用进程内计数。
- 登录、外部登录、OAuth 回调、refresh 和 logout 会写认证审计；审计写失败只记 Warn，不改变认证主流程结果。

## 6. 路由划分（认证视角）

```text
/api/identity/login                          -> 公开；本地或 provider_id 外部密码登录
/api/identity/login/external                 -> 公开；LDAP/AD 等外部密码登录
/api/identity/login/providers                -> 公开；已启用身份源摘要
/api/identity/oauth/:provider_id/authorize   -> 公开；签发授权 URL 与 state
/api/identity/oauth/:provider_id/callback    -> 公开；GET/POST OAuth 回调
/api/identity/refresh                        -> 公开；凭 refresh_token 换发
/api/identity/logout                         -> 公开；凭 refresh_token 吊销当前会话
/api/identity/roles              -> Authed + RBAC（app:identity.roles:read）
/api/identity/permissions        -> Authed + RBAC（app:identity.permissions:read）
/api/identity/me                 -> Authed + RBAC（app:identity.profile:read）
/api/identity/me/roles           -> Authed + RBAC（app:identity.profile.roles:read）
/api/identity/authorize          -> Authed + RBAC（app:identity.authorization:execute）
```

角色、权限、当前用户与统一授权校验的请求/响应字段见 `identity-api-contract.md`。上述 Authed 接口**不是**公开只读：未登录返回 401（`UNAUTHENTICATED`），权限不足返回 403（`PERMISSION_DENIED`）。

**联调提示**：若仅执行迁移 `0001` 而未执行 `0002` / `0016`，可能没有默认管理员或登录成功后 Authed 接口全部 403。请执行 `make migrate` 或叠加 `docker-compose.dev.yml`（`AUTO_MIGRATE=true`），详见 `migration-contract.md`。

## 7. 认证相关错误码

| code | HTTP | 说明 |
| --- | --- | --- |
| `OK` | 200 | 成功 |
| `INVALID_ARGUMENT` | 400 | 参数缺失或非法 |
| `UNAUTHENTICATED` | 401 | token 缺失、无效或过期；登录凭据错误 |
| `PERMISSION_DENIED` | 403 | RBAC 权限不足或登录来源 IP 未通过白名单（与 401 区分） |
| `RESOURCE_EXHAUSTED` | 429 | 登录/refresh 请求超过窗口或失败次数触发临时锁定 |
| `UNAVAILABLE` | 503 | 认证服务未配置（如未注入 Authenticator） |
| `INTERNAL` | 500 | 服务内部异常 |

## 8. 企业身份源登录（LDAP / AD / OAuth2 / OIDC）

### 8.1 公开接口

| 接口 | 请求/响应要点 |
| --- | --- |
| `POST /api/identity/login` | 本地账号；请求体可带 `provider_id` 走已配置 LDAP/AD |
| `POST /api/identity/login/external` | 请求体 `provider_id`、`username`、`password`；成功返回 TokenPair |
| `GET /api/identity/login/providers` | 返回 `providers[]`，仅含 `id/type/name/enabled/priority`，不返回连接密码或 client_secret |
| `GET /api/identity/oauth/:provider_id/authorize` | 返回 `authorization_url` 与 `state`，前端跳转到企业身份源 |
| `GET/POST /api/identity/oauth/:provider_id/callback` | GET 从 query 读取 `code/state`；POST 使用 JSON `provider_id/code/state`；成功返回 TokenPair |

OAuth2/OIDC `state` 由服务端签发并存储；回调时会校验 `provider_id`、发起授权时的客户端 IP/User-Agent 指纹，并一次性消费。校验失败统一返回 `UNAUTHENTICATED`。Redis 可用时 state 可跨实例共享，否则为进程内存储，多实例部署不能用内存模式完成可靠回调。

### 8.2 预置绑定策略（重要）

外部登录**不会**：

- 将 `alice@example.com` 截断为 `alice` 后匹配本地用户并自动绑定；
- 在首次登录时自动创建平台用户。`auto_create_user` 字段仅为兼容现有配置结构保留，当前实现即使读到 `true` 也不会放开未预置用户。

仅当 `iam_external_identity` 中已存在 **`provider_id` + `external_subject`** 绑定时，认证成功后才签发平台 JWT。

未预置时统一返回 `UNAUTHENTICATED` / `invalid username or password`，避免泄露目录信息。

### 8.3 域账号启用流程

```text
管理员预置绑定（前端「域账号导入」或 Admin API）
  → 可选：导入时绑定平台角色（role_codes）
  → 在 identity.providers 启用同 id 的 LDAP/AD/OAuth 身份源
  → 用户使用企业身份登录
  → 登录成功后同步 display_name / email / 外部组，并按 group_role_mapping 映射角色
```

预置与导入 API 见 `identity-api-contract.md` §8。

### 8.4 配置要点

| 配置项 | 说明 |
| --- | --- |
| `identity.providers[].id` | 身份源唯一 ID，须与导入时的 `provider_id` 一致 |
| `identity.providers[].*.auto_create_user` | 兼容字段；建议保持 `false`，当前未预置绑定无论该值如何都拒绝 |
| `identity.providers[].ldap.server_url` | 生产须 `ldaps://` 或 `start_tls: true` |

## 9. 路由划分（补充 Admin）

在 §6 基础上，管理员专用 Authed 路由（均需 RBAC）：

```text
GET /api/identity/admin/users                         -> app:identity.users:read
POST /api/identity/admin/users                        -> app:identity.users:create
/api/identity/admin/external-identities                -> app:identity.external_identities:create
/api/identity/admin/ldap/connect                       -> app:identity.external_identities:create
/api/identity/admin/ldap/sessions/:session_id/...     -> app:identity.external_identities:create
/api/identity/admin/ldap/:provider_id/...              -> app:identity.external_identities:create
```

**联调提示**：域账号导入页需要迁移 `0004`；若仅执行到 `0002`，Admin 接口可能 403。详见 `migration-contract.md`。
