# internal/identity 模块说明

Identity 负责平台账号、认证、外部身份绑定、授权判断和权限管理。稳定对外字段以 `ops/auth-contract.md`、`ops/identity-api-contract.md` 为准；本文说明当前代码职责、调用链和实现边界。

## 1. 当前能力与完成边界

| 能力 | 当前实现 | 完成边界 |
| --- | --- | --- |
| 本地认证 | 用户名密码、bcrypt 校验、JWT access/refresh、当前用户 | 用户不存在、密码错误和非 active 账号统一按认证失败处理 |
| 会话生命周期 | refresh 换发新 TokenPair、logout 吊销 refresh token | Redis 可用时登记、轮换和吊销 refresh JTI；dev 无 Redis 时降级为不强制轮换/吊销，prod 强制要求 Redis |
| 企业身份源 | LDAP/AD 密码登录、OAuth2/OIDC 授权码登录、身份源列表 | 仅允许已预置 `provider_id + external_subject` 绑定的用户；不按用户名自动开户或绑定 |
| 登录安全 | IP/CIDR 白名单、IP/用户名限流与临时锁定、认证审计、OAuth state 一次性消费 | 白名单覆盖登录、外部登录、OAuth authorize/callback、refresh/logout；MFA 未落地 |
| RBAC | 角色、权限、用户角色、角色权限、路由授权中间件 | 静态路由只校验操作权限；前端静态菜单不等于服务端授权 |
| 数据范围 | all、department、team、region、tag、custom 判断及团队集合解析 | 仅在显式传入对象上下文或实现查询过滤的调用点生效，不是所有列表/详情自动过滤 |
| AI 工具权限 | read_only、require_confirm、deny 及确认状态判断 | 工具网关仍需二次校验，不能只依赖路由或前端按钮 |
| 权限管理 | 用户角色、角色权限、数据范围、AI 工具权限查询与全量替换 | 用户角色 PUT 只替换 manual 来源；其他来源保留 |
| 审计 | 认证事件审计、权限变更 Operation Audit | 审计均为业务主流程外的 best-effort 写入，失败不回滚认证或授权变更 |
| 前端 | 登录、LDAP/AD 导入、用户角色与角色授权页面 | 本地用户创建、认证审计查询和身份源配置暂无专门管理 UI |

## 2. 分层与共享依赖

```text
internal/identity
  -> domain
       用户、角色、权限、数据范围、外部身份、认证审计、Repository 接口
  -> application
       AuthService、AccessControlService、AuthorizationService、LDAP 导入与用户预置
  -> infrastructure
       GORM 仓储、LDAP/OAuth Adapter、Redis/内存会话存储、审计适配器
  -> interfaces/http
       路由、Handler、DTO 绑定与响应

pkg/auth
  -> JWT、bcrypt、refresh JTI store、登录限流、IP 白名单、HTTP Authenticator
pkg/transport/http/middleware
  -> Bearer 认证、静态/动态授权中间件
```

Identity 不直接替代各业务模块的数据权限判断。其他上下文通过 application 端口或授权适配器传入对象属主、部门、团队、区域和标签，由 `AuthorizationService` 统一判断。

## 3. 路由分组

### 3.1 公开认证路由

| 路由 | 用途 |
| --- | --- |
| `POST /api/identity/login` | 本地登录；带 `provider_id` 时也可走外部密码身份源 |
| `POST /api/identity/login/external` | LDAP/AD 等外部密码登录 |
| `GET /api/identity/login/providers` | 已启用身份源摘要 |
| `GET /api/identity/oauth/:provider_id/authorize` | 获取 OAuth2/OIDC 授权 URL 与 state |
| `GET/POST /api/identity/oauth/:provider_id/callback` | OAuth 回调换发平台 TokenPair |
| `POST /api/identity/refresh` | refresh token 换发新 TokenPair |
| `POST /api/identity/logout` | 吊销 refresh token |

这些路由不要求 Bearer，但不代表没有安全限制。登录、外部登录、OAuth authorize/callback、refresh 和 logout 会执行 IP 白名单；登录、外部登录和 refresh 还进入限流链路。

