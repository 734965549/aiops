# Identity 管理 API 契约（角色 / 权限 / 当前用户 / 管理员预置）

本文档定义 Identity 模块角色、权限、当前用户、统一授权校验，以及**管理员专用**用户预置与 LDAP/AD 导入接口的稳定契约。

登录、JWT 签发与刷新见 `auth-contract.md`。外部登录安全策略见 `auth-contract.md` §8。

## 1. 范围

- 角色与权限只读分页查询。
- 当前用户资料与当前用户绑定角色。
- 统一授权校验调试接口（`POST /api/identity/authorize`）。
- **管理员专用**：本地用户创建、外部身份预置、LDAP/AD 目录浏览与批量导入（不对外公开注册）。

## 2. 鉴权要求

所有本节接口均挂载在 **Authed** 路由组，要求：

```http
Authorization: Bearer <access_token>
```

当平台已注入 `AuthorizationService` 时，各路由还会挂载统一授权中间件，按 `resource` + `action` 校验 RBAC：

| 接口 | 所需权限（`app:{resource}:{action}`） |
| --- | --- |
| `GET /api/identity/roles` | `app:identity.roles:read` |
| `GET /api/identity/permissions` | `app:identity.permissions:read` |
| `GET /api/identity/me` | `app:identity.profile:read` |
| `GET /api/identity/me/roles` | `app:identity.profile.roles:read` |
| `POST /api/identity/authorize` | `app:identity.authorization:execute` |
| `POST /api/identity/admin/users` | `app:identity.users:create` |
| `POST /api/identity/admin/external-identities` | `app:identity.external_identities:create` |
| `POST /api/identity/admin/users/:user_id/external-identities` | `app:identity.external_identities:create` |
| `POST /api/identity/admin/ldap/connect` 及同前缀 LDAP 会话接口 | `app:identity.external_identities:create` |
| `GET /api/identity/admin/ldap/:provider_id/*`（已配置身份源） | `app:identity.external_identities:create` |
| `GET /api/identity/admin/auth-audits` | `app:identity.auth_audits:read` |

授权失败返回 `PERMISSION_DENIED`（HTTP 403）。未登录或 token 无效返回 `UNAUTHENTICATED`（HTTP 401）。

系统管理员角色 `admin` 嘅默认权限由迁移 `migrations/0002_seed_admin_permissions.up.sql`、`0004_user_provisioning_permissions.up.sql` 同 `0006_auth_audit.up.sql` 种子；默认管理员用户由 `migrations/0016_seed_default_admin_user.up.sql` 直接种子为 `admin/admin123` 并绑定 `admin` 角色。启动期 `EnsureBootstrapUser` 仍保留作 dev/test 兼容链路。

## 3. 统一响应格式

与平台其它业务接口一致：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | string | 成功为 `"OK"`；失败为业务错误码 |
| `message` | string | 成功为 `"ok"` |
| `trace_id` | string | 请求追踪 ID |
| `data` | object | 业务数据 |

## 4. 接口列表

### 4.1 获取角色分页列表

- Method: `GET`
- Path: `/api/identity/roles`
- 鉴权: 是（Bearer + `app:identity.roles:read`）

#### 查询参数

| 参数名 | 类型 | 默认值 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `page` | number | `1` | 否 | 页码，从 1 开始 |
| `page_size` | number | `20` | 否 | 每页条数，最大 `100` |
| `status` | string | 空 | 否 | 角色状态：`active` / `disabled` |
| `is_system` | boolean | 空 | 否 | 是否系统角色 |

#### 响应示例

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": {
    "items": [
      {
        "id": "role-uuid",
        "code": "admin",
        "name": "系统管理员",
        "description": "平台内置管理员角色",
        "status": "active",
        "is_system": true
      }
    ],
    "total": 1,
    "page": 1,
    "page_size": 20
  }
}
```

### 4.2 获取权限分页列表

- Method: `GET`
- Path: `/api/identity/permissions`
- 鉴权: 是（Bearer + `app:identity.permissions:read`）

#### 查询参数

| 参数名 | 类型 | 默认值 | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `page` | number | `1` | 否 | 页码，从 1 开始 |
| `page_size` | number | `20` | 否 | 每页条数，最大 `100` |
| `resource` | string | 空 | 否 | 资源域筛选，如 `identity.roles`、`ai.tools` |
| `action` | string | 空 | 否 | 动作筛选，如 `read`、`execute`、`invoke` |

#### 响应示例

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": {
    "items": [
      {
        "id": "perm-uuid",
        "code": "app:identity.roles:read",
        "name": "查看角色",
        "resource": "identity.roles",
        "action": "read",
        "description": "查看 Identity 角色列表"
      }
    ],
    "total": 1,
    "page": 1,
    "page_size": 20
  }
}
```

