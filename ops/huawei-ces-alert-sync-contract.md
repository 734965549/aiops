# 华为云 CES 告警拉取稳定契约

本文定义 CES 告警记录拉取的配置、数据与权限边界、同步语义和验收口径。相关契约见 [告警主契约](alert-contract.md)、[CES 资产同步](huawei-ces-sync-contract.md)、[可观测接入](cloud-observability-contract.md) 和 [数据库迁移](migration-contract.md)。

## 实施状态

Phase A/B 已落地：迁移 `0046`/`0047`（`0047` 幂等补齐部分列）、CES V2 客户端与字段映射、scope 游标与去重、PostgreSQL 源级租约、手动同步 API、`alert_source_sync` 审计、可选 worker 以及前端拉取配置（含立即同步与同步状态展示）均已实现。worker 由 `alert_sync.worker_enabled` 显式启用，默认关闭，多实例部署只应在一个 API 实例开启；`auth_type=none` 的 fake 账号仅在非生产环境允许。Phase C 的验收脚本 `scripts/e2e-alert-sync.ps1` 已提供，但目标区域的官方接口、时间过滤、分页行为和脱敏响应样例仍须在真实测试账号上核对并留存。

## §1 范围

- 一期同步 **CES V2 告警记录**，一条云端告警流水对应一条平台告警。覆盖手动触发和可选定时拉取，复用 Alert 的状态转换、时间线与审计。
- 云端调用限于只读查询。真实接入使用 AK/SK。§13 所述 fixture 仅供测试环境使用。
- 一期暂不覆盖 agency、完整历史回填、写入 CES 规则及其他云厂商。平台无需调用规则列表来推断告警状态。

## §2 流程

1. 读取接入源，校验绑定账号和 scope，获得该源的同步租约。
2. 按 scope 分页拉取 CES 记录，使用流水号和真实状态变化归一化。
3. 通过告警入库用例处理新触发、云端恢复与重复观察。成功完成的查询窗口推进自己的游标。
4. 持久化本轮摘要所对应的源状态和审计，释放租约。

## §3 架构与职责

```text
cmd/api/main.go
  Alert HTTP handler / scheduler → Alert application 同步用例
    → Alert application 记录去重、状态机与事务
    → Alert 定义的拉取端口 → Alert infrastructure/observability 适配器
      → Observability application 账号及能力解析、ProviderRegistry
        → Huawei CES V2 客户端（分页、超时、错误分类）和 mapper（字段转换）
    → Asset 匹配端口 → Asset application 精确匹配云资源键
```

- 编排、配置校验、调度、记录状态合并分别归于相应职责。业务端口由消费模块定义，跨模块适配通过对方 application 入口。接入源管理、现有 Webhook 鉴权和 CES 拉取分别保留独立入口。
- 凭据复用现有 integration 仓储及 Huawei `CredentialProvider.ResolveAKSK`。账号与凭据在每轮重新解析。AK/SK、原始响应和签名头不得进入日志、审计或 API。
- 云资源和告警拉取共用账号的区域及项目解析。CES 告警记录查询的项目范围由区域 `project_id` 和该凭据的云端 IAM 授权决定。接入源可以收窄区域。

## §4 接入源配置

### 4.1 迁移与存量兼容

迁移拆分为两个版本：`0046_alert_source_pull` 已发布并应用，新增 `account_id` / `pull_config` / `last_sync_at` / `last_sync_status` 与租约表；`0047_alert_source_pull_state` 补充 `delivery_mode` / `sync_state`。已发布版本不得改写，字段拆分以迁移表为准。`alert_source` 增加以下字段。

| 列 | 类型与初值 | 说明 |
| --- | --- | --- |
| `delivery_mode` | `VARCHAR(16) NOT NULL DEFAULT 'webhook'` | `webhook` / `pull`，存量源保留 Webhook |
| `account_id` | `VARCHAR(64) NOT NULL DEFAULT ''` | 拉取源绑定的 integration 账号业务 ID |
| `pull_config` | `JSONB NOT NULL DEFAULT '{}'` | 接入源拉取配置 |
| `last_sync_at` | `TIMESTAMPTZ NULL` | 最近一轮已记录结果的完成时间 |
| `last_sync_status` | `VARCHAR(32) NOT NULL DEFAULT ''` | `success` / `partial` / `failed` / 空 |
| `sync_state` | `JSONB NOT NULL DEFAULT '{}'` | 内部状态，按 `region/project_id` 保存已处理至的时间。API 不返回 |

