# 华为云 CES 资源同步架构决策记录 (ADR)

> 本文档记录华为云 CES 资源分组同步的核心架构决策及其动因（WHY）。稳定的接口、状态机与运维契约以《华为云 CES 同步稳定契约》([../ops/huawei-ces-sync-contract.md](../ops/huawei-ces-sync-contract.md))为准；发布升级步骤见《华为云 CES 同步发布 Runbook》([huawei-ces-sync-runbook.md](huawei-ces-sync-runbook.md))。

## 决策 1：以 CES 资源分组为权威同步范围

### 背景

早期实现调用 ECS/CCE/RDS/ELB 等云服务原生 List API 发现资源，曾出现：

- CES 控制台显示 1,614 个监控资源，平台只同步到几十个。
- 仅授予 CES 只读权限的账号能看 CES 总览，但不一定能调用 ECS/RDS/ELB/EVS/VPC/OBS 等服务的 List API。
- EVS、VPC、OBS、DCS、DMS、CBR、VPN、终端节点、可用性监控等 CES 资源类型没有进入平台资产表。

### 决策

以 CES 资源分组接口（`ListResourceGroups` / `ShowResourceGroup` / `ListResourceGroupsServicesResources`）为主路径，将用户指定 CES 资源分组下的资源同步到 `asset_resource`。同步范围严格等同于“指定资源分组下资源”，而不是“CES 总览页所有资源”。

### 动因

- CES 官方 `ListResourceGroups` 只返回**用户创建的资源分组**，`ListResourceGroupsServicesResources` 只查询**指定 `group_id` 下**的资源。CES 控制台“总览”页若展示超出任何资源分组的聚合视图，该视图并**不**由上述 API 提供。
- 以云服务原生 List API 为准时，资源完整性取决于账号被授予了多少服务的只读权限，无法用单一权限口径保证“数量完整”。
- CES 资源分组视图天然聚合了所有被监控资源，与监控/告警口径一致。

### 关键口径

- 默认候选名“全部资源”并非 CES 系统内置分组，需要用户在 CES 控制台预先创建同名资源分组；未创建且未显式指定 `resource_group_id` 时，同步批次将失败，**不会**静默回退到任意资源组。
- `ShowResourceGroupResponse.product_names` 完整定义了资源组内的类型集合，是权威 scope。需要注意，华为官方把 `product_names` 放在“云产品资源层级”语境下定义，因此它只有在**非空**且**资源组层级适用**时，才能作为 `stale` 类型的权威边界；当其为空或不适用于资源组层级时，不能把它误解为“全局云产品全集”。

## 决策 2：`sync_mode=ces` 为默认同步模式

### 背景

平台需要明确“资源是否同步完整”的主判断依据，并与“资源详情是否丰富”解耦。

### 决策

`huawei_cloud + auth_type=ak_sk` 默认 `sync_mode=ces`。三层能力分层：

| 层级 | 同步模式 | 目标 | 权限 | 结果 |
| --- | --- | --- | --- | --- |
| P0/P1 | `ces` | 指定资源分组下看到多少，平台同步多少 | `CES ReadOnlyAccess` | 资源数量与指定分组一致，可用于告警匹配、Dashboard、AI 分析入口 |
| P2 | `hybrid` | 先按指定资源分组发现，再用原生云服务 API 补详情 | CES 只读 + 按需云服务只读 | 补充 IP、规格、VPC、磁盘、引擎版本、配置等 |
| P3 | `hybrid` + topology/inspection | 结合 CES、原生 API、日志、链路做拓扑、巡检和根因分析 | 按能力逐项授权 | 支持拓扑、配置巡检、容量风险、根因候选 |

### 动因

- 成熟产品不应把“资源是否可见”和“资源详情是否丰富”绑在一起。
- `ces` 模式只依赖 CES 资源分组/资源列表发现指定分组下资源，不调用 ECS/RDS/ELB/EVS/VPC 等原生 List API 作为入库前置条件，单一权限即可保证完整性。
- `native` 仅作为旧 ECS/CCE/RDS/ELB 路径兼容，不作为完整性口径。

## 决策 3：`hybrid` 模式做增强而非替换

### 决策

`hybrid` 模式先完整执行 `ces` 同步流程（基础资源必须入库），再对本批次 active 资源按 `cloud_resource_type` 分组，对已授权类型调用原生 API 补充详情。增强失败不回滚基础入库，但会根据失败类型将批次置为 `partial`：增强端口缺失（装配错误）或增强阶段整体失败时设置 `EnrichmentStageError` 并置 partial；逐类型增强失败计入 `EnrichmentFailedTypes` 并置 partial；label 回写失败计入 `WritebackFailedCount` 并置 partial。

### 动因

