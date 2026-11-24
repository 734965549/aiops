# Web API 调用说明

前端 API 封装位于 `web/src/api/`，需鉴权的请求必须统一经过 `request.ts` 导出的 `http`。`identity.ts` 中登录、刷新、OAuth 等**公开接口**使用独立 `publicClient`（独立 axios 实例 + `unwrapPublic`），避免与鉴权拦截器的循环依赖；其余模块不得自行创建 axios 实例。后端成功响应使用 `code === "OK"`，前端不要再判断数字 `0`。

全项目调用关系见 `docs/AI运维平台整体流程与调用关系.md`。前端只负责传业务 ID、展示脱敏结果、发起确认动作；权限、风险、审计、凭据保存和真实执行全部由后端 application / Execution 状态机约束。

## 请求与错误处理（`request.ts`）

- 统一 axios 实例：`baseURL` 取 `VITE_API_BASE`（缺省 `/`，dev 由 Vite 把 `/api`、`/healthz` 等代理到后端 8080）；默认超时 30 秒，个别接口可覆盖（`analyzeAlert` 与对话同步封装均为 90 秒；流式对话走 fetch 不受该超时约束）。请求拦截器自动附 `Authorization: Bearer <token>`。
- 成功判断：HTTP 200 且 `code === "OK"` 才算成功；`code` 非 OK 时按业务失败弹 `Message.error` 并 reject `ApiHttpError(200, code)`。不能只看 HTTP 状态码，也不能判断数字 `0`。
- `ApiHttpError` 统一携带 `status/code/message/traceId`；用 `isApiHttpError` 类型守卫或 `getApiError` 提取，`traceId` 可用于审计中心按 trace 关联。
- 401：非登录/刷新/登出路径会做**一次** `auth.refresh()` 重试（`_retry` 标记防循环）；刷新失败或无 refresh token 时登出，并在非公开路由上带 `redirect` 跳转 `/login`。
- 403：`Message.warning`；`PERMISSION_DENIED` 时提示检查权限迁移与角色绑定。
- 409 两类业务冲突不弹全局错误：`POST /api/assets/sync` 的 `sync already in progress`（页面用 `isAssetSyncInProgressError` 提示）与 AI 对话消息 409（归档/幂等冲突由对话区给出中文说明）。
- 503 / `UNAVAILABLE`：服务未就绪提示。
- 无响应的网络错误统一 `Message.error`。
- `http<T>()` 返回解包后的 `data.data`；响应无 `data` 字段时得到 undefined，调用方需自行兜底，不能把 undefined 当有效业务值。
- `request.ts` 当前没有专项 Vitest；401 刷新、403 提示、409 静默语义依赖页面测试与人工验收。
- AI 工具调用即使 HTTP 200 且 `code="OK"`，也必须检查 `data.allowed`。

## SSE（`sse.ts`）

对话流式接口使用 POST + Bearer Token，不能用浏览器原生 EventSource，因此基于 fetch ReadableStream 自行解析：

- `SSEParser`：跨分片缓冲并归一化 CRLF/CR；按空行切帧，合并多行 `data`（`\n` 连接），保留 `event` 字段，忽略注释行与 `id/retry` 字段；`flush()` 兜底解析缺少结尾空行的最后一帧。
- `consumeSSE(response, onFrame, signal)`：逐帧回调；`onFrame` 抛错会 cancel 响应体并中断读取（收到 error 事件后立即停止）；每次 read 前检查 signal，主动取消抛 `AbortError`，`releaseLock` 在 finally 保证不泄漏。
- `isAbortError` 同时识别 `AbortError` 与 axios `CanceledError`。
- 当前唯一使用方是 `ai.ts` 的 `streamChatMessage`（`stage/tool/delta/answer/error` 五类事件，`: ping` 注释被解析器忽略）。
- 覆盖：`sse.spec.ts` 共 7 个用例（跨分片拼接、CRLF+注释、多行 data、flush、无 data 帧、取消识别）；`consumeSSE` 的读取与取消链路无专项测试，由 useChat.spec 间接覆盖。

## Identity（`identity.ts`）

公开接口的 `unwrapPublic` 会把 HTTP 错误（401/403/429/5xx 等）和网络错误统一转换为 `PublicApiError`（含 `status/code/message/trace_id`），`message` 优先取后端响应体；调用方用 `isPublicApiError` 判断。登录页对 `status === 401` 统一提示“账号或密码错误”（后端对用户不存在/密码错误/账号禁用统一返回 401，避免信息差）。

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `login` | `POST /api/identity/login` | 公开；返回 TokenPair；可选 `provider_id` 切换外部密码身份源 |
| `loginExternal` | `POST /api/identity/login/external` | 公开；LDAP/AD 域登录，必须传 `provider_id` |
| `fetchLoginProviders` | `GET /api/identity/login/providers` | 公开；已启用身份源，不含连接密码或 client_secret |
| `refresh` | `POST /api/identity/refresh` | 公开；凭 refresh_token 换发；Redis 模式轮换并作废旧 JTI |
| `logout` | `POST /api/identity/logout` | 公开；吊销当前 refresh token，不吊销既有 access token |
| `fetchOAuthAuthorizeURL` | `GET /api/identity/oauth/:provider_id/authorize` | 公开；获取 OAuth/OIDC 授权跳转 URL 与 state |
| `completeOAuthCallback` | `POST /api/identity/oauth/:provider_id/callback` | 公开；OAuth 回调换发 TokenPair |
| `authorize` | `POST /api/identity/authorize` | 需 Bearer + RBAC；拒绝时按 403 错误响应处理，不能假设 HTTP 200 + `allowed=false` |

登录页只暂存 OAuth state 与 provider 的对应关系到 sessionStorage，用于回调匹配；身份源仍由后端配置，外部用户必须已预置绑定。dev 无 Redis 时 refresh 轮换/吊销和 OAuth state 共享会降级，不能用该模式替代生产验收。

`stores/auth.ts` 负责 TokenPair、当前用户、并发 refresh 复用与 logout。**当前实现会将 access/refresh token 和用户摘要写入 localStorage，与项目“不将 Token 写入前端持久化状态”的安全约束存在差距，需单独整改。**

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

实现边界：

