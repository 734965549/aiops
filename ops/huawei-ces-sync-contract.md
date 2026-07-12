# 华为云 CES 资源同步稳定契约

> 本文档定义华为云 CES 资源分组同步的稳定接口、状态机、权限与运维契约（WHAT）。本次 CES 同步相关 Go 代码注释中的 `§X` 引用指向本文档对应章节；非 CES 同步注释（如资产仓储分页）请使用完整引用指明所属文档，避免误跳。若与设计背景或 backlog 文档冲突，以本文档为准。
>
> 相关文档：
> - 架构决策（WHY）：[../docs/adr-huawei-ces-sync.md](../docs/adr-huawei-ces-sync.md)
> - 发布升级步骤：[../docs/huawei-ces-sync-runbook.md](../docs/huawei-ces-sync-runbook.md)
> - 已知缺口与待办：[../docs/huawei-ces-sync-backlog.md](../docs/huawei-ces-sync-backlog.md)
> - 迁移契约：[migration-contract.md](migration-contract.md)
> - 可观测总契约：[cloud-observability-contract.md](cloud-observability-contract.md)

## §3 最小心智模型

主流程：**先发现，再入库，再增强，再 stale，最后 finalize。**

对应到系统里就是：

1. **发现**：从 CES 资源分组拿到本轮应该同步的资源清单。
2. **入库**：把可识别资源写入 `asset_resource`，形成基础事实。
3. **增强**：在 `hybrid` 模式下，用原生云服务 API 补充详情 labels。
4. **stale**：把本轮不再属于权威范围、且没有失败/截断风险的旧资源标记为 `stale`。
5. **finalize**：写完批次终态、计数、审计与可观测摘要。

### §3.1 常见状态解释表

### §3.2 快速理解版

主流程：**先发现，再入库，再增强，再 stale，最后 finalize。**

1. **发现**：从 CES 资源分组拿到本轮应该同步的资源清单。
2. **入库**：把可识别资源写入 `asset_resource`，形成基础事实。
3. **增强**：在 `hybrid` 模式下，用原生云服务 API 补充详情 labels。
4. **stale**：把本轮不再属于权威范围、且没有失败/截断风险的旧资源标记为 `stale`。
5. **finalize**：写完批次终态、计数、审计与可观测摘要。

### §3.3 开发实现版

代码注释中的 `§X` 引用只应指向**主契约中的稳定章节**，不要再引用索引页、历史拆分页或会频繁重排的临时附录。

建议代码中优先使用以下稳定章节：

- §3 最小心智模型
- §4 产品分层与同步模式
- §5 权限要求
- §6 华为 CES API 路径
- §8 同步流程
- §9 CES 资源到平台资源的映射
- §11 配置建议
- §13 错误处理
- §14 审计与日志
- §15 前端对接契约
- §18 风险与注意事项
- §21 已知增强缺口

章节编号尽量保持稳定，新增内容优先追加到末尾或预留区间，避免未来注释漂移。

- 新增稳定规则优先放入既有稳定章节，不要插入会打散引用的中间编号。
- 历史章节、迁移说明、待办和索引页都不应作为 `§X` 注释入口。
- 如果某个规则必须单独引用，优先新增稳定锚点，而不是重用旧的临时编号。

### §3.4 运维 / 前端版

运维与前端更适合直接读稳定摘要，不需要追所有实现细节：

- 配置：见 §11
- 终态与审计：见 §14
- 批次展示：见 §15.3
- 风险与已知缺口：见 §18、§21

### §3.5 summary 字段分层

`summary` 只按四类理解：展示、对账、门控、诊断。新增字段先归类，再决定是否进入主契约正文。

- 展示字段：`sync_mode`、`resource_group_name`、`resource_group_id`、`projects`、`regions`、`summary.scopes[]`
- 对账字段：`raw_fetched_count`、`mapped_count`、`unique_discovered_count`、`persisted_count`、`duplicate_count`、`persist_failed_count`、`discovered_count`、`enriched_count`、`enrichment_failed_count`
- 门控字段：`max_resources_reached`、`product_names_empty`、`query_failed_types`、`conversion_failed_types`、`enrichment_failed_types`、`enrichment_stage_error`、`writeback_failed_count`
- 诊断字段：`failed_scopes`、`partial_reason`、`partial_reasons`、`enrichment_warnings`、`config_mode_fallback`

约束：

- 对账字段用于验收、测试和批次归档，不应由前端重新计算同义字段。
- 门控字段用于决定是否允许 stale / finalize / partial 判定，不能把 message 当作协议字段再解析一遍。
- 诊断字段用于排障，不作为业务判断的唯一来源。
- 展示字段仅用于 UI 说明和检索辅助，不能承载新的业务语义。
- `partial`、`stale`、`fallback whitelist` 的判定必须由实现层单点产生，前端 / 审计 / 报表只消费结果，不允许复算。
- `partial` 的单点来源优先级：`failed_scopes` / `enrichment_failed_types` / `enrichment_stage_error` / `writeback_failed_count` / `product_names_empty` / `config_mode_fallback`，禁止仅凭 `message` 推导。
- `stale` 的单点来源是权威发现路径的成功集合与租约校验结果，禁止用 `native` / `fake` / `generic` / fallback whitelist 的输出反向标记。
- `max_resources` 只表示 raw row budget，不是 unique 资源数上限。

### §3.6 状态机

```text
running
  ├─ 基础发现成功且无 required failure → success
  ├─ 基础发现成功，但部分 scope / 增强 / 回写失败 → partial
  ├─ 前置失败 / 鉴权失败 / 租约丢失 / 上下文取消 → failed
  └─ 兜底白名单 / max_resources / 其他退化信号 → 至少 partial
```

| 状态 | 一句话解释 | 典型含义 |
| --- | --- | --- |
| `running` | 同步正在进行中 | 任务已创建，后台还在跑，可能在续租 |
| `success` | 基础发现完成且没有必须失败的增强 | 资源入库完成，结果可视为当前口径 |
| `partial` | 基础结果已落地，但有部分 scope / 增强 / 回写失败 | 需要看 `failed_scopes`、`enrichment_failed_types`、`enrichment_warnings` |
| `failed` | 本批次未能完成或无法保证结果可信 | 通常是前置校验、鉴权、租约、上下文取消等问题 |

### §3.6 稳定引用与防漂移规则

代码注释中的 `§X` 引用只应指向**主契约中的稳定章节**，不要再引用索引页、历史拆分页或会频繁重排的临时附录。

建议代码中优先使用以下稳定章节：

- §3 最小心智模型
- §4 产品分层与同步模式
- §5 权限要求
- §6 华为 CES API 路径
- §8 同步流程
- §9 CES 资源到平台资源的映射
- §11 配置建议
- §13 错误处理
- §14 审计与日志
- §15 前端对接契约
- §18 风险与注意事项
- §21 已知增强缺口

章节编号尽量保持稳定，新增内容优先追加到末尾或预留区间，避免未来注释漂移。

- 新增稳定规则优先放入既有稳定章节，不要插入会打散引用的中间编号。
- 历史章节、迁移说明、待办和索引页都不应作为 `§X` 注释入口。
- 如果某个规则必须单独引用，优先新增稳定锚点，而不是重用旧的临时编号。

## §4 产品分层与同步模式

三层能力：

| 层级 | 同步模式 | 目标 | 权限 | 结果 |
| --- | --- | --- | --- | --- |
| P0/P1 | `ces` | 指定资源分组下看到多少，平台同步多少 | `CES ReadOnlyAccess` | 资源数量与指定分组一致 |
| P2 | `hybrid` | 先按指定资源分组发现，再用原生云服务 API 补详情 | CES 只读 + 按需云服务只读 | 补充 IP、规格、VPC 等 |
| P3 | `hybrid` + topology/inspection | 结合 CES、原生 API、日志、链路做拓扑、巡检 | 按能力逐项授权 | 拓扑、配置巡检、根因候选 |

同步模式定义：

| 模式 | 定位 | 行为 | 是否推荐 |
| --- | --- | --- | --- |
| `ces` | 默认模式 | 只依赖 CES 资源分组/资源列表发现指定分组下资源，不调用 ECS/RDS/ELB/EVS/VPC 等原生 List API 作为入库前置条件 | 推荐，P0/P1 默认 |
| `hybrid` | 增强模式 | 先执行 `ces` 指定分组发现并入库，再对已发现资源按类型调用原生 API 增强详情；**任一增强失败都将批次统一标记为 `partial`**，但不影响基础资源入库 | P2 推荐 |
| `native` | 兼容旧模式 | 沿用 ECS/CCE/RDS/ELB 等原生 API 发现资源，主要用于回退或历史兼容 | 不作为完整性口径 |