### 4.3 获取当前用户信息

- Method: `GET`
- Path: `/api/identity/me`
- 鉴权: 是（Bearer + `app:identity.profile:read`）

#### 响应示例

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": {
    "id": "user-uuid",
    "username": "alice",
    "display_name": "Alice",
    "email": "alice@example.com",
    "status": "active"
  }
}
```

### 4.4 获取当前用户角色列表

- Method: `GET`
- Path: `/api/identity/me/roles`
- 鉴权: 是（Bearer + `app:identity.profile.roles:read`）

返回当前登录用户绑定的角色集合（非全局角色分页查询）。

#### 响应示例

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": {
    "items": [
      {
        "id": "role-uuid",
        "code": "admin",
        "name": "系统管理员",
        "description": "平台内置管理员角色",
        "status": "active",
        "is_system": true
      }
    ],
    "total": 1,
    "page": 1,
    "page_size": 1
  }
}
```

### 4.5 统一授权校验

- Method: `POST`
- Path: `/api/identity/authorize`
- 鉴权: 是（Bearer + `app:identity.authorization:execute`）

供前端或工具网关调试一次 RBAC + 数据权限 + AI 工具权限判断。请求体中的 `user_id` 由服务端从 token 覆盖，客户端无需传入。

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `resource` | string | 否 | 资源域，与 `action` 组合校验操作权限 |
| `action` | string | 否 | 动作 |
| `object_owner` | string | 否 | 数据对象属主，用于自定义数据范围 |
| `object_dept` | string | 否 | 部门，用于部门数据范围 |
| `object_team` | string | 否 | 团队 |
| `object_region` | string | 否 | 区域 |
| `object_tags` | string[] | 否 | 标签 |
| `tool_code` | string | 否 | AI 工具编码 |
| `require_confirmed` | boolean | 否 | 是否已人工确认（`require_confirm` 类工具） |
| `required_permission` | string | 否 | 额外要求的权限编码 |

#### 响应 `data`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `allowed` | boolean | 是否允许 |
| `reason` | string | 拒绝原因（`allowed=false` 时） |
| `matched_role_names` | string[] | 匹配到的角色编码 |
| `matched_permissions` | string[] | 用户聚合权限编码 |
| `matched_scopes` | string[] | 匹配的数据范围编码 |
| `tool_mode` | string | 工具权限模式（如 `read_only`、`require_confirm`） |