- LDAP 连接支持 `attr_subject`，须与后续登录身份源的主体属性一致；Bind 密码只进入约 30 分钟的服务端会话，不写前端本地存储。
- 用户角色 PUT 只替换 `manual` 来源，`ldap_import` 与 `external_group` 来源由页面保留并禁用编辑；三类角色授权 PUT 都是幂等全量替换，空数组表示清空。
- 权限写操作由服务端审计；当前审计失败不回滚授权变更，前端不能把“保存成功”解释为审计一定落库。
- 本地用户创建、外部身份手工预置、认证审计查询和身份源配置目前没有专门前端封装/管理页，不能仅凭权限管理页宣称这些 UI 已完成。

403 时页面必须保留结构并显示权限不足提示；当前路由只校验登录、侧栏为静态配置，动态菜单权限不在当前阶段内。

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

页面：`views/dashboard/index.vue`。权限：`app:dashboard:read`。契约：`ops/dashboard-contract.md`。

实现边界：

- `alerts.active_total/p0/p1` 只排除 closed；recovered 和 silenced 仍可能计入。
- `executions.recent` 最近最多 10 条，不限成功任务；`processing_alerts` 最多 5 条，实际包含 new/acknowledged/processing，不是仅 processing。
- 推荐由前端再取前 3 条告警调用 `listRunbookRecommendations`，每组最多展示 3 个模板；单条推荐失败会被忽略。
- 后端单项统计失败返回 0/空列表，响应没有 degraded/error 字段，真实 0 与降级 0 不可区分。
- 前端 summary 整体失败时指标显示 `—`，最近执行和推荐为空，当前没有专门错误 Alert；版本与 readiness 独立加载。
- 聚合只有静态 RBAC，没有对象级数据范围过滤。
- 当前没有独立 Dashboard E2E 或前端 Vitest；`e2e-identity-access.ps1` 只以 viewer 调用一次摘要，不逐项验证字段、降级或 UI。

## Alert（`alert.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listAlerts` | `GET /api/alerts` | 告警分页查询；`active_only=true` 只排除 closed |
| `getAlert` | `GET /api/alerts/:alert_id` | 告警详情与时间线，`related` 当前为空对象 |
| `acknowledgeAlert` | `POST /api/alerts/:alert_id/acknowledge` | 认领告警 |
| `assignAlert` | `POST /api/alerts/:alert_id/assign` | 转派负责人 |
| `startProcessingAlert` | `POST /api/alerts/:alert_id/start-processing` | 开始处理 |
| `recoverAlert` | `POST /api/alerts/:alert_id/recover` | 人工标记恢复 |
| `closeAlert` | `POST /api/alerts/:alert_id/close` | 关闭告警 |
| `silenceAlert` / `unsilenceAlert` | `POST /api/alerts/:alert_id/silence` / `unsilence` | 静默或取消静默；到期不会自动恢复状态 |
| `commentAlert` | `POST /api/alerts/:alert_id/comments` | 追加时间线评论 |
| `requestAlertAIAnalysis` | `POST /api/alerts/:alert_id/ai-analysis` | 请求 AI 分析，写时间线与审计；实际结果再调 `analyzeAlert` |
| `listAlertSources` / `getAlertSource` | `GET /api/alerts/sources` | 告警源查询 |
| `createAlertSource` / `updateAlertSource` / `deleteAlertSource` | `/api/alerts/sources` | 告警源维护；创建字段为 `id/secret`，不是 `source_id/webhook_secret` |
| `syncAlertSource` | `POST /api/alerts/sources/:source_id/sync` | 手动触发 CES 拉取；返回 `AlertSyncResult`，`status` 区分 `success/partial/failed` |

页面：`views/alerts/index.vue`（接入源弹窗拆分为 `views/alerts/components/AlertSourceManager.vue` 与 `AlertSourceForm.vue`）。契约：`ops/alert-contract.md`、`ops/huawei-ces-alert-sync-contract.md`。

实现边界：

- Webhook secret 必须在创建前由调用方安全保存；后端只存哈希，响应只有 `secret_masked`，不会返回可用于 ingest 的明文 Token。
- 当前创建/更新任何来源类型都要求 `app:alerts:ingest` + `app:integrations:read`；删除/查询只要求 `app:alerts:ingest`。拉取源还会校验账号数据范围。
- 手动同步是等待本轮结果的 API，HTTP `code=OK` 时仍须检查 `data.status` 和 `failed_scopes`；worker 默认关闭，真实 CES 验收不能用 fake E2E 替代。
- 告警列表/详情当前仅做 RBAC，未自动应用数据范围过滤；AI 分析的对象级数据范围校验是独立链路。
- 详情支持友好/原始字段、指标语义、完整 ID 复制、敏感值掩码和 matched/unregistered/ambiguous 资源解析；多候选不得默认选择。
- 告警 AI 弹窗目前展示风险、摘要、建议和引用，未完整展示 `diagnosis`；完整诊断卡片在 AI 助手，不能将弹窗视为已覆盖 RDS 结构化诊断展示。
- `/alerts?alert_id=` 支持深链；“问 AI 助手”写入当前告警上下文并打开会话选择。

调用链：Webhook / 页面操作 -> Alert Handler -> AlertService / IngestService -> Asset matcher -> Audit recorder。告警状态流转必须遵守 domain 状态机，不能在页面里模拟。

验证边界：`e2e-alert.ps1` 不含转派、静默、备注、无效 token、通用 Webhook 或完整 UI；`e2e-alert-sync.ps1` 使用 fake，不覆盖真实 CES、多实例、worker 定时调度及全部规则富化分支。

