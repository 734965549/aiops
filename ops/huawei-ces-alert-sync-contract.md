# 华为云 CES 告警拉取稳定契约

> 本文档定义华为云 CES 告警拉取（Pull-based Alert Sync）的接入源配置、同步语义、API、权限、错误处理与验收契约（WHAT）。告警拉取相关 Go 代码注释中的 `§X` 引用指向本文档对应章节。
>
> 相关文档：
> - 告警主契约（状态机 / 去重 / Webhook 接入）：[alert-contract.md](alert-contract.md)
> - 资产同步契约（region/project 解析先例）：[huawei-ces-sync-contract.md](huawei-ces-sync-contract.md)
> - 可观测总契约（Provider Port 模式）：[cloud-observability-contract.md](cloud-observability-contract.md)
> - 迁移契约：[migration-contract.md](migration-contract.md)

## 实施状态

本契约**契约先行**：文档落盘时代码尚未实现。实现范围与阶段划分见 §13.2；实现落地后本节同步更新为实际状态。

## §1 范围与非目标

### 1.1 范围

- 为 `type=huawei_ces` 的告警接入源提供平台主动拉取 CES 告警实例的能力。
- 拉取结果复用告警主契约的归一化管道：dedup_key、生命周期、状态机、资产匹配、时间线事件、审计，与 Webhook 接入逐字节一致。
- 支持手动触发（API）与定时轮询（后台 worker）。
- 只读接入：平台只调用 CES 查询类 API，不做任何写操作。

### 1.2 非目标（本期不做）

- agency 委托认证（与现有 observability huawei adapter 一致，返回 unsupported）。
- 告警历史精确对账 / `first_seen_at` 精化（快照式精度有限，见 §7.4）。
- `insufficient_data` 的业务处理策略（仅计入 ignored 统计）。
- 多实例分布式调度锁（当前仅单实例启用定时拉取，见 §9.3）。
- SigNoz / Zabbix 等其余来源的拉取式接入（端口设计可复用，按需扩展）。
- CES 告警规则的创建 / 修改 / 删除。

## §2 最小心智模型

主流程：**拉快照 → 归一化 → 复用 ingest 管道落库**。

1. **拉快照**：按账号配置的 region/project 调 CES `ListAlarms`，拿到告警规则及当前状态（快照）。
2. **归一化**：按 §6.2 映射为平台 `NormalizedAlert`（firing / resolved 两类动作）。
3. **落库**：调用与 Webhook 接入相同的归一化管道（dedup、状态机、资产匹配、时间线、审计）。

快照式**不记忆上一轮状态**：`alarm` 直接提交 firing，`ok` 直接提交 resolved；对无 active 记录的 `ok`，复用 §7.2 语义自然落入 `ignored`，天然幂等。

## §3 架构与依赖

```text
cmd/api/main.go 装配
        │
        ▼
alert/application.AlertSyncService            拉取编排（手动 API / 后台 ticker / 单飞锁 / 统计）
        │  复用 alert/application.IngestService 归一化管道
        │
        ▼ port（alert/application 定义，自包含类型，不 import observability）
alert/infrastructure/observability/           薄适配器（同 alert/infrastructure/asset 模式）
        │
        ▼
observability/application                     账号解析 → capability 校验 → ProviderRegistry 路由
        │
        ▼
observability/infrastructure/provider/huawei/
  ├─ ces_alarm_client.go                      CES ListAlarms 客户端（只读）
  └─ alarm_mapper.go                          CES payload → 统一 AlarmInstance 模型
```

约束：

- 依赖方向保持 `interfaces -> application -> domain <- infrastructure`；跨上下文只经 application 层端口，不 import 对方 infrastructure。
- 凭据解析复用 integration 模块（`CredentialProvider.ResolveAKSK`），不新增凭据存储；凭据不进入日志、Prompt、审计。
- region/project 解析复用 integration 账号 `extra_config` 的 `SyncModeConfig.region_projects`（单一事实来源），接入源侧不做第二套 region 配置。
- CES 告警客户端与 mapper 属 observability infrastructure，遵循现有 `ces_client.go` / `ces_resource_client.go` 的目录与测试风格。

## §4 接入源配置

### 4.1 迁移

迁移版本 `0046_alert_source_pull`（up / down 成对）：

```sql
ALTER TABLE alert_source
    ADD COLUMN IF NOT EXISTS account_id VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pull_config JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS last_sync_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS last_sync_status VARCHAR(32) NOT NULL DEFAULT '';
```

- 时间戳由 Go 程序维护（项目硬约束，无 DB DEFAULT、无触发器）。
- `alert_source` 领域模型同步增加 `AccountID / PullConfig / LastSyncAt / LastSyncStatus`。
- `pull_config` 落库为 JSONB，API 层输出结构化对象。

### 4.2 `pull_config` 结构

