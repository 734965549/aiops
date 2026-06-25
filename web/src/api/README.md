# Web API 调用说明

前端 API 封装位于 `web/src/api/`，所有请求必须统一经过 `request.ts`。后端成功响应使用 `code === "OK"`，前端不要再判断数字 `0`。

全项目调用关系见 `docs/AI运维平台整体流程与调用关系.md`。前端只负责传业务 ID、展示脱敏结果、发起确认动作；权限、风险、审计、凭据保存和真实执行全部由后端 application / Execution 状态机约束。

## 请求与错误处理（`request.ts`）

- 401：尝试 `auth.refresh()`，失败则登出并跳转 `/login`。
- 403：`Message.warning`；`PERMISSION_DENIED` 时提示检查权限迁移与角色绑定。
- 503 / `UNAVAILABLE`：服务未就绪提示。
- AI 工具调用即使 HTTP 200 且 `code="OK"`，也必须检查 `data.allowed`。

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

页面：`views/identity/ldap-import/index.vue`、`views/identity/access-control/index.vue`。

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `connectLDAPSession` | `POST /api/identity/admin/ldap/connect` | 填写 LDAP/AD 连接，建立浏览会话 |
| `closeLDAPSession` | `DELETE /api/identity/admin/ldap/sessions/:id` | 关闭会话 |
| `browseLDAPOrganizations` | `GET .../sessions/:id/organizations` | 浏览 OU |
| `previewLDAPUsers` | `GET .../sessions/:id/users` | 预览可导入用户 |
| `importLDAPUsers` | `POST .../sessions/:id/import` | 勾选/整 OU 导入并绑定角色 |
| `fetchUsers` | `GET /api/identity/admin/users` | 用户分页查询 |
| `fetchUserRoles` / `replaceUserRoles` | `GET/PUT /api/identity/admin/users/:user_id/roles` | 查询或替换用户 manual 来源角色 |
| `fetchRoles` | `GET /api/identity/roles` | 角色字典 |
| `fetchPermissions` | `GET /api/identity/permissions` | 权限字典 |
| `fetchDataScopes` | `GET /api/identity/data-scopes` | 数据范围字典 |
| `fetchAIToolPermissions` | `GET /api/identity/ai-tool-permissions` | AI 工具权限字典 |
| `fetchRolePermissions` / `replaceRolePermissions` | `GET/PUT /api/identity/admin/roles/:role_id/permissions` | 查询或替换角色权限 |
| `fetchRoleDataScopes` / `replaceRoleDataScopes` | `GET/PUT /api/identity/admin/roles/:role_id/data-scopes` | 查询或替换角色数据范围 |
| `fetchRoleAIToolPermissions` / `replaceRoleAIToolPermissions` | `GET/PUT /api/identity/admin/roles/:role_id/ai-tool-permissions` | 查询或替换角色 AI 工具权限 |

403 时页面必须保留结构并显示权限不足提示；动态菜单权限不在当前阶段内。

## 系统（`system.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `fetchVersion` | `GET /version` | 版本信息 |
| `fetchReadiness` | `GET /readyz` | 就绪与迁移状态 |
| `fetchCurrentUser` | `GET /api/identity/me` | 需 Bearer + RBAC |

## Dashboard（`dashboard.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `fetchDashboardSummary` | `GET /api/dashboard/summary` | 告警、执行、资产、Runbook 聚合摘要 |

页面：`views/dashboard/index.vue`。权限：`app:dashboard:read`。

## Alert（`alert.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listAlerts` | `GET /api/alerts` | 告警分页查询 |
| `getAlert` | `GET /api/alerts/:alert_id` | 告警详情与时间线 |
| `acknowledgeAlert` | `POST /api/alerts/:alert_id/acknowledge` | 认领告警 |
| `assignAlert` | `POST /api/alerts/:alert_id/assign` | 转派负责人 |
| `startProcessingAlert` | `POST /api/alerts/:alert_id/start-processing` | 开始处理 |
| `recoverAlert` | `POST /api/alerts/:alert_id/recover` | 人工标记恢复 |
| `closeAlert` | `POST /api/alerts/:alert_id/close` | 关闭告警 |
| `silenceAlert` / `unsilenceAlert` | `POST /api/alerts/:alert_id/silence` / `unsilence` | 静默或取消静默 |
| `commentAlert` | `POST /api/alerts/:alert_id/comments` | 追加时间线评论 |
| `requestAlertAIAnalysis` | `POST /api/alerts/:alert_id/ai-analysis` | 请求 AI 分析，写时间线与审计 |
| `listAlertSources` / `getAlertSource` | `GET /api/alerts/sources` | 告警源查询 |
| `createAlertSource` / `updateAlertSource` / `deleteAlertSource` | `/api/alerts/sources` | 告警源维护；secret 只写入不回显 |

页面：`views/alerts/index.vue`。契约：`ops/alert-contract.md`。

调用链：Webhook / 页面操作 -> Alert Handler -> AlertService / IngestService -> Asset matcher -> Audit recorder。告警状态流转必须遵守 domain 状态机，不能在页面里模拟。

## Asset（`asset.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listApplications` / `createApplication` / `updateApplication` / `deleteApplication` | `/api/assets/applications` | 应用注册表 CRUD |
| `listResources` / `createResource` / `updateResource` / `deleteResource` | `/api/assets/resources` | 资源注册表 CRUD，云同步字段只由后端同步链路写入 |
| `triggerAssetSync` | `POST /api/assets/sync` | 触发云资源同步 |
| `listSyncBatches` / `getSyncBatch` | `/api/assets/sync/batches` | 查询同步批次 |