#### 响应示例

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": {
    "allowed": true,
    "matched_role_names": ["admin"],
    "matched_permissions": ["app:identity.roles:read", "app:ai.tools:invoke"],
    "matched_scopes": ["all-data"],
    "tool_mode": "read_only"
  }
}
```

授权拒绝时 HTTP 仍为 403，`code` 为 `PERMISSION_DENIED`，`message` 为具体原因（如 `missing operation permission`）。

### 4.6 认证审计查询

- Method: `GET`
- Path: `/api/identity/admin/auth-audits`
- 鉴权: 是（Bearer + `app:identity.auth_audits:read`）

查询本地登录、LDAP/AD 登录、OAuth 回调、refresh 同 logout 嘅成功/失败审计事件。

#### 查询参数

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `page` | number | 否 | 页码，默认 1 |
| `page_size` | number | 否 | 每页条数 |
| `user_id` | string | 否 | 平台用户业务 ID |
| `username` | string | 否 | 登录用户名 |
| `provider_id` | string | 否 | 企业身份源 ID |
| `event` | string | 否 | `login` / `refresh` / `logout` |
| `result` | string | 否 | `success` / `failure` |

#### 响应 `data`

返回 `PageData<AuthAudit>`。

## 5. 前端字段表

### 5.1 `Role`

| 字段 | 类型 | 必有 | 说明 |
| --- | --- | --- | --- |
| `id` | string | 是 | 角色业务 ID |
| `code` | string | 是 | 角色编码 |
| `name` | string | 是 | 角色名称 |
| `description` | string | 否 | 角色描述 |
| `status` | string | 是 | `active` / `disabled` |
| `is_system` | boolean | 是 | 是否系统角色 |

### 5.2 `Permission`

| 字段 | 类型 | 必有 | 说明 |
| --- | --- | --- | --- |
| `id` | string | 是 | 权限业务 ID |
| `code` | string | 是 | 权限编码，格式 `app:{resource}:{action}` |
| `name` | string | 是 | 权限名称 |
| `resource` | string | 是 | 资源域 |
| `action` | string | 是 | 动作 |
| `description` | string | 否 | 权限描述 |

### 5.3 `PageData<T>`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `items` | array\<T\> | 列表数据 |
| `total` | number | 总条数 |
| `page` | number | 页码 |
| `page_size` | number | 每页条数 |

### 5.4 `CurrentUser`

| 字段 | 类型 | 必有 | 说明 |
| --- | --- | --- | --- |
| `id` | string | 是 | 用户业务 ID |
| `username` | string | 是 | 用户名 |
| `display_name` | string | 否 | 显示名 |
| `email` | string | 否 | 邮箱 |
| `status` | string | 是 | 用户状态 |

### 5.5 `AuthAudit`

| 字段 | 类型 | 必有 | 说明 |
| --- | --- | --- | --- |
| `id` | string | 是 | 审计业务 ID |
| `user_id` | string | 否 | 平台用户业务 ID；失败或 logout 场景可能会为空 |
| `username` | string | 否 | 登录用户名 |
| `provider_id` | string | 否 | 企业身份源 ID |
| `event` | string | 是 | `login` / `refresh` / `logout` |
| `method` | string | 是 | `local` / `external` / `oauth` / `refresh` |
| `result` | string | 是 | `success` / `failure` |
| `ip` | string | 否 | 客户端 IP |
| `user_agent` | string | 否 | 客户端 UA |
| `reason` | string | 否 | 失败原因；认证失败会统一脱敏 |
| `created_at` | number | 是 | Unix 秒 |

## 6. 数据模型

迁移 `migrations/0001_init_identity.up.sql`、`0002_seed_admin_permissions.up.sql`、`0003_external_identity.up.sql`、`0004_user_provisioning_permissions.up.sql`、`0005_user_role_source.up.sql` 与 `0006_auth_audit.up.sql` 已落地：

| 表名 | 说明 |
| --- | --- |
| `iam_user` | 用户主表 |
| `iam_role` | 角色主表 |
| `iam_permission` | 权限主表 |
| `iam_user_role` | 用户角色关联 |
| `iam_role_permission` | 角色权限关联 |
| `iam_data_scope` | 数据范围 |
| `iam_role_data_scope` | 角色数据范围关联 |
| `iam_ai_tool_permission` | AI 工具权限 |
| `iam_role_ai_tool_permission` | 角色 AI 工具权限关联 |
| `iam_external_identity` | 外部身份与平台用户绑定（LDAP DN / OIDC sub 等） |
| `iam_auth_audit` | 认证审计事件（登录、刷新、登出嘅成功/失败） |

运行时统一鉴权链路已接入：`AuthRequired` → 路由授权中间件 → `AuthorizationService`（RBAC + 数据范围 + AI 工具权限）。

## 7. 错误语义

| code | HTTP | 说明 |
| --- | --- | --- |
| `OK` | 200 | 成功 |
| `INVALID_ARGUMENT` | 400 | 参数缺失或非法 |
| `UNAUTHENTICATED` | 401 | token 无效或缺失 |
| `PERMISSION_DENIED` | 403 | RBAC / 数据范围 / 工具权限校验未通过 |
| `NOT_FOUND` | 404 | 当前用户不存在；LDAP 浏览会话过期 |
| `ALREADY_EXISTS` | 409 | 用户名或外部绑定已存在 |
| `UNAVAILABLE` | 503 | 访问控制或授权服务未配置；LDAP 连接失败 |
| `INTERNAL` | 500 | 数据库或服务内部异常 |

## 8. 管理员用户预置与 LDAP/AD 导入

本节接口均挂载在 **Authed** 路由组，**不对外公开**；无自助注册入口。平台用户名（本地账号、域账号）在 `iam_user.username` 上**全局唯一**。

### 8.1 安全与流程约定

| 约定 | 说明 |
| --- | --- |
| 预置后方可登录 | 外部用户首次 LDAP/OAuth 登录前，必须已有 `iam_external_identity` 绑定；**禁止**按用户名自动关联已有本地账号 |
| 身份源 ID | 导入时填写的 `provider_id` 须与 `configs/config.yaml` → `identity.providers[].id` 一致，导入后用户方可域登录 |
| 平台用户名 | 未指定时自动生成 `{provider_id}:{外部登录名}`，避免与本地账号重名 |
| LDAP 会话 | 前端填写连接参数后，`POST .../ldap/connect` 建立约 **30 分钟**短期会话；Bind 密码存 Redis/内存，**不**写入前端 localStorage |
| 导入上限 | 单次批量导入最多 **200** 人；导入时可同时绑定多个平台角色（`role_codes`） |

**推荐运维流程**：

```text
1. 管理员登录平台 → 侧栏「域账号导入」（/identity/ldap-import）
2. 填写 LDAP/AD 连接 → 连接目录 → 浏览 OU → 勾选用户 → 选择角色 → 导入
3. 在 identity.providers 中启用同 id 的 LDAP/AD 身份源（供用户登录）
4. 用户使用域账号登录
```

前端实现：`web/src/views/identity/ldap-import/index.vue`；API 封装：`web/src/api/identity-admin.ts`。

### 8.2 创建本地平台用户

- Method: `POST`
- Path: `/api/identity/admin/users`
- 鉴权: Bearer + `app:identity.users:create`

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `username` | string | 是 | 平台用户名，全局唯一 |
| `password` | string | 是 | 8–256 字符 |
| `display_name` | string | 否 | 显示名 |
| `email` | string | 否 | 邮箱 |

### 8.3 预置外部身份绑定

- Method: `POST`
- Path: `/api/identity/admin/external-identities`
- 鉴权: Bearer + `app:identity.external_identities:create`

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `provider_id` | string | 是 | 身份源 ID |
| `external_subject` | string | 是 | LDAP DN / OIDC `sub` 等外部唯一标识 |
| `external_username` | string | 否 | 外部登录名 |
| `display_name` | string | 否 | 显示名 |
| `email` | string | 否 | 邮箱 |
| `platform_username` | string | 否 | 指定平台用户名；空则自动生成命名空间用户名 |
| `user_id` | string | 否 | 绑定到已有平台用户时不新建用户 |

绑定到已有用户：

- Method: `POST`
- Path: `/api/identity/admin/users/:user_id/external-identities`
- 请求体同上（无需 `user_id` 字段）

### 8.4 建立 LDAP 浏览会话（前端填连接）

- Method: `POST`
- Path: `/api/identity/admin/ldap/connect`
- 鉴权: Bearer + `app:identity.external_identities:create`

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `provider_id` | string | 是 | 身份源 ID（字母数字、`-`、`_`，最长 64） |
| `type` | string | 否 | `ldap` / `ad`，默认 `ldap` |
| `server_url` | string | 是 | 如 `ldaps://ad.example.com:636` |
| `bind_dn` | string | 否 | 服务账号 DN |
| `bind_password` | string | 否 | 服务账号密码 |
| `base_dn` | string | 是 | 目录根 DN |
| `start_tls` | boolean | 否 | 是否 StartTLS |
| `insecure_skip_verify` | boolean | 否 | 跳过 TLS 校验（仅 dev） |
| `browse_org_filter` | string | 否 | 浏览 OU 的 LDAP 过滤；空则用默认 |
| `browse_user_filter` | string | 否 | 浏览用户的 LDAP 过滤；空则按类型默认 |