本期还需持久化每源租约持有者、到期时间和防止过期持有者写入的令牌。迁移时同步更新 [迁移契约](migration-contract.md)，补充回退和 readiness 验收。时间戳由 Go 维护，无数据库时间默认值或触发器。回退前停用新入口及 worker，保留已导入的告警和审计。

### 4.2 `pull_config`

```json
{
  "pull_interval_seconds": 300,
  "severity_map": {"1": "p0", "2": "p1", "3": "p2", "4": "p3"},
  "region_filter": ["cn-north-4"]
}
```

| 字段 | 默认值 | 校验 |
| --- | --- | --- |
| `pull_interval_seconds` | `300` | 整数 `60..86400`，`0` 表示只手动。负数、小数、`1..59` 或超上限返回 `INVALID_ARGUMENT` |
| `severity_map` | `1→p0 / 2→p1 / 3→p2 / 4→p3` | 部分覆盖默认映射，键为 CES 整数级别的字符串形式。值限 `p0/p1/p2/p3/info`。未知云端级别归 `info` |
| `region_filter` | `[]` | 空表示使用账号所有区域。去空白、大小写归一并去重。创建、更新时拒绝不在账号区域列表中的值 |

### 4.3 创建、更新与失效

- 创建请求省略 `delivery_mode` 时默认为 `webhook`。Webhook 源创建时必传 `secret`，更新时省略则保留。Webhook 源拒绝拉取配置，原有 `huawei_ces` Webhook 继续可用。
- `delivery_mode=pull` 要求 `type=huawei_ces`、`account_id` 及有效 scope。绑定账号须已启用、属于 `huawei_cloud`、声明 `alerts` 能力并使用 `ak_sk`（测试环境可显式允许 `none`）。请求不得提交 `secret`，`secret_hash` 存空。公开 Webhook 在校验 token 前拒绝拉取源，返回 `NOT_FOUND`。
- `delivery_mode` 创建后不可修改。拉取源的 `type` 和 `account_id` 也不可修改。更换账号或接入方式时新建源，验证后停用旧源。更新 `pull_config` 传入对象时整体替换并应用默认值，省略则保留。删除、停用和区域变更不自动恢复已有告警。
- scope 来自账号 `regions` 与 `region_filter` 的交集，`extra_config.region_projects` 按区域覆盖账号 `project_id`。保存时拒绝空 scope 或无项目 ID 的 scope。运行中若账号配置变化，已移除、无法解析或无项目 ID 的 scope 按失败报告，其他有效 scope 继续。零有效 scope 返回失败摘要。scope 的项目变更时使用新游标。
- 源绑定的账号删除后，手动触发返回 `NOT_FOUND`。账号被停用或不再满足 provider、认证或能力要求时返回 `FAILED_PRECONDITION`。定时触发记录失败及原因，并按轮询间隔重新检查。

## §5 权限

| 操作 | 授权 |
| --- | --- |
| 列出、查看接入源 | Bearer + `app:alerts:ingest` |
| 新建、修改、手动同步拉取源 | Bearer + `app:alerts:ingest` + `app:integrations:read`，服务端按绑定的 `account_id` 执行账号读取授权。接入账号有数据范围限制时沿用该限制 |
| 删除拉取源 | Bearer + `app:alerts:ingest`，删除仅作用于平台接入源 |
| 定时拉取 | 依据已授权保存的源与账号绑定执行，每轮校验账号和源仍启用，审计 actor 为系统 |

云账号选择器和服务端绑定使用同一准入条件。直接提交 `account_id` 不得绕过账号授权。

## §6 CES API 与字段映射

### 6.1 接口与分页