默认策略：

```text
huawei_cloud + auth_type=ak_sk:
  default sync_mode = ces

huawei_cloud + auth_type=none:
  fake provider, only for CI/E2E

huawei_cloud + auth_type=agency:
  first phase unsupported; later reuse ces/hybrid semantics
```

## §5 权限要求

### §5.1 最小目标权限

如果目标是“和指定 CES 资源分组资源数一致”，优先授予 CES 只读类权限。本文中的 `CES ReadOnlyAccess` 表示平台侧所需的 CES 只读能力口径；实际授权时需按华为云 IAM / 细粒度授权体系选择对应策略（如 `CES ReadOnlyAccessPolicy` / `CESServiceReadOnlyPolicy`，具体以官方说明为准：<https://support.huaweicloud.com/intl/en-us/productdesc-ces/ces_07_0009.html>）。

该类权限应允许读取 CES 指标、资源分组、资源分组下资源列表等只读数据。

### §5.2 增强权限

补齐资源详情（ECS 私网 IP、VPC ID、云硬盘挂载关系、OBS bucket 区域等）需额外授予：

```text
ECS ReadOnlyAccess
EVS ReadOnlyAccess
VPC ReadOnlyAccess
ELB ReadOnlyAccess
RDS ReadOnlyAccess
OBS ReadOnlyAccess
DCS ReadOnlyAccess
DMS ReadOnlyAccess
CCE ReadOnlyAccess
CBR ReadOnlyAccess
```

增强权限不是 CES 资源分组同步的前置条件。没有增强权限时，平台仍应能用 CES 返回的 namespace、dimension、resource_name 同步基础资产。

### §5.3 多项目与多区域

华为云 `project_id` 通常与区域相关。平台账号模型处理：

- 单区域账号：现有 `project_id` + `regions[0]` 即可。
- 多区域账号：通过 `region_projects` 表达 `region -> project_id` 映射。
- 企业项目：CES 资源分组支持 `enterprise_project_id` / `all_granted_eps`。

`region_projects` 已落地（放入 `extra_config` JSON，无需 DB 迁移）。`SyncModeConfig` 新增 `RegionProjects` 字段与 `ResolveProjectID(region, fallback)` 方法；`adapter.go` 在 `queryMetricsCES`/`listResourcesReal`/`listAllResourcesReal` 三处均按当前 region 解析 project_id，未命中回落 `Account.ProjectID`，保证旧的单 project_id 账号零行为变化。解析阶段过滤空 region/project_id、大小写不敏感去重。

资源组同样支持按 region 解析：`RegionProject` 可选填 `resource_group_id` / `resource_group_name`，`SyncModeConfig` 提供 `ResolveResourceGroupID(region)` / `ResolveResourceGroupName(region)`，`listAllResourcesCES` 按当前 region 选用对应资源组，未命中或对应项为空时回落全局 `ResourceGroupID` / `ResourceGroupName`。资源组是 project 作用域的，多区域账号需为每个 region 指定对应 `resource_group_id`。

## §6 华为 CES API 路径

Go SDK 中可用的 CES v2 资源分组相关接口：

```text
ListResourceGroups
  GET /v2/{project_id}/resource-groups

ShowResourceGroup
  GET /v2/{project_id}/resource-groups/{group_id}

ListResourceGroupsServicesResources
  GET /v2/{project_id}/resource-groups/{group_id}/services/{service}/resources
```

核心字段：

- `ListResourceGroupsResponse.count`
- `resource_groups[].group_id`
- `resource_groups[].group_name`
- `resource_groups[].resource_statistics.total`
- `ShowResourceGroupResponse.product_names`
- `ListResourceGroupsServicesResourcesResponse.count`
- `resources[].resource_name`
- `resources[].dimensions[]`
- `resources[].status`
- `resources[].event_status`
- `resources[].enterprise_project_id`
- `resources[].tags`

`product_names` 格式示例：

```text
SYS.ECS,instance_id;SYS.EVS,disk_name;SYS.RDS,rds_cluster_id
```

解析为 `{ service, dim_name }` 对，再按 `service + dim_name` 查询该资源组下资源列表。

## §7.2 Asset Sync 专用分页端口

当前 `AssetDiscoveryPort.ListResources` 经 `QueryService.normalizeAssetQuery`，`limit` 最大 500，适合前端或 Agent 工具的交互查询，不适合全量同步。

Asset Sync 使用专用内部端口，不受 `limit <= 500` 限制。端口分两层：

- 观测层 `CloudFullSyncPort`（`internal/observability/application`）：provider 侧全量同步端口，`ListAllResources` 返回资源列表与同步摘要；华为云 adapter 实现该端口。
- 资产层 `CloudDiscoveryPort`（`internal/asset/application`）：asset SyncService 依赖的发现端口，含 `ListResources`（交互式）与 `ListAllResources`（全量），由 `DiscoveryAdapter` 适配观测层 QueryService。

要求：

- 不受 `limit <= 500` 限制。
- 内部按 CES API 分页读取。
- 仍要有 `max_resources` 防护，默认建议 20,000，避免异常配置导致无限扫描。该上限按 region 独立计额，多区域账号实际可同步 `区域数 × max_resources`。
- 返回同步摘要，写入 batch message 或扩展字段。

## §8 同步流程

### §8.1 `ces` 默认同步流程

```text
POST /api/assets/sync
  -> Auth + app:assets:write
  -> SyncService.TriggerSync
  -> Resolve integration account
  -> sync_mode = ces
  -> Resolve AK/SK
  -> for each region/project:
       1. 创建或加载 cloud application
       2. 调用 CES ListResourceGroups
       3. 选择目标资源组
       4. 调用 ShowResourceGroup
       5. 解析 product_names
       6. 对每个 service/dim_name 调用 ListResourceGroupsServicesResources
       7. 分页收集资源
       8. map CES resource -> CloudResource
       9. upsert asset_resource
      10. 对成功 scope 执行 stale 标记
  -> 更新 asset_sync_batch
  -> 写审计
```

### §8.2 `hybrid` 增强同步流程

`hybrid` 严格拆分为两阶段，保证「增强失败不影响基础入库」：

```text
POST /api/assets/sync
  阶段一（发现 + 基础落库）：
    -> provider ListAllResources (sync_mode=hybrid) 仅走 CES 资源分组全量发现
    -> 返回基础资源（只含 CES labels）与 discovery summary
    -> SyncService 按 chunk upsert 基础资源（UpsertCloudSyncBatchWithLease，带租约）
  阶段二（增强 + 带租约回写，仅基础 upsert 成功后）：
    -> SyncService 调用 CloudEnrichmentPort.EnrichAllResources
    -> provider 按 cloud_resource_type 分组，对已授权类型调用原生 API 补充详情
    -> 按 ProviderRef 匹配原生资源，合并 labels（不覆盖已有 CES label）
    -> 返回增强 summary 与实际合并了 label 的资源子集（Enriched）
    -> SyncService 将 Enriched 转为 CloudSyncLabelPatch，通过 PatchCloudSyncLabelsBatchWithLease 带租约回写
       （只更新本轮已 upsert 的 active 资源 labels 列，不影响 created/updated 计数）
    -> enrichment 失败：记录 warning + EnrichmentFailedTypes，不回滚 CES 资源入库
    -> batch message 写入 enrichment summary
```

`hybrid` 的成功标准是 CES 基础资源完整入库；原生 API 增强只影响详情丰富度，不影响资源发现完整性。增强失败分两类：

| 分类 | 触发条件 | 影响 | scope 命名 |
| --- | --- | --- | --- |
| **required enrichment failure** | 整个类型原生 API 调用失败、增强阶段致命错误（端口不可用等）、label 回写失败 | 批次终态 `partial`，记入 `enrichment_failed_types` | 大类名（`dms`、`ecs` 等） |
| **best-effort warning** | DMS 单一子服务失败、VPC `subnet_count` 统计失败 | 不影响批次状态，记入 `enrichment_warnings` | 细粒度（`dms.kafka`、`dms.rocketmq`、`vpc.subnet_count`） |

