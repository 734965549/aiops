# internal/asset 模块说明

Asset 负责应用/资源注册表、告警匹配规则、云资源同步、CMDB 资源拓扑和云资源解析。资产与拓扑接口以 `ops/cloud-observability-contract.md` §5.5 为准，CES 同步细节以 `ops/huawei-ces-sync-contract.md` 为准。

## 1. 当前能力与完成边界

| 能力 | 当前实现 | 完成边界 |
| --- | --- | --- |
| 应用/资源注册表 | CRUD、标准分页、资源筛选/排序/facets、labels、删除保护 | 没有独立的应用/资源 GET 详情路由；列表只返回当前页，不是完整字典 |
| 匹配规则 | glob 规则、优先级、启停、应用/资源目标、回退默认匹配 | 复杂 matcher 与 Incident 降噪不属于本模块当前能力 |
| 云资源同步 | fake/CES/hybrid/native、异步批次、账号互斥、租约/fencing、结构化 summary、stale 门控 | 真实云对账需独立验收；EVS 增强不命中，ELB/CCE 增强空跑，OBS 等类型无原生增强 |
| CMDB 拓扑 | 注册表 contains、跨应用手工关系、查询期云派生边、根资源 1–3 层、截断与未解析计数 | 不使用 Observability fake 调用拓扑；ELB/CCE 等更深层自动关系仍待扩展 |
| 云资源解析 | matched/unregistered/ambiguous，按账号、Region、类型和云 ID 精确筛选 | 多候选不自动选择；不会创建资源或返回凭据 |
| 数据范围 | AI 资源读取/发现有对象级校验 | 普通注册表/拓扑路由只有静态 RBAC；Sync 触发和批次查询的契约所述数据范围当前未完整接入 |
| 前端 | 注册表、云同步、匹配规则、独立拓扑页、资源解析与 AI 上下文 | 后端已支持 updated_at 排序，前端当前只开放 created_at 排序；拓扑没有独立 E2E 脚本 |

## 2. 分层与调用关系

```text
interfaces/http
  -> AssetService          应用/资源、删除保护、AI 资源读取
  -> MatchRuleService      匹配规则 CRUD 与审计
  -> SyncService           异步云同步、批次、租约与终态
  -> TopologyService       注册表/手工/云派生拓扑与资源解析

application ports
  -> AssetMatcher          供 Alert 调用匹配
  -> CloudDiscoveryPort    全量云资源发现
  -> IntegrationAccountPort 同步账号快照与 capabilities
  -> AuthorizationPort     AI 资源对象级数据范围
  -> AuditRecorder         Operation Audit

infrastructure
  -> GORM Repositories     asset_application / asset_resource / asset_match_rule
  -> SyncBatchRepository   asset_sync_batch 与 running/lease/fencing
  -> RelationRepository    asset_resource_relation
  -> ApplicationDeleteExecutor / ReferenceChecker
```

## 3. 路由与权限

所有 Asset 路由都位于 Authed 组：

- 读：`app:assets:read`。
- 写：`app:assets:write`。

| 领域 | 路由 |
| --- | --- |
| 应用 | `GET/POST /api/assets/applications`、`PUT/DELETE /api/assets/applications/:id` |
| 资源 | `GET /api/assets/applications/:application_id/resources`、`POST /api/assets/resources`、`PUT/DELETE /api/assets/resources/:id` |
| 筛选字典 | `GET /api/assets/applications/:application_id/resource-facets` |
| 云资源解析 | `GET /api/assets/resources/resolve` |
| 匹配规则 | `GET/POST /api/assets/match-rules`、`PUT/DELETE /api/assets/match-rules/:id` |
| 云同步 | `POST /api/assets/sync`、`GET /api/assets/sync/batches`、`GET /api/assets/sync/batches/:batch_id` |
| CMDB 拓扑 | `GET /api/assets/topology` |
| 手工关系 | `GET/POST /api/assets/resource-relations`、`DELETE /api/assets/resource-relations/:relation_id` |

路由中间件使用 `AuthorizeStatic`，只校验操作权限。当前没有 `GET /api/assets/applications/:application_id` 或 `GET /api/assets/resources/:resource_id` 的公开详情接口。

## 4. 注册表与匹配规则

### 4.1 列表、筛选和排序

