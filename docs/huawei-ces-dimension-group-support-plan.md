# 华为云 CES dimension 级资源组支持方案（排期文档）

> 状态：**M2/M3 已实施（2026-11-10），M4 待 203 真实账号验收**。本文档是实施与验收的唯一依据。dimension 级资源组支持已落地：发现分流、映射规范化、覆盖对账、摘要字段、前端标签与契约均已按本方案实现（稳定契约 §8.4/§8.5/§8.5.1/§9.2/§13.1/§15.3 已转为正式规范）；剩余工作为 203 部署 + 真实账号验收（见 §7）与发布说明。
>
> 关联文档：[稳定契约](../ops/huawei-ces-sync-contract.md) · [已知缺口与待办](huawei-ces-sync-backlog.md)（第 4 节待办 1 已关闭）· [发布 Runbook](huawei-ces-sync-runbook.md)（§1.4.1 dimension 验收）

## 1. 背景与现状

华为云 CES 资源分组有两种资源层级，由 `ShowResourceGroup` 的**响应字段** `resource_level` 区分：

| 层级 | 含义 | `product_names` | 平台现状 |
| --- | --- | --- | --- |
| `product` | 按云产品建组 | 有意义（权威产品/维度集合） | 已支持 |
| `dimension` | 按子维度（可用区、业务线等）建组 | 为空（官方语义：仅 product 级有意义） | **P0 不支持，跳过** |

2026-11-10 真实账号（`acc-50843711-8bfe-4e30-88d6-1e016043be23`，cn-south-1，`sync_mode=ces`，未指定资源组 → 聚合口径 `all_groups`）实测：

- 该 project/region 共 **10 个资源组**：6 个 product 级 + 4 个 dimension 级。
- product 级 6 组正常同步：**621 个资源**全部入库（evs 373 / ecs 186 / elb 38 / obs 22 / gaussdb 1 / dms 1）。
- dimension 级 4 组被跳过：**大数据 118 / 户用电站 358 / 商用电站 205 / 极光系统 524**，合计 **1205 个资源（约占 66%）**。
- 批次因此恒为 `partial`，message 含 `failed_scopes=4 … scope_incomplete=true` 与 4 条 `cn-south-1/<组名>: resource_level="dimension" is not supported, group skipped`。

这不是故障，是当前 P0 设计的预期行为；但对按业务线建维度组的账号，**66% 的资产永远进不来**，需要正式支持。

## 2. 目标与非目标

### 目标

1. 聚合口径（`all_groups`）与显式指定口径（`specified_id` / `specified_name`）下，dimension 级资源组**不再被跳过**，其资源正常发现、映射、入库。
2. dimension 组不再导致 `failed_scopes` / `scope_incomplete` / `product_names_empty`；全部组（product + dimension）解析与查询成功时，批次可为 `success`。
3. stale 安全语义不弱化：dimension 组**不得**成为区域级反向 stale 的权威 scope。
4. 组级覆盖可对账：每组声明 total 与实际发现数的差异必须能被摘要/日志解释。

### 非目标

- 不改变 product 级组的现有发现、映射与 stale 语义。
- 不引入对任意未知 namespace 的自动发现（见 §4.6 覆盖限制）。
- 不改动 hybrid 增强链路本身（dimension 发现的资源按既有 `cloud_resource_type` 自然进入增强流程；EVS 匹配键等既有缺口不在本方案范围）。
- 不引入新的账号级配置项作为前置条件（可选开关见 §8 发布策略）。

## 3. 已验证的 API 事实（2026-11-10 真实账号探测）

1. `resource_level` 是 `ShowResourceGroup` 的**响应字段**（非查询参数），SDK `huaweicloud-sdk-go-v3 v0.1.201` 已暴露（`ShowResourceGroupResponse.ResourceLevel.Value()`）。
2. **`ListResourceGroupsServicesResources` 的 `service` 是路径参数，必传**。漏传会返回 404「API does not exist」——这是路径参数缺失的表现，不是"维度组不可查"。排查 404 时先核对 SDK `ces_meta.go` 中 `GenReqDefFor...` 的 `WithPath(...)`。
3. **带上 `service`（可省略 `dim_name`）即可返回该组该服务的全部资源**，且每条资源的 `dimensions[]` 带**实际维度名**。现有 `listResourcesForProduct` 在 `DimNames` 为空时本就不传 `dim_name`，可直接复用。
4. 探测 4 个 dimension 组覆盖 **1191 个资源**，与声明 total 1205 基本吻合（差 14，验收时必须解释，见 §7）。
5. 涉及服务：ECS / EVS / ELB / OBS / DMS / VPC / RDS / DCS / CBR；实际维度含 `instance_id` / `disk_name` / `lbaas_instance_id` / `bucket_name` / `kafka_broker` / `publicip_id` / `rds_cluster_id` / `dcs_instance_id` 等。