#### 响应 `data`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `session_id` | string | 浏览会话 ID |
| `provider_id` | string | 身份源 ID |
| `base_dn` | string | 根 DN |
| `expires_in` | number | 会话有效秒数 |

### 8.5 浏览组织与用户（会话）

| 接口 | 说明 |
| --- | --- |
| `GET /api/identity/admin/ldap/sessions/:session_id/organizations?parent_dn=` | 列出 `parent_dn` 下一级 OU；`parent_dn` 空则从 `base_dn` 开始 |
| `GET /api/identity/admin/ldap/sessions/:session_id/users?org_dn=&limit=` | 预览组织下用户；`imported` 表示是否已预置 |
| `DELETE /api/identity/admin/ldap/sessions/:session_id` | 关闭会话 |

组织响应 `organizations[]`：`{ "dn", "name" }`。用户响应 `users[]`：`{ "external_subject", "external_username", "display_name", "email", "imported" }`。

### 8.6 批量导入（会话）

- Method: `POST`
- Path: `/api/identity/admin/ldap/sessions/:session_id/import`
- 鉴权: Bearer + `app:identity.external_identities:create`

#### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `external_subjects` | string[] | 二选一 | 要导入的用户 DN 列表 |
| `import_all` | boolean | 二选一 | 导入当前 `org_dn` 下全部用户（≤200） |
| `org_dn` | string | `import_all` 时必填 | 组织 DN |
| `role_codes` | string[] | 否 | 导入成功后绑定的平台角色编码，如 `["viewer"]` |

#### 响应 `data`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `created` | number | 新导入成功数 |
| `skipped` | number | 已存在绑定跳过数 |
| `failed` | number | 失败数 |
| `users` | array | 逐条结果：`external_subject`、`status`（`created`/`skipped`/`failed`）、`message`、`user` |

