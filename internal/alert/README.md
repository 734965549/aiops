# internal/alert 模块说明

Alert 负责外部告警接入、标准化、去重、生命周期、时间线、接入源配置，以及向 Asset、AI、Execution 和 Audit 提供关联入口。稳定接口以 `ops/alert-contract.md` 为准，CES 拉取细节以 `ops/huawei-ces-alert-sync-contract.md` 为准。

## 1. 当前能力与完成边界

| 能力 | 当前实现 | 完成边界 |
| --- | --- | --- |
| Webhook 接入 | Alertmanager 与通用 Webhook，共享 token、1MB 请求体限制、X-Request-ID 幂等 | Signoz/Zabbix 等类型可用于 Webhook 来源配置，但没有对应专用拉取 Adapter |
| 去重与生命周期 | fingerprint/external_id 或稳定标签计算 dedup_key；支持重复更新、恢复、恢复后重开、关闭后新 lifecycle | 请求级幂等与告警业务去重是两层机制，生产多实例请求级幂等依赖 Redis |
| 告警处理 | 列表、详情、时间线、认领、转派、处理、恢复、关闭、静默、取消静默、备注 | `related` 当前返回空对象；复杂 Incident 聚合和降噪编排未落地 |
| 接入源 | CRUD、secret 哈希与掩码、启停、Webhook/Pull 配置、同步状态 | secret 不明文回显；接入源删除不删除已入库告警 |
| CES 拉取 | V2 告警记录、scope 游标、源级租约、规则富化、手动同步、可选 worker | 真实账号的 API 行为与权限仍需专项验收；worker 默认关闭，完整历史回填未落地 |
| AI 入口 | 写 `ai_analysis_requested` 时间线和审计，前端再调 AI 分析 API | Alert 不直接执行模型；告警 AI 弹窗尚未展示完整 `diagnosis`，完整诊断卡片在 AI 助手 |
| Execution 关联 | 接收 `execution_created/started/finished` 时间线回写 | 真实执行状态机和确认由 Execution 负责 |
| 数据权限 | AI 分析读取告警前可执行对象级数据范围校验 | 告警列表/详情当前仅做 RBAC，未自动套用数据范围过滤 |
| 静默 | 单告警静默、截止时间与取消静默，外部恢复清除静默 | 当前没有到期自动恢复状态的 worker；`silenced_until` 到期不等于状态自动变回 new |

## 2. 分层与调用关系

```text
interfaces/http
  -> AlertService             查询与人工状态操作
  -> IngestService            Webhook 鉴权、归一化、去重与生命周期
  -> SourceService            接入源、账号准入、配置租约与审计
  -> AlertSyncService         CES 拉取、scope 游标、结果与审计
  -> AlertSyncWorker          可选定时调度

application ports
  -> AssetMatcher             资产/应用匹配
  -> IntegrationAccountReader 云账号读取与准入
  -> AccountAccessChecker     账号数据范围检查
  -> AuditRecorder            Operation Audit
  -> AlertSyncLeaseStore      PostgreSQL 源级租约

infrastructure
  -> GORM Repositories        alert_alert / alert_event / alert_source / alert_silence
  -> Redis/MemoryStore        Webhook 请求级幂等
  -> Observability Adapter    CES V2 记录与规则只读查询
```

## 3. 路由与权限

### 3.1 公开 Webhook

| 路由 | 鉴权与限制 |
| --- | --- |
| `POST /api/alerts/ingest/alertmanager/:source_id` | `X-AIOPS-Webhook-Token`；Alertmanager payload |
| `POST /api/alerts/ingest/webhook/:source_id` | `X-AIOPS-Webhook-Token`；平台通用字段 payload |

两条路由都校验源存在、启用且不是 Pull 源。无效 token 返回 `UNAUTHENTICATED`，源不存在/禁用或 Pull 源按 `NOT_FOUND` 处理；请求体最大 1MB。可选 `X-Request-ID` 与 source_id 组成短期幂等键。

### 3.2 告警查询与处理

