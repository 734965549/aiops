# Web API 调用说明

前端 API 封装位于 `web/src/api/`，需鉴权的请求必须统一经过 `request.ts` 导出的 `http`。`identity.ts` 中登录、刷新、OAuth 等**公开接口**使用独立 `publicClient`（独立 axios 实例 + `unwrapPublic`），避免与鉴权拦截器的循环依赖；其余模块不得自行创建 axios 实例。后端成功响应使用 `code === "OK"`，前端不要再判断数字 `0`。

全项目调用关系见 `docs/AI运维平台整体流程与调用关系.md`。前端只负责传业务 ID、展示脱敏结果、发起确认动作；权限、风险、审计、凭据保存和真实执行全部由后端 application / Execution 状态机约束。

## 请求与错误处理（`request.ts`）

- 401：尝试 `auth.refresh()`，失败则登出并跳转 `/login`。
- 403：`Message.warning`；`PERMISSION_DENIED` 时提示检查权限迁移与角色绑定。
- 409 `ALREADY_EXISTS` 且 `POST /api/assets/sync` 返回 `sync already in progress for this account`：`request.ts` 不弹全局错误，由页面用 `isAssetSyncInProgressError` 统一提示“该账号正在同步，请稍后重试”。
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
| `fetchOAuthAuthorizeURL` | `GET /api/identity/oauth/:provider_id/authorize` | 公开；获取 OAuth/OIDC 授权跳转 URL 与 state |
| `completeOAuthCallback` | `POST /api/identity/oauth/:provider_id/callback` | 公开；OAuth 回调换发 TokenPair |
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
| `listApplications` / `createApplication` / `updateApplication` / `deleteApplication` | `/api/assets/applications` | 应用注册表 CRUD；`listApplications` 返回 `PageResult<Application>`，支持 `page` / `page_size` |
| `listResources` / `createResource` / `updateResource` / `deleteResource` | `/api/assets/applications/:application_id/resources`、`/api/assets/resources` | 资源注册表 CRUD，云同步字段只由后端同步链路写入；`listResources` 按 `application_id` 返回 `PageResult<Resource>`，支持 `page` / `page_size` / `cloud_resource_type` / `region` / `sync_status` 服务端筛选 |
| `listMatchRules` / `createMatchRule` / `updateMatchRule` / `deleteMatchRule` | `/api/assets/match-rules` | 匹配规则 CRUD；`listMatchRules` 返回 `PageResult<MatchRule>` |
| `triggerAssetSync` | `POST /api/assets/sync` | 触发云资源同步，立即返回 `running` 批次 |
| `pollSyncBatch` | 轮询 `GET /api/assets/sync/batches/:batch_id` | 触发后轮询到终态（`success`/`partial`/`failed`）或超时（默认 10 分钟）抛 `SyncStillRunningError`；`hybrid` 下只要任一增强失败就应落 `partial`；支持 `shouldStop` 取消（组件卸载） |
| `listSyncBatches` / `getSyncBatch` | `/api/assets/sync/batches` | 查询同步批次，`listSyncBatches` 返回 `PageResult<SyncBatch>` |
| `getSyncBatchSummaryDisplay` | 纯前端辅助 | 从 `SyncBatchSummary` 提取列表页展示字段（sync_mode、resource_group、计数等），避免主列表被诊断字段淹没 |
| `getSyncBatchNotice` | 纯前端辅助 | 按批次 `status` 生成 `SyncBatchNotice`（`success`/`warning`/`error` 类型 + 中文摘要文案） |

页面：`views/assets/index.vue`。契约：`ops/cloud-observability-contract.md` §5.5、`ops/huawei-ces-sync-contract.md` §15。

分页约定：`page` 从 1 开始，默认 `page_size=20`，后端最大按 `100` 处理。上述 `list*` 函数只返回当前页，不能当作完整应用/资源字典；下拉选择、深链回显或跨页定位需要额外维护已选项，或后续补充按业务 ID 查询/搜索接口。

