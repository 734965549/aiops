# 华为云 CES 同步已知缺口与待办

> 本文档记录华为云 CES 资源同步的已落地进度、已知增强缺口与后续待办。大部分缺口仅影响 `sync_mode=hybrid` 的详情增强覆盖面，按需排期；少数缺口（如 `resource_level`，见第 4 节）涉及 CES 发现与 stale 边界，已修复。稳定的接口与状态机定义见《华为云 CES 同步稳定契约》([../ops/huawei-ces-sync-contract.md](../ops/huawei-ces-sync-contract.md))。

## 1. 实现进度总览

### 阶段 1：P0/P1 CES-only 资源发现（已落地）

- CES resource client、`ListResourceGroups` 分页、资源组选择策略、`ShowResourceGroup` 解析 `product_names`、`ListResourceGroupsServicesResources` 分页均已实现。
- mapper 和 namespace 映射表已实现（见稳定契约 §9.3）。
- CES `tags` 已解析并持久化为 `tag.<key>` label（数量上限 20、key/value 长度截断、敏感 key 过滤），见 §9.1。
- CES `status`/`event_status` 已写入 `ces_status`/`ces_event_status` label 持久化到 `asset_resource.labels`，见 §9.1/§15.5。
- 全量同步端口 `CloudFullSyncPort.ListAllResources`（观测层）+ `CloudDiscoveryPort.ListAllResources`（资产层）已落地，避免受 `limit <= 500` 限制。
- `SyncService.TriggerSync` 对 `huawei_cloud + ak_sk` 默认走 `sync_mode=ces`。
- 保留 `auth_type=none` fake 同步。
- 同步批次 message 输出 scope 摘要：`region=cn-south-1 group=全部资源 ces_total=1614 discovered=1614 upserted=1614 failed_scopes=0`。
- 成功 scope 维度从旧路径的固定 `region/resource_type` 调整为 CES 返回的成功 `region/cloud_resource_type`。
- stale 标记只对成功 scope 生效（见稳定契约 §13.1）。
- 前端批次列表保留 message tooltip。
- 批次摘要字段已统一为 `sync_mode / resource_group_name / resource_group_id / projects / regions / ces_total / raw_fetched_count / mapped_count / unique_discovered_count / persisted_count / completed_count / duplicate_count / persist_failed_count / discovered_count / failed_scopes / enriched_count / enrichment_failed_count / enrichment_failed_types / enrichment_warnings / enrichment_stage_error / writeback_failed_count / unknown_namespace_count / invalid_resource_count / max_resources_reached / product_names_empty / query_failed_types / conversion_failed_types`，旧的 `invalid_count`、`conversion_failed_count`、`resource_group`、`project_id` 等旧命名不再作为新协议字段。

### 阶段 2：P2 Native API enrichment（部分落地）