| 动作 | 路由 | 权限 |
| --- | --- | --- |
| 列表/详情 | `GET /api/alerts`、`GET /api/alerts/:alert_id` | `app:alerts:read` |
| 认领 | `POST .../:alert_id/acknowledge` | `app:alerts:acknowledge` |
| 转派 | `POST .../:alert_id/assign` | `app:alerts:assign` |
| 开始处理/恢复/备注/AI 入口 | `start-processing`、`recover`、`comments`、`ai-analysis` | `app:alerts:update` |
| 关闭 | `POST .../:alert_id/close` | `app:alerts:close` |
| 静默/取消静默 | `POST .../:alert_id/silence`、`unsilence` | `app:alerts:silence` |
| 接入源列表/详情/删除 | `/api/alerts/sources*` | `app:alerts:ingest` |
| 接入源创建/更新/手动同步 | `/api/alerts/sources*`、`.../:source_id/sync` | `app:alerts:ingest` + `app:integrations:read` |

**当前路由边界**：POST/PUT 接入源对所有类型都同时要求 `app:integrations:read`，包括纯 Webhook 源；CES 拉取源还会在 application 层校验绑定账号及数据范围。这是现时的路由授权约束，不能只在文档中按拉取源业务需求推断 Webhook 创建权限。

## 4. Webhook 去重与生命周期

### 4.1 两层幂等

1. **请求级幂等**：`source_id + X-Request-ID` 缓存首次结果；生产使用 Redis 共享，dev 可进程内存储。缓存结果写失败返回可重试错误，不伪装成功。
2. **告警业务去重**：优先使用 fingerprint/external_id，否则使用规则、资源和稳定标签计算 dedup_key。数据库通过 `(dedup_key, lifecycle_seq)` 与 active 唯一索引约束生命周期。

### 4.2 重复与重开

| 场景 | 当前行为 |
| --- | --- |
| active 告警重复 firing | 更新 last_seen、labels、annotations、occurrence_count，写 `updated` |
| active 告警 resolved | 外部恢复到 `recovered`，写 `recovered`；从 silenced 恢复时清除静默截止 |
| 重复 resolved | 计入 ignored，不重复写恢复事件 |
| recovered 后同 dedup_key firing | 重开当前 lifecycle 为 new，清恢复、认领与处理人信息 |
| closed 后同 dedup_key firing | 创建新 lifecycle 记录 |
| 静默期间 firing | 仍更新告警，不因为静默计入 ignored |
| 无 active 时 resolved | 计入 ignored，不创建“仅恢复”的告警 |

CES 拉取复用同一归一化管道，但按云端 record_id 和触发时间处理重复观测；已恢复/关闭的同一云流水不会被旧 firing 重开。

## 5. 人工状态机

| 操作 | 允许状态 | 成功状态/结果 |
| --- | --- | --- |
| 认领 | new | acknowledged，同时设置当前用户为处理人 |
| 转派 | 除 closed 外 | 更新 assignee，写 assigned 事件 |
| 开始处理 | acknowledged | processing |
| 手动恢复 | processing | recovered |
| 关闭 | new/acknowledged/processing/recovered | closed；silenced 需先取消静默 |
| 静默 | new/acknowledged/processing | silenced，记录 reason、ends_at，最长 30 天 |
| 取消静默 | silenced | new，清除 silenced_until |
| 备注 | 除 closed 外 | 写 commented 事件，最长 2000 字符 |
| AI 分析入口 | 除 closed 外 | 写 ai_analysis_requested 事件 |

非法流转返回 `INVALID_ARGUMENT`。外部 resolved 不走人工 recover 限制，允许 new/acknowledged/processing/silenced 直接进入 recovered。

## 6. 接入源与 CES 拉取

### 6.1 接入源配置

- Webhook 源创建必须提交 `id/name/secret`，`type` 默认 `prometheus_alertmanager`，`enabled` 默认 true。
- 更新省略 secret 时保留原哈希；响应只给 `secret_masked`。
- `delivery_mode` 创建后不可改；Pull 源的 `type/account_id` 也不可改。
- Pull 源仅支持 `type=huawei_ces`，要求绑定启用、具备 alerts 能力的华为云账号，真实环境使用 AK/SK。
- `pull_interval_seconds=0` 表示只手动；默认 300 秒，自动模式允许 60–86400 秒。
- 更新、停用和删除 Pull 源与在途同步通过同一源级租约互斥。

### 6.2 手动同步与 worker