`success` 仅表示基础发现完整且无 required enrichment failure。`enrichment_warnings` 非空时批次仍可为 `success`，表示增强数据部分缺失但不影响资源发现完整性。增强阶段整体失败或 label 回写失败只置 `partial` 并分别记录 `EnrichmentStageError` / `WritebackFailedCount`，不递增 `EnrichmentFailedCount`（保持 `enrichment_failed_count == len(enrichment_failed_types)` 不变式），不影响基础计数与 stale 门控；租约丢失属于致命错误，立即中止整批同步。

**装配错误防护**：`sync_mode=hybrid` 但 `CloudEnrichmentPort` 未注入（装配遗漏）时，SyncService 不静默成功，而是将批次置 `partial` 并在 `enrichment_stage_error` 记录 `hybrid enrichment port not injected (assembly error)`；基础资源仍按阶段一结果入库，不回滚，不递增 `EnrichmentFailedCount`（与增强阶段失败一致，保持 `enrichment_failed_count == len(enrichment_failed_types)` 不变式）。非 hybrid 模式（`ces`/`native`/`fake`）下端口未注入仍静默跳过增强。

不变式：`enrichment_failed_count == len(enrichment_failed_types)`；`enrichment_warnings` 独立于 `enrichment_failed_types`，不参与 partial 判定。

增强内部按子服务/可选字段独立处理：

- **DMS**：Kafka 与 RocketMQ 是独立子服务，任一失败不阻断另一个；单个子服务失败记为 best-effort warning（scope `dms.kafka` 或 `dms.rocketmq`），两者都失败才记为 required enrichment failure（scope `dms`，进 `enrichment_failed_types`）。部分成功时返回已收集结果。
- **VPC**：`subnet_count` 为可选增强；子网统计失败时 VPC 仍正常返回，仅缺少 `subnet_count` label，记为 best-effort warning（scope `vpc.subnet_count`），不影响批次状态。

### §8.3 `native` 兼容同步流程

`native` 仅用于兼容旧路径或紧急回退：

```text
sync_mode=native
  -> ECS/CCE/RDS/ELB resource client
  -> 按旧 scope 执行 upsert/stale
```

`native` 不承诺与任何 CES 资源分组数量一致。`listAllResourcesNative` 显式遍历 `legacyNativeResourceTypes`（固定 ECS/CCE/RDS/ELB 旧 4 类）逐类调用；单类失败只记入 `FailedScopes` 并跳过，全部类型失败才返回错误；查询成功的类型写入 `SuccessfulTypes`（即使返回 0 条资源）。EVS/VPC/DCS/DMS 的详情增强仅在 `hybrid` 模式下进行，不进入 native 兼容路径。

### §8.4 资源组选择策略

同步范围严格限定为“指定 CES 资源分组下资源”。不存在“CES 总览全量”的隐式口径；**不再提供“选择 total 最大的资源组”这类静默回退**。

默认候选名“全部资源”并非 CES 系统内置分组，需要用户在 CES 控制台预先创建同名资源分组。

选择顺序：

1. 如果请求或账号配置指定 `resource_group_id`，使用该分组（仍经 `ShowResourceGroup` 校验存在）。
2. 如果指定 `resource_group_name`，按名称精确匹配（大小写不敏感）。**任何名称未命中均直接失败**，不回退到其他分组。
3. 未指定 `resource_group_name` 时，依次尝试默认候选名：`全部资源` / `All resources` / `All Resources`。
4. 以上均未命中，批次失败，返回脱敏错误：`no CES resource group matched (specified id/name or default candidates)`。

生产环境推荐显式填写 `resource_group_id`。

### §8.5 product_names 解析

`ShowResourceGroupResponse.product_names` 解析规则：

```text
SYS.ECS,instance_id;SYS.EVS,disk_name
```

转为：

```json
[
  { "service": "SYS.ECS", "dim_name": "instance_id" },
  { "service": "SYS.EVS", "dim_name": "disk_name" }
]
```

要求：

- 去重。
- 忽略空项。
- service 统一保留大写原值用于 CES 查询。
- CES `product_names` 单项只含单个首层维度名称（`服务命名空间,首层维度名称`）；同一 service 多维度须拆成多个 product 项（以 `;` 分隔），不支持单项多 dim。解析时若遇单项多 dim，按单项单 dim 拆成多个 `cesProduct` 分别查询，并输出 warn 日志标记异常格式。
  - **拆分策略说明**：对 `SYS.VPC` 等聚合 namespace，不同 `dim_name` 映射不同 `cloud_resource_type`（见 §9.3），拆分是必需且正确的。对其他 namespace，拆分属防御性兜底；官方 `ListResourceGroupsServicesResources` 的 `dim_name` 参数支持逗号多维度联合查询（"多维度用`,`分隔"），拆分为多次单维度查询在语义上不等价，但 CES `product_names` 正常格式单项只含单个 dim，拆分仅在异常格式时触发。
- **`resource_level` 响应字段**：官方 `ShowResourceGroup` API 的 `resource_level` 是**响应字段**（非查询参数），取值 `product`（云产品）或 `dimension`（子维度），见 https://support.huaweicloud.com/api-ces/ShowResourceGroup.html 。`product_names` 仅在 `resource_level=product` 时有意义（"创建资源层级为云产品时的云产品名称"）。SDK `huaweicloud-sdk-go-v3 v0.1.201` 的 `ShowResourceGroupResponse.ResourceLevel` 已暴露该字段（`Value()` 返回字符串）。代码从 Show 响应读取 `resource_level` 并传递到 `CESResourceDiscoverySummary.ResourceLevel`；P0 阶段仅支持 `resource_level=product` 的资源组，`dimension` 级或未知/空层级直接返回 `FAILED_PRECONDITION`，不静默回退。仅 `resource_level=product` 且 `product_names` 非空时 scope 才是权威范围，允许反向 stale（见 §13.1）。
- 平台 `cloud_resource_type` 使用映射表归一化为小写类型。
- `namespace` 的权威性分层：
  - `resource_level=product` 且 `product_names` 非空时，`ShowResourceGroup` 返回的 namespace/dim_name 组合是**权威 scope**，可用于反向 stale。
  - `product_names` 为空时，`fallbackCESProducts` 只是**兜底白名单**；它不完整，且仅允许已证实的维度进入同步范围。
  - `SYS.VPC` 是聚合 namespace，其 `dim_name` 决定最终平台类型；其余 namespace 不因 `dim_name` 改变平台类型。
- 如果 `product_names` 为空，使用内置 CES namespace 白名单兜底，但该白名单不完整且部分维度可能错误；它只能用于有限发现与兼容展示，**不能作为权威 scope，也不得用于反向 stale**。batch message 必须记录 `product_names_empty=true`，且批次至少标记 `partial`，提示同步可能不完整，见 §13.1。前端批次页必须对该标志做显著风险提示，推荐展示“兜底发现，可能不完整”。
- 兜底白名单只能用于兼容展示，不得提升为权威 scope，不得触发反向 stale，批次必须至少 `partial`。代码里建议用 `isFallbackWhitelistScope()` / `isAuthoritativeCESScope()` 显式区分，避免把 fallback 误当正常流程。

### §8.6 分页策略

`ListResourceGroupsServicesResources` 分页参数：`offset`、`limit`。

```text
page_limit = 100
offset = 0
while offset < count:
    request offset/page_limit
    append resources
    if returned < page_limit: break
    offset += page_limit
```

停止条件：当前 service/dim_name 资源取完；已从云端接收的原始行数（`raw_fetched_count`，含重复和无效资源）达到 `max_resources`；context cancelled；CES 返回不可恢复错误。

失败策略：

- 单个 `service/dim_name` 失败，不应导致整个账号同步立刻失败。
- 记录 `failed_count++` 和 scope message。
- 只有所有 scope 都失败且没有任何资源 upsert 时，批次状态为 `failed`。
- 部分成功时，批次状态为 `partial`。

## §9 CES 资源到平台资源的映射

### §9.1 CloudResource 映射