- 已落地 `sync_mode=hybrid` 路由：先执行 CES 指定资源分组发现，再对 CES 返回的 ECS/CCE/RDS/ELB/EVS/VPC/DCS/DMS 资源按类型调用原生 API 增强（CCE/ELB 原生映射暂不产出 label，见下文缺口 2）。`SYS.VPC` 已按 `dim_name` 拆分为 eip/bandwidth/subnet/peering/vpc 子类型，hybrid 对 EIP/带宽/子网/对等连接增强命中。
- 已落地 ECS label 增强：`private_ip`、`flavor`、`vpc_id`、`az`。
- 已落地 RDS label 增强：`private_ip`、`vpc_id`、`subnet_id`、`flavor`。
- label 合并只新增缺失字段，不覆盖 CES 已有 label；增强失败记录到 `EnrichmentFailedTypes`，批次 message 追加 `enriched=N enrichment_failed=type1,type2`。
- 已落地 label 落库：迁移 `0025` 为 `asset_resource` 增加 `labels JSONB` 列；`domain.Resource` 与 `resourceModel` 增加 `Labels` 字段；`SyncService.upsertCloudResourcesWithFallback` 把 `CloudResource.Labels` 写入 `Resource.Labels`；`updateCloudSync` 每次同步整体覆盖 labels，避免陈旧 label 残留。
- label 持久化已有真实 Postgres 往返测试覆盖（`TestResourceRepository_UpsertCloudSyncLabelsRoundtrip`）。
- CES `namespace/dim_name/resource_group` label 仅在 `sync_mode=ces/hybrid`（默认 ces）路径由 `mapCESResource` 产出；`sync_mode=native` 路径不产出 CES label。排查“DB labels 为空”时优先确认账号 `sync_mode` 与数据是否 `0025` 之前的旧行。
- 已落地 RDS `resource_type` 修正：`mapCloudResourceToAssetFields` 将 `rds` 映射为 `database`，对齐稳定契约 §9.3；`SYS.RDS` 不再被存成 `service`。
- 已落地云资源唯一键加 region：迁移 `0026` 重建 `idx_asset_resource_cloud_key` 为 `(integration_account_id, cloud_resource_type, cloud_resource_id, region)` 部分唯一索引；`CloudResourceKey` 增加 `Region`。Repository 查找键接入（`FindByCloudKey` 的 WHERE 与 `UpsertCloudSync` 构造 key 带 region）已补齐，避免多区域同类型同 ID 互相覆盖。
- 资源 DTO 已暴露 `labels` 字段，前端资源列表与详情已展示 labels。
- 已落地 DCS/DMS 原生增强客户端；EVS 客户端已实现但 hybrid 匹配键不成立（见缺口 4）；VPC 子资源（eip/bandwidth/subnet/peering）客户端与 mapper 已实现且匹配键成立。`supportedCloudResourceTypes` 追加 `evs/vpc/dcs/dms/eip/bandwidth/subnet/peering`；`resource_client.go` 新增 `listEVS/listVPC/listDCS/listDMS/listEIP/listBandwidth/listSubnet/listPeering` 及对应 SDK client。
- 已落地 stale 门控修正（稳定契约 §13.1）：`ConversionFailedTypes`、per-type upsert 失败跟踪、`SuccessfulTypes − ConversionFailedTypes − persistFailedTypes`。
- 已落地同类型多 scope 部分失败修正（稳定契约 §13.1）：`QueryFailedTypes`，任一 scope 失败时把对应类型记入并从 `SuccessfulTypes` 剔除。批次 message 增加 `query_failed_types=...`。
- 已落地 native 兼容路径修正（稳定契约 §8.3）：`listAllResourcesNative` 显式遍历 `legacyNativeResourceTypes`（固定旧 4 类）逐类调用；EVS/VPC/DCS/DMS 详情增强仅 `hybrid` 模式。
- 已落地账号配置快照冻结（稳定契约 §13.2）。
- 已落地批量 upsert + chunk 租约校验（稳定契约 §13.3）。
- 已落地同账号并发同步互斥（稳定契约 §18.1）。
- 已落地 hybrid 两阶段拆分（稳定契约 §8.2）：`listAllResourcesHybrid` 仅做 CES 发现与基础 upsert；增强独立为 `CloudEnrichmentPort.EnrichAllResources`（provider 实现），由 SyncService 在基础落库后调用，并通过 `PatchCloudSyncLabelsBatchWithLease` 带租约回写 labels。增强失败只置 `partial` 并分别记录 `EnrichmentFailedTypes` / `EnrichmentStageError` / `WritebackFailedCount`（不递增 `EnrichmentFailedCount`，保持不变式），不影响基础计数与 stale 门控。

### 阶段 3：P3 Hybrid topology / inspection（部分落地，后续待办）

- `region_projects` 已落地：放入 `extra_config` JSON，无需新增 DB 列。
- 后端 `SyncModeConfig`/adapter/契约已落地；前端接入表单已暴露 `region_projects`/`sync_mode`/`resource_group_name`/`resource_group_id`/`enterprise_project_id`/`max_resources` 等配置项。
- 每个 region 使用对应 project_id 创建 CES client 已落地。
- batch summary 按 region/project 聚合已落地：`SyncBatchSummaryDTO` 新增 `scopes[]`，每个 scope 保留单 `region/project/resource_group` 组合的明细。旧顶层聚合字段保留作兼容；多区域排查必须读取 `scopes[]`。审计 payload 同步增加 `scopes` 明细。