### 3.2 鉴权与管理员路由

- 当前用户：`GET /me`、`GET /me/roles`。
- 字典：`GET /roles`、`GET /permissions`、`GET /data-scopes`、`GET /ai-tool-permissions`。
- 授权调试：`POST /authorize`。
- 用户与绑定：用户分页、本地用户创建、用户角色查询/替换、外部身份预置。
- 角色授权：角色权限、数据范围、AI 工具权限查询/全量替换。
- LDAP/AD：短期连接会话、OU 浏览、用户预览、批量导入及已配置身份源连接测试。
- 认证审计：`GET /admin/auth-audits`。

每条路由对应的权限码见 `ops/identity-api-contract.md` §2、§9。未认证返回 401，权限不足返回 403；不能把前端可访问路由当作已授权。

## 4. 认证调用链

### 4.1 本地登录

```text
POST /api/identity/login
  -> requireLoginIP
  -> loginLimiter.Allow
  -> AuthService.Login
  -> UserRepository.FindByUsername
  -> pkg/auth.VerifyPassword
  -> 加载当前角色编码
  -> JWTManager.IssueAccess / IssueRefresh
  -> Redis 可用时登记 refresh JTI
  -> 写认证审计
  -> 返回 TokenPair
```

JWT 中的 `roles` 只用于身份摘要。敏感操作仍由服务端重新加载授权上下文，不信任客户端提交的角色或权限。

### 4.2 Refresh 与 Logout

- Refresh 校验 token 类型、签名、过期时间和用户 active 状态；Redis 会话启用时还要校验 JTI，作废旧 JTI 后签发新 TokenPair。
- Logout 只吊销本次 refresh token，不是全账号强制下线。
- 已签发的 access token 仍按原 TTL 有效，当前没有 access token 吊销列表；缩短 access TTL 是降低泄露窗口的必要措施。
- dev 无 Redis 时使用 Noop store，不能据此验证生产级 refresh 轮换和吊销语义。

### 4.3 LDAP/AD 与 OAuth2/OIDC

```text
管理员预置/导入外部身份
  -> iam_user + iam_external_identity (+ 可选角色)
  -> 用户通过企业身份源认证
  -> 按 provider_id + external_subject 查找既有绑定
  -> 同步 display_name/email/外部组
  -> 同步 external_group 来源角色
  -> 签发平台 TokenPair
```

`auto_create_user` 配置字段仍保留用于兼容配置结构，但当前 `provisionExternalUser` 对未预置绑定一律拒绝，即使配置为 true 也不会自动开户。OAuth state 绑定 provider、客户端 IP/User-Agent 指纹并一次性消费；Redis 可用时共享存储，否则使用进程内存储。

## 5. 授权与数据范围的实际语义

### 5.1 RBAC 与工具权限

`AuthorizeStatic` 用于固定资源/动作的路由权限，并设置 `SkipDataScope=true`。`AuthorizeDynamic`、AI 工具网关和业务授权端口可以额外传入对象上下文及工具代码。

工具权限模式：

- `read_only`：不要求人工确认。
- `require_confirm`：默认要求确认；仅当工具权限显式允许未确认调用时例外。
- `deny`：拒绝。

### 5.2 数据范围

- 没有配置数据范围或具有 `all`：按当前实现放行。
- 非 all 范围缺少对象上下文：拒绝，防止客户端省略上下文绕过。
- department/team/region/custom：匹配对应对象字段。
- tag：对象标签与允许标签存在交集即可。
- 静态 RBAC 路由跳过数据范围；需要数据隔离的业务必须显式接入动态授权或查询过滤。

当前已有调用点包括 AI 告警/账号证据访问、Asset 云同步授权和同步批次 owner_team 过滤等。其他模块不能仅凭“用户绑定了数据范围”就宣称列表、详情已自动隔离。

### 5.3 角色来源

| 来源 | 维护方式 |
| --- | --- |
| `manual` | 权限管理页或 Admin API 全量替换 |
| `ldap_import` | LDAP/AD 导入时按 `role_codes` 绑定 |
| `external_group` | 外部登录时按组映射和默认角色同步 |