| 平台字段 | 来源 | 说明 |
| --- | --- | --- |
| `ResourceID` | `ces:{region}:{namespace}:{primary_dim_value}` | 平台内临时资源 ID |
| `Name` | `resource_name` 或主维度值 | 控制台显示名优先 |
| `Type` | namespace 映射 | 例如 `SYS.ECS -> ecs` |
| `Region` | account region | 当前查询区域 |
| `Status` | `status` / `event_status` | 优先 `status`（指标告警状态），`event_status`（事件告警状态）兜底；仅 CloudResource 临时字段，不直接落库 |
| `ProviderRef` | 主维度值 | 后续指标查询用 |
| `Labels.namespace` | service | 例如 `SYS.ECS` |
| `Labels.dim_name` | dim_name | 例如 `instance_id` |
| `Labels.enterprise_project_id` | response field | 可选 |
| `Labels.resource_group_id` | selected group | 便于追溯 |
| `Labels.resource_group_name` | selected group | 便于排查 |
| `Labels.ces_status` | `status` | CES 指标告警状态（health/unhealthy/no_alarm_rule），原始值不复用 fallback |
| `Labels.ces_event_status` | `event_status` | CES 事件告警状态，原始值不复用 fallback |
| `Labels.tag.<key>` | `tags` JSON 字符串解析 | CES 资源标签，前缀 `tag.` 防碰撞；单资源上限 20 个，key 截断 128、value 截断 256；命中敏感 pattern（secret/token/password/passwd/pwd/key/authorization/credential，大小写不敏感）的 tag key 直接丢弃不落库 |

### §9.2 主维度选择

主维度值用于 `cloud_resource_id` 和 `ProviderRef`。选择顺序：

1. 优先使用请求中的 `dim_name` 对应的 dimension value。
2. 如果没有匹配，使用第一个非空 dimension value。
3. 如果仍为空，使用 `resource_name`。
4. 如果都为空，丢弃该资源并记录 `invalid_resource_count`。

### §9.3 namespace 映射表

| CES namespace | 平台 `cloud_resource_type` | 平台 `resource_type` | 说明 |
| --- | --- | --- | --- |
| `SYS.ECS` | `ecs` | `host` | 弹性云服务器 |
| `SYS.EVS` | `evs` | `storage` | 云硬盘 |
| `SYS.VPC`（`dim_name=publicip_id`） | `eip` | `network` | 弹性公网 IP |
| `SYS.VPC`（`dim_name=bandwidth_id`） | `bandwidth` | `network` | 共享带宽 |
| `SYS.VPC`（`dim_name=subnet_id`） | `subnet` | `network` | 子网 |
| `SYS.VPC`（`dim_name=peering_id`） | `peering` | `network` | VPC 对等连接 |
| `SYS.VPC`（`dim_name=vpc_id` 或未知 dim） | `vpc` | `network` | VPC 实体（兜底） |
| `SYS.ELB` | `elb` | `service` | 弹性负载均衡 |
| `SYS.RDS` | `rds` | `database` | 关系型数据库（单机/主备） |
| `SYS.RDS_MYSQL_CLUSTER` | `rds` | `database` | RDS for MySQL 集群版实例（维度 `rds_cluster_id`+`rds_instance_id`，见 https://support.huaweicloud.com/usermanual-rds-mysql/rds_06_0001.html ） |
| `SYS.OBS` | `obs` | `storage` | 对象存储 |
| `SYS.DCS` | `dcs` | `middleware` | 分布式缓存 |
| `SYS.DMS` | `dms` | `middleware` | 分布式消息 |
| `SYS.CCE` | `cce` | `service` | 容器集群 |
| `SYS.CBR` | `cbr` | `backup` | 云备份 |
| `SYS.VPCEP` | `vpcep` | `network` | VPC 终端节点 |
| `SYS.NAT` | `nat` | `network` | NAT 网关 |
| `SYS.SFS` | `sfs` | `storage` | 弹性文件服务 |
| `SYS.APM` | `apm` | `service` | 应用性能管理 |
| `SYS.CES` | `ces` | `monitor` | 可用性监控等 CES 自身资源 |

未知 namespace：

- `cloud_resource_type = lower(namespace without SYS. prefix)`
- `resource_type = service`
- `Labels.namespace` 必须保留原始 namespace
- batch summary 中记录 `unknown_namespace_count`

> **SYS.VPC 拆分说明**：CES `SYS.VPC` 是网络资源聚合 namespace，其主维度按 `dim_name` 拆分为 `publicip_id`/`bandwidth_id`/`subnet_id`/`peering_id`/`vpc_id`（参考 https://support.huaweicloud.com/eu/usermanual-ces/en-us_topic_0202622212.html ），并非单一 VPC 实体。映射由 `resolveNamespaceMappingByDim(namespace, dimName)` 按 `dim_name` 分派：`publicip_id->eip`、`bandwidth_id->bandwidth`、`subnet_id->subnet`、`peering_id->peering`、`vpc_id` 及未知 dim 兜底为 `vpc`。映射逻辑见 `ces_resource_mapper.go`。迁移 `0034` 已把存量 `cloud_resource_type='vpc'`（`labels.namespace='SYS.VPC'`）的行按 `labels.dim_name` 回填为对应子类型；native VPC 实体（无 namespace label）保持 `vpc` 不受影响。
>
> **区域差异注意**：`SYS.VPC` 的 `publicip_id` / `bandwidth_id` / `subnet_id` 视为已确认可兜底的维度；`vpc_id` 与 `peering_id` 只在 `product_names` 显式包含且资源组返回这些维度时才查询，不纳入兜底白名单。若后续要把它们升级为兜底维度，必须补充对应区域的真实 CES 响应证据。

### §9.4 asset_resource 字段

复用迁移 `0023` 的字段：

```text
source = cloud_sync
integration_account_id = account_id
cloud_resource_id = primary_dim_value
cloud_resource_type = mapped type
region = account region
sync_status = active / stale
last_synced_at = now
sync_batch_id = batch_id（最近成功同步批次；只有整个批次成功完成后，才更新到资源上）
```

唯一约束使用部分唯一索引（迁移 `0026`）：

```text
(integration_account_id, cloud_resource_type, cloud_resource_id, region) WHERE source='cloud_sync' AND cloud_resource_id<>''
```

`SYS.VPC` 多 `dim_name` scope 的区分已由 §9.3 的子类型拆分落地：不同 dim 映射为不同 `cloud_resource_type`（eip/bandwidth/subnet/peering/vpc），上述 4 列唯一键即可区分，无需额外纳入 `namespace`/`dim_name` 列。

### §9.5 同步计数对账公式

批次摘要计数满足以下恒等式，用于验收对账与测试 mock 构造：

```text
raw_fetched_count        = mapped_count + invalid_resource_count
mapped_count             = unique_discovered_count + duplicate_count
unique_discovered_count  = persisted_count + persist_failed_count
```

字段语义：

- `raw_fetched_count`：**本轮预算内实际进入后续处理链路的原始行数**。它只统计已经被当前 `max_resources` 预算接住、并进入映射/去重/落库流水线的原始行，不表示“云端总返回数”，也不表示“远端接口实际吐出的总行数”。实现上，它对应 `listResourcesForProduct` 按 `remaining` 截断后返回的条目数；`RawFetchedCount += len(pageResources)` 只在这些已纳入预算的行上累加。被 `max_resources` 裁掉的尾部行一律不计入任何计数，仅由 `max_resources_reached` 标志体现。`max_resources` 预算就是按这个字段消耗的，而不是按去重后的 unique 资源数：`remaining = max_resources - raw_fetched_count`；重复资源和无效资源同样会消耗预算，确保超大重复/异常返回不会绕开 `max_resources` 保护持续翻页，见 §7.2/§8.6。
- `mapped_count`：映射成功（含重复）的资源数。
- `invalid_resource_count`：无法转换为资产的非法资源数（缺主维度/格式不合规）。
- `unique_discovered_count`：按唯一键（见 §9.4）去重后进入待写入集合的资源数；`Discovered` 与之同义，保留以兼容既有字段语义。
- `duplicate_count`：同一批次内按唯一键去重折损的资源数。
- `persisted_count`：本批实际成功写入 `asset_resource.sync_status=active` 的行数。
- `persist_failed_count`：进入待写入集合（`unique_discovered_count`）但未能成功落库的资源数。含两类：asset 层 `buildCloudResourceBatch` 因缺 `type`/`id` 转换失败（provider 漏网进入 `result.Resources` 的非法资源），以及批量 upsert 回退逐条后仍失败的资源（见 §13.1/§13.3）。两者均计入本字段，以保证恒等式 `unique_discovered_count = persisted_count + persist_failed_count` 成立；asset 层转换失败的类型同步进入 `persistFailedTypes`，按 §13.1 保守门控禁止 stale。

## §11 配置建议

账号扩展配置（写入 `extra_config` JSON）：