后续待办：

1. 建立资源关系模型，例如 ECS -> EVS、ECS -> VPC、ELB -> ECS/RDS。
2. 将拓扑和资源详情接入 Inspection evidence。
3. 支持按资源类型、region、应用、告警状态触发巡检策略。

## 2. 前端状态与后续待办

以下能力后端接口/`extra_config` 已就绪，前端已完成接入账号表单的基础对接，剩余展示能力列入后续前端演进待办：

1. **接入表单已暴露 `extra_config` 配置项**（[../web/src/views/integrations/index.vue](../web/src/views/integrations/index.vue)）：
   - `sync_mode`（ces/hybrid/native 选择器，默认 ces）
   - `resource_group_name`（占位提示“全部资源”，留空提交即未指定；后端按默认候选名回退）
   - `resource_group_id`（可选，指定后优先，未配置 region 专属资源组时作为回落值）
   - `enterprise_project_id`（可选，支持 `all_granted_eps`）
   - `max_resources`（默认 20000，单区域单次同步保护上限；多区域账号按区域独立计额）
   - `region_projects`（region → project_id 映射数组，表单按 `region=xxx,project_id=xxx[,resource_group_id=xxx[,resource_group_name=xxx]]` 每行录入；每项可选填 `resource_group_id` / `resource_group_name` 按 region 解析资源组，空值或未配置时回落全局值）
   - 上述字段写入 `extra_config` JSON；编辑时回显现有配置；未填写新凭据时不提交 `credential`；保留后端返回的未知 `extra_config` key。
2. **资源 labels 已暴露与展示**：
   - 后端 `ResourceDTO` 已返回 `labels` 字段，前端资源列表与详情页已展示 CES `namespace/dim_name/resource_group` 及已生效增强字段（ECS `private_ip/flavor/vpc_id/az`，RDS `private_ip/vpc_id/subnet_id/flavor`，DCS `engine/capacity_gb`，DMS `engine/spec_code`，EIP `public_ip/private_ip/bandwidth_id`，Bandwidth `size_mbps/share_type`，Subnet `cidr/gateway_ip/vpc_id/az` 等）。
   - EVS 当前仅展示 CES 基础 labels；`volume_type/size_gb/attached_to` 等详情增强因匹配键不成立尚未支持，不能作为完成定义或 UI 承诺。
   - 后续仅需继续优化 labels 分组、排序和批量排查体验，并保持 `web/src/api/README.md` 同步。
3. **同步批次摘要展示**：批次列表 `message` tooltip 已保留；后端 `SyncBatch` DTO/API 已返回正式 `summary` 对象，批次详情页优先读取 `summary` 展示 `ces_total/discovered_count/failed_scopes/enriched_count/enrichment_failed_types`，仅对旧数据保留 `message` 解析兜底。`message` 不再作为新数据的半结构化协议。
4. **OBS 原生增强**：后端尚未落地（需另引 OBS SDK），前端无需处理。

## 3. P2 增强缺口（非阻塞，按需）

> 以下缺口不影响 P0/P1 CES 指定资源分组发现与入库，仅影响 `sync_mode=hybrid` 的详情增强覆盖面，按需排期。

### 缺口 1：OBS 原生增强未落地

- `resource_client.go` 的 `supportedCloudResourceTypes` 不含 `obs`，且无 OBS SDK 客户端。
- `sync_mode=hybrid` 下 OBS 资源只有 CES 基础信息（namespace/dim_name/resource_group），无详情增强。
- OBS 不在华为云统一 SDK 包内，需另引 `huaweicloud-sdk-go-obs` 且 endpoint 走 OBS 域名，列入后续待办。

### 缺口 2：ELB / CCE 增强实际无效果