### 8.7 已配置身份源的 LDAP 接口（可选）

当 `identity.providers` 中已启用 LDAP/AD 时，也可直接使用服务端配置（无需前端填密码）：

| 接口 | 说明 |
| --- | --- |
| `GET /api/identity/admin/ldap/:provider_id/connection-test` | 测试连接 |
| `GET /api/identity/admin/ldap/:provider_id/organizations?parent_dn=` | 浏览 OU |
| `GET /api/identity/admin/ldap/:provider_id/users?org_dn=` | 预览用户 |
| `POST /api/identity/admin/ldap/:provider_id/import` | 批量导入（请求体同 §8.6） |
## 9. 权限管理 P1（migration 0015）

迁移 `0015_identity_access_control_management` 新增内置 `viewer` 角色和以下权限：

| 权限码 | 说明 |
| --- | --- |
| `app:identity.users:read` | 分页查询用户与用户角色绑定 |
| `app:identity.data_scopes:read` | 查询数据范围字典与角色数据范围绑定 |
| `app:identity.ai_tool_permissions:read` | 查询 AI 工具权限字典与角色工具权限绑定 |
| `app:identity.access_control:write` | 替换用户/角色授权绑定 |

`viewer` 是系统只读角色，绑定 dashboard、alerts、assets、runbooks、executions 的 read 权限，以及当前用户 profile read / profile.roles read 和 `all-data` 数据范围。新增管理权限全部绑定给 `admin`。

### 9.1 管理接口

所有接口均要求 `Authorization: Bearer <access_token>`，并走 RBAC。

| 接口 | 权限 |
| --- | --- |
| `GET /api/identity/admin/users` | `app:identity.users:read` |
| `GET /api/identity/admin/users/:user_id/roles` | `app:identity.users:read` |
| `PUT /api/identity/admin/users/:user_id/roles` | `app:identity.access_control:write` |
| `GET /api/identity/data-scopes` | `app:identity.data_scopes:read` |
| `GET /api/identity/ai-tool-permissions` | `app:identity.ai_tool_permissions:read` |
| `GET /api/identity/admin/roles/:role_id/permissions` | `app:identity.roles:read` |
| `PUT /api/identity/admin/roles/:role_id/permissions` | `app:identity.access_control:write` |
| `GET /api/identity/admin/roles/:role_id/data-scopes` | `app:identity.data_scopes:read` |
| `PUT /api/identity/admin/roles/:role_id/data-scopes` | `app:identity.access_control:write` |
| `GET /api/identity/admin/roles/:role_id/ai-tool-permissions` | `app:identity.ai_tool_permissions:read` |
| `PUT /api/identity/admin/roles/:role_id/ai-tool-permissions` | `app:identity.access_control:write` |

### 9.2 请求与响应语义

`GET /api/identity/admin/users` 返回标准分页结构，支持 `page`、`page_size`、`status`、`keyword`。用户对象只返回业务 `id`，不暴露数据库自增 ID 和密码字段。

`GET /api/identity/admin/users/:user_id/roles` 返回：

```json
{
  "items": [
    {
      "id": "role-business-id",
      "code": "viewer",
      "name": "Viewer",
      "description": "Read-only role",
      "status": "active",
      "is_system": true,
      "source": "manual"
    }
  ]
}
```

`PUT /api/identity/admin/users/:user_id/roles` 请求体：

```json
{ "role_ids": ["role-business-id"] }
```

该接口只替换目标用户 `manual` 来源角色；`ldap_import` 和 `external_group` 来源绑定保持不变。若请求中包含已由非 manual 来源绑定的角色，后端会保持该来源绑定，不重复写入 manual 关系。

角色授权 PUT 均为幂等的全量替换：

```json
{ "permission_ids": ["permission-business-id"] }
{ "data_scope_ids": ["data-scope-business-id"] }
{ "tool_permission_ids": ["tool-permission-business-id"] }
```

空数组表示清空对应绑定集合。引用不存在返回 `NOT_FOUND`；鉴权失败返回 `PERMISSION_DENIED`（HTTP 403）；未登录或 token 无效返回 `UNAUTHENTICATED`（HTTP 401）。

### 9.3 审计

权限写接口成功后写 Operation Audit，审计失败只记录告警日志，不回滚主业务写入。

| resource_type | action |
| --- | --- |
| `identity_user` | `set_user_roles` |
| `identity_role` | `set_role_permissions` |
| `identity_role` | `set_role_data_scopes` |
| `identity_role` | `set_role_ai_tools` |

审计 payload 记录 old/new 业务 ID 和 `actor_user_id`，不得记录密码、token、密钥。