```json
{
  "pull_interval_seconds": 300,
  "severity_map": {
    "critical": "p0",
    "major": "p1",
    "minor": "p2",
    "info": "p3"
  },
  "region_filter": []
}
```

| 字段 | 类型 | 默认 | 说明 |
| --- | --- | --- | --- |
| `pull_interval_seconds` | number | `300` | 定时拉取间隔秒数；下限 `60`，超过下限才启用后台轮询；`0` 或缺省表示按默认值启用 |
| `severity_map` | object | critical→p0 / major→p1 / minor→p2 / info→p3 | CES severity 到平台级别的映射；key 为 CES 原值，value 必须是 `p0/p1/p2/p3/info` |
| `region_filter` | array\<string\> | 空（全部） | 可选 region 收窄；为空时拉取账号全部配置 region |

### 4.3 校验规则

创建 / 更新 `type=huawei_ces` 的接入源时：

- `account_id` 必填，且对应 integration account 存在、`provider=huawei_cloud`、能力声明包含 `alerts`。
- `secret` 改为可选（拉取源不需要 Webhook token；`secret_hash` 列保留兼容，Webhook 源仍必填）。
- `pull_interval_seconds` 非法值（非正数）视为使用默认 `300`；`severity_map` 含非法目标值时返回 `INVALID_ARGUMENT`。
- `region_filter` 中未出现在账号 region 集合的值不报错，按最终无告警处理（记入本轮 scope 摘要）。

## §5 权限

| 操作 | 权限 | 说明 |
| --- | --- | --- |
| 创建 / 修改 / 删除拉取源 | `app:alerts:ingest` | 沿用接入源管理权限，不新增权限码 |
| 手动同步 `POST /api/alerts/sources/:source_id/sync` | `app:alerts:ingest` | 同上 |
| 后台定时拉取 | 系统身份 | 不经用户 RBAC；审计 actor 见 §11 |
| 前端展示 `last_sync_*` / 立即同步按钮 | `app:alerts:ingest` | 无权限时隐藏操作入口，列表只读 |

## §6 CES API 与字段映射

### 6.1 API 路径

- `ListAlarms`：CES v2 查询告警规则列表（含 `alarm_status` 快照状态）。
- 告警历史接口（`alarm-histories`）本期不使用；仅作为 §14 后续精化方向的预留。

具体端点路径、分页参数与响应字段名以华为云 CES SDK 实际结构体为准；实现时以 fixture 驱动 mapper 单测固化契约。

### 6.2 字段映射

| CES 字段（示意） | 平台字段 | 说明 |
| --- | --- | --- |
| `alarm_id` | `external_id` / `fingerprint` | dedup_key 基础；走 `ComputeDedupKey(source_id, fingerprint, ...)` 主路径 |
| `alarm_name` | `name` / `rule_name` | |
| severity | `severity` | 按 `pull_config.severity_map` 映射（§4.2）；未命中映射的值按 `info` 兜底并记 warning |
| `alarm_status=alarm` | firing | 新建 / 更新 / recovered 重开，复用告警主契约 §7.2 |
| `alarm_status=ok` | resolved | 复用 external_recover 分支；无 active 记录时 ignored |
| `alarm_status=insufficient_data` | 忽略 | 仅计入 ignored 统计 |
| 维度（dim）资源标识 | `resource_name` / `resource_type` | 供 AssetMatcher 自动关联；云资源已由资产同步覆盖，命中率应较高 |
| 规则时间字段 | `first_seen_at` | 快照式精度有限，见 §7.4 |
| `alarm_description` | `summary` / `description` | |

- 来源字段固定：`source=huawei_ces`、`source_id=<接入源 ID>`、`source_name=<接入源名>`。
- `environment` / `business_line` 取接入源默认值（与 Webhook 源一致）。

## §7 同步语义

### 7.1 快照动作表

| CES 快照状态 | 提交动作 | 预期结果（复用告警主契约 §7.2） |
| --- | --- | --- |
| `alarm` | firing | 无 active → created；有 active → updated（`occurrence_count` 递增）；recovered → 重开为 `new`；closed → 新 lifecycle |
| `ok` | resolved | 有 active → `recovered`（external_recover，清 `silenced_until`）；无 active → ignored |
| `insufficient_data` | 忽略 | ignored |
| 规则缺失 / 无 name | 忽略 | ignored（同 Webhook 空 name 语义） |

### 7.2 幂等

- 每轮快照重复拉取不产生重复告警：`dedup_key + lifecycle_seq` 兜底，重复 firing 走 updated。
- 拉取不需要 `X-Request-ID` 幂等键（Webhook 专用机制）；`dedup_key` 已覆盖重复场景。
- 同一轮内同一 `alarm_id` 只提交一次（客户端按 `alarm_id` 去重）。

### 7.3 恢复语义

