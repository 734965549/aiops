# internal/audit 模块说明

Audit 是平台操作审计上下文，提供统一的 `audit_operation` 写入服务和只读分页查询。各业务模块通过自己的 application 审计端口和 infrastructure recorder 写入，Audit 不代替业务模块决定审计动作、载荷或失败策略。对外契约见 `ops/audit-contract.md`。

## 1. 当前能力与完成边界

| 能力 | 当前实现 | 完成边界 |
| --- | --- | --- |
| 操作审计写入 | UUID 审计 ID、资源、动作、操作人、Payload、IP/UA 字段 | 标准模块 recorder 当前不传 IP/User-Agent；Payload 无统一服务端脱敏 |
| Trace 关联 | 写入时从 context 补充 `payload.trace_id` | 调用方已提供同名字段时保留原值 |
| 操作审计查询 | `GET /api/audits`，五个精确筛选条件和分页 | 无时间范围、模糊搜索、单条详情或服务端聚合 API |
| 权限 | Bearer + `app:audits:read` 静态 RBAC | 不应用业务对象数据范围；默认只给 admin 种子，其他角色需另行授权 |
| 前端明细 | 列表、组合筛选、详情抽屉、Payload 展示、Trace 复制 | 原始 API 仍返回落库 Payload，前端掩码不是存储脱敏 |
| 前端聚合 | 按 Resource、User、Trace 在当前页数据上聚合 | 不是当前筛选下全量数据或服务端统计 |
| CSV 导出 | 前端按当前筛选重新查询并生成带 BOM 的 CSV | 请求 1000 条会被后端截为 100，实际最多导出第一页 100 条 |
| 认证审计 | Identity 独立实现 | 不在 `audit_operation` 或 `/api/audits` 中 |
| 防篡改与保留 | 没有外部修改/删除 HTTP API | 没有哈希链、WORM、签名、保留策略或自动归档，不能宣称存储级不可篡改 |

## 2. 写入调用关系

```text
业务模块 application
  -> 本模块 AuditRecorder 端口
  -> internal/<module>/infrastructure/audit.Recorder
  -> audit.OperationAuditService.Record
  -> OperationAuditRepository
  -> audit_operation
```

`Record` 的当前行为：

- 生成 UUID 作为对外 `audit_id`；数据库自增 `id` 不进入 API。
- 修剪操作人、资源类型、资源 ID 和动作；资源类型、资源 ID、动作为空时返回参数错误。
- Payload 为空时保存 `{}`；context 有 Trace ID 且 Payload 没有 `trace_id` 时自动补入。
- IP 和 User-Agent 分别限制为 64 和 512 字节，但现有标准 recorder 只转发用户、资源、动作和 Payload，所以常规业务记录通常没有这两个值。
- 服务或仓储未配置时，写入按 no-op 返回；真实写入错误会返回给调用方。

Audit 不与各业务写操作共享统一事务。当前多数业务调用点采用 best-effort：忽略错误或只记录日志，因此业务成功不等于审计必定落库。CES 告警轮次也是先提交同步状态，再独立尝试写 `alert_source_sync`，不是同事务提交。需要强审计保证的功能必须单独设计事务、失败策略和验证，不能由统一 `Record` 方法反推。

Payload 在服务端按调用方提供的内容保存，没有通用敏感键清洗。各写入方必须避免写入凭据、Token、完整 Prompt、原始执行日志等敏感内容；前端详情和 CSV 的递归敏感键掩码只影响展示/导出，不改变 API 响应和数据库内容。

## 3. 当前资源类型与动作目录

`resource_type + resource_id` 共同定位审计对象。`resource_id` 的具体含义由写入模块决定，不能脱离资源类型全局解释。

| 写入模块 | `resource_type` | 当前典型 `action` |
| --- | --- | --- |
| Alert | `alert` | `ingest`、`acknowledge`、`assign`、`start_processing`、`recover`、`close`、`silence`、`unsilence`、`comment`、`ai_analysis_requested`、`execution_create` |
| Alert Source | `alert_source` | `source_create`、`source_update`、`source_delete`、`alert_source_sync` |
| Asset | `application`、`resource`、`match_rule`、`resource_relation` | 按对象使用 `create` / `update` / `delete` |
| Asset Sync | `asset_sync_batch` | `sync`；尚未拆成独立生命周期 action |
| Runbook | `runbook` | `runbook_create`、`runbook_update`、`runbook_delete`、`runbook_enable`、`runbook_disable` |
| Runbook 推荐 | `alert` | `runbook_recommend` |
| Execution | `execution` | `create`、`create_from_runbook`、`confirm`、`reject`、`execute`、`dispatch`、`agent_log`、`agent_result` |
| Execution 关联告警 | `alert` | `execution_create` |
| Execution Medium / Agent | `execution_medium`、`execution_agent` | Medium 为 `create/update/disable`；Agent 为 `register/heartbeat` |
| Identity 权限管理 | `identity_user`、`identity_role` | `set_user_roles`、`set_role_permissions`、`set_role_data_scopes`、`set_role_ai_tools` |
| AI | `ai` | `analyze_alert`、`tool_invoke`、`chat_session`、`chat_message`、`chat_feedback`、`chat_session_status`、`chat_session_delete`、`chat_canceled`、`chat_share_create`、`chat_share_revoke`、`chat_share_view`、`knowledge_edit`、`knowledge_review`、`memory_review` |
| Integration | `integration_account` | `create`、`update`、`delete`、`check` |
| Observability | `observability_query` | `metrics_query`、`logs_search`、`traces_query`、`topology_get`、`resources_list`、`alerts_list` |
| Inspection | `inspection_policy`、`inspection_run`、`inspection_recommendation` | 策略 `create/update/delete/enable/disable`；运行 `create/start/finish`；建议 `create/create_execution` |

