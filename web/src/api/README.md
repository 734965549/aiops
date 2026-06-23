# Web API 调用说明

封装位于 `web/src/api/`，统一经 `request.ts` 解包 `code === "OK"` 的响应。

全项目调用关系见 `docs/AI运维平台整体流程与调用关系.md`。前端只负责传业务 ID、展示脱敏结果同发起确认动作；权限、风险、审计、凭据保存同真实执行全部由后端 application / Execution 状态机约束。

## Identity（`identity.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `login` | `POST /api/identity/login` | 公开；返回 TokenPair |
| `loginExternal` | `POST /api/identity/login/external` | 公开；LDAP/AD 域登录 |
| `fetchLoginProviders` | `GET /api/identity/login/providers` | 公开；已启用身份源 |
| `refresh` | `POST /api/identity/refresh` | 公开；凭 refresh_token 换发 |
| `logout` | `POST /api/identity/logout` | 公开；吊销 refresh token |
| `authorize` | `POST /api/identity/authorize` | 需 Bearer + RBAC |

契约：`ops/auth-contract.md`、`ops/identity-api-contract.md`。

## Identity 管理员（`identity-admin.ts`）

需管理员权限 `app:identity.external_identities:create` / `app:identity.users:create`（迁移 `0004`）。

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `connectLDAPSession` | `POST /api/identity/admin/ldap/connect` | 填写 LDAP/AD 连接，建立浏览会话 |
| `closeLDAPSession` | `DELETE /api/identity/admin/ldap/sessions/:id` | 关闭会话 |
| `browseLDAPOrganizations` | `GET .../sessions/:id/organizations` | 浏览 OU |
| `previewLDAPUsers` | `GET .../sessions/:id/users` | 预览可导入用户 |
| `importLDAPUsers` | `POST .../sessions/:id/import` | 勾选/整 OU 导入并绑定角色 |
| `fetchRoles` | `GET /api/identity/roles` | 导入时选择角色 |

页面：`views/identity/ldap-import/index.vue`（侧栏「域账号导入」）。

契约：`ops/identity-api-contract.md` §8、`ops/auth-contract.md` §8。

## 系统（`system.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `fetchVersion` | `GET /version` | 版本信息 |
| `fetchReadiness` | `GET /readyz` | 就绪与迁移状态 |
| `fetchCurrentUser` | `GET /api/identity/me` | 需 Bearer + RBAC |

## AI（`ai.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listProviders` | `GET /api/ai/providers` | Provider 列表 |
| `upsertProvider` | `POST /api/ai/providers` | 新增或更新 |
| `deleteProvider` | `DELETE /api/ai/providers/:id` | 删除 |
| `invokeTool` | `POST /api/ai/tools/invoke` | 工具调用；成功响应内须检查 `allowed` |

契约：`ops/ai-contract.md`。页面实现：`views/ai-assistant/index.vue`。

## Audit（`audit.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `fetchAudits` | `GET /api/audits` | 操作审计分页查询，支持 `resource_type` / `resource_id` / `user_id` / `action` 筛选 |

页面：`views/audits/index.vue`。权限：`app:audits:read`。

## Integration（`integration.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listIntegrationAccounts` | `GET /api/integrations/accounts` | 分页列表，需 `app:integrations:read` |
| `getIntegrationAccount` | `GET /api/integrations/accounts/:account_id` | 账号详情 |
| `createIntegrationAccount` | `POST /api/integrations/accounts` | 创建，需 `app:integrations:create` |
| `updateIntegrationAccount` | `PUT /api/integrations/accounts/:account_id` | 更新；`credential` 省略保留原凭据 |
| `deleteIntegrationAccount` | `DELETE /api/integrations/accounts/:account_id` | 软删除 |
| `checkIntegrationAccount` | `POST /api/integrations/accounts/:account_id/check` | 连通性测试 |

页面：`views/integrations/index.vue`。契约：`ops/cloud-observability-contract.md` §4。

调用链：页面表单 -> `integration.ts` -> `request.ts` -> `/api/integrations/accounts` -> Integration AccountService -> CredentialVault / Provider checker。前端唔保存明文凭据，更新时若用户唔填 `credential` 就由后端保留原凭据引用。