`POST /api/alerts/sources/:source_id/sync` 同步等待本轮结果。当前每轮云查询预算 20 秒，另预留最多 5 秒写最终状态和审计。

- `data.status=success/partial/failed` 与 HTTP `code=OK` 可以同时出现；调用方必须检查业务状态。
- 每个 region/project scope 独立游标，失败窗口不推进；重复重放依赖 record 去重。
- 同源手动竞争返回 `ABORTED`（409）；不同源可并行。
- worker 由 `alert_sync.worker_enabled` 显式开启，默认 false；多实例只应在一个 API 实例启用。
- 规则富化只补缺失的指标/条件等字段，查询失败可降级，不参与健康状态推断，也不阻断游标。

真实 CES API、IAM 权限、分页和区域行为不能由 fake E2E 替代验收。

## 7. 前端实现

- `/alerts` 支持筛选分页、详情抽屉、时间线、认领、转派、处理、恢复、关闭、静默、取消静默、备注和 Runbook 入口。
- 告警源管理支持 Webhook/Pull 配置、启停、删除、立即同步及 success/partial/failed 摘要和 scope 失败原因。
- 详情提供友好/原始字段、指标语义、原始阈值、完整 ID 复制、敏感值掩码和规则富化降级提示。
- 资源解析区分 matched、unregistered、ambiguous；多候选不会默认选中，未注册资源保留原始 ID。
- “AI 分析”先写 Alert 时间线再调用 AI API；“问 AI 助手”携带当前告警上下文并支持选择会话。
- 当前告警 AI 弹窗展示风险、摘要、建议和引用，未完整展示 `diagnosis`；该差距不能用 AI 助手的诊断卡片替代验收。
- `/alerts?alert_id=` 支持深链打开详情；列表/详情数据权限边界见 §1，不因页面隐藏字段而视为服务端隔离。

## 8. 数据与审计

主要迁移：

| 版本 | 作用 |
| --- | --- |
| `0007` | alert_alert、alert_event、alert_source、alert_silence 与基础权限 |
| `0046` | Pull 账号、配置、最近同步状态和 PostgreSQL 源级租约 |
| `0047` | delivery_mode 与按 scope 保存的 sync_state |

审计动作包括：

- 告警：`acknowledge`、`assign`、`start_processing`、`recover`、`close`、`silence`、`unsilence`、`comment`、`ai_analysis_requested`。
- 接入：`ingest`（created/updated/recovered 结果）。
- 接入源：`source_create`、`source_update`、`source_delete`、`alert_source_sync`。
- Execution 关联：时间线 `execution_created`、`execution_started`、`execution_finished`。

Alert/Source 审计写入均为 best-effort，失败不回滚主业务；Pull 轮次最终状态和 `alert_source_sync` 审计按 CES 契约在同一事务提交。

## 9. 验证覆盖

### 单元、集成与前端测试

仓库已有覆盖：

- 状态机、级别、事件类型、dedup、secret、PullConfig。
- Webhook 鉴权、请求体限制、重复/恢复/重开、请求幂等与 Redis 并发。
- 接入源默认值、创建/更新、创建审计、Pull 准入和不可变字段。
- CES 游标、重复观测、恢复与旧 firing、partial、租约冲突/过期、规则富化降级。
- 告警列表/详情 Handler、权限拒绝、分页和 AI 入口。
- 前端详情布局、语义展示、降级状态、复制、掩码、资源解析和 AI 助手入口。

### E2E 的实际范围

`scripts/e2e-alert.ps1` 覆盖：创建 Alertmanager 源、firing、重复 firing、resolved、关闭后新 lifecycle、认领、开始处理、手动恢复和 AI 分析入口。它不覆盖转派、静默/取消静默、备注、无效 token、通用 Webhook、完整 UI 或真实 Provider。

`scripts/e2e-alert-sync.ps1` 使用非生产 fake 华为账号，覆盖 Pull 源创建、Webhook 源 sync 返回 400、三轮拉取、两条独立流水、恢复、重复观测、最近同步状态和 `alert_source_sync` 审计。它不覆盖真实 CES、多实例租约争抢、worker 定时调度、账号权限负向和全部规则富化分支。

本轮仅静态复核，未重新执行测试或脚本。