```json
{
  "sync_mode": "ces",
  "resource_group_name": "全部资源",
  "resource_group_id": "",
  "enterprise_project_id": "all_granted_eps",
  "max_resources": 20000,
  "region_projects": [
    { "region": "cn-south-1", "project_id": "xxx", "resource_group_id": "rg-south", "resource_group_name": "南方全量" },
    { "region": "cn-north-4", "project_id": "yyy", "resource_group_id": "rg-north", "resource_group_name": "北方全量" }
  ]
}
```

字段说明：

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `sync_mode` | `ces` | `ces` 为默认完整性口径；`hybrid` 为增强模式；`native` 仅兼容旧路径 |
| `resource_group_name` | 空（占位提示 `全部资源`） | 前端仅把 `全部资源` 作为占位提示，留空提交即“未指定”；后端按 §8.4 step 3 依次尝试默认候选名 `全部资源` / `All resources` / `All Resources`，显式填写后按名称精确匹配，未命中即失败、不回退候选名。作为未命中 region 时的全局回落值 |
| `resource_group_id` | 空 | 指定后优先使用，生产推荐；若未配置 region 专属资源组，顶层值作为该 region 的回落值 |
| `enterprise_project_id` | 空 | 可选，支持 `all_granted_eps` |
| `max_resources` | `20000` | 单区域单次同步保护上限（按 region 独立计额，多区域为 区域数 × max_resources） |
| `region_projects` | 空 | region → project_id 映射数组；adapter 按当前 region 选用对应 project_id，未命中回落账号 `project_id`。每项可选填 `resource_group_id` / `resource_group_name`，按 region 选用对应资源组，未命中或为空时回落全局值。已落地（放 `extra_config`，无 DB 迁移） |

## §13 错误处理

本章统一描述错误、部分失败、截断与 stale 门控；细分规则只作为内部锚点保留，代码注释优先引用本章而非子节。

| 场景 | 批次状态 | 处理 |
| --- | --- | --- |
| AK/SK 缺失 | `failed` | 返回 `FAILED_PRECONDITION` |
| project_id 缺失 | `failed` | 返回 `INVALID_ARGUMENT` |
| region 缺失 | `failed` | 返回 `INVALID_ARGUMENT` |
| CES 鉴权失败 | `failed` | 脱敏为 `provider authentication failed` |
| 找不到资源组 | `failed` | message 写 project/region，不写敏感信息 |
| 某 namespace 查询失败 | `partial` | 其他 namespace 继续 |
| 部分资源无主维度 | `partial` | 计入 invalid_resource_count |
| 达到 max_resources | `partial` | message 写 `max_resources_reached=true` |
| `product_names` 为空（兜底白名单） | `partial` | message 写 `product_names_empty=true`，提示同步可能不完整，见 §8.5/§13 |
| context cancelled | `failed` | 可重试 |

### §13.1 stale 标记门控

本节只定义 stale 允许条件；实现里不要把 message 文本当协议字段再解析一遍。

stale 标记按类型逐项判断，只有同时满足以下三项的类型才允许执行 stale；否则该类型旧资产保持 active：

1. **provider 查询完整**：该类型在 `SuccessfulTypes` 中。同一资源类型可能由多个 `service+dim_name` scope 组成（例如 `SYS.ELB/loadbalancer_id` 与 `SYS.ELB/l7policy_id` 均映射到 `elb`），只要任一 scope 查询失败，该类型即记入 `QueryFailedTypes` 并从 `SuccessfulTypes` 剔除——只有该类型的**所有 scope 都成功**才留在 `SuccessfulTypes` 中。
2. **资源转换完整**：该类型不在 `ConversionFailedTypes` 中（无 `mapCESResource` 因缺主维度而丢弃）。
3. **全部资源成功持久化**：该类型本轮无 upsert 失败。

达到 `max_resources` 的 region 整体禁止 stale。`ces`/`hybrid`/`native` 路径均填充 `SuccessfulTypes`：native 逐类调用旧 4 类，查询成功的类型（即使返回 0 条资源）计入 `SuccessfulTypes`，因此某类资源全部消失时旧记录可被标记 stale。仅 `auth_type=none` 的 fake 路径不填充 `SuccessfulTypes`，此时回退到本轮成功入库类型，仍排除存在 upsert 失败的类型。通用（非 CES）同步路径同样遵循“全部资源成功持久化才执行 stale”。`QueryFailedTypes` 会写入批次 message（`query_failed_types=...`）用于排查。

**fake 路径 sync_mode 标记（P1 修正）**：`auth_type=none` 的 fake 路径 adapter 显式返回 `sync_mode="fake"`，不填充 `SuccessfulTypes`。SyncService 对未显式声明 `sync_mode` 的 summary **防御性按 `fake`（非权威）处理，不再回落到 `ces`**，避免覆盖面有限的路径被误当作权威 scope 触发反向 stale（误把未在本轮返回的旧资产标记 stale）。真实 `ces`/`hybrid`/`native` 路径均由 adapter 显式设置 `sync_mode`。

**CES/hybrid 权威 scope 反向 stale 标记**：`ces`/`hybrid` 的资源组在 `resource_level=product` 且 `product_names` 非空时，`product_names` 完整定义了资源组内的类型集合，是权威 scope。对 account+region 调用 `MarkStaleByAccountRegionExceptTypes`，把该 scope 下所有 `active` 的 cloud_sync 资源（排除当前批次）标记为 `stale`，但跳过不确定类型 `exceptTypes = QueryFailedTypes ∪ ConversionFailedTypes ∪ persistFailedTypes`。这样：

- 查询成功且本轮 0 资源的类型 → 旧资产 stale（与逐类型语义一致）；
- 从资源组移除的类型（不在 scope，也不在 exceptTypes）→ 旧资产 stale（修复点）；
- 查询失败/转换失败/持久化失败的类型 → 保持 active（保守，与逐类型一致）。

`native`/通用/fake 路径 scope 非权威，仍用逐类型标记，不反向标记。

**`product_names` 为空时的批次状态**：回落到内置兜底白名单（见 §8.5），该白名单不完整。此场景批次至少标记 `partial`，batch message 含 `product_names_empty=true (fallback whitelist used, sync may be incomplete)`。

**截断探测（P1 修正）**：

- **CES/hybrid 权威路径**：`remaining` 按 `raw_fetched_count`（已从云端接收的原始行数）计算：`remaining = max_resources - raw_fetched_count`。`listResourcesForProduct` 翻页拉取时，一旦 `len(out) >= remaining`（当前产品已达到剩余原始行数配额上限）即置 `truncated=true` 并提前退出翻页。`discoverCESResources` 收到 `truncated=true` 时无条件置 `MaxResourcesReached=true` 并 `break`，被截断类型**不计入** `SuccessfulTypes`；SyncService 整 region 跳过 stale。该判断提前于 `raw_fetched_count >= maxResources`，覆盖"单产品自身超过上限但全部 conversion failed 或全部重复导致 `raw_fetched_count` 未达上限"的边界。仅当某产品资源数恰好等于 `remaining` 且远端确无更多数据（`remoteExhausted=true`、无后续产品）时才视为完整扫描，不标记截断。
- **native 路径**：`listAllResourcesNative` 对每类请求 `remaining+1` 条探测截断。返回超过 `remaining` 条即置 `MaxResourcesReached=true`，只取 `remaining` 条并 `break`，被截断类型不计入 `SuccessfulTypes`；SyncService 整 region 跳过 stale。因截断提前中断不算“全部类型失败”，不会触发空结果错误。
- **通用（非华为）路径**：`AssetDiscoveryPort.ListResources` 返回签名扩展为 `(resources, hasMore, err)`，`AssetDiscoveryResult` 增加 `HasMore` 字段。provider 请求 `limit+1` 探测截断。SyncService 通用路径在 upsert 完成后若 `result.HasMore=true` 则跳过该类型 stale。资源仍正常入库，仅不标记 stale。
- 例：`max_resources=1`、云端 2 台 ECS → 请求 2、返回 2 → `2>1` → `MaxResourcesReached=true`、`ecs` 不入 `SuccessfulTypes`，整 region 跳过 stale。

### §13.2 账号配置快照冻结（批次一致性）

本节只强调“触发时冻结一次，批次内不重读”；更细的调用链约束不再在文档中重复展开。

`TriggerSync` 触发时通过 `ResolveSyncAccount` 解析一次完整账号快照（`SyncAccountSnapshot` 现含 `ProjectID`/`AuthType`/`CredentialRefID`/`Capabilities`/`ExtraConfig`），冻结后贯穿整个后台批次：