使用华为云 [CES V2 ListAlarmHistories](https://support.huaweicloud.com/api-ces/ListAlarmHistories.html)，`GET /v2/{project_id}/alarm-histories`，云端 IAM 授权项为 `ces:alarmHistory:list`。仓库锁定的 Go SDK V2 提供 `ListAlarmHistoriesRequest` 和 `AlarmHistoryItemV2`。V1 `ListAlarms` 返回规则的 `alarm_state`，V2 `ListAlarmRules` 返回规则配置，二者均不参与本期同步。

- 每个 scope 使用 `offset/limit` 分页，`limit` 最大 100，使用 `order_by=record_id`。增量查询使用 `from/to` 更新时间窗口。
- 新 scope 先查询云端可见的 `alarm` 记录，再读取最近 24 小时的更新窗口。增量轮次固定本轮截止时间，从上次成功游标回退 5 分钟读重叠窗口。记录按 scope 和 `record_id` 去重。同一流水在同一轮被两个查询读到时优先采用明确的恢复状态，活动记录取较新的云端触发时间。历史覆盖受云端保留期及凭据权限限制。
- 大窗口分段读取。页失败、分页重复或数据不完整时保留该段游标，已提交的记录可安全重放。只有完整处理的连续窗口才能推进该 scope 的游标。优先处理较久未推进的 scope。不得用 `last_sync_at` 代替分 scope 游标。
- 已入库且仍待恢复的记录若持续未出现在增量窗口，可用 `record_id` 定向查询。查询失败、记录不可见或消失均不作为恢复证据。上线前核对目标区域的更新时间过滤、翻页结束条件及长时间活动告警的可见性。

### 6.2 字段映射

| CES 字段 | 平台字段或语义 |
| --- | --- |
| `record_id` | `external_id`。`fingerprint` 使用 `account_id + region + project_id + record_id` 的无歧义编码，和 `source_id` 一起走 `ComputeDedupKey` |
| `alarm_id` | 云规则 ID，保留在 labels。不同 `record_id` 独立处理 |
| `name` / `level` | `name`、`rule_name` / 按 §4.2 映射到 `severity` |
| `status` | 按 §7.1 处理 |
| `first_alarm_time` / `begin_time` | `first_seen_at` 依次取第一个有效值，最后回退到首次观察时间 |
| `last_alarm_time` / `alarm_recovery_time` | 区分同一流水的较新触发与重放，供外部恢复时间使用。恢复时间缺失则使用首次观察到恢复的时间 |
| `metric.namespace` / `metric.dimensions` | 按已验证的云资源类型和主维度提取云资源 ID。保留必要的定位标签 |
| `metric.metric_name` | 写入 `labels.metric_name` 与 `annotations.ces_metric_name`，并参与 `description` 拼装，供 AI 分析直接引用 |
| `condition` | 触发条件（聚合方式、比较符、阈值、单位、周期、连续次数、抑制间隔）归一化为 `annotations.ces_condition`，例如 `> 80% (average, 300s x3)`；空条件不构造 |

`source=huawei_ces`，环境与业务线使用接入源默认值。经 Asset 端口以 `account_id + region + cloud_resource_type + cloud_resource_id` 精确匹配云同步资产。缺少有效维度或匹配有歧义时保留告警并留空资产关联。

`description` 在有指标名时按 `<namespace> <metric_name> <condition> · <resource_name>` 拼装；`summary` 保持资源名不变。`annotations` 同时保留 `ces_namespace`、`ces_level`、`ces_metric_name`、`ces_condition`、`ces_status` 与 `ces_last_alarm_time`。

## §7 去重与生命周期

### 7.1 状态映射

| CES 状态 | 平台动作 |
| --- | --- |
| `alarm`，首次观察此流水 | 创建平台告警，`occurrence_count=1` |
| `alarm`，流水已存在且处于处理中的状态 | 仅当 `last_alarm_time` 比已记录的云端触发时间更新才记一次 `updated`。重复或旧观测计入 `ignored` |
| `alarm`，流水对应记录已恢复或关闭 | `ignored`，保留人工处理和既有恢复结果。云端新触发的独立流水可创建新告警 |
| `ok` / `ok_manual` | 对已存在且未恢复或关闭的记录执行外部恢复，清除静默并写一次恢复事件。时间线保留 CES 状态以区分两种恢复。其余情况计入 `ignored` |
| `invalid` / `insufficient_data` / 未知状态 | `ignored`，按原因计数。现有平台告警状态保持，失效和缺失不推断健康恢复 |
| 缺 `record_id` / 缺 `name` | 计入 `ignored` 并记录脱敏诊断 |

### 7.2 重放和持久化

- 同一源和同一 scope 下的流水最多对应一条平台告警，查重范围包含已关闭记录。复用 `dedup_key + lifecycle_seq` 唯一约束，CES 单个流水固定生命周期序号。现有 Webhook 生命周期规则保持兼容。
- 观察顺序按云端触发时间与终态处理，已观察到恢复的记录不会被旧 firing 覆盖。告警状态、最后处理的云端触发时间与时间线在同一事务提交。写入失败计入本轮失败记录，相关窗口可安全重放。告警行更新须和人工状态操作保持并发一致性。
- 计数反映平台观察到的不同云端触发时间，不能还原云端触发总次数。历史窗口首次看到已恢复且此前未入库的流水只计 `ignored`。

## §8 手动同步 API

`POST /api/alerts/sources/:source_id/sync`，权限见 §5。仅适用于已启用的拉取源。同步等待并返回本轮结果。工作截止 20 秒，另预留最多 5 秒写最终状态和审计，适配当前 30 秒 HTTP 写超时与前端超时。浏览器断开时停止云查询和新写入，已持久化的进度保留，本轮用有界收尾上下文记录结果。

`data` 结构示例（相同格式覆盖 `success/partial/failed`）。

```json
{
  "source_id": "huawei-prod",
  "account_id": "acc-1",
  "regions": ["cn-north-4", "cn-south-1"],
  "status": "partial",
  "accepted": 4,
  "created": 1,
  "updated": 0,
  "recovered": 1,
  "ignored": 1,
  "failed_records": 1,
  "failed_scopes": [
    {"region": "cn-south-1", "project_id": "proj-2", "reason": "RECORD_WRITE_FAILED"}
  ],
  "duration_ms": 1234,
  "last_sync_at": 1762830000
}
```

- `accepted = created + updated + recovered + ignored + failed_records`，统计已处理的去重记录。失败页中尚未拿到的记录只反映在 `failed_scopes`。`regions` 是本轮计划处理的区域，包括因轮次超时未开始的区域。空 `failed_scopes` 输出数组 `[]`。
- 已执行且结果已记录的轮次返回统一响应 `code=OK`，客户端以 `data.status` 判断结果。同步前的权限、配置、资源不存在或租约冲突返回对应业务错误。
- 接入源响应新增 `delivery_mode`、`account_id`、`pull_config`、`last_sync_at`、`last_sync_status`。Webhook 源仅提供 `delivery_mode=webhook` 和原有 Webhook 字段。`last_sync_*` 为空表示尚无已记录轮次。

## §9 触发与并发

- 手动与定时触发共享 **PostgreSQL 持久化的源级租约**。可参考资产同步的租约模式。租约竞争使用原子条件写，写入告警、游标、源状态和审计时在同一数据库事务内校验持有令牌，过期持有者不能写入。
- 同源竞争时手动请求返回 `ABORTED`（409），定时请求跳过。不同源可并行。源的更新、停用和删除与租约获取互斥，同步在途时返回 `ABORTED`。账号禁用、删除或 scope 配置变化时应在后续写入前失效旧账号快照，终止受影响的同步并记录原因。
- worker 可由部署配置显式启用，默认关闭。启用后按各源间隔调度，`pull_interval_seconds=0` 的源只允许手动触发。轮询跳过禁用源，优雅退出复用资产同步生命周期模式。部署仅启用一个定时 worker 以避免重复调度，手动请求可由任意 API 实例接收，互斥由共享租约保证。

## §10 错误与结果

| code | HTTP | 场景 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | 400 | 非拉取源调 sync、源禁用、配置格式或绑定条件非法 |
| `UNAUTHENTICATED` | 401 | Bearer 无效 |
| `PERMISSION_DENIED` | 403 | 操作权限或绑定账号授权不足 |
| `NOT_FOUND` | 404 | 源或绑定账号不存在 |
| `ABORTED` | 409 | 同源租约正在被持有 |
| `FAILED_PRECONDITION` | 412 | 已绑定账号在运行前被停用，或 provider、认证、能力已不符合准入条件 |
| `UNAVAILABLE` | 503 | 拉取端口未装配 |
| `INTERNAL` | 500 | 本轮最终状态或审计无法持久化 |

- `success` 表示全部计划 scope 均完整处理。`partial` 表示至少有一个 scope 完成或有记录已成功提交，同时有查询、写入或超时失败。`failed` 表示轮次开始后无 scope 完成且无成功提交，例如凭据解析失败、零有效 scope 或全部 scope 失败。
- 失败只影响相关 scope。失败 scope 给出稳定原因码，例如 `PROJECT_ID_MISSING`、`REGION_UNAVAILABLE`、`UPSTREAM_TIMEOUT`、`PAGE_INCOMPLETE`、`RECORD_WRITE_FAILED`、`ROUND_TIMEOUT`。部分写入时报告已提交的条数，并保留失败窗口游标。
- 最终状态和轮次审计使用同一事务提交。收尾失败返回 500，原 `last_sync_*` 保留，已提交的告警可在重试时安全重放。日志记录脱敏的轮次、scope 和原因码。

## §11 审计

| 动作 | 审计语义 |
| --- | --- |
| 轮次完成 | `action=alert_source_sync`，`resource_type=alert_source`，`resource_id=<source_id>`。payload 含 `trace_id`、scope、状态、计数、失败原因和耗时。手动记实际触发用户，定时记 `user_id=system` |
| 单条告警实际变更 | 复用 `AuditIngest`，`user_id=source:<source_id>`。重复观测不追加逐条审计 |
| 时间线 | `actor_type=integration`，`actor_id=<source_id>`。HTTP 沿用请求 trace，worker 为每轮生成 trace 并传到日志、审计和时间线 |

## §12 前端对接

- 源管理增加接入方式。选择 CES 拉取时展示账号、轮询间隔和区域配置，Webhook 源继续使用共享密钥表单。
- 账号选择器读取分页 `GET /api/integrations/accounts?provider=huawei_cloud&enabled=true`，按 §4.3 过滤。无账号权限时展示明确的权限提示，并禁用依赖账号权限的操作。
- 源列表展示同步时间与状态。立即同步处理 409、业务失败摘要和超时，按 scope 展示失败原因。
- 更新 `web/src/api/alert.ts` 和 [前端 API 说明](../web/src/api/README.md)。告警列表和详情复用既有数据与状态流转。

## §13 测试与实施阶段

### 13.1 关键验收

- **Provider** 使用符合 V2 响应结构的脱敏 fixture 验证字段、分页、相同 `alarm_id` 的多流水、多维度、权限错误和后续页失败。
- **应用** 覆盖重复观测零变更、旧 firing 在恢复后到达、人工恢复或关闭后再拉取、新流水可触发、缺页/失效不推断恢复、游标不越过失败窗口、写入失败后重试。
- **权限和兼容** 覆盖存量 CES Webhook 更新及接入、拉取源拒绝公开 Webhook、普通角色缺账号权限、账号禁用/删除，以及拉取期更新/停用冲突。
- **集成** 两个 API 实例同源争抢，以及租约过期旧持有者写入失败。测试部分已落库、轮次超时和最终状态/审计事务失败。
- **E2E** 采用仅测试装配可启用的 fake Provider。测试控制同一 scope 两次返回的 V2 fixture，从两条同规则、不同资源流水的 `alarm` 切到一条 `ok`、另一条仍为 `alarm`。核对资产关联、恢复时间线及 `alert_source_sync` 审计。已有凭据账号禁止降级到 fake。见 [验收清单 §1.4](../docs/acceptance-checklist.md#14-ces-拉取接入)。

### 13.2 实施顺序

1. **Phase A** 确认目标区域官方 API 行为，落地迁移、客户端与 mapper、scope 游标、去重及事务单测。
2. **Phase B** 接入共享租约、权限、手动 API、审计、调度和前端，验证两实例并发。
3. **Phase C** 已提供 `scripts/e2e-alert-sync.ps1`（覆盖创建拉取源、三轮拉取、恢复、重复观测与审计）；真实环境验收后继续同步本节、主契约、迁移与可观测能力说明。

## §14 后续扩展

更长历史回填、事件类额外字段、资源分组范围和其他云厂商。若规模超出有界同步窗口，再评估异步批次 API。