页面：`views/assets/index.vue`。契约：`ops/cloud-observability-contract.md` §5.5。

调用链：页面触发 -> `asset.ts` -> Asset SyncService -> Observability discovery port -> Provider Adapter。同步失败或局部失败不得破坏 P0 告警匹配闭环。

## Runbook（`runbook.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listRunbookRecommendations` | `GET /api/runbooks/recommendations?alert_id=` | 按告警上下文推荐模板 |
| `listRunbookTemplates` | `GET /api/runbooks/templates` | 模板分页查询 |
| `getRunbookTemplate` | `GET /api/runbooks/templates/:template_id` | 模板详情与步骤 |
| `updateRunbookTemplate` | `PUT /api/runbooks/templates/:template_id` | 更新模板状态或字段 |

页面：`views/runbooks/index.vue`。契约：`ops/runbook-contract.md`。

调用链：告警详情 / Runbook 页面 -> Runbook TemplateService -> Alert port / Execution adapter。多步骤任务必须保留步骤结构，不能在前端拼成纯文本。

## Execution（`execution.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listExecutionTasks` | `GET /api/executions/tasks` | 执行任务分页查询 |
| `getExecutionTask` | `GET /api/executions/tasks/:task_id` | 任务详情与步骤 |
| `createExecutionTask` | `POST /api/executions/tasks` | 创建任务；中高风险进入 `pending_confirm` |
| `confirmExecutionTask` | `POST /api/executions/tasks/:task_id/confirm` | 输入确认文本，推进到 `pending_execute` |
| `executeTask` | `POST /api/executions/tasks/:task_id/execute` | 执行已确认任务 |

页面：`views/executions/index.vue`。契约：`ops/execution-contract.md`、`ops/execution-agent-contract.md`。

调用链：Runbook / AI / Inspection 只能创建 Execution Task；真实执行必须由 Execution 状态机、权限、风险、确认文本和审计共同约束。

## AI（`ai.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listProviders` | `GET /api/ai/providers` | Provider 列表 |
| `upsertProvider` | `POST /api/ai/providers` | 新增或更新 |
| `deleteProvider` | `DELETE /api/ai/providers/:id` | 删除 |
| `invokeTool` | `POST /api/ai/tools/invoke` | 工具调用；成功响应内须检查 `allowed` |

页面：`views/ai-assistant/index.vue`。契约：`ops/ai-contract.md`。

## Audit（`audit.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `fetchAudits` | `GET /api/audits` | 操作审计分页查询，支持 `resource_type` / `resource_id` / `user_id` / `action` 筛选 |

页面：`views/audits/index.vue`。权限：`app:audits:read`。

## Integration（`integration.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listIntegrationAccounts` | `GET /api/integrations/accounts` | 分页列表 |
| `getIntegrationAccount` | `GET /api/integrations/accounts/:account_id` | 账号详情 |
| `createIntegrationAccount` | `POST /api/integrations/accounts` | 创建账号；凭据只写入不回显 |
| `updateIntegrationAccount` | `PUT /api/integrations/accounts/:account_id` | 更新；`credential` 省略时后端保留原凭据 |
| `deleteIntegrationAccount` | `DELETE /api/integrations/accounts/:account_id` | 软删除 |
| `checkIntegrationAccount` | `POST /api/integrations/accounts/:account_id/check` | 连通性测试 |

页面：`views/integrations/index.vue`。契约：`ops/cloud-observability-contract.md` §4。

调用链：页面表单 -> `integration.ts` -> Integration AccountService -> CredentialVault / Provider checker。前端不保存明文凭据。

## Observability（`observability.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `queryMetrics` | `POST /api/observability/metrics/query` | 指标时序 |
| `searchLogs` | `POST /api/observability/logs/search` | 日志搜索，返回脱敏摘要 |
| `queryTraces` | `POST /api/observability/traces/query` | 链路 Span |
| `queryTopology` | `GET /api/observability/topology` | 服务拓扑 |

页面：`views/observability/index.vue`。契约：`ops/cloud-observability-contract.md` §5。

调用链：页面查询条件 -> `observability.ts` -> Observability QueryService -> IntegrationAccountPort -> ProviderRegistry -> fake / Huawei / SigNoz / Prometheus adapter。返回结果要通过 `evidence_id` 串联巡检和审计。

## Inspection（`inspection.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listPolicies` / `createPolicy` / `updatePolicy` / `deletePolicy` | `/api/inspections/policies` | 巡检策略 CRUD |
| `triggerRun` | `POST /api/inspections/policies/:policy_id/runs` | 手动触发巡检 |
| `getRun` / `listRuns` | `/api/inspections/runs` | 运行状态与时间线 |
| `listFindings` | `GET /api/inspections/findings` | 发现与嵌套建议，含 `evidence_refs` |

页面：`views/inspections/index.vue`。契约：`ops/cloud-observability-contract.md` §6。迁移依赖 `0020`。

调用链：策略/运行操作 -> `inspection.ts` -> Inspection RunService -> ObservabilityQueryPort -> EvidenceAnalyzer -> Finding / Recommendation。建议转执行时只创建 Execution Task，不在前端或 AI 侧直接执行。

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `VITE_API_BASE` | axios baseURL；dev 默认 `/`（走 Vite proxy）；生产见 `deployments/README.md` |
