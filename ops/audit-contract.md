# Audit 审计中心契约

本文档定义 `audit_operation` 操作审计的写入边界、只读查询接口和当前前端行为。Identity 认证审计使用独立表和接口，不属于本契约的 `/api/audits` 数据集。

## 1. 当前实现状态

| 能力 | 状态 | 边界 |
| --- | --- | --- |
| 统一操作审计写入服务 | 已落地 | 各模块通过 recorder 写入；载荷和失败策略由调用方决定 |
| 分页与精确筛选 | 已落地 | 单接口 `GET /api/audits`，每页最多 100 条 |
| 明细与 Payload 抽屉 | 已落地 | 前端按敏感键名做展示掩码，原始 API 不做相同转换 |
| Resource/User/Trace 聚合 | 前端已落地 | 只统计当前页，不是服务端全量聚合 |
| CSV 导出 | 前端已落地 | 实际最多导出当前筛选第一页的 100 条 |
| 认证审计统一展示 | 未落地 | 登录/refresh/logout 需走 Identity 独立接口 |
| 时间范围、模糊搜索、详情 API | 未提供 | 当前只有精确筛选和分页列表 |
| 服务端聚合/异步全量导出 | 未提供 | 前端导出不能替代服务端导出任务 |
| 存储级防篡改与保留策略 | 未提供 | 无修改/删除 HTTP API 不等于 WORM 或哈希防篡改 |
| 专用 Audit E2E/前端测试 | 未提供 | 有服务单元测试和跨模块 E2E 审计断言 |

## 2. 鉴权与权限

`GET /api/audits` 要求 Bearer Token 和 `app:audits:read`。

- migration `0009_init_audit` 种子该权限并绑定 admin；其他角色需要另行授权。
- 未认证返回 401，权限不足返回 403 `PERMISSION_DENIED`。
- 路由使用 `AuthorizeStatic`，不执行 Alert、Asset、Execution 等对象级数据范围过滤。
- 前端 `/audits` 路由只校验登录，侧栏为静态配置；页面可见不代表接口已授权。
- 查询和导出共用同一个读权限，没有独立导出权限，也不会为查询/导出再写操作审计。

## 3. 获取操作审计列表

- Method：`GET`
- Path：`/api/audits`
- 响应：统一 `code/message/trace_id/data` 结构，成功码为字符串 `OK`。

### 3.1 Query 参数

| 参数 | 默认值/上限 | 当前语义 |
| --- | --- | --- |
| `page` | 默认 1 | 页码；小于 1 时归一为 1 |
| `page_size` | 默认 20，最大 100 | 超过 100 时按 100 处理，不是报错 |
| `resource_type` | 可选 | 资源类型精确相等筛选 |
| `resource_id` | 可选 | 资源业务 ID 精确相等筛选；前端“模糊或精确匹配”提示不代表后端支持模糊查询 |
| `user_id` | 可选 | 操作人业务 ID 精确相等筛选；系统来源也可能是 `system` 或 `source:<id>` |
| `action` | 可选 | 动作精确相等筛选；同一动作可在不同资源类型下复用 |
| `trace_id` | 可选 | 精确匹配 `payload->>'trace_id'` |

`trace_id` 不是独立列。写入时 context 的 Trace ID 会补入 Payload，但调用方已提供 `payload.trace_id` 时保留调用方值。查询参数与本次响应顶层 `trace_id` 不是同一概念。

当前没有开始/结束时间、用户名、关键字、多个动作、多个资源类型或模糊匹配参数。