## Asset（`asset.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listApplications` / `createApplication` / `updateApplication` / `deleteApplication` | `/api/assets/applications` | 应用注册表 CRUD；`listApplications` 返回 `PageResult<Application>`，支持 `page` / `page_size` |
| `listResources` / `createResource` / `updateResource` / `deleteResource` | `/api/assets/applications/:application_id/resources`、`/api/assets/resources` | 资源注册表 CRUD，云同步字段只由后端同步链路写入；`listResources` 按 `application_id` 返回 `PageResult<Resource>`，支持 `page` / `page_size`、`cloud_resource_type` / `region` / `sync_status` / `source` 精确筛选、`keyword` 跨字段模糊筛选，以及 `sort_by` / `sort_order` 服务端排序（排序字段白名单见 `ops/cloud-observability-contract.md` §5.5.4）；筛选与排序均为服务端行为，前端不做跨页本地假筛选/假排序 |
| `getResourceFilterOptions` | `GET /api/assets/applications/:application_id/resource-facets` | 资源页云类型/Region 下拉字典；仅返回当前应用下非空去重值，按字典序升序；只作候选值，实际过滤仍走 `listResources` 的精确参数（契约 §5.5.5） |
| `listMatchRules` / `createMatchRule` / `updateMatchRule` / `deleteMatchRule` | `/api/assets/match-rules` | 匹配规则 CRUD；`listMatchRules` 返回 `PageResult<MatchRule>` |
| `triggerAssetSync` | `POST /api/assets/sync` | 触发云资源同步，立即返回 `running` 批次 |
| `pollSyncBatch` | 轮询 `GET /api/assets/sync/batches/:batch_id` | 触发后轮询到终态（`success`/`partial`/`failed`）或超时（默认 10 分钟）抛 `SyncStillRunningError`；`hybrid` 下只要任一增强失败就应落 `partial`；支持 `shouldStop` 取消（组件卸载） |
| `listSyncBatches` / `getSyncBatch` | `/api/assets/sync/batches` | 查询同步批次，`listSyncBatches` 返回 `PageResult<SyncBatch>` |
| `getSyncBatchSummaryDisplay` | 纯前端辅助 | 从 `SyncBatchSummary` 提取列表页展示字段（sync_mode、resource_group、计数等），避免主列表被诊断字段淹没 |
| `getSyncBatchNotice` | 纯前端辅助 | 按批次 `status` 生成 `SyncBatchNotice`（`success`/`warning`/`error` 类型 + 中文摘要文案） |

页面：`views/assets/index.vue`；拓扑 API 独立封装于 `asset-topology.ts`，页面为 `views/assets/topology/index.vue`（路由 `/assets/topology`）。契约：`ops/cloud-observability-contract.md` §5.5、`ops/huawei-ces-sync-contract.md` §15。

分页约定：`page` 从 1 开始，默认 `page_size=20`，后端最大按 `100` 处理。上述 `list*` 函数只返回当前页，不能当作完整应用/资源字典；下拉选择、深链回显或跨页定位需要额外维护已选项，或后续补充按业务 ID 查询/搜索接口。

排序边界：后端白名单已包含 `created_at` 与 `updated_at`，但前端 `SORTABLE_RESOURCE_FIELDS` 当前只开放 `created_at` 等字段，`updated_at` 可展示和导出、暂不提供表头排序。不能写成“后端尚未支持 updated_at”，也不能默认前端已开放。

云同步字段由后端写入，前端只读展示：`source`、`integration_account_id`、`cloud_resource_id`、`cloud_resource_type`、`region`、`sync_status`、`last_synced_at`、`sync_batch_id`、`labels`。未知 `cloud_resource_type` 必须原样展示，不能在前端丢弃。`sync_status=stale` 应明显标识。资源列表的 `cloud_resource_type`、`region`、`sync_status` 筛选必须经 `listResources` 透传到后端，不能做跨页本地假筛选。

`Resource.labels` 来源于后端 `asset_resource.labels`，页面只读展示 CES `namespace/dim_name/resource_group`、已生效 hybrid 增强字段以及未知 key；EVS 当前仅承诺 CES 基础 labels，`volume_type/size_gb/attached_to` 等详情增强尚未支持；VPC 子资源（EIP/带宽/子网/对等连接）与 native VPC `subnet_count` 详情增强已支持。未知 key 不丢弃，敏感 key 名称命中 `secret/token/password/key/authorization/credential` 等兜底掩码。

同步批次展示约定：`status` 至少区分 `running` / `success` / `partial` / `failed`；批次级计数为 `created_count`、`updated_count`、`completed_count`（等于 `created_count + updated_count`）、`stale_count`、`failed_count`。`summary` 是正式结构化摘要，包含 `ces_total`、`raw_fetched_count`、`mapped_count`、`unique_discovered_count`、`persisted_count`、`duplicate_count`、`persist_failed_count`、`discovered_count`、`failed_scopes`、`enriched_count`、`enrichment_failed_count`、`enrichment_failed_types`、`enrichment_warnings`、`enrichment_stage_error`、`writeback_failed_count`、`resource_group_count`、`dimension_group_count`、`group_coverage_mismatches`、`scope_incomplete` 等字段，以及 `scopes[]` 逐 region/project/resource_group 明细。`enrichment_failed_count` 始终等于去重后的 `enrichment_failed_types` 长度（不变式）；`enrichment_warnings` 记录 best-effort 增强缺失，不影响批次状态；增强阶段整体致命错误由 `enrichment_stage_error` 记录，label 回写失败由 `writeback_failed_count` 记录，两者驱动 `partial` 判定但不递增 `enrichment_failed_count`。`resource_group_count > 1` 表示聚合口径（全部资源组并集），`scope_incomplete=true` 表示有资源组未被覆盖、该 region 不标记 stale。`dimension_group_count` 为本轮覆盖的 dimension 级资源组数（按服务枚举 + 资源实际维度发现，顶层为各 scope 求和）；`group_coverage_mismatches` 非空表示对应 dimension 组声明总数大于实际发现且差值不可解释（组内未知 namespace 服务未被枚举），此时 `scope_incomplete=true`、该 region 禁止任何 stale。页面应优先读取 `summary`，多区域失败排查必须读取 `scopes[]`（包含 `region`、`project_id`、`sync_mode`、`resource_group_id`、`resource_group_name`、各类失败类型与计数）。`message` 必须保留 tooltip 或可展开展示，仅作为人类可读排查说明；只允许对历史旧数据解析 `message` 兜底。`partial` 表示部分资源或增强信息失败，不等同于整个同步失败，UI 不应把它展示成“失败”。

触发同步错误提示约定：同账号已有 `running` 批次时，后端返回 409 `ALREADY_EXISTS` 与 `message=sync already in progress for this account`。页面必须通过 `asset.ts` 的 `isAssetSyncInProgressError` 判断该错误，并提示“该账号正在同步，请稍后重试”；不要只依赖按钮 loading，也不要直接展示英文后端 message。