- `resource_mapper.go` `mapELBLoadBalancer` 和 `mapCCECluster` 都不产生 `Labels`。
- `enrichResources` 按 `ProviderRef` 匹配原生资源后，因原生资源 `Labels` 为空，合并循环不写入任何字段，`enrichedCount` 不增加。
- hybrid 模式仍会调用 ELB/CCE 原生 List API，但合并不到任何 label，增强对这些类型是空跑。

### 缺口 3：CBR/SFS/NAT/VPCEP/APM 无原生增强客户端

- CES mapper 能发现 `SYS.CBR`/`SYS.SFS`/`SYS.NAT`/`SYS.VPCEP`/`SYS.APM` 并入库为基础资源。
- 上述类型均不在 `supportedCloudResourceTypes` 中，无对应原生 API 增强客户端。
- 属“按需增强”范畴，可接受，但需知晓：这些类型在 hybrid 模式下也只有 CES 基础信息。

### 缺口 4：EVS hybrid 增强匹配键不成立（P1，已知缺陷）；VPC 已修复

- **VPC（已修复）**：`SYS.VPC` 按 `dim_name` 拆分为 `eip`/`bandwidth`/`subnet`/`peering`/`vpc` 子类型，新增原生客户端 `listEIP`/`listBandwidth`（eip/v2 SDK）/`listSubnet`/`listPeering`（vpc/v2 SDK）与 mapper `mapEIP`/`mapBandwidth`/`mapSubnet`/`mapPeering`。`ProviderRef` 对齐 CES 维度值（eip→publicip id、bandwidth→bandwidth id、subnet→subnet id、peering→peering id），`enrichResources` 按 type 分组 + ProviderRef 匹配，hybrid 增强对 EIP/带宽/子网/对等连接命中。`mapVPC`（native VPC 实体）保留不变，对齐 `vpc_id`。测试 `TestAdapterListAllResourcesHybridVPCSubtypeEnrichment` 验证 `EnrichedCount=2`（eip+bandwidth）。

  VPC 子资源增强 label（已生效）：
  - EIP `public_ip/private_ip/bandwidth_id/share_type/status/ip_type`
  - Bandwidth `size_mbps/share_type/charge_mode/status`
  - Subnet `cidr/gateway_ip/vpc_id/az/available_ip_count`
  - Peering `request_vpc_id/accept_vpc_id/status`

- **EVS（仍未修复）**：`mapEVSVolume` 取 `ProviderRef=vol.Name`（卷显示名称），试图对齐 CES `SYS.EVS` 的 `dim_name=disk_name`。但官方定义 `disk_name` 实际格式为「服务器ID-盘符」（如 `6f3a...-vda`）或「服务器ID-volume-卷ID」，并非卷显示名称。参考 <https://support.huaweicloud.com/usermanual-evs/evs_01_0044.html> 。两者无交集，`enrichResources` 永远匹配不到。

  EVS 增强 label（客户端已实现，但匹配键不成立，hybrid 实际不生效）：`volume_id`、`volume_type`、`size_gb`、`attached_to`、`az`、`created_at`、`charging_mode`（从 `Metadata.orderID` 推断）。`ProviderRef=volume name`（试图对齐 CES `dim_name=disk_name`）。

- **影响范围**：不影响 `sync_mode=ces` 的资源发现完整性（CES 基础资源仍正常入库）；仅影响 hybrid 模式下 EVS 的 label 详情增强——EVS 在 hybrid 下退化为只有 CES 基础信息（namespace/dim_name/resource_group），无 `volume_type/size_gb/attached_to` 等增强字段。VPC 子资源（eip/bandwidth/subnet/peering）增强已生效。

- **测试**：`TestAdapterListAllResourcesHybridEVSVPCDCSDMS` 仍用人为相等的 `ProviderRef` 验证 EVS/DCS 增强链路（不含 VPC 真实 dim）；`TestAdapterListAllResourcesHybridEVSRealDimNoMatch` 用真实 dim 格式验证 EVS 不命中（VPC 已移出此测试，因 VPC 已修复）；`TestAdapterListAllResourcesHybridVPCSubtypeEnrichment` 验证 VPC 子类型增强命中。