Dashboard 只有只读聚合，不新增操作审计。审计查询和 CSV 导出本身也不会再写操作审计。

## 4. 查询接口与语义

`GET /api/audits` 支持：

- `page`：默认 1。
- `page_size`：默认 20，超过 100 时按 100 处理。
- `resource_type`、`resource_id`、`user_id`、`action`：全部为精确相等筛选，不支持模糊匹配。
- `trace_id`：精确匹配 `payload->>'trace_id'`，不是独立数据库列，也不是查询本次响应的顶层 `trace_id`。

结果按 `created_at DESC, id DESC` 排序，响应采用统一 `code/message/trace_id/data` 封装。`data` 是分页对象；审计项的 `id` 是 UUID，`created_at` 是 Unix 秒，空 `user_id/ip/user_agent` 因 `omitempty` 可省略。

路由使用 `AuthorizeStatic`，只检查 `app:audits:read`。它不会按 Alert、Asset、Execution 等业务对象的数据范围再次过滤；获得审计读权限的主体可以查询该接口返回的跨模块记录。

## 5. 前端页面

页面为 `web/src/views/audits/index.vue`，API 封装为 `web/src/api/audit.ts`，路由和静态侧栏入口均为 `/audits`。

- 明细：时间、资源、动作、操作人、Trace、IP 和详情抽屉；点击资源、用户或 Trace 可转为精确筛选。
- 指标卡：总数使用当前筛选的服务端 `total`；Alert、Execution、AI 三张分类卡只统计当前页 `items`，彼此口径不同。
- 聚合：Resource、User、Trace 三个视图都只遍历当前页 `items`，换页或改变每页条数会改变聚合结果。
- Trace 占位：无 Trace 的当前页记录被合并为 `NO_TRACE`，它只是本地占位分组，不是真实链路 ID。
- 导出：前端固定请求 `page=1&page_size=1000`，服务端实际最多返回 100 条；没有翻页拉取、后端导出任务或全量导出保证。
- 脱敏：详情和 CSV 按敏感键名递归显示 `******`；这不替代服务端写入治理，也不构成任意文本内容的通用脱敏。
- 权限体验：路由只校验登录，未授权请求由后端返回 403；侧栏不是按审计权限动态生成。

资源类型下拉选项与真实目录并不完全一致：`asset`、`identity`、`inspection`、`observability`、`integration` 不是上表中的实际写入值，选择它们通常查不到对应模块记录；`application`、`resource`、`identity_user`、`alert_source` 等真实类型又没有快捷选项。需要按这些类型精确检索时可使用 API，前端选项归一化属于待整改实现差距。

## 6. 认证审计不是操作审计

登录、refresh、logout 成功/失败写入 Identity 的 `iam_auth_audit`，通过 `GET /api/identity/admin/auth-audits` 和 `app:identity.auth_audits:read` 查询，迁移为 `0006_auth_audit`。

这些记录不会同步到 `audit_operation`，当前 `/audits` 页面和 `audit.ts` 也不查询认证审计。因此不能在操作审计中期待 `login` / `refresh` action，也不能把 `/audits` 称为已统一展示全部登录审计。

## 7. 迁移与验证覆盖

- `0009_init_audit.up.sql` 创建 `audit_operation`、资源/用户/动作/时间索引，并种子 `app:audits:read` 及 admin 绑定。
- `operation_service_test.go` 覆盖 Record、自动 Trace 注入及按 Trace 查询/未命中。
- 多个业务模块有自己的 recorder/application 审计单元测试；这些测试归业务模块，不等于 Audit 查询 API 的完整测试。
- `e2e-identity-access.ps1`、`e2e-alert-sync.ps1` 和多个 `e2e-ai-*.ps1` 含操作审计断言，但没有专用 `e2e-audit.ps1`。
- 当前没有 Audit HTTP Handler、GORM 仓储排序/筛选、分页上限、401/403、前端聚合/导出或资源类型选项的专项测试，也没有 Audit 前端 Vitest。

本轮仅做静态复核和文档更新，未执行上述测试或 E2E，不形成运行通过结论。