异步同步轮询约定：`triggerAssetSync` 立即返回 `running` 批次（后端后台 goroutine 执行同步），页面须用 `pollSyncBatch(batch_id)` 轮询到终态再展示结果。轮询默认 2 秒间隔、10 分钟上限；超时抛 `SyncStillRunningError`，页面应提示「同步仍在进行，可在同步批次页查看」。组件卸载时通过 `shouldStop` 取消轮询，避免泄漏。全局 30 秒超时不影响同步：触发是瞬时 POST，轮询是快速 GET。

调用链：页面触发 -> `asset.ts` -> Asset SyncService -> Observability discovery port -> Provider Adapter。同步失败或局部失败不得破坏 P0 告警匹配闭环。

当前同步实现差距：

- Sync 触发只有 `app:assets:write` 和账号 enabled/assets capability 校验，未按早期契约接入 owner_team/region 数据范围；批次列表没有 owner_team 过滤，详情也没有完整对象级校验。
- 审计统一使用 `action=sync`，不是三个独立生命周期 action；没有独立即时 `sync_started` 审计或完整嵌套 summary/scopes 审计载荷。
- `triggered_by` 未在 TriggerSync 写入，过期 running 只在下一次同账号触发时内联 reap；未装配周期后台 reaper。
- 新建云应用名仍可能为 `<provider>-cloud`，历史迁移归一化不代表后续新建名称附带 account_id。

## Asset Topology / CMDB（`asset-topology.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `getAssetTopology` | `GET /api/assets/topology` | CMDB 应用归属、手工与云派生关系图；按 `application_id` 或 `root_resource_id + depth + direction` 查询，支持跨应用链路；必须展示 `meta.truncated` 与 `unresolved_relation_count` |
| `listAssetResourceRelations` | `GET /api/assets/resource-relations` | 按应用查询手工关系；跨应用关系可出现在源应用和目标应用列表中，不限同一应用 |
| `createAssetResourceRelation` / `deleteAssetResourceRelation` | `POST/DELETE /api/assets/resource-relations` | 创建/删除手工关系；允许跨应用，禁止自环和重复三元组；前端不伪造 `origin` 或云端关系 |
| `getAssetResourceResolution` | `GET /api/assets/resources/resolve` | 按云 ID、账号、Region 和云类型解析真实资源及应用；区分 `matched` / `unregistered` / `ambiguous`，多候选不得默认选择 |

页面：`views/assets/topology/index.vue`，路由 `/assets/topology`。拓扑数据来自 Asset 注册表、`asset_resource_relation` 和查询期已验证 labels 派生边，不使用 Observability fake 调用拓扑。

实现边界：手工关系类型为 `depends_on/attached_to/located_in/routes_to`；应用归属为 `origin=registry`、手工关系为 `manual`、云派生为 `cloud_sync`。根资源支持 1–3 层、上下游和循环保护；最多 299 个资源节点，超出必须展示截断提示。普通拓扑与解析路由当前只有静态 RBAC，没有自动数据范围过滤。拓扑没有独立 E2E 脚本，依赖后端单元/HTTP 测试和前端 Vitest。

契约：`ops/cloud-observability-contract.md` §5.5.6。

## Runbook（`runbook.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listRunbookRecommendations` | `GET /api/runbooks/recommendations?alert_id=` | 按告警上下文推荐模板；API 本身不限制告警状态，创建 Execution 时才强制 processing |
| `listRunbookTemplates` | `GET /api/runbooks/templates` | 模板分页查询，支持 page/page_size/keyword |
| `getRunbookTemplate` | `GET /api/runbooks/templates/:template_id` | 模板详情与有序步骤 |
| `updateRunbookTemplate` | `PUT /api/runbooks/templates/:template_id` | 全量更新模板并替换步骤；启停前先读取完整详情 |

页面：`views/runbooks/index.vue`。契约：`ops/runbook-contract.md`。

实现边界：

- 后端另有 `POST /templates` 和 `DELETE /templates/:template_id`，DELETE 实际是 enabled=false 软停用；当前前端没有 create/delete API 封装，也没有新建、完整编辑或删除 UI。
- 更新模板会全量替换步骤，历史 Execution 使用创建时保存的 runbook_snapshot，不随模板变化。
- 告警页只在 processing 显示推荐处置入口，支持任务名、dry-run 和 JSON object 参数；后端仍再次校验 processing、模板 enabled、步骤和 schema。
- 有效 dry-run 为任务 `dry_run && step.dry_run_supported`；模板 `default_dry_run` 只是 snapshot 元数据，不单独触发模拟。
- `command_spec_id`、`medium_selector` 和 Runbook→Agent 展开属于规划，当前步骤模型未实现。
- 当前没有独立 Runbook 前端 Vitest，不能把告警页其他测试当作 Runbook 交互覆盖。

调用链：告警详情 / Runbook 页面 -> Runbook TemplateService -> Alert port / Execution adapter。多步骤任务必须保留步骤结构，不能在前端拼成纯文本。

验证边界：`e2e-runbook.ps1` 覆盖推荐、创建、确认、模拟执行和时间线，不覆盖模板 CRUD E2E、非 processing 拒绝、low 风险、步骤失败、完整 UI、Command Spec 或 Runbook→Agent。

## Execution（`execution.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listExecutionTasks` | `GET /api/executions/tasks` | 任务分页查询，支持状态/来源/keyword |
| `getExecutionTask` | `GET /api/executions/tasks/:task_id` | 任务详情与步骤 |
| `createExecutionTask` | `POST /api/executions/tasks` | 创建普通模拟或 Runbook 任务；alert 必须 processing |
| `confirmExecutionTask` | `POST /api/executions/tasks/:task_id/confirm` | 固定提交 confirm=true 和 CONFIRM，推进到 pending_execute |
| `executeTask` | `POST /api/executions/tasks/:task_id/execute` | 同步模拟执行；Agent 任务返回 FAILED_PRECONDITION |

页面：`views/executions/index.vue`。契约：`ops/execution-contract.md`、`ops/execution-agent-contract.md`。

实现边界：