- `runSync` → `syncCloudFullSync` 用冻结快照构造 `obsdomain.AccountSnapshot`，每个 region 调用 `ListAllResources` 时设 `q.Account`（`AssetFullSyncQuery.Account`，`json:"-"` 不进 evidence hash）。
- `QueryService.resolveFullSyncEntry`：`q.Account` 非 nil 时跳过 `ResolveAccount` 的 DB 重读，直接用冻结快照构造 `ProviderContext`，仍做能力校验与 `providers.Get`；为 nil 时回退 `resolveEntry`，保持交互式/旧调用方行为不变。
- `Adapter.listAllResourcesReal` 从 `pctx.Account` 读取的 `ExtraConfig`/`ProjectID` 即冻结值，保证所有 region 用同一套 `sync_mode`/`resource_group`/`project_id`。

**凭据处理**：`ResolveAKSK` 按 `accountID` 经 `GetByAccountID` 重新加载并解密（非按 `CredentialRefID`），因此冻结快照不缓存明文凭据。同账号 AK/SK 密文轮换在 30 分钟窗口内仍可能跨 region 不一致——这是有意的安全取舍（最小化明文凭据内存驻留时间），可接受。冻结的是“用哪个账号”，而非“账号当前密文”。

**未覆盖范围**：通用回退路径 `syncGeneric`（用于不支持 `CloudFullSyncPort` 的 provider，华为云不走此路径）仍每 region 重读 `AssetDiscoveryQuery`。该路径用与交互式共享的 `domain.AssetDiscoveryQuery`，加冻结字段语义混乱，暂不纳入；华为云全量同步路径已完整冻结。

### §13.3 批量 upsert 与 chunk 租约校验（20,000 资源性能）

本节只定义批量写入与租约校验的边界；性能细节与 fallback 只保留一个落点，不再在其他章节重复。

新增 `ResourceRepository.UpsertCloudSyncBatchWithLease(ctx, resources []*Resource, batchID, fencingToken) (created, updated int, err error)`，按固定 chunk（500，与 `ListResources` 的 `Limit:500` 对齐）批量写入：

- 每个 chunk 仅一次事务、一次 `SELECT ... FOR UPDATE` 租约校验（复用 `checkSyncLeaseOwnedForUpdate`）+ 一次原生 `INSERT ... ON CONFLICT DO UPDATE`，推断迁移 `0026` 的部分唯一索引 `(integration_account_id, cloud_resource_type, cloud_resource_id, region) WHERE source='cloud_sync' AND cloud_resource_id<>''`。
- 通过 `RETURNING (xmax = 0) AS inserted` 精确区分新增/更新计数，保留 `batch.CreatedCount/UpdatedCount` 语义；`DO UPDATE` 字段集合与 `updateCloudSync` 完全一致（不含 `sync_batch_id`/`created_at`/`resource_id`/`source`）。
- 直写 SQL 绕过 GORM 钩子，故显式维护 `created_at`/`updated_at`。
- `syncCloudFullSync`/`syncGeneric` 按固定 500 chunk 循环调用 `upsertCloudResourcesWithFallback`（先批量 upsert，非租约丢失失败时回退逐条）；20,000 资源约 40 个 chunk、40 次事务。
- **失败隔离回退**：批量 upsert 失败（非租约丢失）时，对该 chunk 回退逐条 `UpsertCloudSyncWithLease`，定位坏资源并计入 `persistFailedTypes`，保证 §13.1 stale 门控的失败隔离能力不退化；租约丢失则立即终止整批同步。
- 参数上限校验：chunk=500、列数=19，单语句参数量 9,500，远低于 PostgreSQL 65,535 参数上限。

## §14 审计与日志

审计覆盖批次全生命周期，`resource_type = asset_sync_batch`，按阶段写入三个 action（不再使用单一 `action=sync`）：

| action | 写入时机 | actor | 说明 |
| --- | --- | --- | --- |
| `sync_started` | `TriggerSync` 创建 running 批次成功后立即写入 | 触发请求用户 | 记录触发人、`account_id`、`sync_mode`、`regions`、`fencing_token`；进程崩溃时仍可据此还原原批次操作者 |
| `sync_finished` | 终态 finalize（`success`/`partial`/`failed`） | 触发请求用户（detached finalize context） | payload 含完整 summary 与计数；前置失败已建 running 批次的场景也走终态 finalize + 此审计 |
| `sync_reaped` | `ReapExpiredRunning`/`ReapAllExpiredRunning` 把租约过期的崩溃批次落 `failed` 时 | 批次 `triggered_by`（原操作者） | payload 含 `reap_reason=lease_expired`、`reaped_by`（触发 reap 的来源：`TriggerSync` 路径为当次请求用户，后台 reaper 为 `system`）；actor 固定为原操作者 |

批次表通过 `triggered_by` 持久化触发人，`sync_reaped` 审计的 actor 取自该字段而非当次请求 context。

过期 running 批次有两条 reap 路径：

1. **TriggerSync 内联 reap**：触发同账号同步时先 `ReapExpiredRunning(accountID, now)` 清理本账号过期批次，`reaped_by` 为当次请求用户。
2. **后台定时 reaper**：`SyncService.StartReaper()` 在进程启动时由 `main.go` 拉起，受 `rootCtx` 管理，每 60 秒调用 `ReapAllExpiredRunning(now)` 扫描**所有账号**的过期 running 批次。这保证无人再触发同步的账号也能自愈，崩溃批次不会永久卡 `running`。后台 reaper 的 `reaped_by` 固定为 `system`，actor 仍取批次 `triggered_by`。`rootCtx` 取消时 reaper 退出。

`sync_finished` Payload 建议字段（`sync_started` 为其子集，仅含 `account_id`/`provider`/`sync_mode`/`regions`/`resource_group`/`fencing_token`/`triggered_by`，不含计数）：

```json
{
  "account_id": "acc_xxx",
  "provider": "huawei_cloud",
  "status": "success",
  "sync_mode": "ces",
  "regions": ["cn-south-1"],
  "resource_group": "全部资源",
  "fencing_token": "550e8400-e29b-41d4-a716-446655440000",
  "triggered_by": "usr_xxx",
  "created_count": 1000,
  "updated_count": 614,
  "stale_count": 0,
  "failed_count": 0,
  "ces_total": 1614,
  "discovered_count": 1614,
  "unknown_namespace_count": 0,
  "invalid_resource_count": 0,
  "max_resources_reached": false,
  "summary": {
    "sync_mode": "ces",
    "ces_total": 1614,
    "raw_fetched_count": 1614,
    "mapped_count": 1614,
    "unique_discovered_count": 1614,
    "persisted_count": 1614,
    "duplicate_count": 0,
    "persist_failed_count": 0,
    "discovered_count": 1614,
    "failed_scopes": [],
    "enriched_count": 0,
    "enrichment_failed_count": 0,
    "enrichment_warnings": [],
    "unknown_namespace_count": 0,
    "invalid_resource_count": 0,
    "max_resources_reached": false,
    "product_names_empty": false,
    "scopes": [ { "region": "cn-south-1", "ces_total": 1614, "persisted_count": 1614 } ]
  }
}
```

顶层保留 `account_id`/`provider`/`status`/`regions`/`fencing_token`/`triggered_by` 及常用计数标量（`ces_total`/`discovered_count`/`created_count`/`updated_count`/`stale_count`/`failed_count`/`unknown_namespace_count`/`invalid_resource_count`/`max_resources_reached`）用于审计检索；完整 summary 与计数以嵌套 `summary` 对象写入，结构与 `SyncBatchSummaryDTO` / 批次 `summary` 列一致（同源于 `buildSyncBatchSummaryDTO`），含 §9.5 全部对账字段（`raw_fetched_count`/`mapped_count`/`unique_discovered_count`/`persisted_count`/`duplicate_count`/`persist_failed_count`/`failed_scopes`/`product_names_empty` 等）。`scopes[]` 仅出现在 `summary.scopes`，不在顶层重复；多区域同步时每个 scope 保留单 `region/project/resource_group` 组合的计数摘要，支持按 region/project 聚合排查（与 §15.3 前端 `summary.scopes[]` 对齐）。无 summary（前置失败且无 scope 数据）时不写 `summary`/`scopes` 键。

`partial_reason` / `partial_reasons` 的语义必须明确：`partial_reason` 是首要可读原因，`partial_reasons` 是全部可读原因；两者都只保证去重与稳定输出，不保证严重性排序。机器判断应优先使用结构化门控字段，而不是解析 `message` 文本。

