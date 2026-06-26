# 华为云 CES 资源全量同步实现方案

## 1. 目标

平台的华为云资产同步目标调整为：**CES 控制台“云监控服务 CES -> 总览 -> 全部资源/资源分组”能看到多少资源，平台就同步多少资源到 `asset_resource`**。

这意味着同步口径不再以 ECS/CCE/RDS/ELB 等云服务原生 List API 为准，而是以 CES 的监控资源视图为准。云服务原生 API 仍可作为补充信息来源，但不能作为“资源总数是否完整”的判断依据。

目标闭环：

```text
华为云账号只读授权
  -> CES 资源分组/全部资源发现
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

- CES 资源全量同步以 **CES 资源分组接口** 为主路径。
- 云服务原生 API 只做增强，不影响“是否同步完整”的主判断。
- 真实凭据账号不能回落到 fake 数据。
- 资源同步仍走 Asset Sync 批次、审计、stale 标记，不直接删除历史资源。
- 不暴露 AK/SK、Authorization header、原始云端错误详情。
- 同步过程必须记录每个 region、project、resource group、namespace 的成功/失败摘要，便于解释“为什么数量不一致”。
- `QueryService.ListResources` 面向交互查询，当前 `limit` 最大 500；CES 全量同步不能直接受这个上限限制，需要独立分页同步能力。

## 4. 产品分层与同步模式

成熟产品不应把“资源是否可见”和“资源详情是否丰富”绑在一起。平台采用三层能力：

| 层级 | 同步模式 | 目标 | 权限 | 结果 |
| --- | --- | --- | --- | --- |
| P0/P1 | `ces` | CES 页面看到多少，平台同步多少 | `CES ReadOnlyAccess` | 资源数量完整，可用于告警匹配、Dashboard、AI 分析入口 |
| P2 | `hybrid` | 先按 CES 全量发现，再用原生云服务 API 补详情 | CES 只读 + 按需云服务只读 | 补充 IP、规格、VPC、磁盘、引擎版本、配置等 |
| P3 | `hybrid` + topology/inspection | 结合 CES、原生 API、日志、链路做拓扑、巡检和根因分析 | 按能力逐项授权 | 支持拓扑、配置巡检、容量风险、根因候选 |

同步模式定义：

| 模式 | 定位 | 行为 | 是否推荐 |
| --- | --- | --- | --- |
| `ces` | 默认模式 | 只依赖 CES 资源分组/资源列表发现资源，不调用 ECS/RDS/ELB/EVS/VPC 等原生 List API 作为入库前置条件 | 推荐，P0/P1 默认 |
| `hybrid` | 增强模式 | 先执行 `ces` 全量发现并入库，再对已发现资源按类型调用原生 API 增强详情；增强失败不影响基础资源入库 | P2 推荐 |
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

- `CES 资源同步`：推荐，按云监控可见资源同步，适合只授予 CES 只读权限。
- `混合同步`：先 CES 全量发现，再用云服务只读权限补充资源详情。
- `原生云资产同步`：兼容旧路径，需要更多云服务只读权限，不保证与 CES 总览数量一致。

## 5. 权限要求

### 5.1 最小目标权限

如果目标是“和 CES 控制台资源总数一致”，优先授予：

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

增强权限不是 CES 口径全量同步的前置条件。没有增强权限时，平台仍应能用 CES 返回的 namespace、dimension、resource_name 同步基础资产。

### 5.3 多项目与多区域

华为云 `project_id` 通常与区域相关。平台当前账号模型只有一个 `project_id` 和一个 `regions[]`，实现 CES 全量同步时需要处理以下情况：

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
- 选中的 `resource_group_id`
- 选中的 `resource_group_name`
- CES 返回的 `resource_statistics.total`
- 每个 `namespace/dim_name` 的 `count`
- 每个 `namespace/dim_name` 的成功或失败原因

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
- 仍要有 `max_resources` 防护，默认建议 20,000，避免异常配置导致无限扫描。
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

### 8.3 `native` 兼容同步流程

`native` 仅用于兼容旧路径或紧急回退：

```text
sync_mode=native
  -> ECS/CCE/RDS/ELB resource client
  -> 按旧 scope 执行 upsert/stale