云同步字段由后端写入，前端只读展示：`source`、`integration_account_id`、`cloud_resource_id`、`cloud_resource_type`、`region`、`sync_status`、`last_synced_at`、`sync_batch_id`、`labels`。未知 `cloud_resource_type` 必须原样展示，不能在前端丢弃。`sync_status=stale` 应明显标识。资源列表的 `cloud_resource_type`、`region`、`sync_status` 筛选必须经 `listResources` 透传到后端，不能做跨页本地假筛选。

`Resource.labels` 来源于后端 `asset_resource.labels`，页面只读展示 CES `namespace/dim_name/resource_group`、已生效 hybrid 增强字段以及未知 key；EVS 当前仅承诺 CES 基础 labels，`volume_type/size_gb/attached_to` 等详情增强尚未支持；VPC 子资源（EIP/带宽/子网/对等连接）与 native VPC `subnet_count` 详情增强已支持。未知 key 不丢弃，敏感 key 名称命中 `secret/token/password/key/authorization/credential` 等兜底掩码。

同步批次展示约定：`status` 至少区分 `running` / `success` / `partial` / `failed`；`summary` 是正式结构化摘要，包含 `ces_total`、`raw_fetched_count`、`mapped_count`、`unique_discovered_count`、`persisted_count`、`duplicate_count`、`persist_failed_count`、`discovered_count`、`failed_scopes`、`enriched_count`、`enrichment_failed_count`、`enrichment_failed_types`、`enrichment_warnings`、`enrichment_stage_error`、`writeback_failed_count` 等字段，以及 `scopes[]` 逐 region/project/resource_group 明细。`enrichment_failed_count` 始终等于去重后的 `enrichment_failed_types` 长度（不变式）；`enrichment_warnings` 记录 best-effort 增强缺失，不影响批次状态；增强阶段整体致命错误由 `enrichment_stage_error` 记录，label 回写失败由 `writeback_failed_count` 记录，两者驱动 `partial` 判定但不递增 `enrichment_failed_count`。页面应优先读取 `summary`，多区域失败排查必须读取 `scopes[]`（包含 `region`、`project_id`、`sync_mode`、`resource_group_id`、`resource_group_name`、各类失败类型与计数）。`message` 必须保留 tooltip 或可展开展示，仅作为人类可读排查说明；只允许对历史旧数据解析 `message` 兜底。`partial` 表示部分资源或增强信息失败，不等同于整个同步失败，UI 不应把它展示成“失败”。

触发同步错误提示约定：同账号已有 `running` 批次时，后端返回 409 `ALREADY_EXISTS` 与 `message=sync already in progress for this account`。页面必须通过 `asset.ts` 的 `isAssetSyncInProgressError` 判断该错误，并提示“该账号正在同步，请稍后重试”；不要只依赖按钮 loading，也不要直接展示英文后端 message。

异步同步轮询约定：`triggerAssetSync` 立即返回 `running` 批次（后端后台 goroutine 执行同步），页面须用 `pollSyncBatch(batch_id)` 轮询到终态再展示结果。轮询默认 2 秒间隔、10 分钟上限；超时抛 `SyncStillRunningError`，页面应提示「同步仍在进行，可在同步批次页查看」。组件卸载时通过 `shouldStop` 取消轮询，避免泄漏。全局 30 秒超时不影响同步：触发是瞬时 POST，轮询是快速 GET。

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
| `analyzeAlert` | `POST /api/ai/analyze-alert` | 告警上下文 AI 分析；返回 summary、risk_level、recommendations 等 |

页面：`views/ai-assistant/index.vue`、`views/alerts/index.vue`（告警详情先 `requestAlertAIAnalysis` 写时间线，再 `analyzeAlert` 取结果展示）。契约：`ops/ai-contract.md`。

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
| `deleteIntegrationAccount` | `DELETE /api/integrations/accounts/:account_id` | 软删除账号行；凭据密文一并硬删除 |
| `checkIntegrationAccount` | `POST /api/integrations/accounts/:account_id/check` | 连通性测试 |