- 后端支持 confirm=false 将 pending_confirm 转为 cancelled，但当前 API 封装和页面没有拒绝入口；pending_execute/running 也没有通用取消接口。
- 来源 alert/manual/ai_conversation/inspection 已有后端路径；前端 Create 类型和 Agent 字段并不完整，不能据此认为 Agent 创建 UI 已落地。
- 后端另有 `GET /tasks/:task_id/logs?step_id=`、Medium 和 Command Spec 查询/创建接口，当前 `execution.ts` 未封装日志、Medium、Command Spec、execution_mode/medium_id/command_spec_id/arguments 或 Agent 协议。
- `/executions` 只有列表、详情、CONFIRM、同步执行、Runbook/dry-run 和 task_id 深链；无拒绝按钮、Agent 管理/创建、租约视图或实时日志流，也没有独立 Execution 前端 Vitest。
- Agent 任务创建/lease/result 当前不回写 Alert execution_created/started/finished；不能将模拟任务时间线能力直接套用到 Agent。
- `adapter` 是保留枚举，不是可用生产执行模式。

调用链：Runbook / AI / Inspection 只能创建 Execution Task；真实执行必须由 Execution 状态机、权限、风险、确认文本和审计共同约束。

验证边界：`e2e-execution.ps1` 只覆盖普通 alert 模拟闭环；`e2e-execution-agent.ps1` 使用 PowerShell fake Agent，不运行真实命令、不检查日志读取/脱敏、审计、幂等、Alert 时间线或 Runbook→Agent；permission 脚本实际只验证错误注册令牌。

## AI（`ai.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listProviders` | `GET /api/ai/providers` | 数据库 Provider 全量列表；不返回 `api_key`，以 `has_api_key` 表示密钥存在；headers 原样返回 |
| `upsertProvider` | `POST /api/ai/providers` | 同 ID 全量 upsert；类型 `b` 必须填 `model`，省略/空白 `api_key` 保留原密钥，其他省略字段按零值覆盖；写库并同步当前进程 |
| `deleteProvider` | `DELETE /api/ai/providers/:id` | 硬删除并移除当前进程运行时；不存在的 ID 当前也可返回 deleted=true |
| `invokeTool` | `POST /api/ai/tools/invoke` | 直接授权并转发给指定 Provider；成功响应内须检查 `allowed`，类型 b 还须识别 `data.degraded` |
| `analyzeAlert` | `POST /api/ai/analyze-alert` | 告警上下文 AI 分析；返回 summary、risk_level、recommendations、references，以及长事务场景 `diagnosis`（事实、根因状态、假设、缺失证据） |
| `listChatModels` | `GET /api/ai/chat/models` | 普通用户可见的启用中类型 b 模型（`provider_id/name/model/vision_enabled`，不含敏感字段） |
| `createChatSession` | `POST /api/ai/chat/sessions` | 创建 AI 助手会话，可带页面上下文与 `provider_id`（会话级模型） |
| `listChatSessions` | `GET /api/ai/chat/sessions?limit=` | 当前用户最近会话 |
| `getChatSession` | `GET /api/ai/chat/sessions/:session_id` | 会话及最近 200 条消息（assistant 消息带 `answer`），不是全量历史分页 |
| `sendChatMessage` | `POST /api/ai/chat/sessions/:session_id/messages` | 同步返回结构化回答；Axios 超时 90s，与后端默认 600s Loop 预算不一致；支持 `provider_id`、`client_message_id` 与 `images` |
| `streamChatMessage` | `POST /api/ai/chat/sessions/:session_id/messages/stream` | fetch 读取 SSE（`stage`/`tool`/`delta`/`answer`/`error`）；`signal` 可中断，取消检查点不写 assistant，最终 answer 落库后的推送失败不回滚 |
| `archiveChatSession` | `POST /api/ai/chat/sessions/:session_id/archive` | 归档会话（只读，可看历史）；幂等 |
| `unarchiveChatSession` | `POST /api/ai/chat/sessions/:session_id/unarchive` | 恢复归档会话；幂等 |
| `deleteChatSession` | `DELETE /api/ai/chat/sessions/:session_id` | 删除会话及消息，不可恢复；重复删除返回 404 |
| `listKnowledgeItems` | `GET /api/ai/knowledge/items?status=&scope=&limit=&offset=` | 知识条目列表（`app:ai:knowledge:manage`） |
| `createKnowledgeItem` | `POST /api/ai/knowledge/items` | 新建知识条目，固定为 `draft`，需审核激活 |
| `updateKnowledgeItem` | `PUT /api/ai/knowledge/items/:item_id` | 编辑条目；编辑 active 条目会回到 `draft` |
| `reviewKnowledgeItem` | `POST /api/ai/knowledge/items/:item_id/review` | `{ "action": "activate" \| "disable" }` |
| `deleteKnowledgeItem` | `DELETE /api/ai/knowledge/items/:item_id` | 删除知识条目 |
| `aggregateKnowledgeFromFeedback` | `POST /api/ai/knowledge/aggregate` | 反馈聚合：`{application_id, min_cases}` → draft；不覆盖 active 条目 |
| `listMemoryCases` | `GET /api/ai/memory/cases` | 管理端检索记忆条目（状态/归属用户/关键词） |
| `updateMemoryCaseStatus` | `POST /api/ai/memory/cases/:case_id/status` | `{ "status": "active" \| "retired" }`；停用后不再参与回答检索 |
| `sendChatFeedback` | `POST .../messages/:message_id/feedback` | 回答有效/无效反馈 |
| `createChatShare` | `POST /api/ai/chat/sessions/:session_id/shares` | 创建只读分享（默认 7 天，1–720 小时）；token 只返回一次 |
| `listChatShares` | `GET /api/ai/chat/sessions/:session_id/shares` | 分享记录（含撤销/过期），仅 owner |
| `revokeChatShare` | `DELETE /api/ai/chat/sessions/:session_id/shares/:share_id` | 撤销分享，幂等 |
| `getSharedChatSession` | `GET /api/ai/chat/shares/:token` | 只读访问共享会话；过期/撤销/删除统一 404 |

页面：