日志要求：

- 使用 `logger.From(ctx)`。
- 不记录 AK/SK。
- 不记录 Authorization header。
- 华为云原始错误只写脱敏 code/status。
- 每个 region/project/resource_group/namespace 的数量可以记录。

上下文与审计链路要求：

- 异步同步不得直接使用 `context.Background()` 作为业务写入、续租、终态 finalize 的基础 context。
- 后台任务应从触发请求 context 派生一个“保留 trace/user/logger values、移除请求取消”的 detached context（Go 1.21+ 可用 `context.WithoutCancel(ctx)`），再叠加进程级 shutdown/硬超时控制。
- `RenewLease`、upsert/stale 写入、前置失败批次落库、终态 finalize 与审计 recorder 都必须使用携带 trace/user logger 的 context。
- 短超时 finalize context 只能在 detached context 上派生，禁止从 `context.Background()` 直接派生。
- `hybrid` 模式必须先持久化 CES 基础资产，再执行原生增强；增强失败仅影响 `summary.enrichment_failed_types` 和批次 `partial` 状态，不得影响基础资产落库结果。
- `hybrid` 的权威性只来自 CES 基础发现层，原生增强层只补充 labels/详情，不能改变基础发现的资源集合与 stale 结论。
- `sync_started` 审计必须在创建 running 批次成功后立即写入，使用触发请求 context（非 detached）；审计失败只记 warn，不回滚批次创建。
- `sync_reaped` 审计的 actor 必须取自批次 `triggered_by`，不得使用当次触发 reap 的请求用户作为 actor；当次请求用户记入 payload `reaped_by`。
- 前置校验失败如果已经创建 running batch，必须同时完成批次终态写入（`action=sync_finished`）和审计记录；审计失败只能记录 warn/error，不能导致批次继续卡在 running。

## §15 前端对接契约

本章只定义前端需要展示和提交的稳定契约：表单字段、批次摘要、资源列表与 labels。实现细节与门控结论不在本章重复展开。

### §15.1 `/integrations` 接入表单

华为云接入账号表单在 `provider=huawei_cloud` 时暴露 CES 资源同步相关配置，统一写入 `extra_config`。前端不得把这些字段平铺为独立顶层字段。

| 表单项 | 写入字段 | 默认值 | 说明 |
| --- | --- | --- | --- |
| 同步模式 | `extra_config.sync_mode` | `ces` | 可选 `ces` / `hybrid` / `native` |
| 资源组名称 | `extra_config.resource_group_name` | 空（占位提示 `全部资源`） | 留空时后端按 §8.4 step 3 依次尝试默认候选名；显式填写则精确匹配，未命中即失败。填写 `resource_group_id` 时名称仅用于展示 |
| 资源组 ID | `extra_config.resource_group_id` | 空 | 指定后后端优先使用该分组；生产推荐显式填写 |
| 企业项目 ID | `extra_config.enterprise_project_id` | 空 | 可选，支持 `all_granted_eps` |
| 最大同步资源数 | `extra_config.max_resources` | `20000` | 单区域单次同步保护上限（按 region 独立计额） |
| 区域项目映射 | `extra_config.region_projects` | 空数组 | 每项为 `{ region, project_id, resource_group_id?, resource_group_name? }` |

表单行为要求：

- 新建 `huawei_cloud + ak_sk` 账号时，`sync_mode` 默认展示为 `ces`，并标记为推荐。
- 编辑账号时回显后端返回的 `extra_config`；字段为空时使用前端默认展示值，但提交时只写用户实际确认的配置。
- 更新账号时如果用户没有修改凭据，不传 `credential`。
- `credential` 只允许写入，不允许从 `extra_config`、本地状态或表单回显中保存 AK/SK、Token、密码等敏感值。
- `region_projects` 中的 `region` 应来自账号 `regions` 或用户明确新增的区域；空 `region` / 空 `project_id` 不提交。
- 如果配置了多个 `regions` 且缺少对应 `region_projects`，前端应提示“未配置的区域将回落使用账号 project_id”。

### §15.2 同步模式文案

| sync_mode | 推荐文案 | 风险提示 |
| --- | --- | --- |
| `ces` | CES 资源同步（推荐）：按指定 CES 资源分组同步，适合仅授予 CES 只读权限 | 资源详情较少，数量口径为指定资源分组；默认候选名需预先创建 |
| `hybrid` | 混合同步：先按指定资源分组发现，再按资源类型补充原生云服务详情 | 需要更多云服务只读权限；增强失败不影响基础资源入库 |
| `native` | 原生云资产同步：兼容旧 ECS/CCE/RDS/ELB 路径 | 不保证与 CES 总览数量一致，仅用于兼容或回退 |

### §15.3 触发同步与批次展示

本节只保留批次列表与详情页需要的展示口径，具体计数语义以 `summary` 为准。

```text
字段语义图

trigger / input
  -> sync_mode = ces | hybrid | native | fake | generic
  -> discovery result / adapter summary
  -> structure gates: partial_reason(s), stale gate, fallback whitelist
  -> batch state: success | partial | failed
  -> UI: message = human-readable summary only
        summary = authoritative machine-readable contract
```

- `status` 使用状态 Tag 展示，至少区分 `running` / `success` / `partial` / `failed`。
- `created_count`、`updated_count`、`stale_count`、`failed_count` 直接展示为批次数量摘要。
- `message` 只保留面向人的排障摘要，建议控制为一到两句，不再堆叠完整调试上下文；详情排查应优先看 `summary`、`partial_reasons`、`failed_scopes`、`enrichment_*` 等结构化字段。
- `partial` 不等同于失败；页面应提示“部分资源或增强信息失败，基础同步结果以批次 summary 为准”。
- `native` 模式是旧原生 API 兼容路径，只对旧 ECS/CCE/RDS/ELB 资源发现负责，不应被当作 CES 权威口径。
- `fake` 模式只用于 CI/E2E 或本地替身，不代表真实云侧语义；它的输出只能用于测试，不得反向驱动 stale 或对账结论。
- `generic` 是跨云/通用发现语义，强调“适配器返回什么就同步什么”，不承诺 CES 资源分组完整性，也不应与 `ces`/`hybrid` 混为一谈。
- 语义归类上，`ces`/`hybrid` 共享同一权威发现链路，`hybrid` 只是 `ces` 基础发现后的增强分支；`native` / `fake` / `generic` 都不能覆盖或改写 `ces` 结果。
- `SyncBatch` DTO/API 已提供正式 `summary` 对象（`ces_total`、`discovered_count`、`failed_scopes`、`enriched_count`、`enrichment_failed_count`、`enrichment_failed_types`、`enrichment_warnings`、`enrichment_stage_error`、`writeback_failed_count` 等）；`enrichment_failed_count` 是可执行对账字段，始终等于去重后的 `enrichment_failed_types` 长度（不变式：`enrichment_failed_count == len(enrichment_failed_types)`）。`enrichment_warnings` 记录 best-effort 增强缺失（如 `dms.kafka`、`dms.rocketmq`、`vpc.subnet_count`），不影响批次状态，独立于 `enrichment_failed_types`。增强阶段整体致命错误由 `enrichment_stage_error`（字符串）记录，label 回写失败由 `writeback_failed_count` 记录，两者均驱动 `partial` 判定但不递增 `enrichment_failed_count`。
- `message` 仅保留人类可读的排障说明，建议持续收敛到最小必要信息，不应作为半结构化协议。
- `config_mode_fallback=true` 表示本次同步曾因配置非法/不可用而回落到默认 CES 配置；它本身不是错误，但应被视为至少部分退化信号。该字段应优先体现在 `partial_reasons` 中，前端和运维不要单独解析 `message` 才判断是否发生 fallback。
- 多区域同步时，前端排查“哪个 region 失败/用了哪个 project/group”必须读取 `summary.scopes[]`，不要依赖顶层 `regions`/`projects`/`resource_group_name`（它们是兼容聚合字段，会丢失归属关系）。

### §15.4 `/assets` 资源列表与详情

资源列表首期至少保持云同步字段可见或可筛选：

- `source`、`integration_account_id`、`cloud_resource_type`、`cloud_resource_id`、`region`、`sync_status`、`last_synced_at`、`sync_batch_id`

展示要求：