```

`native` 不承诺与 CES 控制台“全部资源”数量一致。

### 8.4 资源组选择策略

首期目标是对齐 CES 总览“全部资源”。

选择顺序：

1. 如果请求或账号配置指定 `resource_group_id`，使用该分组。
2. 如果指定 `resource_group_name`，按名称匹配。
3. 否则优先匹配名称：

```text
全部资源
All resources
All Resources
```

4. 如果没有明确名称，选择 `resource_statistics.total` 最大的资源组，并在 batch message 中标记：

```text
selected_resource_group=max_total
```

5. 如果仍找不到资源组，批次失败，返回脱敏错误：

```text
no CES resource group found for project/region
```

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
- 如果 `product_names` 为空，使用内置 CES namespace 白名单兜底，但 batch message 必须记录 `product_names_empty=true`。

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
| `resource_group_name` | `全部资源` | 对齐 CES 控制台总览 |
| `resource_group_id` | 空 | 指定后优先使用 |
| `enterprise_project_id` | 空 | 可选，支持 `all_granted_eps` |
| `max_resources` | `20000` | 单账号单次同步保护上限 |
| `region_projects` | 空 | 中期支持多 region project |

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
13. stale 标记只对成功 scope 生效。
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

### 阶段 3：P3 Hybrid topology / inspection

目标：结合 CES + 原生 API + 日志/链路做巡检和根因分析。

实施顺序：

1. 明确账号模型是否新增 `region_projects`。
2. 如果新增字段，补迁移、DTO、前端表单、API README。
3. 每个 region 使用对应 project_id 创建 CES client。
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
| context cancelled | `failed` | 可重试 |

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

## 15. 前端展示

前端不需要为首期 CES sync 新增复杂页面，但建议：

- `/integrations` 账号表显示 `sync_mode=ces`。
- 账号创建/编辑时将 `ces` 作为默认推荐同步模式。
- `hybrid` 需要提示“将按资源类型请求更多云服务只读权限”。
- `native` 需要提示“兼容旧路径，不保证与 CES 总览数量一致”。
- 触发同步后展示 batch status。
- batch message tooltip 展示 CES total、discovered、failed scopes。
- `/assets` 资源列表支持按 `cloud_resource_type`、`region`、`sync_status` 筛选。
- 未知 namespace 显示原始 `cloud_resource_type`，不要丢弃。

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
- `asset_resource` 当前唯一约束如果不能区分 region/namespace，需要补迁移。
- 大账号同步可能超过当前 HTTP 请求时间，后续应把 `/api/assets/sync` 改为异步任务：立即返回 running batch，由 worker 执行。
- CES API rate limit 需要退避重试；重试仍失败时标记 partial，不要阻塞其他 namespace。

## 19. 最终完成定义

满足以下条件才算实现完成：

- `sync_mode=ces` 是默认推荐模式。
- 使用仅 CES 只读权限的账号，可以同步 CES 控制台“全部资源”中可见的资源。
- 平台 active 资源数与 CES 控制台总数一致，或 batch message 能解释所有差异。
- EVS/VPC/OBS/DCS/DMS/RDS/ELB/ECS 等截图中出现的类型能进入 `asset_resource`。
- `sync_mode=hybrid` 可以在 CES 全量发现基础上按权限补充详情，增强失败不影响基础资源入库。
- `sync_mode=native` 仅作为旧路径兼容，不作为完整性验收口径。
- 同步批次有审计、有失败摘要、有 stale 语义。
- 真实账号不会返回 fake 数据。
- P0 告警接入、资产匹配、Runbook 推荐、Execution 闭环不受影响。