## 4. 方案设计

### 4.1 总体流程

```text
selectResourceGroups（不变）
  -> 逐组 ShowResourceGroup 解析
       product 级：现有路径（product_names -> (组, 产品+维度) 工作项）
       dimension 级：新路径（服务枚举白名单 -> (组, 服务) 工作项，不带 dim_name）
  -> 统一分页拉取（max_resources 原始行预算仍在全部工作项间共享，契约 §11）
  -> 映射：dimension 工作项用资源实际 dimensions 推导维度（§4.3）
  -> 跨组去重、计数、stale 门控（§4.4/§4.5）
```

dimension 组解析失败（`ShowResourceGroup` 失败等）仍按现行聚合口径规则跳过并记 `failed_scopes`——**只有"层级不支持"这一跳过理由被移除**。

### 4.2 发现：dimension 组的服务枚举

- 对 dimension 组，按**已知 namespace 集合**（`cesNamespaceMappings` 的 16 个 key，含 `SYS.RDS_MYSQL_CLUSTER`）逐服务构造工作项：`cesProduct{Service: svc}`（`DimNames` 为空）。
- 不使用 `fallbackCESProducts` 的固定维度——dimension 组资源自带实际维度，无需猜测；`product_names_empty` 兜底语义与 dimension 路径完全无关。
- 每个服务查询即"该组内该服务的全量"，分页与截断探测复用现有 `listResourcesForProduct`。
- 调用量评估：16 服务 × dimension 组数（本账号 4 组 = 64 次服务级查询，含分页）。全部走既有 `callCESWithRetry` 退避重试；`max_resources` 共享预算不放大。

### 4.3 映射：用资源实际维度，且主维度需规范化

dimension 工作项的查询不带 `dim_name`，映射时以**每条资源实际返回的 `dimensions[]`** 替代查询维度：

1. `labels.dim_name` = 资源实际维度名（按返回顺序 join）。
2. `resolveNamespaceMappingByDim` 用实际维度解析——`SYS.VPC` 子类型（eip/bandwidth/subnet/peering/vpc）因此自动正确拆分。
3. **主维度规范化（关键）**：直接取"首个非空维度"会导致同一物理资源在不同层级组里得到不同 `ProviderRef`。例如 ELB 在 product 组用 `loadbalancer_id`，在 dimension 组可能返回 `lbaas_instance_id` → 同一实例生成两条资产、跨组去重失效。
   - 因此引入**每服务优选维度表**（初始值 = `fallbackCESProducts` 中该服务的维度，如 ECS→`instance_id`、EVS→`disk_name`、ELB→`loadbalancer_id`、OBS→`bucket_name`）：资源实际维度中包含优选维度时优先取它，缺失时才回落首个非空维度。
   - 目标：同一资源无论来自 product 组还是 dimension 组，`ResourceID` / `ProviderRef` 一致，唯一键 `(cloud_resource_type, cloud_resource_id, region)` 去重生效。
   - 实现位置建议：`selectPrimaryDimension` 增加"优选维度"入参，或在 dimension 分支构造 `effectiveDimNames = [优选维度若命中] + 实际维度` 后复用现有函数；`mapCESResource` 签名保持不变或仅加可选参数，避免影响 product 路径。
4. `invalid_resource_count` / `unknown_namespace_count` / `conversion_failed_types` 等既有计数语义不变。

### 4.4 去重与计数恒等式

- 跨组去重键不变：`(cloud_resource_type, cloud_resource_id, region)`（迁移 0026）。
- 契约 §9.5 三条恒等式不变：`raw = mapped + invalid`、`mapped = unique + duplicate`、`unique = persisted + persist_failed`。
- dimension 组带来的预期变化仅体现在：`duplicate_count` 可能上升（同一资源命中多个组），这是正常折损，验收时用于解释总数差异。

### 4.5 scope 与 stale 口径（核心安全设计，不可妥协）