- 应用、资源、匹配规则统一 PageData，`page=1`、`page_size=20`，最大 100。
- 资源支持 source/cloud_resource_type/region/sync_status 精确筛选，keyword 跨字段模糊匹配，通配符按字面转义。
- 排序在服务端完成并追加 `id ASC` 稳定次序；后端白名单已包含 `created_at` 和 `updated_at`。
- facets 只返回当前应用下非空云类型和 Region 数组，最终筛选仍走资源列表。
- 前端导出 CSV 会按当前筛选和排序跨页拉取，最多 2000 条；不是只导出当前页。

### 4.2 删除保护

- 删除应用前需处理其资源和匹配规则；存在未关闭告警引用时阻断，已关闭告警可在事务内解除引用；Inspection Policy 引用也会阻断。
- 删除资源前需删除匹配规则和手工/云关系；有引用时返回 `FAILED_PRECONDITION`。
- 云同步产生的应用 ID 使用 `cloud-<账号前17个字符>-<sha1前12位>`，不使用完整账号 ID。

**当前命名差距**：迁移 0036/0038 已归一化部分历史云应用名称，但当前 `ensureCloudApplication` 创建新应用时仍使用 `<provider>-cloud`，未按契约附带 account_id。历史迁移完成不代表后续新建名称一定符合目标契约。

### 4.3 匹配规则

规则包含 label_key、glob pattern、priority、source_type、target_type、application_id 和可选 resource_id。高优先级且启用的规则先匹配，未命中时回退默认应用/Pod/Instance 匹配。创建、更新、删除均写审计，审计失败不回滚主业务。

## 5. 云资源同步

### 5.1 同步模式与批次

| 模式 | 当前行为 | 边界 |
| --- | --- | --- |
| `ces` | 默认推荐，按 CES 指定资源组发现 | 显式组未命中失败；默认候选名未命中时可走全部资源组并集 |
| `hybrid` | 先 CES 入库，再按类型调用原生 API 增强 labels | 增强失败不回滚基础入库，但按失败类型置 partial；已知增强缺口见 backlog |
| `native` | 旧 ECS/CCE/RDS/ELB 路径 | 不作为 CES 数量完整性口径 |
| fake | 非生产 `auth_type=none` | 只用于测试，不能证明真实云行为 |

`POST /sync` 立即返回 running，后台 goroutine 执行，客户端轮询批次详情到 success/partial/failed。running 时 summary/message 尚未生成。

当前生命周期：

- 同账号只有一个 running 批次，重复触发返回 409 `ALREADY_EXISTS`。
- 批次使用 5 分钟租约、60 秒续租、fencing token 和 30 分钟硬超时。
- 进程退出时等待在途任务，终态使用独立短 context 写入。
- 过期 running 批次会在**下一次同账号 TriggerSync** 时被 reap；当前代码没有装配文档所述的周期后台 reaper，无人再次触发时不保证自动自愈。
- stale 只标记、不物理删除；权威 product scope、聚合/dimension scope 和失败门控按契约分别处理。

### 5.2 结构化摘要

终态 summary 包含计数、失败类型、增强结果、资源组覆盖和 `scopes[]`。多区域排查必须读 `scopes[]`，顶层 regions/projects/resource_group 只是聚合展示。

三条计数恒等式：

```text
raw_fetched_count       = mapped_count + invalid_resource_count
mapped_count            = unique_discovered_count + duplicate_count
unique_discovered_count = persisted_count + persist_failed_count
```

批次 DTO 的 `completed_count = created_count + updated_count`；summary 的 `completed_count = persisted_count`。`enrichment_failed_count` 始终等于去重后的 `enrichment_failed_types` 长度；warnings 不驱动 partial，逐类型增强失败、阶段错误和回写失败驱动 partial。

### 5.3 数据范围与审计的当前差距

- `SyncService` 当前只经过路由 `app:assets:write` 和账号 enabled/assets capability 校验，没有按契约调用 AuthorizationPort 对 owner_team/regions 做二次授权。
- 批次列表当前只按 account_id 和分页查询，没有按 owner_team 过滤；批次详情也没有契约所述的完整数据范围检查。
- AI 资源读取/发现的独立数据范围已落地，不能用该事实证明 Sync 和批次查询也已隔离。
- 当前终态和 reap 审计统一使用 `resource_type=asset_sync_batch`、`action=sync`，不是 `sync_started/sync_finished/sync_reaped` 三个 action。
- 当前没有创建 running 批次后的独立即时审计；终态审计包含 status、基础计数和部分同步摘要，不包含契约所述完整嵌套 summary/scopes。
- migration 0033 虽有 `triggered_by` 列，但 TriggerSync 当前未写入该字段；内联 reap 审计 actor 使用当次请求用户，不是原触发人。

这些差距需要单独补实现与验收，不能只修改文档宣称已闭环。