### 3.2 响应

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "request-trace-id",
  "data": {
    "items": [
      {
        "id": "00000000-0000-0000-0000-000000000001",
        "user_id": "user-admin-001",
        "resource_type": "alert",
        "resource_id": "alt-prod-cpu-high-01",
        "action": "close",
        "payload": {
          "comment": "已完成扩容处置",
          "trace_id": "request-trace-id"
        },
        "ip": "192.0.2.10",
        "user_agent": "client-example",
        "created_at": 1773300000
      }
    ],
    "total": 1,
    "page": 1,
    "page_size": 20
  }
}
```

字段约束：

- `id` 是 Audit 生成的 UUID，不是数据库自增 ID。
- `created_at` 是 Unix 秒。
- `payload` 为对象；空载荷返回 `{}`。
- `user_id`、`ip`、`user_agent` 为空时可省略。
- 结果按 `created_at DESC, id DESC` 排序。
- `total` 是当前筛选条件的总数，`items` 仅为当前页。

## 4. 写入模型与失败语义

写入链路：

```text
业务 application AuditRecorder
  -> 模块 infrastructure/audit Recorder
  -> audit.OperationAuditService.Record
  -> audit_operation
```

`Record` 负责生成 UUID、修剪必要字段、补空 Payload 和自动注入 Trace。`resource_type`、`resource_id`、`action` 任一为空会拒绝写入。IP 和 User-Agent 字段虽存在并有长度限制，但当前标准模块 recorder 不转发这两项，所以常规操作记录通常没有 IP/UA；认证审计的 IP/UA 是另一条 Identity 写入链路。

Audit 不与业务操作共享统一事务。当前主要业务写入点普遍采用 best-effort，审计错误被忽略或只记录日志：

- 业务接口成功不能作为审计一定存在的证明。
- Alert CES 轮次先提交最终同步状态，再独立尝试写 `alert_source_sync`；二者不是同一事务。
- Asset Sync 审计失败只记录告警日志，不回滚同步终态。
- 需要“审计失败即业务失败”的场景必须另建明确契约和测试，不能套用本模块默认行为。

服务端不会对任意 Payload 做通用敏感字段清洗。写入方必须只放业务 ID、状态、计数、原因和已脱敏摘要，不得写入凭据、Token、JWT secret、完整 Prompt 或原始敏感日志。前端掩码只改变页面和 CSV 展示，数据库及原始 API 响应仍按已存内容返回。

## 5. 资源类型与动作

以下为当前代码中的主要写入目录；动作必须结合 `resource_type` 解释。新增写入点时应同步更新本目录和对应模块契约。

| 模块 | `resource_type` | 典型动作 |
| --- | --- | --- |
| Alert | `alert`、`alert_source` | 告警 `ingest/acknowledge/assign/start_processing/recover/close/silence/unsilence/comment/ai_analysis_requested/execution_create`；来源 `source_create/source_update/source_delete/alert_source_sync` |
| Asset | `application`、`resource`、`match_rule`、`resource_relation`、`asset_sync_batch` | 注册表与关系 `create/update/delete`；同步批次 `sync` |
| Runbook | `runbook`、`alert` | 模板 `runbook_create/runbook_update/runbook_delete/runbook_enable/runbook_disable`；推荐 `runbook_recommend` |
| Execution | `execution`、`alert`、`execution_medium`、`execution_agent` | 任务 `create/create_from_runbook/confirm/reject/execute/dispatch/agent_log/agent_result`；告警关联 `execution_create`；Medium `create/update/disable`；Agent `register/heartbeat` |
| Identity 权限管理 | `identity_user`、`identity_role` | `set_user_roles/set_role_permissions/set_role_data_scopes/set_role_ai_tools` |
| AI | `ai` | `analyze_alert/tool_invoke/chat_session/chat_message/chat_feedback/chat_session_status/chat_session_delete/chat_canceled/chat_share_create/chat_share_revoke/chat_share_view/knowledge_edit/knowledge_review/memory_review` |
| Integration | `integration_account` | `create/update/delete/check` |
| Observability | `observability_query` | `metrics_query/logs_search/traces_query/topology_get/resources_list/alerts_list` |
| Inspection | `inspection_policy`、`inspection_run`、`inspection_recommendation` | 策略 `create/update/delete/enable/disable`；运行 `create/start/finish`；建议 `create/create_execution` |

表中使用 `/` 只是为了紧凑列举独立 action，查询时必须传完整动作字符串。例如应传 `chat_share_create`，而不是 `chat_share_create/revoke/view`。

## 6. 前端明细、聚合与导出

页面为 `web/src/views/audits/index.vue`，只通过 `web/src/api/audit.ts` 调用列表接口。

### 6.1 明细和筛选

- 支持五个筛选条件、分页、详情抽屉、Payload 格式化、Trace 复制和点击记录反向筛选。
- 所有筛选最终都调用同一个精确查询 API。
- 详情和 CSV 会按 `password/secret/token/apikey/authorization/jwt/accesskey/ak/sk` 等敏感键名递归显示 `******`；这不是任意文本的通用脱敏或存储脱敏。
- 查询失败时显示错误消息；页面没有独立 Audit 错误状态契约。

### 6.2 指标与聚合口径

- “总操作审计”卡片使用当前筛选的 `total`。
- Alert、Execution、AI 分类卡片只统计当前页 `items`，与总数的统计范围不同。
- Resource、User、Trace 三个聚合视图都只遍历当前页 `items`，不代表当前筛选下所有记录。
- 无 Trace 的记录在前端合并为 `NO_TRACE` 占位组；该值不是真实 Trace ID，不应当作链路查询依据。

后端没有 `/audits/aggregate` 等聚合接口，也没有按资源、用户或 Trace 的全量统计响应。

### 6.3 CSV 实际范围

导出在浏览器端执行：

1. 固定请求 `page=1&page_size=1000`，携带当前五个筛选条件。
2. 后端将 `page_size` 截为 100，所以最多返回最新的第一页 100 条。
3. 前端生成带 UTF-8 BOM 的 CSV 并下载，字段包含 Payload 的掩码 JSON。

当前没有循环翻页、全量校验、服务端导出任务、独立导出审计或超过 100 条的完整导出保证。页面“成功导出 N 条”只表示实际生成的行数，不能解释为已导出筛选总数。

### 6.4 资源类型快捷选项差距

当前下拉选项中的 `asset`、`identity`、`inspection`、`observability`、`integration` 与实际写入的 `application/resource/...`、`identity_user/identity_role`、`inspection_*`、`observability_query`、`integration_account` 不一致，通常无法查到目标模块记录；多个真实类型也没有快捷选项。

API 精确查询不受该下拉选项限制。前端类型字典归一化属于已确认实现差距，本轮仅登记，未修改代码。

## 7. 认证审计边界

登录、refresh、logout 的成功/失败事件写入 `iam_auth_audit`，通过以下独立接口查询：

- `GET /api/identity/admin/auth-audits`
- 权限：`app:identity.auth_audits:read`
- migration：`0006_auth_audit`

这些记录不写入 `audit_operation`，也不会出现在 `GET /api/audits` 或当前 `/audits` 页面。认证审计字段和筛选语义见 `ops/auth-contract.md` 与 `ops/identity-api-contract.md`。

## 8. 完整性与保留边界

当前没有外部修改或删除操作审计的 HTTP API，但数据库记录没有哈希链、签名、WORM、保留期限、自动归档或清理 worker。因此：

- 可以描述为“只读查询 API”。
- 不能描述为“存储级不可篡改”或“永久保留”。
- 数据库运维清理、保留和防篡改要求需要独立技术与运维方案。

## 9. 验证覆盖与剩余验收

现有覆盖：

- `operation_service_test.go`：Record、自动 Trace 注入、Trace 查询命中与未命中。
- 业务模块单元测试：各模块 recorder/application 的写入动作和部分 Payload。
- 跨模块 E2E：Identity 用户角色、Alert CES 同步及多项 AI 流程包含审计断言。

现有脚本不是专用 Audit E2E，不能据此认定以下项目已通过：

- 所有筛选条件、精确匹配、排序、分页上限和空字段。
- 401/403、数据范围边界和 Handler 错误响应。
- GORM 仓储的真实 PostgreSQL 查询。
- 当前页指标/聚合、`NO_TRACE`、资源类型快捷选项和 CSV 100 条上限。
- 审计写失败时各业务模块的实际影响。

本轮仅静态复核，未执行 Go 测试、前端 Vitest 或业务 E2E，不形成运行通过结论。