| 项 | 口径 |
| --- | --- |
| dimension 组是否权威 scope | **否**。其资源集合由任意维度规则定义，且服务枚举白名单未必覆盖组内全部服务 |
| 区域级反向 stale | **禁止**用于 dimension 组。`Summary.ResourceLevel` 维持现状：仅当全部组均为 product 级且零跳过时为 `product`；含 dimension 组时不为 `product` |
| 聚合口径逐类型 stale | 沿用现行规则：dimension 组某服务**完整查询成功**（未失败、未截断）后，对应类型可计入 `SuccessfulTypes`，参与该 region 的逐类型 stale；失败/截断类型按现行 `QueryFailedTypes` / 截断门控剔除 |
| 显式指定 dimension 组（单组口径） | 不再返回 `FAILED_PRECONDITION`；发现后走逐类型 stale，**不做**区域级反向 stale |
| `failed_scopes` | dimension 组不再因层级被记入；仅真实失败（Show/查询失败）才记入 |
| `scope_incomplete` | 不再因"存在 dimension 组"置位；仍用于真实覆盖缺口（含 §4.6 的对账不符） |
| `product_names_empty` | dimension 组**不置位**（该标志专属于 product 级组白名单兜底场景） |

### 4.6 覆盖对账与已知限制

- **组级对账**：每组 `resource_statistics.total`（声明值）与该组实际发现数比对，差异必须可解释（unknown namespace / invalid / 跨组去重 / 未覆盖服务）。实现上至少在 provider 层输出逐组 `Warn` 日志（组名、声明 total、实际发现、差值）。
- **已知限制**：dimension 组内属于**未知 namespace**（不在 `cesNamespaceMappings`，如 `SYS.GAUSSDB`）的服务不会被枚举查询 → 这些资源发现不了。
  - 因此：**当某 dimension 组声明 total > 实际发现且差值无法被既有计数解释时，必须置 `scope_incomplete=true`**（该 region 不标 stale），并在 message/摘要中给出可操作提示（如 `group_coverage_mismatch=<组名>`），不允许静默当作全覆盖。
  - 长期解法：把新 namespace 补入映射表/枚举白名单；不开放任意 namespace 自动探测。
- 本账号待解释差值：探测 1191 vs 声明 1205（差 14），验收时逐组定位。

### 4.7 摘要与前端展示（均为新增可选字段，向后兼容）

- `CloudSyncSummary` / `SyncBatchSummaryDTO` / `summary.scopes[]` 新增 `dimension_group_count`（本轮覆盖的 dimension 级组数，顶层为各 scope 求和）。
- 批次 message 在存在 dimension 组时追加 `dimension_groups=<N>`。
- 前端批次诊断新增信号标签：`Dimension groups = N 个（按资源实际维度发现）`；现有 `Scope incomplete` / `Failed scopes` 标签语义不变。
- 批次状态变化说明：全组成功后由 `partial` 变为 `success`，属预期改善，不是回归。
- 实现时同步更新：`web/src/api/asset.ts` 类型、`web/src/api/README.md`、`ops/cloud-observability-contract.md` §5.5、契约 §15.3。

## 5. 改动范围清单（实施时按此执行；M2/M3 已按本表落地）

| 层 | 文件 | 改动要点 |
| --- | --- | --- |
| provider | `internal/observability/infrastructure/provider/huawei/ces_resource_client.go` | 第一阶段按 `resource_level` 分流；dimension 组构造服务枚举工作项；移除"层级不支持"跳过/单组 `FAILED_PRECONDITION`；组级覆盖对账与 `scope_incomplete`；摘要新增 `dimension_group_count` |
| provider | `.../ces_resource_mapper.go` | 每服务优选维度表；dimension 资源按实际 dimensions 推导 `dim_name`/主维度/子类型 |
| provider | `.../adapter.go` | 透传 `dimension_group_count` 等新摘要字段 |
| observability | `internal/observability/application/dto.go` | `CloudSyncSummary` 新增字段 |
| asset | `internal/asset/application/sync_service.go` | DTO/scope 明细/message 贯通新字段；stale 门控保持 §4.5 口径（含对账不符 → 禁 stale） |
| web | `web/src/views/assets/composables/assetUtils.ts`、`useCloudSync.ts`、`web/src/api/asset.ts` | 新信号标签与类型；`web/src/api/README.md` 同步 |
| 契约/文档 | `ops/huawei-ces-sync-contract.md` §8.4/§8.5/§8.5.1/§9.2/§13.1/§15.3、`ops/cloud-observability-contract.md` §5.5、本 backlog、runbook | 实现落地后将"排期备注"转为正式规范 |
| 数据库 | 无 | **零迁移** |

## 6. 测试计划

provider 单测（mock `cesResourceGroupAPI`）：