## Asset（`asset.ts`）
| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listApplications` / `createApplication` / `updateApplication` / `deleteApplication` | `/api/assets/applications` | 应用注册表 CRUD，需 `app:assets:read/write` |
| `listResources` / `createResource` / `updateResource` / `deleteResource` | `/api/assets/resources` | 资源注册表 CRUD，云同步字段只由后端同步链路写入 |
| `triggerAssetSync` | `POST /api/assets/sync` | 触发云资源同步，需 `app:assets:write` |
| `listSyncBatches` / `getSyncBatch` | `/api/assets/sync/batches` | 查询同步批次，需 `app:assets:read` |

页面：`views/assets/index.vue`。契约：`ops/cloud-observability-contract.md` §5.5。
调用链：页面触发 -> `asset.ts` -> Asset SyncService -> Observability discovery port -> Provider Adapter。同步失败或局部失败不得破坏 P0 告警匹配闭环。

## Observability（`observability.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `queryMetrics` | `POST /api/observability/metrics/query` | 指标时序，需 `app:observability:read` |
| `searchLogs` | `POST /api/observability/logs/search` | 日志搜索（脱敏摘要） |
| `queryTraces` | `POST /api/observability/traces/query` | 链路 Span |
| `queryTopology` | `GET /api/observability/topology` | 服务拓扑 |

页面：`views/observability/index.vue`。契约：`ops/cloud-observability-contract.md` §5。

调用链：页面查询条件 -> `observability.ts` -> Observability QueryService -> IntegrationAccountPort -> ProviderRegistry -> fake / Huawei / SigNoz / Prometheus adapter。返回结果要以 `evidence_id` 串返巡检同审计。

## Inspection（`inspection.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listPolicies` / `createPolicy` / `updatePolicy` / `deletePolicy` | `/api/inspections/policies` | 巡检策略 CRUD，需 `app:inspections:write`（读列表需 read） |
| `triggerRun` | `POST /api/inspections/policies/:policy_id/runs` | 手动触发巡检 |
| `getRun` / `listRuns` | `/api/inspections/runs` | 运行状态与时间线 |
| `listFindings` | `GET /api/inspections/findings` | 发现与嵌套建议，含 `evidence_refs` |

页面：`views/inspections/index.vue`。契约：`ops/cloud-observability-contract.md` §6。迁移依赖 `0020`。

调用链：策略/运行操作 -> `inspection.ts` -> Inspection RunService -> ObservabilityQueryPort -> EvidenceAnalyzer -> Finding / Recommendation。建议转执行时只创建 Execution Task，唔会喺前端或 AI 侧直接执行。

## 错误处理（`request.ts`）

- 401：尝试 `auth.refresh()`，失败则登出并跳转 `/login`
- 403：`Message.warning`，`PERMISSION_DENIED` 时提示检查迁移 `0002` / `0004`
- 503 / `UNAVAILABLE`：服务未就绪提示

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `VITE_API_BASE` | axios baseURL；dev 默认 `/`（走 Vite proxy）；生产见 `deployments/README.md` |
## Identity 权限管理（`identity-admin.ts`）

页面：`views/identity/access-control/index.vue`，路由 `/identity/access-control`。

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `fetchUsers` | `GET /api/identity/admin/users` | 用户分页查询，需 `app:identity.users:read` |
| `fetchUserRoles` | `GET /api/identity/admin/users/:user_id/roles` | 查询用户角色绑定，返回 `source` |
| `replaceUserRoles` | `PUT /api/identity/admin/users/:user_id/roles` | 仅替换 manual 来源角色，需 `app:identity.access_control:write` |
| `fetchPermissions` | `GET /api/identity/permissions` | 权限字典 |
| `fetchDataScopes` | `GET /api/identity/data-scopes` | 数据范围字典 |
| `fetchAIToolPermissions` | `GET /api/identity/ai-tool-permissions` | AI 工具权限字典 |
| `fetchRolePermissions` / `replaceRolePermissions` | `GET/PUT /api/identity/admin/roles/:role_id/permissions` | 查询/替换角色权限 |
| `fetchRoleDataScopes` / `replaceRoleDataScopes` | `GET/PUT /api/identity/admin/roles/:role_id/data-scopes` | 查询/替换角色数据范围 |
| `fetchRoleAIToolPermissions` / `replaceRoleAIToolPermissions` | `GET/PUT /api/identity/admin/roles/:role_id/ai-tool-permissions` | 查询/替换角色 AI 工具权限 |

403 时页面保留结构并显示权限不足提示；动态菜单权限不在本轮实现。