- CES 资源发现解决“数量完整”和“监控口径一致”；原生 API 只属于增强阶段，补充 IP、规格、VPC、磁盘关系等 labels。
- CES 发现成功的资源必须入库；原生 API 增强失败不回滚基础入库，但 required enrichment failure（端口缺失/阶段失败/逐类型失败/回写失败）会将批次终态置为 `partial`，不因缺少 ECS/EVS/VPC 权限导致 CES 基础同步失败。
- 增强内部按子服务/可选字段独立处理，避免一个子服务失败丢弃另一个的结果（例如 DMS Kafka 与 RocketMQ 独立、VPC `subnet_count` 为可选增强）。

## 决策 4：Lease + Fencing Token 防止并发批次互相标记 stale

### 背景

`TriggerSync` 创建 running 批次若无账号级互斥，同一账号并发批次交错执行 `MarkStaleByAccountScopeExceptBatch` 时，A 会把 B 刚 upsert 的资源（`sync_batch_id=B`）标记为 stale，产生错误资产状态。仅靠前端按钮 loading 不足以保证一致性。

### 决策

通过迁移 `0028`/`0030`/`0033`（不依赖 Redis）实现：

- `asset_sync_batch` 新增 `lease_expires_at` 列；建部分唯一索引 `(integration_account_id) WHERE status='running'`，确保每个账号同一时刻只有一个 running 批次。
- `asset_sync_batch` 新增 `fencing_token` 列；后台任务持有 token，`RenewLease` 必须按 `batch_id + fencing_token + status=running + lease_expires_at 未过期` 续租（过期不可复活），upsert/stale 前必须校验 `batch_id + fencing_token + status=running + lease_expires_at 未过期`。
- `asset_sync_batch` 新增 `triggered_by` 列（触发用户 user_id）；`TriggerSync` 创建 running 批次时写入，用于崩溃后还原原批次操作者。
- `TriggerSync` 在 `Create` 前先 `ReapExpiredRunning(accountID, now)` 清理本账号租约过期的 running 批次（崩溃批次自愈）。
- 已有 running 批次时 `Create` 触发唯一冲突 → 映射为 `ALREADY_EXISTS`（HTTP 409）。
- 终态 `Update` 把 `lease_expires_at` 清空，释放槽位。

### 动因

- 并发批次的根本风险是 stale 标记交错，fencing token 让旧任务在被 reap 后无法继续写入，避免“幽灵批次”污染资产状态。
- 不依赖 Redis，所有互斥通过 DB 部分唯一索引 + fencing token 实现，与现有技术栈一致。
- `triggered_by` 保证崩溃批次被 reap 时审计 actor 归因到原操作者，而非当次请求用户。
- `RenewLease` 同样校验 `lease_expires_at 未过期`：过期租约不可续租（标准 lease 语义），避免宽松续租架空 upsert/stale 写入门控，并消除“过期批次复活抢走 reap 窗口、导致新 TriggerSync 伪 409”的竞态。过期批次只能由 `ReapExpiredRunning` 回收。

## 决策 5：CES/hybrid 权威 scope 采用反向 stale 标记

### 背景

逐类型 stale 标记只能触及本轮 `SuccessfulTypes` 中的类型。当某类型从资源组 `product_names` 移除时，CES 不再查询该类型，它不会进入 `SuccessfulTypes`，逐类型标记无法触及它的旧资产——导致删除资源类型后旧资产永久保持 `active`。

### 决策

`ces`/`hybrid` 路径改用**反向 stale 标记**：对 account+region 调用 `MarkStaleByAccountRegionExceptTypesWithLease`，把该 scope 下所有 `active` 且 `last_synced_at < batch.StartedAt`（或 `last_synced_at IS NULL`）的 cloud_sync 资源标记为 `stale`，但跳过不确定类型 `exceptTypes = QueryFailedTypes ∪ ConversionFailedTypes ∪ persistFailedTypes`。防误删的主要依据是 `last_synced_at >= batch.StartedAt`（即本轮已 upsert 的资源），而非 `sync_batch_id` 排除——`sync_batch_id` 仅在 `FinalizeSuccess`（批次完整成功）时才提升为当前批次，中途失败时不 advance，无法作为可靠的排除条件。

### 动因

- CES 资源组的 `product_names` 在 `resource_level == "product"` 且非空时完整定义了资源组内的类型集合，是权威 scope。不在 scope 内的类型视为已移除，应标 stale。当 `product_names` 为空或 `resource_level != "product"` 时，scope 非权威，回退逐类型标记（见决策 1 关键口径）。
- 反向标记在 account+region 级别执行，而非逐类型，确保从资源组移除的类型也能被清理。
- `native`/通用/fake 路径 scope 非权威（只覆盖固定/有限类型），仍用逐类型标记，避免把未覆盖类型的资产误标 stale。
- 仍保留三类“不确定类型”豁免（查询失败/转换失败/持久化失败），保守保持 active，与逐类型语义一致。

## 决策 6：新增专用全量同步端口，避免 `limit≤500` 限制

### 背景