- AI 运维助手工作台 `views/ai-assistant/index.vue`（三栏：会话列表 / 对话 / 上下文与工具），对话逻辑在 `views/ai-assistant/composables/useChat.ts`，与全局浮层 `layouts/components/AIAssistantOverlay.vue` 共用。独立工作台使用 streamChatMessage；同步 sendChatMessage 仅保留为 API 能力。
- 全局浮层与排队（契约 §10.8/§10.9）：控制器由 `BasicLayout` provide，跨页路由不丢状态，但不写 localStorage，刷新即丢失排队条目与乐观消息。顶栏/`Alt + A` 默认入口自动选中最近会话；告警详情「问 AI 助手」先写入告警上下文并打开会话选择器，新建会话携带 `alert_id`。同一会话生成中继续发送进入本地队列（context/provider/幂等键按入队快照冻结），回答结束自动逐条发送；「停止」清空队列并中止当前流；队列管理只作用于当前激活会话，后台队列需切回才能管理。
- 当前会话交互差距：归档提示前输入/附图已被清空；生成中模型选择未禁用；刷新按钮只取列表，无通用当前详情刷新；重试复用旧 key 会重放旧回答且本地消息可能重复；历史重试内容被清空。模型切换时前端会同时发送顶层 provider_id 和 context.provider_id，自定义客户端只传顶层字段时，已有会话模型不会被持久改写。
- AI 配置（Provider 管理 + 工具调用调试 + 知识沉淀 + 记忆管理）在 `views/system/ai-config/index.vue`，路由 `/system/ai-config`；`views/ai-assistant` 不再承载 Provider 管理。ProviderManager 无健康检查/测试连接，普通编辑不携带 headers 并会清空旧值；ToolConsole 不提供 owner/dept/team/region/tags 输入，两者均无专项 Vitest。
- AI 工具权限字典与角色绑定由 `identity-admin.ts` 和 `views/identity/access-control/index.vue` 维护；角色绑定是全量替换。对话右侧固定工具目录不读取当前角色实际授权，不能据页面显示推断工具必定 allowed。
- 告警详情 `views/alerts/index.vue` 会写入页面上下文（`stores/aiContext.ts`），并支持 `/alerts?alert_id=` 深链打开详情。
- 资源级上下文写入方：资产页（应用 + 高亮资源）、执行页（任务详情 `execution_id`）、观测页（指标查询后的资源维度与时间范围）；字段与规则见 `ops/ai-contract.md` §10.8。

契约：`ops/ai-contract.md`（§10 为 AI 助手会话与对话）。要点：

- 成功判断使用 `code === "OK"`；`sendChatMessage` 的 `data.status` 为 `ok` / `degraded` / `insufficient_context` / `incomplete`，前端必须分别展示降级、无足够上下文和生成中断状态。
- `data.answer.diagnosis` 为可选长事务诊断结构：`facts`（平台事实及 evidence_id/pointer，可含匹配 Query ID 的 Top SQL 脱敏摘要）、`root_cause`（undetermined/suspected/confirmed、code、说明、证据 ID）、`hypotheses` 和 `missing_evidence`；前端不得把 suspected 渲染成人工已确认根因。空闲长事务的 confirmed 只表示分类有直接会话证据，页面显示“分类已确认”，应用代码级原因仍为未定。
- `data.answer.actions` 由服务端按权限生成；`execution.propose.allowed=false` 时必须禁用按钮并展示 `reason`。
- 「生成执行计划」调用既有 `createExecutionTask`（`source_type=ai_conversation`，`source_id=session_id`），任务进入 Execution `pending_confirm`；前端不得提供任何直接执行入口。响应 tool_calls 中的 execution.tasks.propose 只是 finalize 阶段的权限校验和审计记录，不表示模型已创建任务，也不是 Agent Loop 白名单工具。
- 工具调用结果必须检查 `allowed`；被拒绝是业务结果，需展示 `reason`（`toolReason.ts` 负责原因码的中文映射）。
- 对话内置只读工具共 8 个：`alarm.analyze`、自由参数 `metrics.query`、`postgres.activity.query`、`alert.rules.list`、`alert.rules.health`、`accounts.list`、`resource.analyze`、`resources.list`。它们由后端本地 Port 执行，不经 AI Provider 转发；`status=invalid_argument / skipped / unsupported / permission_denied / denied` 都需展示原因，不能当作网络错误。实时 ChatPanel/ContextPanel 展示较完整，AnswerCard 当前只对 allowed=false 的调用把 reason 放进提示，allowed=true 的非 ok 原因仍有展示缺口。
- 规则体检证据有覆盖边界：规则资源优先通过 CES `ListAlarmRuleResources` 补全并校正 `resource_count`、透传 `dimension_name`；分组规则再展开实例；无绑定规则按 namespace + 维度名发现实例兜底（`scope_source=resource_discovery`，此时 `resource_count` 为空），并按引擎线索匹配实例；零数据规则返回 `probe_point_count` 区分“近期无数据/窗口无点”；预算内尽量覆盖全部实例（单规则上限 200），统计取越限最严重样本（`checked_resources`/`sampled` 标明覆盖），并附带 `recent_*` 近 24h/60s 分钟级复核；曲线定稿最多 288 点，峰值时刻与越限时长在降采样前计算保留，规则窗口小于 60s 时以 `recent_*`（24h/60s）近似评估频率，不再要求 1s 原始数据。页面展示峰值不代表原始粒度证据完整，不能以“展示正常”排除取数问题。
- 引用类型：`alert`、`observability_evidence` 带 `url` 可跳转；`memory_case`（相似案例记忆，`ops/ai-contract.md` §10.14）与 `knowledge_item`（知识沉淀，§10.15）无 `url`，只展示标签与条数提示。
- 知识沉淀管理接口（§10.15）要求 `app:ai:knowledge:manage`（当前仅 admin），入口在「系统设置 → AI 配置 → 知识沉淀」（`KnowledgeManager.vue`）；未授权时全局拦截器提示。页面未展示审核人/审核时间/创建者/时间，scope_ref 非空与 128 字符依赖后端校验，激活/停用无二次确认，聚合固定默认 `min_cases=3` 且未暴露入参。
- 记忆管理接口（§10.19）要求 `app:ai:memory:manage`（当前仅 admin），入口在「系统设置 → AI 配置 → 记忆管理」（`MemoryManager.vue`）；停用/启用只影响后续检索并写 `memory_review` 审计，回答注入仍按归属用户隔离。页面只传 status/keyword/user_id，limit 固定 100，未使用 application_id/resource_id/offset 或分页，详情未展示 session_id。
- 会话归档后只读：`sendChatMessage` 会返回 409 `FAILED_PRECONDITION`，前端会提示先恢复，但当前输入已被清空，不能宣称不丢输入。重复归档/恢复不推进 updated_at，仍会写状态审计；已生成回答在归档后仍可幂等重放。
- 消息幂等：每次提问生成 `client_message_id`，传输重试复用同一值；服务端不比较内容 hash，已生成回答（含 degraded/incomplete）一律重放，回答缺失时返回 409。新的提问必须换新 key；当前“重新提问”按钮复用旧 key，不是重新生成。
- 归档/恢复/删除入口在会话列表（`SessionList.vue`），三者均需二次确认；删除当前会话后自动切到下一个会话。详情只含最近 200 条，模型 Prompt 历史另取最近 12 条；用户与助手消息分属两个事务。
- 流式对话（§10.17）：`streamChatMessage` 走 `fetch` + `web/src/api/sse.ts`，401 刷新一次，error 事件转 `ApiHttpError`，无 answer 结束转 ABORTED。stage 驱动阶段，tool 展示观测，thinking delta 展示过程文本，answer delta 只计数；`: ping` 注释与未知事件忽略。停止后回读会话；取消检查点保留 user/不写 assistant并写 chat_canceled，最终 answer 已落库后的断开不回滚。当前无 answer 的传输中断可能仍显示 failed，需与主动停止分开验收。
- 会话分享（§10.18）：仅**对内只读**，访问者必须登录且具备 `app:ai:chat`；token 只在创建响应返回一次，前端不持久化。共享页路由 `/ai-assistant/shared/:token`（`SharedSession.vue`）只读渲染，禁用反馈与动作；过期/撤销/源会话删除统一 404。入口在会话列表「分享」→ `ShareDialog.vue`。共享页当前只渲染文本气泡，不展示用户附图（接口已返回 `images`），见 §10.20 已知差距。
- Provider 列表不返回 `api_key` 或 `****` 掩码字段，只用 `has_api_key` 和页面“已配置/未配置”表达；自定义 headers 会原样返回，不应存放 Token/Authorization。页面上下文只保存业务 ID，不保存凭据或告警正文。
- 直接 `/tools/invoke` 入口对非 all 数据范围用户会因缺少路由对象上下文返回 403；进入 Gateway 后的业务拒绝才是 200 + allowed=false。其审计不含 allowed/reason且拒绝时仍可能 result=success；Provider CRUD 没有操作审计，不能用审计中心证明这些配置变更已完整留痕。