## 6. CMDB 拓扑与资源解析

### 6.1 拓扑来源

| 边来源 | origin | 生成方式 |
| --- | --- | --- |
| 应用归属 | `registry` | 根据 asset_resource.application_id 实时生成 contains，不重复落库 |
| 手工关系 | `manual` | 持久化 asset_resource_relation，支持跨应用 |
| 云派生关系 | `cloud_sync` | 查询时根据已验证 labels 派生，不按名称/IP 猜边 |

手工关系支持 depends_on、attached_to、located_in、routes_to；两端必须存在且不同，三元组唯一，跨应用关系会出现在两端应用的关系列表中。

云派生边只在账号、Region、类型和云 ID 唯一匹配时生成。无法解析的端点不创建幽灵节点，计入 `meta.unresolved_relation_count`。

### 6.2 查询与展示

- 应用查询：`application_id`，展示应用及资源归属。
- 根资源查询：`root_resource_id + depth=1..3 + direction=both/upstream/downstream`，支持跨应用链路和循环保护。
- 支持 Region、云类型、关键字、关系类型和 stale 筛选；默认不含 stale。
- 最多 299 个资源节点，超出 `meta.truncated=true`，不能宣称是完整拓扑。
- 前端提供链路、全景和列表视图；Observability 的“调用拓扑”是另一领域模型。

### 6.3 云资源解析

`resources/resolve` 要求 `cloud_resource_id + account_id`，Region 和 cloud_resource_type 可选但精确匹配：

- 一个候选：`matched`，返回真实资源和应用。
- 无候选：`unregistered`，只保留原始 ID。
- 多候选：`ambiguous`，返回全部候选，前端不得默认第一项。

普通拓扑和解析路由当前同样只有静态 RBAC。

## 7. 前端实现

- `/assets` 包含注册表、云同步、匹配规则三个 Tab；支持 URL 筛选/排序回显、列设置、深链高亮、CSV 导出和 labels 掩码。
- 云同步支持 running 轮询、批次列表/详情、partial 诊断、scope 失败明细和同账号 409 提示。
- 后端 updated_at 已在排序白名单中，但前端 `SORTABLE_RESOURCE_FIELDS` 当前未开放 updated_at；该列可展示和导出，created_at 可排序。
- `/assets/topology` 提供链路/全景/列表、根资源切换、跨应用手工关系维护、截断和未解析提示。
- 资产页会写入 AI 页面上下文（应用与可选资源）；只传业务 ID，不传凭据。

## 8. 数据与迁移

| 版本 | 作用 |
| --- | --- |
| `0008` | asset_application / asset_resource 基表 |
| `0014` | asset_match_rule |
| `0023` | 云同步字段与 asset_sync_batch |
| `0024–0031` | extra_config、labels、region 唯一键、批次 message/running 互斥/fencing/summary 等 |
| `0032–0043` | 云应用 ID/名称与跨上下文引用完整性收敛 |
| `0045` | 已删除 Inspection Policy scope 清理与引用视图修正 |
| `0058` | asset_resource_relation 手工关系 |

`v_asset_app_ref_integrity` 用于检查应用孤儿引用。迁移文件的历史目标与当前运行时行为需分别核对，尤其是云应用命名和 0033 触发人归因。

## 9. 验证覆盖

### 单元、集成与前端测试

仓库已有测试覆盖：

- CRUD、默认/规则匹配、筛选排序/facets、删除保护、审计调用。
- 云应用 ID、同步 stale/partial、并发互斥、续租/fencing、取消/硬超时、结构化 summary。
- 拓扑注册表/跨应用/云派生边、截断、循环扩展、资源解析和关系删除保护。
- 前端同步摘要、URL 状态、labels、CSV、拓扑布局和数据加载。

### E2E 的实际范围

- `e2e-asset.ps1`：应用/资源/规则创建、资源筛选排序/facets、告警绑定、应用更新、删除保护及清理。不覆盖云同步、拓扑、资源解析、CSV 或完整 UI。
- `e2e-asset-sync.ps1`：fake 账号、running 批次、终态 success、批次列表、云资源入库、第二次稳定清单不产生 stale 和清理。不覆盖变化清单 stale、真实 CES、hybrid、租约并发或数据范围。
- `e2e-asset-sync-real.ps1`：真实华为账号可选对账，需只读凭据和 CES 权限，不进 CI；本轮未执行。
- 拓扑目前没有独立 E2E 脚本，依赖后端单元/HTTP 测试和前端 Vitest。

本轮仅静态复核，未重新执行测试或脚本。