用户角色 PUT 不删除 `ldap_import` 和 `external_group` 来源绑定，也不会把它们重复写成 manual。

授权聚合适配 Redis 短期缓存；用户/角色绑定变更会执行缓存失效，TTL 作为兜底。JWT roles 摘要和授权缓存都不能替代服务端最终检查。

## 6. 权限管理与审计

权限、数据范围和 AI 工具权限的角色 PUT 都是幂等全量替换，空数组表示清空。引用不存在返回 `NOT_FOUND`。

权限变更审计动作：

- `identity_user / set_user_roles`
- `identity_role / set_role_permissions`
- `identity_role / set_role_data_scopes`
- `identity_role / set_role_ai_tools`

当前 `AccessControlService.recordAudit` 忽略审计 recorder 返回错误，不回滚已完成的授权变更；契约不应写成审计失败会回滚。认证审计由 Handler 记录，写失败会记 Warn，但同样不阻断登录、refresh 或 logout。

## 7. 前端实现与差距

- 登录页加载 LDAP/AD 与 OAuth/OIDC 身份源，支持本地/域账号和 OAuth 回调；401 统一提示“账号或密码错误”。
- `stores/auth.ts` 负责 TokenPair、当前用户、并发 refresh 复用和 logout。
- 域账号导入页支持连接、OU 浏览、用户预览、勾选/整 OU 导入和角色绑定；Bind 密码只进入短期服务端会话，不写入前端本地存储。
- 权限管理页支持用户筛选、manual 用户角色替换，以及角色权限/数据范围/AI 工具权限全量替换；403 时保留页面结构并禁用操作。

**安全差距**：当前 `stores/auth.ts` 将 access token、refresh token 和用户摘要写入 `localStorage`。这与项目宪章“不将 Token 写入前端持久化状态”的约束不一致，应作为独立安全整改项处理；在整改前不能宣称前端满足该条约束。

## 8. 数据与初始化

主要 Identity 迁移：

| 版本 | 作用 |
| --- | --- |
| `0001–0006` | 用户/角色/权限/数据范围/工具权限、外部身份、用户预置、角色来源和认证审计 |
| `0015` | viewer、用户/角色授权管理权限及管理绑定 |
| `0016` | 种子默认管理员并绑定 admin 权限超集 |
| `0017` | 修复旧 0016 环境的默认管理员与权限超集 |
| `0044` | 锁定仍使用已知 admin123 哈希的默认管理员，清空其密码哈希 |

完整迁移后不要假设 `admin/admin123` 可直接登录。受控 dev/test 可通过 bootstrap 创建或重新激活管理员；prod 配置会拒绝启用 bootstrap，必须通过受控发布流程初始化安全管理员。后续业务迁移还会继续向 Identity 权限表种子权限，需执行到目标版本再验收。

## 9. 验证覆盖

### 单元与 HTTP 测试

仓库已有测试覆盖：

- 本地登录成功/失败、禁用用户、refresh、bootstrap 幂等与锁定账号重新激活。
- 外部身份预置、拒绝自动绑定、OAuth state 校验与一次性消费、外部组角色同步。
- RBAC、数据范围、工具权限、团队范围解析和授权缓存。
- 用户角色来源保留、角色权限/数据范围/AI 工具权限替换及审计调用。
- JWT 类型/过期/篡改、密码、登录限流和 IP 白名单。

### E2E 的实际范围

`scripts/e2e-identity-access.ps1` 只覆盖：

1. admin 登录和创建本地测试用户；
2. 查询 viewer 并绑定 manual 角色；
3. viewer 登录、Dashboard 读取和权限写接口 403；
4. `set_user_roles` Operation Audit 查询。

它不覆盖 refresh/logout、Redis 轮换/吊销、LDAP/OAuth、认证审计查询、角色权限/数据范围/AI 工具权限写接口或前端 UI。上述能力需要结合单元测试、专项接口测试和 UI 验收分别记录。

本轮仅静态复核，未重新执行这些测试或脚本。