## Audit（`audit.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `fetchAudits` | `GET /api/audits` | 操作审计分页查询，支持 `resource_type` / `resource_id` / `user_id` / `action` / `trace_id` 精确筛选；每页最多 100 条 |

页面：`views/audits/index.vue`。权限：`app:audits:read`。契约：`ops/audit-contract.md`；真实资源类型和 action 目录见 `internal/audit/README.md`。

实现边界：

- 五个筛选均为精确匹配，资源 ID 输入框的“模糊或精确匹配”提示不代表后端支持模糊查询；`trace_id` 匹配 `payload.trace_id`。
- 明细支持分页、详情抽屉、Payload 格式化、Trace 复制和点击反向筛选；详情/CSV 的敏感键掩码只影响前端展示，不改变原始 API 响应或数据库内容。
- 总数卡使用当前筛选 `total`，Alert/Execution/AI 分类卡和 Resource/User/Trace 聚合只统计当前页 `items`，不能当作全量聚合。
- 无 Trace 的记录被合入前端 `NO_TRACE` 占位组，不是真实链路 ID。
- CSV 在浏览器生成，固定请求第一页 `page_size=1000`，后端截为 100，因此实际最多 100 条；没有循环翻页、服务端全量导出或独立导出审计。
- 资源类型下拉中的 `asset/identity/inspection/observability/integration` 与实际写入类型不一致，多个真实类型也无快捷选项；需要精确查询这些记录时使用 API。该差距本轮只登记，未修改代码。
- 操作审计只有静态 RBAC，无业务对象数据范围过滤；路由和侧栏不按该权限动态控制。
- 登录/refresh/logout 属于 Identity `iam_auth_audit`，通过 `/api/identity/admin/auth-audits` 独立查询，不会出现在 `fetchAudits` 结果中。
- 当前无专用 Audit E2E、Handler/仓储专项测试或前端 Vitest；跨模块 E2E 审计断言不能替代页面和导出边界验收。

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

调用链：页面表单 -> `integration.ts` -> Integration AccountService -> CredentialVault / Provider checker。凭据只在表单内存中短暂持有，不写入前端持久化状态。`extra_config` 仅用于 provider 非敏感扩展配置，例如华为云 `sync_mode/resource_group_name/max_resources`；密钥、Token、AK/SK、密码仍只能通过 `credential` 写入。

实现边界：

- 六个接口分别使用 `app:integrations:read/create/update/delete/check`，当前只有静态 RBAC，无 owner_team/region 数据范围过滤。
- 列表 API 支持 `keyword`（name/account_id/owner_team ILIKE），页面只有 Provider 和 enabled 筛选；删除是账号软删除 + enabled=false + 凭据硬删除，当前无恢复接口。
- 更新省略或传空 credential 都保留原凭据，没有清空语义；切到 `auth_type=none` 不删除旧密文。后端允许 provider 更新，页面禁用 provider 编辑。
- 所有 checker 当前仅做字段校验，不访问真实华为云、SigNoz 或 Prometheus；`status=ok` 和 capabilities 不代表网络、云端权限或实际 Adapter 全部可用。
- checker 失败结果成功落库时 HTTP 仍为 200 + `code="OK"`，必须检查 `data.status`。页面目前统一使用成功 Toast，即使 status=failed；capabilities 非空时还可能不显示失败 message。
- 页面缺少 `agency_name/domain_name`，agency 只能通过 API 完整配置；没有检查历史或凭据清空 UI。
- 当前 Vault 只有当前密钥/版本，未提供多版本解密、外部 Secret Manager 或在线重加密流程。
- `huaweiConfig.spec.ts` 只覆盖配置纯函数。`e2e-integration.ps1` 只使用 `huawei_cloud + none` 覆盖创建、列表/详情、检查 ok、更新、viewer 创建 403 和删除，不覆盖真实凭据/网络、其他 auth_type、检查 failed、keyword、审计或完整 UI。