- CES `ok` 视为外部恢复：与 Webhook resolved 完全一致（`external_recover` 允许 active 态直接进入 `recovered`，清 `silenced_until`）。
- 用户仍可人工 `close` 关闭 `recovered` 告警，行为不变。

### 7.4 已知精度取舍

- 快照式无法还原两轮拉取之间的瞬时变化（如 firing→ok→firing），最终状态正确，中间恢复事件可能缺失。运维场景可接受，写入本契约作为已知取舍。
- `first_seen_at` 取规则时间字段（若 CES 提供），否则为首次拉取到该告警的时间；不代表云端真实首触发时间。

## §8 手动同步 API

- Method: `POST`
- Path: `/api/alerts/sources/:source_id/sync`
- 鉴权: 是（Bearer + `app:alerts:ingest`）

约束：

- 仅拉取源（`type=huawei_ces` 且配置了 `account_id`）可用；Webhook 源返回 `INVALID_ARGUMENT`。
- 同步执行完成才返回（第一阶段无批次表，不做异步任务）；执行摘要通过 `data` 返回。
- 进行中重复触发返回 `CONFLICT`（409），见 §9.2。
- 源不存在返回 `NOT_FOUND`；源被禁用返回 `INVALID_ARGUMENT`（提示启用后再同步）。

#### 响应 `data`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `source_id` | string | 接入源 ID |
| `account_id` | string | 拉取账号 ID |
| `regions` | array\<string\> | 本轮实际执行的 region 列表 |
| `status` | string | `success` / `partial` / `failed`（§10.2） |
| `accepted` | number | 归一化后提交条数 |
| `created` / `updated` / `recovered` / `ignored` | number | 与告警主契约 §6.1 统计口径一致 |
| `failed_scopes` | array\<string\> | 失败 scope（`region[:原因码]`），空数组表示无失败 |
| `duration_ms` | number | 本轮总耗时（毫秒） |
| `last_sync_at` | number | 本轮完成时间，Unix 秒 |