- 未知 namespace 映射出的 `cloud_resource_type` 应原样展示，不要在前端丢弃或强制归为“未知”。
- 建议增加 `cloud_resource_type`、`region`、`sync_status` 筛选；如果后端暂未提供筛选参数，前端不要做跨页假筛选。
- `sync_status=stale` 应有明显标识，避免用户误以为是当前云端仍存在的资源。
- `sync_batch_id` 表示最近成功同步批次；只有整个批次成功完成后，才更新到资源上。

### §15.5 labels 展示契约

本节只定义 labels 如何展示，不定义 labels 如何生成；生成规则归入同步与映射章节。

后端 `asset_resource.labels` 已落库，资源 DTO 已暴露 `labels` 字段：

- CES 基础字段：`namespace`、`dim_name`、`resource_group_id`、`resource_group_name`、`enterprise_project_id`、`ces_status`、`ces_event_status`、`tag.<key>`。
- ECS/RDS/DCS/DMS/EIP/带宽/子网/对等连接等已生效增强字段按实际返回展示，例如 `private_ip`、`flavor`、`vpc_id`、`az`、`engine`、`capacity_gb`、`spec_code`、`public_ip`、`size_mbps`、`cidr`、`gateway_ip`；EVS 当前仅展示 CES 基础 labels，`volume_type`、`size_gb`、`attached_to` 等详情增强尚未支持。
- labels 中的未知 key 应以只读键值形式展示，不能丢弃。
- 敏感字段即使后端误返回，前端也不得明文展示，需按敏感 key 名称进行兜底掩码。

### §15.6 API README 同步要求

实现前端能力时需同步更新 `web/src/api/README.md`：

- `Integration` 章节说明 `extra_config` 字段与凭据不回显规则。
- `Asset` 章节说明同步批次 message、资源云同步字段、labels 暴露状态。
- 如果 `web/src/api/asset.ts` 新增 `labels` 类型，README 必须同步注明该字段来源与展示约束。

## §18 风险与注意事项

本章只保留运维与验收层面的风险提示；错误处理、门控与前端展示规则分别以 §13、§15 为准。

- CES 控制台总数可能按当前 region、企业项目、资源分组过滤变化；验收必须记录这些过滤条件。
- `product_names` 为空时，全量发现依赖内置 namespace 白名单，可能漏资源；需要在 batch message 中暴露。
- CES 资源维度可能不是稳定的云资源 ID，映射时必须保留 namespace 和 dim_name，避免跨类型冲突。
- `asset_resource` 唯一约束已由迁移 `0026` 补齐为含 `region` 的部分唯一索引（见 §9.4），可区分多区域同类型同云 ID 资源。
- `asset_sync_batch` 已由迁移 `0030` 增加 `fencing_token`；续租必须按 `batch_id + fencing_token` 校验所有权，upsert/stale 写入前也必须确认仍持有 running 且未过期租约。
- `asset_sync_batch` 由迁移 `0033` 增加 `triggered_by`（触发用户 user_id）；崩溃批次被 reap 时 `sync_reaped` 审计 actor 取该字段（见 §14）。
- `/api/assets/sync` 已改为异步任务：立即返回 running batch，后台 goroutine 执行同步并续租。
- CES API rate limit 需要退避重试；重试仍失败时标记 partial，不要阻塞其他 namespace。
- 涉及数据库迁移时，执行方式以 `migration-contract.md` 为准：生产 / 预发必须通过自研 runner（`cmd/migrate`）显式执行，禁止手工 `psql` 或手工写入 `schema_migrations`。

### §18.1 P1：同账号并发同步互斥（已解决）

通过迁移 `0028_asset_sync_batch_running_mutex` + `0030_asset_sync_batch_fencing_token` + `0033_asset_sync_batch_triggered_by`（不依赖 Redis）实现：

- `asset_sync_batch` 新增 `lease_expires_at` 列；建部分唯一索引 `(integration_account_id) WHERE status='running'`，确保每个账号同一时刻只有一个 running 批次。
- `asset_sync_batch` 新增 `fencing_token` 列；后台任务持有 token，`RenewLease` 必须按 `batch_id + fencing_token + status=running + lease_expires_at 未过期` 续租（过期不可复活，只能由 `ReapExpiredRunning` 回收），upsert/stale 前必须校验 `batch_id + fencing_token + status=running + lease_expires_at 未过期`。确认租约丢失时立即取消旧任务，禁止继续写入。
- `asset_sync_batch` 新增 `triggered_by` 列（迁移 `0033`）；`TriggerSync` 创建 running 批次时写入。
- `TriggerSync` 在 `Create` 前先 `ReapExpiredRunning(accountID, now)` 清理本账号租约过期的 running 批次：把崩溃批次落 `failed` 并写 `sync_reaped` 审计，actor 取该批次 `triggered_by`、payload 含 `reap_reason=lease_expired` 与当次 `reaped_by`；再插入带 `lease_expires_at = now + 5min`、`triggered_by = 当次请求用户` 的 running 批次。
- `TriggerSync` 创建 running 批次成功后立即写 `sync_started` 审计（使用触发请求 context）。
- 已有 running 批次时 `Create` 触发唯一冲突 → 映射为 `ALREADY_EXISTS`（HTTP 409，`message=sync already in progress for this account`）。
- **异步生命周期**：`runCtx` 先从请求 context 派生 detached context（保留 values、移除请求取消），再受进程级 `shutdownCtx` 与 30 分钟硬超时控制。后台 goroutine 每 60 秒通过 `RenewLease` 续租，把 `lease_expires_at` 推进到 `now+5min`。续租、前置失败处理、终态写入与审计都必须使用该链路 context 派生的短超时 context。即便 `runCtx` 取消，也要用保留 trace/user/logger 的 detached finalize context 尝试落终态。
- 终态 `Update` / `finishBatchFailedDetached` 把 `lease_expires_at` 清空，释放槽位。
- **后台定时 reaper**：`SyncService.StartReaper()` 由 `main.go` 在进程启动时拉起，受 `rootCtx` 管理。每 60 秒调用 `ReapAllExpiredRunning(now)` 扫描全账号过期 running 批次并落 `failed`，写 `sync_reaped` 审计（actor=`triggered_by`，`reaped_by=system`）。这弥补了"仅 TriggerSync 内联 reap"的缺口：无人再触发的账号也能自愈，崩溃批次不会永久卡 `running`。`rootCtx` 取消时 reaper 退出，不纳入 `Wait`/`WaitContext`（无 finalization 工作）。
- 契约：见 `cloud-observability-contract.md` §5.5.1 与 §8 审计动作、`migration-contract.md` `0028`/`0030`/`0033`。

### §18.2 验收检查清单

以下检查项用于评审与回归验收，目标是确认实现层已经把三类关键边界单点化：

- `partial` 是否只由终态聚合逻辑落定，且 `message` 仅作展示，不参与推导。
- `stale` 是否只在权威发现路径上执行，并且会被租约/ fencing token 严格拦住。
- `fallback whitelist` 是否始终走降级路径，不会被当作权威 scope，也不会反向标记 stale。
- `hybrid` 是否始终是“CES 基础发现 + 原生增强”，增强失败不影响基础入库与权威口径。
- `native` / `fake` / `generic` 是否都被隔离在兼容或测试语义内，不会覆盖 `ces` 结果。
- `summary.scopes[]` 是否是多区域排障的唯一归属来源，而不是顶层聚合字段。
- `message` 是否已收敛为短文本，避免继续承担半结构化协议职责。

## §21 已知增强缺口

`sync_mode=hybrid` 下部分资源类型的详情增强存在已知缺口，**不影响** `sync_mode=ces` 的资源发现完整性（CES 基础资源仍正常入库）。完整缺口清单与修复方向见《华为云 CES 同步已知缺口与待办》([../docs/huawei-ces-sync-backlog.md](../docs/huawei-ces-sync-backlog.md))，主要包括：

- **EVS**：匹配键与 CES dim value 从根本上不成立（CES `disk_name` 实际格式为「服务器ID-盘符」或「服务器ID-volume-卷ID」，并非卷显示名称 `vol.Name`），hybrid 增强实际不命中。EVS 在 hybrid 下退化为只有 CES 基础信息。
- **OBS**：原生增强未落地（需另引 OBS SDK）。
- **ELB / CCE**：原生映射不产出 label，hybrid 增强为空跑。
- **CBR/SFS/NAT/VPCEP/APM**：无原生增强客户端，hybrid 下只有 CES 基础信息。
- **VPC（已修复）**：`SYS.VPC` 按 `dim_name` 拆分子类型后，EIP/带宽/子网/对等连接增强已生效。