1. dimension 组按服务枚举发现（请求**不带** `dim_name`）、分页、跨页截断。
2. 映射使用资源实际维度：`SYS.VPC` 子类型拆分；ELB `lbaas_instance_id` 场景下优选维度规范化后与 product 路径 `ProviderRef` 一致。
3. 同一资源同时出现在 product 组与 dimension 组 → 唯一键去重，只入库一次。
4. dimension 组不置 `product_names_empty`、不记 `failed_scopes`、不置 `scope_incomplete`（正常覆盖时）。
5. 显式 `specified_id` 指定 dimension 组不再失败；单组口径也不做区域级反向 stale。
6. 混合口径（product + dimension）聚合：全部成功时批次可 `success`，且 `ResourceLevel` 不为 `product`。
7. dimension 组某服务查询失败 → 类型入 `QueryFailedTypes` 并从 `SuccessfulTypes` 剔除；组声明 total 与发现数不符且不可解释 → `scope_incomplete=true`。
8. `max_resources` 预算在 product 与 dimension 工作项间共享，不因组数放大。
9. dimension 组内未知 namespace 服务不被枚举，差值通过对账暴露。

asset 层测试：dimension 组不参与区域级反向 stale；`scope_incomplete` 时该 region 完全不标 stale（沿用既有守卫测试模式，做"临时禁用守卫 → 测试失败 → 恢复"反向验证）。

前端测试：`dimension_group_count` 标签渲染；`failed_scopes` 为空时不再出现维度跳过提示。

回归：`go test ./cmd/... ./internal/... ./pkg/...` 全绿；`cd web && npm run build`、`npm run test`、`npm run lint`；`scripts/e2e-asset-sync.ps1` 等 P0 脚本不受影响。

## 7. 验收标准（203 真实账号）

账号 `acc-50843711-8bfe-4e30-88d6-1e016043be23`（配置保持 `{"sync_mode":"ces","max_resources":20000}`）：

1. 10 个组全部覆盖：6 product + 4 dimension；`summary` 含 `resource_group_count=10`、`dimension_group_count=4`。
2. 资源规模：active 资源 ≈ 621 + 1205 − 跨组去重 − 非法资源；所有差值可由摘要计数或日志解释（重点解释 1191 vs 1205 的 14 条差值）。
3. 批次状态 `success`（无其他失败时）；message 不再含 `resource_level="dimension" is not supported`。
4. stale 安全：无区域级反向 stale；未归入任何分组的资源不被新增误标；`scope_incomplete` 仅在真实缺口时出现。
5. `hybrid` 模式下 dimension 发现的资源按既有类型增强链路正常运作（EVS 既有匹配键缺口除外）。
6. 使用 `scripts/e2e-asset-sync-real.ps1` 完成真实账号验收并归档对账摘要。

## 8. 风险与发布策略

| 风险 | 缓解 |
| --- | --- |
| API 调用量上升（16 服务 × dimension 组数） | 既有退避重试；共享 `max_resources` 预算；同步耗时增加属预期，验收时记录耗时基线 |
| 同一资源跨层级组重复入库 | 主维度规范化（§4.3）+ 唯一键去重；用真实账号重复资源场景做专项测试 |
| dimension 组内未知 namespace 漏发现 | 组级覆盖对账 + `scope_incomplete` 保守禁 stale + 可操作提示 |
| 维度名不稳定导致 ID 漂移 | 优选维度表与 product 路径对齐；labels 保留 `namespace`/`dim_name` 便于追溯 |
| 批次状态由 partial 翻转为 success 影响既有看板/验收文案 | 属预期改善；发布说明中明确；runbook 验收口径同步更新 |

发布策略：按既有流程（提交 dev → 203 部署 → 真实账号验收）。如需灰度，可在 `extra_config` 增加可选开关 `enable_dimension_groups`（默认开启与否在 M2 评审时定），不作为前置条件。回滚沿用 `sync_mode=native` 兼容路径；已入库资源不做物理删除。

## 9. 排期里程碑

| 里程碑 | 内容 | 粗略工作量 | 状态 |
| --- | --- | --- | --- |
| M1 | 本方案文档 + backlog/契约/runbook 排期指引 | 0.5 人日 | **已完成（2026-11-10）** |
| M2 | provider 发现分流 + 映射规范化 + provider 单测 | 约 1 人日 | **已完成（2026-11-10）** |
| M3 | asset 层贯通 + stale 门控测试 + 前端标签 + 契约同步 | 约 0.5–1 人日 | **已完成（2026-11-10）** |
| M4 | 203 真实账号验收 + 发布 + 文档终稿（移除各文档"排期备注"） | 约 0.5 人日 | 待启动 |

合计约 2–2.5 人日。M2 启动前需评审确认两点：dimension 服务枚举白名单的初始集合（§4.2，已按 `cesNamespaceMappings` 全部 16 个 key 落地）、是否需要灰度开关（§8，已评审：不引入开关，作为默认行为发布，回滚沿用 `sync_mode=native`）。