- **EVS 修复方向（待定，不在本次范围内）**：需改用 CES `disk_name` 的真实格式（「服务器ID-volume-卷ID」或「服务器ID-盘符」）作为匹配键，或改用 `volume_id` 维度（若 CES 暴露）。确定方案后再改 mapper 与测试断言。

### 已生效增强字段汇总

| 资源类型 | 增强状态 | 增强 label 字段 |
| --- | --- | --- |
| ECS | 已生效 | `private_ip`、`flavor`、`vpc_id`、`az` |
| RDS | 已生效 | `private_ip`、`vpc_id`、`subnet_id`、`flavor` |
| DCS | 已生效 | `instance_name`、`engine`、`engine_version`、`capacity_gb`、`spec_code`、`private_ip`、`az`、`vpc_id`、`charging_mode`、`created_at` |
| DMS（Kafka） | 已生效 | `instance_name`、`engine`、`engine_version`、`spec_code`、`capacity_gb`、`private_ip`、`vpc_id`、`charging_mode`、`created_at` |
| DMS（RocketMQ） | 已生效 | `instance_name`、`engine`、`engine_version`、`spec_code`、`capacity_gb`、`az`、`vpc_id`、`charging_mode`、`created_at` |
| EIP | 已生效 | `public_ip`、`private_ip`、`bandwidth_id`、`share_type`、`status`、`ip_type` |
| Bandwidth | 已生效 | `size_mbps`、`share_type`、`charge_mode`、`status` |
| Subnet | 已生效 | `cidr`、`gateway_ip`、`vpc_id`、`az`、`available_ip_count` |
| Peering | 已生效 | `request_vpc_id`、`accept_vpc_id`、`status` |
| VPC（native 实体） | 已生效 | `ProviderRef=vpc id`，对齐 `dim_name=vpc_id` |
| EVS | 未生效（匹配键不成立） | 客户端已实现，hybrid 实际不命中 |
| ELB | 空跑 | 原生映射不产出 label |
| CCE | 空跑 | 原生映射不产出 label |
| OBS | 未落地 | 需另引 OBS SDK |
| CBR/SFS/NAT/VPCEP/APM | 未落地 | 仅 CES 基础信息 |

> 注意：DMS 与 DCS 的 `charging_mode` 数值语义相反（DCS `1=包周期`，Kafka/RocketMQ `0=包周期`），mapper 已分别处理，统一输出 `prepaid/postpaid`。

## 4. CES 发现层待办（非增强类）

### 待办 1：ShowResourceGroup `resource_level` 响应字段已接入（已修复）

- ~~官方 `ShowResourceGroup` API 支持 `resource_level=product|dimension` 查询参数~~ **更正**：`resource_level` 是 `ShowResourceGroup` 的**响应字段**（非查询参数），取值 `product`（云产品）或 `dimension`（子维度），见 https://support.huaweicloud.com/api-ces/ShowResourceGroup.html 。`product_names` 仅在 `resource_level=product` 时有意义。
- ~~SDK `ShowResourceGroupRequest` 尚未暴露 `ResourceLevel` 字段~~ **更正**：SDK `huaweicloud-sdk-go-v3 v0.1.201` 的 `ShowResourceGroupResponse.ResourceLevel` 已暴露该字段（`Value()` 返回字符串），`OneResourceGroupResp.ResourceLevel` 同样存在。
- **已修复**：代码从 Show 响应读取 `resource_level` 并传递到 `CESResourceDiscoverySummary.ResourceLevel` -> `CloudSyncSummary.ResourceLevel`。P0 阶段仅支持 `resource_level=product`，`dimension` 级或未知/空层级直接返回 `FAILED_PRECONDITION`，不静默回退。仅 `resource_level=product` 且 `product_names` 非空时 scope 才是权威范围，允许反向 stale。
- **后续待办**：若需支持 `dimension` 级资源组，基于资源组详情和 `ListResourceGroupsServicesResources` 指定服务/维度资源接口建立精确范围，而非复用产品级反向 stale 逻辑。
- 契约已同步更新，见 `ops/huawei-ces-sync-contract.md` §8.5/§13.1。