### 华为云 CES 资源同步前端契约

`huawei_cloud` 账号的 CES 同步配置统一写入 `extra_config`，不要平铺为账号顶层字段。TypeScript 类型以 `integration.ts` 中的 `HuaweiCloudExtraConfig`、`HuaweiCloudRegionProject`、`HuaweiCloudSyncMode` 为准。

| 字段 | 类型 | 默认/建议值 | 提交与回显规则 | 说明 |
| --- | --- | --- | --- | --- |
| `sync_mode` | `ces` / `hybrid` / `native` | `ces` | 新建 `huawei_cloud + ak_sk` 推荐默认提交 `ces`；编辑时回显后端值 | `ces` 按**指定 CES 资源分组**口径同步（未指定资源组时同步该 region 下全部资源组的并集）；`hybrid` 先 CES 后原生 API 增强，EVS 详情增强尚未支持，VPC 子资源（EIP/带宽/子网/对等连接）详情增强已支持；`native` 仅旧路径兼容，不承诺与资源分组数量一致 |
| `resource_group_name` | `string` | 空（占位提示“全部资源”） | 空值可省略；填写后写入 `extra_config` | 留空时后端按 `ops/huawei-ces-sync-contract.md` §8.4 依次尝试默认候选名（全部资源/All resources/All Resources），均未命中则进入**聚合口径**同步该 region 下全部资源组的并集；显式填写业务分组名则精确匹配，未命中即失败（不回退聚合） |
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

实现边界：

- 只有上述四个公共路由，统一 `app:observability:read` 静态 RBAC，无 owner_team/region 数据范围过滤；资源发现、告警规则、RDS 历史会话和证据详情没有同名公共 API。
- `provider` 省略时由后端按账号解析。公共 `MetricQueryInput` 没有 `project_id`，多项目只能依赖账号顶层 project_id 或 `extra_config.region_projects`；内部 AI 路径才支持显式 project_id。
- 公共指标窗口最长 7 天、period 默认 60/平台最小 10、估算最多 1440 点；logs 最大 500、traces 最大 1000。真实 CES 另有 period/filter 枚举和 1–4 个 dimensions 等约束，period=10 等通用合法值不一定被真实 CES 接受。
- 华为 `none` 全部 fake；AK/SK 指标真实，logs/traces/topology 显式 unsupported，资源/规则走真实 Adapter；agency 全部 unsupported。SigNoz 和 Prometheus 当前即使配置 token 也为 fake，不能把响应当作外部平台实测数据。
- 普通 evidence 保存脱敏 summary/query_hash，普通 Metrics 另有 query_params/window/collected_at 但没有 payload。AI Metrics 先 collecting，定稿快照保存成功后才进入模型；失败不作为支持性引用。
- `ai.metric_evidence.v1` 和 `ai.postgres_activity.v1` 快照已有内部仓储，当前没有公共证据详情 API 或前端抽屉；页面仅显示 evidence_id 文本。
- 页面加载前 100 个 enabled 账号，指标只展示 `series[0].points`，Logs 固定 limit=100、Traces 固定 50；当前 Vitest 只覆盖指标参数与 AI 上下文。
- `e2e-observability.ps1` 使用 `huawei_cloud + none` 覆盖四类 fake 查询；`e2e-ai-evidence.ps1` 使用 fake 指标 + mock 模型 + Docker PostgreSQL 比对定稿 hash，均不代表真实 CES 通过。

## Inspection（`inspection.ts`）

| 函数 | 接口 | 说明 |
| --- | --- | --- |
| `listPolicies` / `createPolicy` / `updatePolicy` / `deletePolicy` | `/api/inspections/policies` | 巡检策略 CRUD |
| `getPolicy` | `GET /api/inspections/policies/:policy_id` | 策略详情；后端和封装均已存在，页面未使用 |
| `triggerRun` | `POST /api/inspections/policies/:policy_id/runs` | 同步执行巡检并通常返回 success/partial/failed 终态 |
| `getRun` / `listRuns` | `/api/inspections/runs` | 运行状态与时间线 |
| `listFindings` | `GET /api/inspections/findings` | 发现与嵌套建议，含 `evidence_refs` |

页面：`views/inspections/index.vue`。契约：`ops/cloud-observability-contract.md` §6。迁移依赖 `0020`。

调用链：策略/运行操作 -> `inspection.ts` -> Inspection RunService -> ObservabilityQueryPort -> EvidenceAnalyzer -> Finding / Recommendation。建议转执行时只创建 Execution Task，不在前端或 AI 侧直接执行。

实现边界：

- 后端共 10 个路由，策略/运行/发现使用 `app:inspections:read/write`，建议转执行使用 `app:executions:create`，全部静态 RBAC，无 scope 数据范围过滤。
- `scope.account_id` 和 checks 必填；application_ids 校验注册表存在。schedule、agent_profile、notification_policy_id 目前只是配置，定时 worker、AI Agent 和通知未落地。
- TriggerRun 在同一请求同步执行，HTTP 200 也可能返回 `status=failed`；页面统一显示成功 Toast，必须检查 run.status。Run 无取消接口。
- 当前采集固定最近 1 小时、cn-north-4、SYS.ECS 且无 dimensions，真实华为指标会失败；environment/resource_types 不参与查询，affected_resources 固定为 payment-service。fake E2E 不能代表真实云巡检。
- `POST /api/inspections/recommendations/:id/execution` 只允许 agent 模式，medium_id 必填，无自动介体选择；Recommendation 不保存 task_id 或完成回写，任务创建与状态更新非同一事务。当前 `inspection.ts` 和页面都没有该接口封装/入口。
- 页面只有策略列表/新建/立即巡检/删除、运行列表和发现筛选；无策略编辑/启停、Run 时间线、建议接受/忽略/通知/转执行，也无前端 Vitest。
- `e2e-inspection.ps1` 使用 fake 账号覆盖创建应用/账号/策略、同步运行、Run 详情、Finding/Recommendation 和删除清理，不覆盖更新/列表、partial/failed、审计、真实云、转执行或 UI；Execution Agent E2E 也不经过 Inspection Recommendation。

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `VITE_API_BASE` | axios baseURL；dev 默认 `/`（走 Vite proxy）；生产见 `deployments/README.md` |