`AssetDiscoveryPort.ListResources` 经 `QueryService.normalizeAssetQuery`，`limit` 最大 500，适合前端或 Agent 工具的交互查询，不适合全量同步。`max_resources` 默认 20,000，复用交互查询端口会触发截断。

### 决策

新增只给 Asset Sync 使用的内部端口：观测层 `CloudFullSyncPort.ListAllResources`（provider 侧，华为云 adapter 实现）+ 资产层 `CloudDiscoveryPort.ListAllResources`（由 `DiscoveryAdapter` 适配）。要求：

- 不受 `limit <= 500` 限制。
- 内部按 CES API 分页读取。
- 仍要有 `max_resources` 防护，默认 20,000，按 region 独立计额。
- 返回同步摘要，写入 batch message 或扩展字段。

### 动因

- 交互查询与全量同步的语义不同：前者受 limit 约束、面向前端；后者需要完整扫描、面向批次。混用端口会导致全量同步被截断后误标 stale。
- 不在 Asset 层硬编码 provider 判断，保持模块边界清晰。

## 决策 7：账号配置快照冻结，保证批次一致性

### 背景

后台同步最长运行 `syncHardTimeout=30min`。`TriggerSync` 此前只冻结 `region`/`provider`，每个 region 调用 `ListAllResources` 时重新读库解析 `ExtraConfig`（`sync_mode`/`resource_group`/`region_projects`/`enterprise_project_id`/`max_resources`）、`project_id` 与凭据。同步窗口内修改这些字段会让同一批次混用多套配置。

### 决策

`TriggerSync` 触发时通过 `ResolveSyncAccount` 解析一次完整账号快照（`SyncAccountSnapshot` 含 `ProjectID`/`AuthType`/`CredentialRefID`/`Capabilities`/`ExtraConfig`），冻结后贯穿整个后台批次。`QueryService.resolveFullSyncEntry` 在 `q.Account` 非 nil 时跳过 `ResolveAccount` 的 DB 重读。

### 动因

- 冻结配置避免同步窗口内修改账号配置导致同批次混用多套 `sync_mode`/`resource_group`/`project_id`。
- 凭据不缓存明文：`ResolveAKSK` 按 `accountID` 重新加载并解密，冻结的是“用哪个账号”而非“账号当前密文”，最小化明文凭据内存驻留时间。
- 通用回退路径 `syncGeneric`（华为云不走此路径）暂不纳入冻结，因其用与交互式共享的 `AssetDiscoveryQuery`，加冻结字段语义混乱。

## 决策 8：批量 upsert + chunk 租约校验，支撑 20,000 资源性能

### 背景

此前对每个云资源调用 `UpsertCloudSyncWithLease`，每次独立事务、`SELECT ... FOR UPDATE` 锁 `asset_sync_batch` 校验租约、`findByCloudKey` 查旧资源再写入。20,000 条资源产生约 20,000 个事务、60,000 次 DB 往返与 20,000 次批次行锁，极易撞上 30 分钟硬超时。

### 决策

新增 `ResourceRepository.UpsertCloudSyncBatchWithLease`，按固定 chunk（500）批量写入：每个 chunk 仅一次事务、一次租约校验 + 一次原生 `INSERT ... ON CONFLICT DO UPDATE`。批量失败时回退逐条写入定位坏资源，保证失败隔离能力不退化。

### 动因

- 20,000 资源约 40 个 chunk、40 次事务，远低于此前数万次，避免硬超时。
- 直写 SQL 绕过 GORM 钩子，显式维护 `created_at`/`updated_at`。
- 失败隔离回退保证 stale 门控的失败隔离能力不退化。

## 决策 9：不暴露数据库自增 id，对外使用业务 ID

### 决策

`asset_resource` 主键为 `id BIGSERIAL`，对外 API 和跨上下文引用使用业务标识（`integration_account_id` + `cloud_resource_type` + `cloud_resource_id` + `region`），不暴露自增 `id`。唯一约束使用部分唯一索引 `(integration_account_id, cloud_resource_type, cloud_resource_id, region) WHERE source='cloud_sync' AND cloud_resource_id<>''`。

### 动因

- 与项目全局硬约束一致：不暴露数据库自增 `id` 作为跨上下文或对外 API 的业务标识。
- 含 `region` 的唯一索引可区分多区域同类型同云 ID 资源，避免互相覆盖。

## 决策 10：真实凭据账号不回落 fake 数据

### 决策

Adapter 路由策略：`auth_type=none` → fake provider；`auth_type=ak_sk` → 按 `sync_mode` 走 ces/hybrid/native；`auth_type=agency` 阶段一返回 unsupported。不在真实账号下静默回退 fake。

### 动因

- 真实凭据账号回落 fake 会掩盖配置/权限错误，产生误导性的“成功”同步结果。
- fake provider 仅用于 CI/E2E，必须与真实账号路径严格分离。
