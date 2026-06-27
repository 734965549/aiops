# 华为云 CES 资源分组同步实现方案

> 说明：稳定 API/运维契约以 `ops/cloud-observability-contract.md` 为准；本文保留 CES 同步设计背景、实施记录和后续清单，若与 `ops/*` 契约冲突，以契约文档为准。

## 1. 目标

平台的华为云资产同步目标为：**以 CES 资源分组接口为主路径，将用户指定的 CES 资源分组（默认候选名“全部资源”）下的资源同步到 `asset_resource`**。

> 重要口径说明：CES 官方 `ListResourceGroups` 只返回**用户创建的资源分组**，`ListResourceGroupsServicesResources` 只查询**指定 `group_id` 下**的资源（见 [资源分组列表](https://support.huaweicloud.com/usermanual-ces/ces_01_0019.html)、[ListResourceGroupsServicesResources](https://support.huaweicloud.com/api-ces/ListResourceGroupsServicesResources.html)）。CES 控制台“总览”页若展示超出任何资源分组的聚合视图，该视图并**不**由上述 API 提供。因此本平台“同步范围”严格等同于“指定资源分组下资源”，而不是“CES 总览页所有资源”。如需对齐总览全量，需另行评估是否存在非资源组作用域的概览 API，不在本方案范围。

这意味着同步口径不再以 ECS/CCE/RDS/ELB 等云服务原生 List API 为准，而是以 CES 的资源分组视图为准。云服务原生 API 仍可作为补充信息来源，但不能作为“资源总数是否完整”的判断依据。

默认候选名“全部资源”并非 CES 系统内置分组，需要用户在 CES 控制台预先创建同名资源分组；未创建且未显式指定 `resource_group_id` 时，同步批次将失败，**不会**静默回退到任意资源组。

目标闭环：

```text
华为云账号只读授权
  -> 指定 CES 资源分组发现
  -> namespace + dimension 归一化为平台 CloudResource
  -> Asset Sync 批次 upsert
  -> stale 标记缺失资源
  -> Dashboard / 告警匹配 / 巡检使用统一资产表
```

## 2. 当前问题

当前实现已支持 `huawei_cloud` + `auth_type=ak_sk` 的真实资源同步，但同步范围是：

```text
ECS / CCE / RDS / ELB
```

对应代码路径：

- `internal/asset/application/sync_service.go`
- `internal/observability/infrastructure/provider/huawei/resource_client.go`
- `internal/observability/infrastructure/provider/huawei/adapter.go`

这套实现调用的是各云服务的只读 List API，例如 ECS、RDS、ELB，而不是 CES 的资源分组 API。因此会出现：

- CES 控制台显示 1,614 个监控资源。
- 平台只同步到几十个资源。
- 只有 `CES ReadOnlyAccess` 权限的账号能看 CES 总览，但不一定能调用 ECS/RDS/ELB/EVS/VPC/OBS 等服务的 List API。
- EVS、VPC、OBS、DCS、DMS、CBR、VPN、终端节点、可用性监控等 CES 资源类型没有进入平台资产表。

结论：当前同步口径与产品目标不一致，需要新增 CES 口径的资源发现能力。

## 3. 设计原则

- CES 资源同步以 **CES 资源分组接口** 为主路径，同步范围严格限定为“指定资源分组下资源”。
- 不存在“CES 总览全量”的隐式口径；默认候选名“全部资源”需用户在 CES 控制台预先创建，未命中即失败，不静默回退到最大资源组。
- 云服务原生 API 只做增强，不影响“是否同步完整”的主判断。
- 真实凭据账号不能回落到 fake 数据。
- 资源同步仍走 Asset Sync 批次、审计、stale 标记，不直接删除历史资源。
- 不暴露 AK/SK、Authorization header、原始云端错误详情。
- 同步过程必须记录每个 region、project、resource group、namespace 的成功/失败摘要，便于解释“为什么数量不一致”。
- `QueryService.ListResources` 面向交互查询，当前 `limit` 最大 500；CES 资源分组同步不能直接受这个上限限制，需要独立分页同步能力。

## 4. 产品分层与同步模式

成熟产品不应把“资源是否可见”和“资源详情是否丰富”绑在一起。平台采用三层能力：

| 层级 | 同步模式 | 目标 | 权限 | 结果 |
| --- | --- | --- | --- | --- |
| P0/P1 | `ces` | 指定资源分组下看到多少，平台同步多少 | `CES ReadOnlyAccess` | 资源数量与指定分组一致，可用于告警匹配、Dashboard、AI 分析入口 |
| P2 | `hybrid` | 先按指定资源分组发现，再用原生云服务 API 补详情 | CES 只读 + 按需云服务只读 | 补充 IP、规格、VPC、磁盘、引擎版本、配置等 |
| P3 | `hybrid` + topology/inspection | 结合 CES、原生 API、日志、链路做拓扑、巡检和根因分析 | 按能力逐项授权 | 支持拓扑、配置巡检、容量风险、根因候选 |

同步模式定义：

| 模式 | 定位 | 行为 | 是否推荐 |
| --- | --- | --- | --- |
| `ces` | 默认模式 | 只依赖 CES 资源分组/资源列表发现指定分组下资源，不调用 ECS/RDS/ELB/EVS/VPC 等原生 List API 作为入库前置条件 | 推荐，P0/P1 默认 |
| `hybrid` | 增强模式 | 先执行 `ces` 指定分组发现并入库，再对已发现资源按类型调用原生 API 增强详情；增强失败不影响基础资源入库 | P2 推荐 |
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

产品文案建议：

- `CES 资源同步`：推荐，按指定 CES 资源分组同步，适合只授予 CES 只读权限。
- `混合同步`：先按指定资源分组发现，再用云服务只读权限补充资源详情。
- `原生云资产同步`：兼容旧路径，需要更多云服务只读权限，不保证与 CES 资源分组数量一致。

## 5. 权限要求

### 5.1 最小目标权限

如果目标是“和指定 CES 资源分组资源数一致”，优先授予：

```text
CES ReadOnlyAccess
```

该权限应允许读取 CES 指标、资源分组、资源分组下资源列表等只读数据。

### 5.2 增强权限

如果后续要补齐资源详情，例如 ECS 私网 IP、VPC ID、云硬盘挂载关系、OBS bucket 区域等，需要额外授予对应云服务只读权限：

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

### 5.3 多项目与多区域

华为云 `project_id` 通常与区域相关。平台当前账号模型只有一个 `project_id` 和一个 `regions[]`，实现 CES 资源分组同步时需要处理以下情况：

- 单区域账号：现有 `project_id` + `regions[0]` 即可。
- 多区域账号：必须能表达 `region -> project_id` 映射，否则不同区域的 CES 资源可能查不到。
- 企业项目：CES 资源分组支持 `enterprise_project_id` / `all_granted_eps`，需要明确默认查询策略。

推荐演进：

1. 短期：保持现有字段，要求一个接入账号只绑定一个 `region + project_id`。
2. 中期：新增账号配置字段 `region_projects`，例如：

```json
[
  { "region": "cn-south-1", "project_id": "xxx" },
  { "region": "cn-north-4", "project_id": "yyy" }
]
```

> **当前进度**：`region_projects` 已落地（放入 `extra_config` JSON，无需 DB 迁移）。`SyncModeConfig` 新增 `RegionProjects` 字段与 `ResolveProjectID(region, fallback)` 方法；`adapter.go` 在 `queryMetricsCES`/`listResourcesReal`/`listAllResourcesReal` 三处均按当前 region 解析 project_id，未命中回落 `Account.ProjectID`，保证旧的单 project_id 账号零行为变化。解析阶段过滤空 region/project_id、大小写不敏感去重。前端接入表单已暴露 `sync_mode`、资源组、企业项目、`max_resources` 与 `region_projects` 等配置项（见 §20）。

3. 长期：支持自动发现项目列表，但凭据和 IAM 权限复杂度更高，暂不作为首期目标。

## 6. 华为 CES API 路径

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

`product_names` 通常表达为：

```text
SYS.ECS,instance_id;SYS.EVS,disk_name;SYS.RDS,rds_cluster_id
```

实现时应解析为：

```text
service = SYS.ECS
dim_name = instance_id
```

再按 `service + dim_name` 查询该资源组下资源列表。

## 7. 目标架构

### 7.1 新增 CES 资源发现客户端

建议新增文件：

```text
internal/observability/infrastructure/provider/huawei/ces_resource_client.go
internal/observability/infrastructure/provider/huawei/ces_resource_mapper.go
internal/observability/infrastructure/provider/huawei/ces_resource_client_test.go
internal/observability/infrastructure/provider/huawei/ces_resource_mapper_test.go
```

核心接口建议：

```go
type CESResourceDiscoveryClient interface {
    ListCESResources(ctx context.Context, cred AKSKCredential, req CESResourceDiscoveryRequest) (*CESResourceDiscoveryResult, error)
}

type CESResourceDiscoveryRequest struct {
    ProjectID           string
    Region              string
    EnterpriseProjectID string
    ResourceGroupName   string
    ResourceGroupID     string
    MaxResources        int
}

type CESResourceDiscoveryResult struct {
    Resources []domain.CloudResource
    Summary   CESResourceDiscoverySummary
}
```

`Summary` 用来记录：

- 目标 `project_id`
- 目标 `region`
- 选中的 `resource_group_id` / `resource_group_name` / 选择策略（`specified_id`/`specified_name`/`default_name`）
- CES 返回的 `resource_statistics.total`（`CESTotal`）
- 聚合 `Discovered`（实际收集资源数）
- `FailedScopes`：失败的 `region/namespace` 错误摘要字符串列表
- `SuccessfulTypes` / `QueryFailedTypes` / `ConversionFailedTypes`：按 `cloud_resource_type` 聚合的查询/转换成功与失败类型（供 stale 门控，见 §13.1）
- `UnknownNamespaceCount` / `InvalidResourceCount` / `ProductNamesEmpty` / `MaxResourcesReached`

> 注意：当前摘要按 `cloud_resource_type` 聚合，不保留逐 `namespace/dim_name` 的独立 count 明细；如需逐 scope 排查，从 `FailedScopes` 字符串中提取 `region/namespace`。

### 7.2 Asset Sync 使用专用分页端口

当前 `AssetDiscoveryPort.ListResources` 经 `QueryService.normalizeAssetQuery`，`limit` 最大 500，适合前端或 Agent 工具的交互查询，不适合全量同步。

推荐新增一个只给 Asset Sync 使用的内部端口：

```go
type CloudFullSyncDiscoveryPort interface {
    ListAllResources(ctx context.Context, actor obsapp.Actor, q obsdomain.AssetFullSyncQuery) (*obsapp.AssetFullSyncResult, error)
}
```

或在现有 `CloudDiscoveryPort` 下新增方法：

```go
ListAllResources(ctx, actor, query)
```

要求：

- 不受 `limit <= 500` 限制。
- 内部按 CES API 分页读取。
- 仍要有 `max_resources` 防护，默认建议 20,000，避免异常配置导致无限扫描。该上限按 region 独立计额，多区域账号实际可同步 `区域数 × max_resources`。
- 返回同步摘要，写入 batch message 或扩展字段。

### 7.3 Adapter 路由策略

`huawei.Adapter.ListResources` 当前真实账号走 ECS/CCE/RDS/ELB。目标状态下建议改为：

```text
auth_type=none
  -> fake provider

auth_type=ak_sk
  -> sync_mode=ces: 只走 CES resource discovery
  -> sync_mode=hybrid: 先走 CES resource discovery，再用原生 API enrichment
  -> sync_mode=native: 兼容旧 ECS/CCE/RDS/ELB resource client

auth_type=agency
  -> 阶段一返回 unsupported
  -> 后续按同一 CES discovery 接口实现 agency credential
```

不要在真实账号下静默回退 fake。

## 8. 同步流程

### 8.1 `ces` 默认同步流程

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

### 8.2 `hybrid` 增强同步流程

```text
POST /api/assets/sync
  -> 先完整执行 ces 同步流程
  -> 对本批次 active 资源按 cloud_resource_type 分组
  -> 对已授权类型调用原生 API 补充详情
  -> enrichment 成功：更新 labels / 扩展字段
  -> enrichment 失败：记录 warning，不回滚 CES 资源入库
  -> batch message 写入 enrichment summary
```

`hybrid` 的成功标准是 CES 基础资源完整入库；原生 API 增强只影响详情丰富度，不影响资源发现完整性。

增强内部按子服务/可选字段独立处理，避免一个子服务失败丢弃另一个的结果：

- **DMS**：Kafka 与 RocketMQ 是独立子服务，任一失败不阻断另一个；只有两者都失败才记为该类型增强失败，部分成功时返回已收集结果。
- **VPC**：`subnet_count` 为可选增强；子网统计失败（如子网权限不足）时 VPC 仍正常返回（含 `vpc_name/cidr/status`），仅缺少 `subnet_count`。

### 8.3 `native` 兼容同步流程

`native` 仅用于兼容旧路径或紧急回退：

```text
sync_mode=native
  -> ECS/CCE/RDS/ELB resource client
  -> 按旧 scope 执行 upsert/stale
```

`native` 不承诺与任何 CES 资源分组数量一致。

### 8.4 资源组选择策略

同步范围严格限定为“指定 CES 资源分组下资源”。CES 官方 `ListResourceGroups` 只返回用户创建的资源分组，不存在“CES 总览全量”的隐式口径；因此**不再提供“选择 total 最大的资源组”这类静默回退**，避免把某个业务组误当作全量。

默认候选名“全部资源”并非 CES 系统内置分组，需要用户在 CES 控制台预先创建同名资源分组。

选择顺序：

1. 如果请求或账号配置指定 `resource_group_id`，使用该分组（仍经 `ShowResourceGroup` 校验存在）。
2. 如果指定 `resource_group_name`，按名称精确匹配（大小写不敏感）。**任何名称（自定义名或默认候选名）未命中均直接失败**，不回退到其他分组，避免同步错误范围。
3. 未指定 `resource_group_name` 时，依次尝试默认候选名：

```text
全部资源
All resources
All Resources
```

4. 以上均未命中（包括默认候选名也不存在），批次失败，返回脱敏错误：

```text
no CES resource group matched (specified id/name or default candidates)
```

生产环境推荐显式填写 `resource_group_id`，避免依赖控制台预先创建的默认候选名。

### 8.5 product_names 解析

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
- 平台 `cloud_resource_type` 使用映射表归一化为小写类型。
- 如果 `product_names` 为空，使用内置 CES namespace 白名单兜底，但该白名单不完整且部分维度可能错误；batch message 必须记录 `product_names_empty=true`，且批次至少标记 `partial`，提示同步可能不完整，见 §13.1。

### 8.6 分页策略

`ListResourceGroupsServicesResources` 分页参数：

- `offset`
- `limit`

建议：

```text
page_limit = 100
offset = 0
while offset < count:
    request offset/page_limit
    append resources
    if returned < page_limit: break
    offset += page_limit
```

停止条件：

- 当前 service/dim_name 资源取完。
- 总资源数达到 `max_resources`。
- context cancelled。
- CES 返回不可恢复错误。

失败策略：

- 单个 `service/dim_name` 失败，不应导致整个账号同步立刻失败。
- 记录 `failed_count++` 和 scope message。
- 只有所有 scope 都失败且没有任何资源 upsert 时，批次状态为 `failed`。
- 部分成功时，批次状态为 `partial`。

## 9. CES 资源到平台资源的映射

### 9.1 CloudResource 映射

CES 返回的资源没有所有云服务的完整详情，因此基础映射应稳定、可追踪：

| 平台字段 | 来源 | 说明 |
| --- | --- | --- |
| `ResourceID` | `ces:{region}:{namespace}:{primary_dim_value}` | 平台内临时资源 ID |
| `Name` | `resource_name` 或主维度值 | 控制台显示名优先 |
| `Type` | namespace 映射 | 例如 `SYS.ECS -> ecs` |
| `Region` | account region | 当前查询区域 |
| `Status` | `status` / `event_status` | 优先指标告警状态 |
| `ProviderRef` | 主维度值 | 后续指标查询用 |
| `Labels.namespace` | service | 例如 `SYS.ECS` |
| `Labels.dim_name` | dim_name | 例如 `instance_id` |
| `Labels.enterprise_project_id` | response field | 可选 |
| `Labels.resource_group_id` | selected group | 便于追溯 |
| `Labels.resource_group_name` | selected group | 便于排查 |

### 9.2 主维度选择

主维度值用于 `cloud_resource_id` 和 `ProviderRef`。

选择顺序：

1. 优先使用请求中的 `dim_name` 对应的 dimension value。
2. 如果没有匹配，使用第一个非空 dimension value。
3. 如果仍为空，使用 `resource_name`。
4. 如果都为空，丢弃该资源并记录 `invalid_resource_count`。

### 9.3 namespace 映射表

首期至少覆盖截图中常见资源：

| CES namespace | 平台 `cloud_resource_type` | 平台 `resource_type` | 说明 |
| --- | --- | --- | --- |
| `SYS.ECS` | `ecs` | `host` | 弹性云服务器 |
| `SYS.EVS` | `evs` | `storage` | 云硬盘 |
| `SYS.VPC` | `vpc` | `network` | 虚拟私有云 |
| `SYS.ELB` | `elb` | `service` | 弹性负载均衡 |
| `SYS.RDS` | `rds` | `database` | 关系型数据库 |
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

### 9.4 asset_resource 字段

继续复用迁移 `0023` 的字段：

```text
source = cloud_sync
integration_account_id = account_id
cloud_resource_id = primary_dim_value
cloud_resource_type = mapped type
region = account region
sync_status = active / stale
last_synced_at = now
sync_batch_id = batch_id
```

唯一约束仍建议使用：

```text
integration_account_id + region + cloud_resource_type + cloud_resource_id
```

如果当前唯一索引不含 `region` 或不能区分 namespace，需要评估新增迁移，避免不同 region 或不同 namespace 的同 ID 资源互相覆盖。

## 10. 与原生云服务 API 的关系

CES 资源发现解决“数量完整”和“监控口径一致”。

原生 API 只属于 `hybrid` 增强阶段：

```text
CES resource
  -> cloud_resource_type=ecs
  -> optional ECS Describe/List detail
  -> 补充 private_ip、flavor、vpc_id、az 等 labels
```

增强失败不影响基础资源入库：

- CES 发现成功，资源必须入库。
- 原生 API 增强失败，只记录 warning scope。
- 不因缺少 ECS/EVS/VPC 权限导致 CES 基础同步失败。

## 11. 配置建议

新增或预留账号扩展配置：

```json
{
  "sync_mode": "ces",
  "resource_group_name": "全部资源",
  "enterprise_project_id": "all_granted_eps",
  "max_resources": 20000,
  "region_projects": [
    { "region": "cn-south-1", "project_id": "xxx" }
  ]
}
```

字段说明：

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `sync_mode` | `ces` | `ces` 为默认完整性口径；`hybrid` 为增强模式；`native` 仅兼容旧路径 |
| `resource_group_name` | `全部资源` | 默认候选名，需用户在 CES 控制台预先创建同名分组；未命中即失败 |
| `resource_group_id` | 空 | 指定后优先使用，生产推荐 |
| `enterprise_project_id` | 空 | 可选，支持 `all_granted_eps` |
| `max_resources` | `20000` | 单账号单次同步保护上限 |
| `region_projects` | 空 | region → project_id 映射数组；adapter 按当前 region 选用对应 project_id，未命中回落账号 `project_id`。已落地（放 `extra_config`，无 DB 迁移） |

如果短期不改数据库，可把这些配置先放入接入账号的扩展配置字段；如果当前模型没有扩展 JSON，建议新增迁移。

## 12. 实现顺序

### 阶段 1：P0/P1 CES-only 资源发现

目标：CES 页面看到多少，平台同步多少。

实施顺序：

1. 更新文档，明确当前 ECS/CCE/RDS/ELB 同步不是最终目标。
2. 新增 CES resource client。
3. 实现 `ListResourceGroups` 分页。
4. 实现资源组选择策略。
5. 实现 `ShowResourceGroup` 并解析 `product_names`。
6. 实现 `ListResourceGroupsServicesResources` 分页。
7. 实现 mapper 和 namespace 映射表。
8. 新增全量同步端口，避免受 `limit <= 500` 限制。
9. `SyncService.TriggerSync` 对 `huawei_cloud + ak_sk` 默认走 `sync_mode=ces`。
10. 保留 `auth_type=none` fake 同步。
11. 在同步批次 message 中输出更清晰的 scope 摘要：

```text
region=cn-south-1 group=全部资源 ces_total=1614 discovered=1614 upserted=1614 failed_scopes=0
```

12. 成功 scope 维度从旧路径的固定 `region/resource_type` 调整为 CES 返回的成功 `region/cloud_resource_type`。
13. stale 标记只对成功 scope 生效；某类型只有同时满足 provider 查询完整、资源转换完整、全部资源成功持久化三者才纳入 stale scope（见 §13.1）。
14. 前端批次列表保留 message tooltip，便于排查。
15. 单测覆盖分页、空 product_names、未知 namespace、无主维度、部分失败。

### 阶段 2：P2 Native API enrichment

目标：对 CES 已发现资源补充详情。

实施顺序：

1. 原 ECS/CCE/RDS/ELB native sync 改为 `hybrid` enrichment 路径。
2. 针对已同步资源按类型补充详情。
3. 增强结果写入 labels 或扩展字段。
4. 增强失败不影响基础同步成功状态，只影响 warning summary。
5. 增加 EVS/VPC/OBS/DCS/DMS 等按需增强客户端。

当前进度：

- 已落地 `sync_mode=hybrid` 路由：先执行 CES 指定资源分组发现，再对 CES 返回的 ECS/CCE/RDS/ELB/EVS/VPC/DCS/DMS 资源按类型调用原生 API 增强（CCE/ELB 原生映射暂不产出 label，见 §21.2）。
- 已落地 ECS label 增强：`private_ip`、`flavor`、`vpc_id`、`az`。
- 已落地 RDS label 增强：`private_ip`、`vpc_id`、`subnet_id`、`flavor`。
- label 合并只新增缺失字段，不覆盖 CES 已有 label；增强失败记录到 `EnrichmentFailedTypes`，批次 message 追加 `enriched=N enrichment_failed=type1,type2`。
- 已落地 label 落库：迁移 `0025` 为 `asset_resource` 增加 `labels JSONB` 列；`domain.Resource` 与 `resourceModel` 增加 `Labels` 字段；`SyncService.upsertCloudResource` 把 `CloudResource.Labels` 写入 `Resource.Labels`；`updateCloudSync` 每次同步整体覆盖 labels，避免陈旧 label 残留。
- label 持久化已有真实 Postgres 往返测试覆盖（`TestResourceRepository_UpsertCloudSyncLabelsRoundtrip`）：create 与 update 路径均写 `labels JSONB`，update 整体覆盖（旧 key 清除）。
- CES `namespace/dim_name/resource_group` label 仅在 `sync_mode=ces/hybrid`（默认 ces）路径由 `mapCESResource` 产出；`sync_mode=native` 路径（`mapELBLoadBalancer`/`mapCCECluster`）不产出 CES label，ECS/RDS native 仅产出增强字段。排查“DB labels 为空”时优先确认账号 `sync_mode` 与数据是否 `0025` 之前的旧行。
- 已落地 RDS `resource_type` 修正：`mapCloudResourceToAssetFields` 将 `rds` 映射为 `database`（原与 `elb/cce/apm` 同归 `service`），对齐 §9.3；`SYS.RDS` 不再被存成 `service`。
- 已落地云资源唯一键加 region：迁移 `0026` 重建 `idx_asset_resource_cloud_key` 为 `(integration_account_id, cloud_resource_type, cloud_resource_id, region)` 部分唯一索引；`CloudResourceKey` 增加 `Region`。Repository 查找键接入（`FindByCloudKey` 的 WHERE 与 `UpsertCloudSync` 构造 key 带 region）已补齐，避免多区域同类型同 ID 互相覆盖。配套 Postgres 测试 `TestResourceRepository_UpsertCloudSyncRegionKey`/`...LabelsRoundtrip` 在 DB 可用时应能通过。
- 资源 DTO 已暴露 `labels` 字段，前端资源列表与详情已展示 labels。后续仅需继续优化 CES `namespace/dim_name/resource_group` 与增强字段（ECS `private_ip/flavor/vpc_id/az`、EVS `volume_type/size_gb/attached_to`、VPC `cidr/subnet_count`、DCS `engine/capacity_gb`、DMS `engine/spec_code` 等）的展示组织方式，并保持 `web/src/api/README.md` 同步。
- 已落地 DCS/DMS 原生增强客户端；EVS/VPC 客户端已实现但 hybrid 匹配键不成立（见 §21.4）（`sync_mode=hybrid`）：`supportedCloudResourceTypes` 追加 `evs/vpc/dcs/dms`；`resource_client.go` 新增 `listEVS/listVPC/listDCS/listDMS` 及对应 SDK client（evs/vpc/dcs/kafka/rocketmq）；`resource_mapper.go` 新增 `mapEVSVolume/mapVPC/mapDCSInstance/mapDMSKafkaInstance/mapDMSRocketMQInstance`。其中 DCS/DMS 匹配键成立、hybrid 增强实际生效；EVS/VPC 匹配键不成立、hybrid 增强实际不命中。
  - EVS 增强 label（客户端已实现，但匹配键不成立，hybrid 实际不生效）：`volume_id`、`volume_type`、`size_gb`、`attached_to`、`az`、`created_at`、`charging_mode`（从 `Metadata.orderID` 推断）。`ProviderRef=volume name`（试图对齐 CES `dim_name=disk_name`）。**问题**：CES `SYS.EVS` 的 `disk_name` 实际格式为「服务器ID-盘符」（如 `6f3a...-vda`）或「服务器ID-volume-卷ID」，并非 EVS 卷显示名称 `vol.Name`，两者无法对齐。参考 https://support.huaweicloud.com/usermanual-evs/evs_01_0044.html 。
  - VPC 增强 label（客户端已实现，但匹配键不成立，hybrid 实际不生效）：`vpc_name`、`cidr`、`status`、`enterprise_project_id`、`created_at`、`subnet_count`（调 `ListSubnets` 按 `vpc_id` 统计）。`az` 不在 VPC 模型（省略）。`ProviderRef=vpc id`（试图对齐 `dim_name=vpc_id`）。**问题**：CES `SYS.VPC` 不存在 `vpc_id` 主维度，其主维度为 `publicip_id`/`bandwidth_id`/`subnet_id`/`peering_id`，`ProviderRef=vpc id` 无法对齐任何 CES 维度。参考 https://support.huaweicloud.com/eu/usermanual-ces/en-us_topic_0202622212.html 。
  - DCS 增强 label：`instance_name`、`engine`、`engine_version`、`capacity_gb`、`spec_code`、`private_ip`、`az`、`vpc_id`、`charging_mode`（0=按需,1=包周期）、`created_at`。`ProviderRef=instance_id`（对齐 `dim_name=dcs_instance_id`）。
  - DMS 增强 label：华为云 SDK 把 DMS 拆为 `kafka` 与 `rocketmq` 两个服务包，`listDMS` 合并两者结果，`cloud_resource_type` 统一为 `dms`。Kafka：`instance_name/engine/engine_version/spec_code/capacity_gb(private_ip)/vpc_id/charging_mode(0=包周期,1=按需)/created_at`（Kafka 响应无 `az`）。RocketMQ：`instance_name/engine/engine_version/spec_code/capacity_gb/az/vpc_id/charging_mode/created_at`（RocketMQ 响应无 `connect_address`，`private_ip` 省略）。`ProviderRef=instance_id`（对齐 `dim_name=dms_instance_id`）。
  - 注意：DMS 与 DCS 的 `charging_mode` 数值语义相反（DCS `1=包周期`，Kafka/RocketMQ `0=包周期`），mapper 已分别处理，统一输出 `prepaid/postpaid`。
  - **已知缺陷（P1）**：EVS/VPC 的匹配键与 CES dim value 从根本上不成立——不是「待对账校正」的渐进问题，而是匹配键选错。DCS/DMS 的匹配键成立。当前测试 `TestAdapterListAllResourcesHybridEVSVPCDCSDMS` 用人为相等的 `ProviderRef`（`disk-01`/`disk-01`、`vpc-1`/`vpc-1`）掩盖了 EVS/VPC 的真实不匹配，详见 §21.4。
- OBS 原生增强仍未落地：OBS 不在统一 SDK 包内，需另引 `huaweicloud-sdk-go-obs` 且 endpoint 走 OBS 域名，列入后续待办。
- 已落地 stale 门控修正（§13.1）：`CESResourceDiscoverySummary`/`CloudSyncSummary` 新增 `ConversionFailedTypes`，`mapCESResource` 失败时按类型记录；`SyncService` 跟踪 per-type upsert 失败，stale scope 改为 `SuccessfulTypes − ConversionFailedTypes − persistFailedTypes`；通用同步路径同样改为"全部资源成功持久化才执行 stale"。修复某类型查询成功但写库全部失败时旧资产被误标 stale 的问题。
- 已落地同类型多 scope 部分失败修正（§13.1）：`CESResourceDiscoverySummary`/`CloudSyncSummary` 新增 `QueryFailedTypes`，CES 资源分组发现按 `service+dim_name` 逐 scope 查询，任一 scope 失败时把对应类型记入 `QueryFailedTypes`，并在循环结束后从 `SuccessfulTypes` 剔除，确保同一类型只有所有 scope 都成功才进入 `SuccessfulTypes`。修复如 `SYS.ELB/loadbalancer_id` 成功而 `SYS.ELB/l7policy_id` 失败时 `elb` 仍被误判查询完整、未查询到的 l7policy 资产被误标 stale 的问题。批次 message 增加 `query_failed_types=...` 用于排查。
- 已落地 native 兼容路径修正（§8.3）：`listAllResourcesNative` 不再以空类型调用 `ListResources`（会遍历 `supportedCloudResourceTypes` 全 8 类并在首类失败时整批失败），改为显式遍历 `legacyNativeResourceTypes`（固定 ECS/CCE/RDS/ELB 旧 4 类）逐类调用；单类失败只记入 `FailedScopes` 并跳过，全部类型失败才返回错误；查询成功的类型写入 `SuccessfulTypes`（即使返回 0 条资源），修复某类资源全部消失后旧记录无法标记 stale 的问题。EVS/VPC/DCS/DMS 的详情增强仅在 `hybrid` 模式下进行，不进入 native 兼容路径。

### 阶段 3：P3 Hybrid topology / inspection

目标：结合 CES + 原生 API + 日志/链路做巡检和根因分析。

实施顺序：

1. ~~明确账号模型是否新增 `region_projects`。~~ 已落地：放入 `extra_config` JSON，无需新增 DB 列。
2. ~~如果新增字段，补迁移、DTO、前端表单、API README。~~ 后端 `SyncModeConfig`/adapter/契约已落地；前端接入表单已暴露 `region_projects`/`sync_mode`/`resource_group_name`/`resource_group_id`/`enterprise_project_id`/`max_resources` 等配置项，并写入 `extra_config`。
3. ~~每个 region 使用对应 project_id 创建 CES client。~~ 已落地：`adapter.go` 在 CES 指标查询、CES 资源发现、native 资源查询三处均按 region 解析 project_id。
4. batch summary 按 region/project 聚合。
5. 建立资源关系模型，例如 ECS -> EVS、ECS -> VPC、ELB -> ECS/RDS。
6. 将拓扑和资源详情接入 Inspection evidence。
7. 支持按资源类型、region、应用、告警状态触发巡检策略。

## 13. 错误处理

| 场景 | 批次状态 | 处理 |
| --- | --- | --- |
| AK/SK 缺失 | `failed` | 返回 `FAILED_PRECONDITION` |
| project_id 缺失 | `failed` | 返回 `INVALID_ARGUMENT` |
| region 缺失 | `failed` | 返回 `INVALID_ARGUMENT` |
| CES 鉴权失败 | `failed` | 脱敏为 `provider authentication failed` |
| 找不到资源组 | `failed` | message 写 project/region，不写敏感信息 |
| 某 namespace 查询失败 | `partial` | 其他 namespace 继续 |
| 部分资源无主维度 | `partial` 或 `success` | 计入 invalid_resource_count |
| 达到 max_resources | `partial` | message 写 `max_resources_reached=true` |
| `product_names` 为空（兜底白名单） | `partial` | message 写 `product_names_empty=true`，提示同步可能不完整，见 §8.5/§13.1 |
| context cancelled | `failed` | 可重试 |

### 13.1 stale 标记门控

stale 标记按类型逐项判断，只有同时满足以下三项的类型才允许执行 stale；否则该类型旧资产保持 active，避免写库失败或转换失败导致误标 stale：

1. **provider 查询完整**：该类型在 `SuccessfulTypes` 中（CES 资源分组查询成功）。同一资源类型可能由多个 `service+dim_name` scope 组成（例如 `SYS.ELB/loadbalancer_id` 与 `SYS.ELB/l7policy_id` 均映射到 `elb`），只要任一 scope 查询失败，该类型即记入 `QueryFailedTypes` 并从 `SuccessfulTypes` 剔除——只有该类型的**所有 scope 都成功**才留在 `SuccessfulTypes` 中。
2. **资源转换完整**：该类型不在 `ConversionFailedTypes` 中（无 `mapCESResource` 因缺主维度而丢弃）。
3. **全部资源成功持久化**：该类型本轮无 upsert 失败。

达到 `max_resources` 的 region 整体禁止 stale。`ces`/`hybrid`/`native` 路径均填充 `SuccessfulTypes`：native 逐类调用旧 4 类，查询成功的类型（即使返回 0 条资源）计入 `SuccessfulTypes`，因此某类资源全部消失时旧记录可被标记 stale。仅 `auth_type=none` 的 fake 路径不填充 `SuccessfulTypes`，此时回退到本轮成功入库类型，仍排除存在 upsert 失败的类型。通用（非 CES）同步路径同样遵循“全部资源成功持久化才执行 stale”。`QueryFailedTypes` 会写入批次 message（`query_failed_types=...`）用于排查为何某类型未执行 stale。

**CES/hybrid 权威 scope 反向 stale 标记**：`ces`/`hybrid` 的资源组 `product_names` 完整定义了资源组内的类型集合，是权威 scope。当某类型从资源组移除（不再出现在 `product_names`）时，CES 不再查询该类型，因此它不会进入 `SuccessfulTypes`，逐类型标记无法触及它的旧资产——这会导致删除资源类型后旧资产永久保持 `active`。为此 `ces`/`hybrid` 路径改用**反向 stale 标记**：对 account+region 调用 `MarkStaleByAccountRegionExceptTypes`，把该 scope 下所有 `active` 的 cloud_sync 资源（排除当前批次）标记为 `stale`，但跳过不确定类型 `exceptTypes = QueryFailedTypes ∪ ConversionFailedTypes ∪ persistFailedTypes`。这样：

- 查询成功且本轮 0 资源的类型 → 旧资产 stale（与逐类型语义一致）；
- 从资源组移除的类型（不在 scope，也不在 exceptTypes）→ 旧资产 stale（修复点）；
- 查询失败/转换失败/持久化失败的类型 → 保持 active（保守，与逐类型一致）。

`native`/通用/fake 路径 scope 非权威（只覆盖固定/有限类型），仍用逐类型标记，不反向标记，避免把未覆盖类型的资产误标 stale。

**`product_names` 为空时的批次状态**：`ShowResourceGroup` 的 `product_names` 为空时回落到内置兜底白名单（见 §8.5），但该白名单不完整且部分维度可能错误。此场景批次至少标记 `partial`（即使无失败 scope），batch message 含 `product_names_empty=true (fallback whitelist used, sync may be incomplete)`，提示操作人员同步可能不完整。

**截断探测（P1 修正）**：native 路径与通用路径此前存在截断后误标 stale 的风险——达到上限时只返回部分资源，却被当作“查询完整”而把云端仍存在的资源误标 stale。

- **native 路径**：`listAllResourcesNative` 对每类请求 `remaining+1` 条探测截断。返回超过 `remaining` 条即置 `MaxResourcesReached=true`，只取 `remaining` 条并 `break`，被截断类型不计入 `SuccessfulTypes`；SyncService 整 region 跳过 stale。因截断提前中断不算“全部类型失败”，不会触发空结果错误。
- **通用（非华为）路径**：`AssetDiscoveryPort.ListResources` 返回签名扩展为 `(resources, hasMore, err)`，`AssetDiscoveryResult` 增加 `HasMore` 字段。provider 请求 `limit+1` 探测截断（华为 `listResourcesReal` 已实现）。SyncService 通用路径在 upsert 完成后若 `result.HasMore=true` 则跳过该类型 stale。资源仍正常入库，仅不标记 stale。
- 例：`max_resources=1`、云端 2 台 ECS → 请求 2、返回 2 → `2>1` → `MaxResourcesReached=true`、`ecs` 不入 `SuccessfulTypes`，整 region 跳过 stale；云端仅 1 台 → 请求 2、返回 1 → 不截断，`ecs` 入 `SuccessfulTypes`，可正常标 stale。

## 14. 审计与日志

审计仍使用：

```text
resource_type = asset_sync_batch
action = sync
```

Payload 增加建议字段：

```json
{
  "account_id": "acc_xxx",
  "provider": "huawei_cloud",
  "sync_mode": "ces",
  "regions": ["cn-south-1"],
  "resource_group": "全部资源",
  "ces_total": 1614,
  "discovered_count": 1614,
  "created_count": 1000,
  "updated_count": 614,
  "stale_count": 0,
  "failed_count": 0,
  "unknown_namespace_count": 0,
  "invalid_resource_count": 0
}
```

日志要求：

- 使用 `logger.From(ctx)`。
- 不记录 AK/SK。
- 不记录 Authorization header。
- 华为云原始错误只写脱敏 code/status。
- 每个 region/project/resource_group/namespace 的数量可以记录。

## 15. 前端对接契约

### 15.1 `/integrations` 接入表单

华为云接入账号表单需要在 `provider=huawei_cloud` 时暴露 CES 资源同步相关配置，并统一写入 `extra_config`。前端不得把这些字段平铺为独立顶层字段，避免破坏后端账号模型。

| 表单项 | 写入字段 | 默认值 | 说明 |
| --- | --- | --- | --- |
| 同步模式 | `extra_config.sync_mode` | `ces` | 可选 `ces` / `hybrid` / `native` |
| 资源组名称 | `extra_config.resource_group_name` | `全部资源` | 默认候选名，需在 CES 控制台预先创建同名分组；未命中即失败；填写 `resource_group_id` 时名称仅用于展示 |
| 资源组 ID | `extra_config.resource_group_id` | 空 | 可选；指定后后端优先使用该分组；生产推荐显式填写 |
| 企业项目 ID | `extra_config.enterprise_project_id` | 空 | 可选，支持 `all_granted_eps` |
| 最大同步资源数 | `extra_config.max_resources` | `20000` | 单账号单次同步保护上限 |
| 区域项目映射 | `extra_config.region_projects` | 空数组 | 多区域账号推荐填写，每项为 `{ region, project_id }` |

表单行为要求：

- 新建 `huawei_cloud + ak_sk` 账号时，`sync_mode` 默认展示为 `ces`，并标记为推荐。
- 编辑账号时回显后端返回的 `extra_config`；字段为空时使用前端默认展示值，但提交时只写用户实际确认的配置。
- 更新账号时如果用户没有修改凭据，不传 `credential`，让后端保留原凭据。
- `credential` 只允许写入，不允许从 `extra_config`、本地状态或表单回显中保存 AK/SK、Token、密码等敏感值。
- `region_projects` 中的 `region` 应来自账号 `regions` 或用户明确新增的区域；空 `region` / 空 `project_id` 不提交。
- 如果配置了多个 `regions` 且缺少对应 `region_projects`，前端应提示“未配置的区域将回落使用账号 project_id”。

### 15.2 同步模式文案

前端文案必须清楚表达三种模式的权限与完整性差异：

| sync_mode | 推荐文案 | 风险提示 |
| --- | --- | --- |
| `ces` | CES 资源同步（推荐）：按指定 CES 资源分组同步，适合仅授予 CES 只读权限 | 资源详情较少，数量口径为指定资源分组；默认候选名需预先创建 |
| `hybrid` | 混合同步：先按指定资源分组发现，再按资源类型补充原生云服务详情 | 需要更多云服务只读权限；增强失败不影响基础资源入库 |
| `native` | 原生云资产同步：兼容旧 ECS/CCE/RDS/ELB 路径 | 不保证与 CES 总览数量一致，仅用于兼容或回退 |

### 15.3 触发同步与批次展示

`/assets` 页面或接入账号详情中触发同步后，前端按 `SyncBatch` 展示结果：

- `status` 使用状态 Tag 展示，至少区分 `running` / `success` / `partial` / `failed`。
- `created_count`、`updated_count`、`stale_count`、`failed_count` 直接展示为批次数量摘要。
- `message` 必须保留 tooltip 或可展开展示，用于排查 `ces_total`、`discovered`、`failed_scopes`、`enriched`、`enrichment_failed` 等摘要。
- `partial` 不等同于失败；页面应提示“部分资源或增强信息失败，基础同步结果以批次 message 为准”。
- `native` 模式的同步结果不得展示“已对齐 CES 总览”之类承诺。

### 15.4 `/assets` 资源列表与详情

资源列表首期至少保持云同步字段可见或可筛选：

- `source`
- `integration_account_id`
- `cloud_resource_type`
- `cloud_resource_id`
- `region`
- `sync_status`
- `last_synced_at`
- `sync_batch_id`

展示要求：

- 未知 namespace 映射出的 `cloud_resource_type` 应原样展示，不要在前端丢弃或强制归为“未知”。
- 建议增加 `cloud_resource_type`、`region`、`sync_status` 筛选；如果后端暂未提供筛选参数，前端不要做跨页假筛选。
- `sync_status=stale` 应有明显标识，避免用户误以为是当前云端仍存在的资源。

### 15.5 labels 展示契约

后端 `asset_resource.labels` 已落库，资源 DTO 已暴露 `labels` 字段，前端资源列表与详情页按以下规则展示：

- CES 基础字段：`namespace`、`dim_name`、`resource_group_id`、`resource_group_name`、`enterprise_project_id`。
- ECS/RDS/ELB/EVS/VPC/DCS/DMS 等增强字段按实际返回展示，例如 `private_ip`、`flavor`、`vpc_id`、`az`、`volume_type`、`size_gb`、`cidr`、`subnet_count`、`engine`、`capacity_gb`、`spec_code`。
- labels 中的未知 key 应以只读键值形式展示，不能丢弃。
- 敏感字段即使后端误返回，前端也不得明文展示，需按敏感 key 名称进行兜底掩码。

### 15.6 API README 同步要求

实现上述前端能力时，需要同步更新 `web/src/api/README.md`：

- `Integration` 章节说明 `extra_config` 字段与凭据不回显规则。
- `Asset` 章节说明同步批次 message、资源云同步字段、labels 暴露状态。
- 如果 `web/src/api/asset.ts` 新增 `labels` 类型，README 必须同步注明该字段来源与展示约束。

## 16. 验收标准

### 16.1 单元测试

必须覆盖：

- `product_names` 解析。
- namespace 映射。
- primary dimension 选择。
- `ListResourceGroups` 分页。
- `ListResourceGroupsServicesResources` 分页。
- 单 namespace 失败不影响其他 namespace。
- `max_resources` 截断。
- stale 只对成功 scope 生效。

建议命令：

```powershell
go test ./internal/observability/infrastructure/provider/huawei/... ./internal/asset/application/...
```

### 16.2 P0/P1 CES-only 集成验收

准备一个华为云账号：

- 授权 `CES ReadOnlyAccess`。
- 能在 CES 控制台看到“全部资源”总数。
- 平台账号配置对应 region/project_id。

验收步骤：

1. 在 CES 控制台记录：

```text
region = cn-south-1
全部资源总数 = N
各资源类型数量 = map[type]count
```

2. 触发平台同步：

```http
POST /api/assets/sync
```

3. 检查 batch：

```text
status = success 或 partial
message 包含 ces_total=N
created_count + updated_count >= N - known_filtered_count
failed_count = 0 时平台 active 资源数应等于 N
```

4. 检查数据库或资源列表：

```text
source = cloud_sync
integration_account_id = acc_xxx
sync_status = active
region = 目标 region
```

5. 对比类型分布：

```text
CES ECS 数量 == platform cloud_resource_type=ecs 数量
CES EVS 数量 == platform cloud_resource_type=evs 数量
CES VPC 数量 == platform cloud_resource_type=vpc 数量
```

如果 `sync_mode=ces` 下 CES 控制台总数与平台 active 数量不一致，batch message 必须能解释差异，例如：

- failed namespace
- unknown namespace
- invalid resource
- max_resources reached
- selected resource group mismatch

### 16.3 P2 hybrid 增强验收

准备一个额外授予 ECS/RDS/ELB/EVS/VPC 等只读权限的账号：

1. 设置 `sync_mode=hybrid`。
2. 触发同步。
3. 验证基础 active 资源数仍与 CES 总数一致。
4. 验证已授权类型出现增强字段，例如 IP、VPC、规格、磁盘关系。
5. 移除某个原生 API 权限后再次同步，基础资源数不下降，batch message 记录对应 enrichment warning。

### 16.4 native 兼容验收

1. 设置 `sync_mode=native`。
2. 验证仍能沿用旧 ECS/CCE/RDS/ELB 同步路径。
3. 验证文案和 batch message 不承诺与 CES 控制台数量一致。

### 16.5 回归验收

变更不能破坏 P0 主链路：

```powershell
go test ./cmd/... ./internal/... ./pkg/...
cd web && npm run build
.\scripts\e2e-alert.ps1
.\scripts\e2e-asset.ps1
.\scripts\e2e-asset-sync.ps1
.\scripts\e2e-runbook.ps1
.\scripts\e2e-execution.ps1
```

根据改动范围可先跑华为 adapter 与 Asset Sync 相关测试，再在合并前跑完整验收。

## 17. 发布策略

推荐灰度：

1. 先补 `sync_mode` 配置，但保留 legacy native 代码路径。
2. 新账号默认 `sync_mode=ces`；已有账号可灰度切换到 `ces`。
3. 对比 CES 控制台资源数和平台资源数。
4. 确认稳定后，将所有 `huawei_cloud + ak_sk` 账号默认同步模式切到 `ces`。
5. 对需要详情的账号启用 `hybrid`。
6. 保留 `native` 配置作为兼容回退，但不作为推荐模式。

回退方式：

- 配置 `sync_mode=native` 回到 ECS/CCE/RDS/ELB 旧路径。
- 已同步的 CES 资源不要物理删除，下一次 native sync 只对 native 成功 scope 标记 stale。

## 18. 风险与注意事项

- CES 控制台总数可能按当前 region、企业项目、资源分组过滤变化；验收必须记录这些过滤条件。
- `product_names` 为空时，全量发现依赖内置 namespace 白名单，可能漏资源；需要在 batch message 中暴露。
- CES 资源维度可能不是稳定的云资源 ID，映射时必须保留 namespace 和 dim_name，避免跨类型冲突。
- `asset_resource` 唯一约束已由迁移 `0026` 补齐为含 `region` 的部分唯一索引（见 §9.4），可区分多区域同类型同云 ID 资源。
- `/api/assets/sync` 已改为异步任务：立即返回 running batch，后台 goroutine 执行同步并续租；后续如需支撑更大账号，可评估 worker/队列化。
- CES API rate limit 需要退避重试；重试仍失败时标记 partial，不要阻塞其他 namespace。

### 18.1 P1：同账号并发同步互斥（已解决）

> 问题：`TriggerSync` 创建 running 批次无账号级互斥，同一账号并发批次交错执行 `MarkStaleByAccountScopeExceptBatch` 时，A 会把 B 刚 upsert 的资源（`sync_batch_id=B`）标记为 stale，产生错误资产状态。仅靠前端按钮 loading 不足以保证一致性。

解决方案（迁移 `0028_asset_sync_batch_running_mutex`，不依赖 Redis）：

- `asset_sync_batch` 新增 `lease_expires_at` 列；建部分唯一索引 `(integration_account_id) WHERE status='running'`，确保每个账号同一时刻只有一个 running 批次。
- `TriggerSync` 在 `Create` 前先 `ReapExpiredRunning(accountID, now)` 清理本账号租约过期的 running 批次（崩溃批次自愈），再插入带 `lease_expires_at = now + 5min` 的 running 批次，立即返回 running DTO。
- 已有 running 批次时 `Create` 触发唯一冲突 → 映射为 `ALREADY_EXISTS`（HTTP 409，`message=sync already in progress for this account`）。
- **异步生命周期**：同步在后台 goroutine 执行（`runCtx` 派生自进程级 `shutdownCtx` + 30 分钟硬超时），与 HTTP 请求生命周期解耦。后台 goroutine 每 60 秒通过 `RenewLease` 续租，把 `lease_expires_at` 推进到 `now+5min`，保证正常同步不会因超时被 reap。终态写入与审计使用独立短 context（10 秒超时），即便 `runCtx` 取消（进程关闭/硬超时）也能落终态，不卡 `running`。
- 终态 `Update` / `finishBatchFailedDetached` 把 `lease_expires_at` 清空，释放槽位。
- 测试：`TestSyncService_TriggerSyncRejectsConcurrentRunning`、`TestSyncService_TriggerSyncReapsExpiredLease`、`TestSyncService_AsyncRenewsLeaseDuringSync`、`TestSyncService_CancelledReachesTerminal`、`TestSyncService_HardTimeoutFails`。
- 契约：见 `ops/cloud-observability-contract.md` §5.5.1、`ops/migration-contract.md` `0028`。

## 19. 最终完成定义

满足以下条件才算实现完成：

- `sync_mode=ces` 是默认推荐模式。
- 使用仅 CES 只读权限的账号，可以同步指定 CES 资源分组（默认候选名“全部资源”，需预先创建）下可见的资源。
- 平台 active 资源数与指定资源分组总数一致，或 batch message 能解释所有差异。
- EVS/VPC/OBS/DCS/DMS/RDS/ELB/ECS 等截图中出现的类型能进入 `asset_resource`。
- `sync_mode=hybrid` 可以在指定资源分组发现基础上按权限补充详情，增强失败不影响基础资源入库。
- `sync_mode=native` 仅作为旧路径兼容，不作为完整性验收口径。
- 同步批次有审计、有失败摘要、有 stale 语义。
- 真实账号不会返回 fake 数据。
- P0 告警接入、资产匹配、Runbook 推荐、Execution 闭环不受影响。

## 20. 前端状态与后续待办

以下能力后端接口/`extra_config` 已就绪，前端已完成接入账号表单的基础对接，剩余展示能力列入后续前端演进待办：

1. **接入表单已暴露 `extra_config` 配置项**（[web/src/views/integrations/index.vue](../web/src/views/integrations/index.vue)）：
   - `sync_mode`（ces/hybrid/native 选择器，默认 ces）
   - `resource_group_name`（默认"全部资源"）
   - `resource_group_id`（可选，指定后优先）
   - `enterprise_project_id`（可选，支持 `all_granted_eps`）
   - `max_resources`（默认 20000，单区域单次同步保护上限；多区域账号按区域独立计额）
   - `region_projects`（region → project_id 映射数组，表单按 `region=project_id` 每行录入）
   - 上述字段写入 `extra_config` JSON；编辑时回显现有配置；未填写新凭据时不提交 `credential`；保留后端返回的未知 `extra_config` key。
2. **资源 labels 已暴露与展示**：
   - 后端 `ResourceDTO` 已返回 `labels` 字段，前端资源列表与详情页已展示 CES `namespace/dim_name/resource_group` 及增强字段（ECS `private_ip/flavor/vpc_id/az`，EVS `volume_type/size_gb/attached_to`，VPC `cidr/subnet_count`，DCS `engine/capacity_gb`，DMS `engine/spec_code` 等）。
   - 后续仅需继续优化 labels 分组、排序和批量排查体验，并保持 `web/src/api/README.md` 同步。
3. **同步批次摘要展示**：批次列表 `message` tooltip 已保留，批次详情页已结构化展示 `ces_total/discovered/failed_scopes/enriched/enrichment_failed`，同时保留原始 `message`。
4. **OBS 原生增强**：后端尚未落地（需另引 OBS SDK），前端无需处理。

## 21. P2 增强缺口（非阻塞，按需）

> 以下缺口不影响 P0/P1 CES 指定资源分组发现与入库，仅影响 `sync_mode=hybrid` 的详情增强覆盖面，按需排期。

1. **OBS 原生增强未落地**（§20-4、§10）

   - `resource_client.go:38` 的 `supportedCloudResourceTypes` 不含 `obs`，且无 OBS SDK 客户端。
   - `sync_mode=hybrid` 下 OBS 资源只有 CES 基础信息（namespace/dim_name/resource_group），无详情增强。
   - OBS 不在华为云统一 SDK 包内，需另引 `huaweicloud-sdk-go-obs` 且 endpoint 走 OBS 域名，列入后续待办。

2. **ELB / CCE 增强实际无效果**

   - `resource_mapper.go:107` `mapELBLoadBalancer` 和 `resource_mapper.go:127` `mapCCECluster` 都不产生 `Labels`。
   - `enrichResources` 按 `ProviderRef` 匹配原生资源后，因原生资源 `Labels` 为空，合并循环不写入任何字段，`enrichedCount` 不增加。
   - hybrid 模式仍会调用 ELB/CCE 原生 List API，但合并不到任何 label，增强对这些类型是空跑。

3. **CBR/SFS/NAT/VPCEP/APM 无原生增强客户端**

   - CES mapper（§9.3）能发现 `SYS.CBR`/`SYS.SFS`/`SYS.NAT`/`SYS.VPCEP`/`SYS.APM` 并入库为基础资源。
   - 上述类型均不在 `supportedCloudResourceTypes` 中，无对应原生 API 增强客户端。
   - 属 §10「按需增强」范畴，可接受，但需知晓：这些类型在 hybrid 模式下也只有 CES 基础信息。

4. **EVS/VPC hybrid 增强匹配键不成立（P1，已知缺陷）**

   - 客户端已实现（`listEVS`/`listVPC`/`mapEVSVolume`/`mapVPC`），但 `enrichResources` 按 `ProviderRef` 匹配 CES 资源时，EVS/VPC 两类从根上无法命中，hybrid 增强对这两类实际不生效（与 §21.2 ELB/CCE 空跑性质类似，但根因是匹配键选错而非 label 为空）。
   - **EVS**：`resource_mapper.go:190` `mapEVSVolume` 取 `ProviderRef=vol.Name`（卷显示名称），试图对齐 CES `SYS.EVS` 的 `dim_name=disk_name`。但官方定义 `disk_name` 实际格式为「服务器ID-盘符」（如 `6f3a...-vda`）或「服务器ID-volume-卷ID」，并非卷显示名称。参考 https://support.huaweicloud.com/usermanual-evs/evs_01_0044.html 。两者无交集，`enrichResources` 永远匹配不到。
   - **VPC**：`mapVPC` 取 `ProviderRef=v.Id`（VPC ID），试图对齐 `dim_name=vpc_id`。但 CES `SYS.VPC` 不存在 `vpc_id` 主维度，其主维度为 `publicip_id`/`bandwidth_id`/`subnet_id`/`peering_id`。参考 https://support.huaweicloud.com/eu/usermanual-ces/en-us_topic_0202622212.html 。`ProviderRef=vpc id` 无法对齐任何 CES 维度。
   - **测试掩盖了问题**：`adapter_test.go:642` `TestAdapterListAllResourcesHybridEVSVPCDCSDMS` 把 CES 资源与 mock 原生资源的 `ProviderRef` 人为写成相等（`disk-01`/`disk-01`、`vpc-1`/`vpc-1`），既不符合真实 CES dim 格式，也未反映 EVS/VPC 的不匹配，断言 `EnrichedCount=3` 因此误判 EVS/VPC 增强生效。已补充 `TestAdapterListAllResourcesHybridEVSVPCRealDimNoMatch` 用真实 dim 格式验证不命中。
   - **影响范围**：不影响 `sync_mode=ces` 的资源发现完整性（CES 基础资源仍正常入库）；仅影响 hybrid 模式下 EVS/VPC 的 label 详情增强——这两类在 hybrid 下退化为只有 CES 基础信息（namespace/dim_name/resource_group），无 `volume_type/size_gb/attached_to`、`cidr/subnet_count` 等增强字段。
   - **修复方向（待定，不在本次范围内）**：EVS 需改用 CES `disk_name` 的真实格式（「服务器ID-volume-卷ID」或「服务器ID-盘符」）作为匹配键，或改用 `volume_id` 维度（若 CES 暴露）；VPC 需放弃「增强 VPC 本体」的思路，改为按 `publicip_id`/`subnet_id` 维度增强 EIP/子网等子资源，而非 VPC 实体。确定方案后再改 mapper 与测试断言。