页面：`views/integrations/index.vue`。契约：`ops/cloud-observability-contract.md` §4、`ops/huawei-ces-sync-contract.md` §15.1。

调用链：页面表单 -> `integration.ts` -> Integration AccountService -> CredentialVault / Provider checker。前端不保存明文凭据。`extra_config` 仅用于 provider 非敏感扩展配置，例如华为云 `sync_mode/resource_group_name/max_resources`；密钥、Token、AK/SK、密码仍只能通过 `credential` 写入。

### 华为云 CES 资源同步前端契约

`huawei_cloud` 账号的 CES 同步配置统一写入 `extra_config`，不要平铺为账号顶层字段。TypeScript 类型以 `integration.ts` 中的 `HuaweiCloudExtraConfig`、`HuaweiCloudRegionProject`、`HuaweiCloudSyncMode` 为准。

| 字段 | 类型 | 默认/建议值 | 提交与回显规则 | 说明 |
| --- | --- | --- | --- | --- |
| `sync_mode` | `ces` / `hybrid` / `native` | `ces` | 新建 `huawei_cloud + ak_sk` 推荐默认提交 `ces`；编辑时回显后端值 | `ces` 按**指定 CES 资源分组**口径同步（默认候选名“全部资源”需预先创建，未命中即失败）；`hybrid` 先 CES 后原生 API 增强，EVS 详情增强尚未支持，VPC 子资源（EIP/带宽/子网/对等连接）详情增强已支持；`native` 仅旧路径兼容，不承诺与资源分组数量一致 |
| `resource_group_name` | `string` | 空（占位提示“全部资源”） | 空值可省略；填写后写入 `extra_config` | 留空时后端按 `ops/huawei-ces-sync-contract.md` §8.4 step 3 依次尝试默认候选名（全部资源/All resources/All Resources）；显式填写则精确匹配，未指定 `resource_group_id` 时未命中即失败 |
| `resource_group_id` | `string` | 空 | 可选；填写后写入 `extra_config` | 指定后后端优先使用该资源组，优先级高于 `resource_group_name` |
| `enterprise_project_id` | `string` | 空 | 可选；支持 `all_granted_eps` | 企业项目过滤或授权范围控制 |
| `max_resources` | `number` | `20000` | 只提交正整数；空值可省略让后端兜底 | 单区域单次同步保护上限，按 region 独立计额；多区域账号实际可同步 `区域数 × max_resources` |
| `region_projects` | `{ region: string; project_id: string; resource_group_id?: string; resource_group_name?: string }[]` | 空数组 | 过滤空行后提交；编辑时按数组回显 | 多区域 `region -> project_id` 映射，每项可选填 `resource_group_id` / `resource_group_name`；未配置的区域后端回落账号顶层 `project_id`，未配置的资源组后端回落全局 `resource_group_id` / `resource_group_name` |

前端实现要求：

- 创建/编辑账号时必须保留后端返回的 `extra_config` 中未知 key，不能因为页面未渲染而丢弃。
- 编辑账号时回显 `extra_config`；未修改凭据时不要提交 `credential`，后端会保留原凭据。
- `extra_config` 不允许放 AK/SK、Token、密码、Authorization header 或任何密钥类字段。
- 多区域账号缺少 `region_projects` 时，页面应提示未配置区域会回落使用账号 `project_id`。
- `sync_mode=native` 需要提示“兼容旧路径，不保证与 CES 控制台全部资源数量一致”。
- `sync_mode=hybrid` 需要提示“增强失败只影响详情丰富度，不影响 CES 基础资源入库；EVS 详情增强尚未支持，VPC 子资源（EIP/带宽/子网/对等连接）详情增强已支持”。
- `views/integrations/index.vue` 已渲染上述字段；后续若调整 UI，仍必须保持本节 `extra_config` 写入、回显、未知 key 保留和凭据不回显规则。

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