响应示例：

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "abc123",
  "data": {
    "source_id": "huawei-prod",
    "account_id": "acc-1",
    "regions": ["cn-north-4"],
    "status": "success",
    "accepted": 10,
    "created": 3,
    "updated": 6,
    "recovered": 1,
    "ignored": 0,
    "failed_scopes": [],
    "duration_ms": 1234,
    "last_sync_at": 1762830000
  }
}
```

- `status=failed`（全部 scope 失败）仍返回 `code=OK` + 执行摘要：调用方是操作员/前端，`status` 字段即结果语义；同时 `last_sync_status=failed` 落库并写审计。伪造成功指的是掩盖结果，这里结果通过 `status` 明确暴露。
- 接入源列表 / 详情响应新增 `account_id`、`pull_config`、`last_sync_at`、`last_sync_status` 字段（§4.1），`pull_config` 结构化输出，凭据永不返回。

## §9 触发与并发

### 9.1 手动触发

见 §8；同步等待，适合验收与运维补拉。

### 9.2 并发单飞

- 同一接入源同一时刻只允许一轮拉取（进程内单飞锁）；进行中重复触发返回 `CONFLICT`。
- 不同接入源可并行拉取。

### 9.3 定时拉取

- main.go 装配后台 worker：单 goroutine，按各源 `pull_interval_seconds` 轮询启用的拉取源；生命周期复用资产同步的 `SetLifecycle / WaitContext` 优雅退出模式。
- 间隔下限 `60` 秒；低于下限的配置按 `60` 执行。
- 每轮定时拉取独立于手动触发的单飞锁判定：定时触发遇手动拉取进行中则跳过本轮（记 Warn 日志），不返回错误。
- 多实例部署限制：当前仅支持单实例启用定时拉取 worker（进程内锁不跨实例）；分布式锁列为后续扩展（§14）。部署多个 API 实例时，须确保只有一个实例启用 worker。

## §10 错误处理

### 10.1 错误码

| code | HTTP | 场景 |
| --- | --- | --- |
| `INVALID_ARGUMENT` | 400 | Webhook 源调用 sync、源被禁用、`pull_config` 非法 |
| `UNAUTHENTICATED` | 401 | Bearer 无效 |
| `PERMISSION_DENIED` | 403 | 无 `app:alerts:ingest` |
| `NOT_FOUND` | 404 | 接入源或绑定账号不存在 |
| `CONFLICT` | 409 | 同源拉取进行中（§9.2） |
| `UNAVAILABLE` | 503 | observability 拉取端口未配置（装配缺失） |

### 10.2 scope 隔离与 `status` 判定

- 单 region / 单 scope 失败不阻塞其他 region：失败记入 `failed_scopes`，成功 scope 的告警照常入库（与资产同步 native 路径策略一致）。
- `status=success`：全部 scope 成功。
- `status=partial`：部分 scope 成功、部分失败。
- `status=failed`：全部 scope 失败（含凭据错误、账号不可用）；`last_sync_status=failed` 落库。
- 账号级前置失败（凭据解析失败、capability 校验失败）视为全部 scope 失败。

### 10.3 日志要求

- 轮次完成：`Info`（source_id、regions、统计计数、耗时）。
- 单 scope 失败：`Warn`（region、error_code，不输出云端原始报文）。
- 全部失败 / worker 异常退出：`Error`。
- 禁止记录 AK/SK、凭据、CES 响应原文。

## §11 审计与日志

| 动作 | 审计 | 说明 |
| --- | --- | --- |
| 每轮拉取完成 | `action=alert_source_sync`、`resource_type=alert_source`、`resource_id=<source_id>` | payload 含 regions、status、计数、failed_scopes、duration_ms；手动触发记录触发用户 ID，定时轮询记录 `user_id=system` |
| 逐条告警落库 | 复用现有 `AuditIngest` | `user_id=source:<source_id>`，与 Webhook 接入一致 |
| 时间线事件 | 复用现有 integration 事件 | `actor_type=integration`、`actor_id=<source_id>`；`payload.trace_id` 闭合本轮链路 |

## §12 前端对接契约

- 接入源管理（告警中心页面）：
  - 类型选项增加 `huawei_ces`；选中后表单切换为「云账号下拉 + 拉取间隔（可选，默认 300s）」，隐藏 Webhook secret 输入（改为可选）。
  - 云账号下拉数据源：`GET /api/integrations/accounts`，仅展示 `provider=huawei_cloud` 账号。
  - 接入源列表增加 `last_sync_at`（相对时间）、`last_sync_status` Tag（success=绿 / partial=橙 / failed=红 / 空=灰）与「立即同步」按钮。
  - 「立即同步」按 §8 调用；409 时提示「该源正在同步中」；`status=partial/failed` 时展示 `failed_scopes`。
- 无 `app:alerts:ingest` 权限的账号：隐藏同步按钮与源管理入口（沿用现有权限判定模式）。
- 告警列表 / 详情零改动：拉取告警与 Webhook 告警同表同链路，仅来源 Tag 显示 `huawei_ces`。
- `web/src/api/alerts.ts` 新增 sync 方法时同步更新 `web/src/api/README.md`（实现阶段执行）。

## §13 测试与验收

### 13.1 测试要求

| 层 | 内容 |
| --- | --- |
| domain / application 单测 | severity 映射（含未命中兜底）、快照→NormalizedAlert、`ok` 无 active → ignored、`enabled=false` 跳过、单 scope 失败不阻塞、单飞锁 CONFLICT |
| huawei 客户端单测 | fixture JSON 驱动 mapper；凭据缺失 / 超时错误映射；`alarm_id` 去重 |
| HTTP 测试 | sync 接口 200 / 401 / 403 / 404 / 400 / 409 主路径 |
| 迁移测试 | `go test ./pkg/database/...` 通过；`/readyz` migration 子项反映 latest |
| E2E | 新增 `scripts/e2e-alert-sync.ps1`（§13.3） |

### 13.2 实施阶段

1. **Phase A**：迁移 0046 + domain/application（ingest 归一化管道公开化）+ CES 客户端 / mapper + 单测。
2. **Phase B**：sync API + 审计 + 后台 worker + 前端。
3. **Phase C**：E2E 脚本 + 本契约实施状态更新 + 验收清单勾验。

### 13.3 E2E 脚本

- 脚本：`scripts/e2e-alert-sync.ps1`（风格对齐 `e2e-alert.ps1`，默认 `admin/admin123` → `http://127.0.0.1:8080`）。
- 流程：创建 fake 云账号（`auth_type=none`，走 fake provider 样本）→ 创建 `huawei_ces` 接入源（绑定账号）→ 手动 sync → 断言 `created >= 1` → 重复 sync 断言 `updated` → 模拟快照恢复断言 `recovered` → 校验时间线与 `alert_source_sync` 审计。
- fake 路径约束：与 observability adapter 现有 `requireFakeOnlyCapability` 模式一致——真实凭据账号不得返回 fake 样本，fake 仅用于测试链路。

### 13.4 验收清单

`docs/acceptance-checklist.md` §1.4（CES 拉取接入）为本模块验收入口，实现落地后按清单勾验。

## §14 已知缺口与后续扩展

- 多实例分布式调度锁（Redis 锁，复用 webhookidempotency RedisStore 基础）。
- 告警历史接口精化 `first_seen_at` 与瞬时变化还原。
- `insufficient_data` 可配置策略（忽略 / 恢复）。
- SigNoz / Zabbix 拉取式接入（复用同一 alert 拉取端口）。
- region_filter 命名空间 / 资源分组级过滤。
