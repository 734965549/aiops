# 华为云 CES 同步发布 Runbook

> 本文档覆盖华为云 CES 资源同步的验收、发布策略、升级步骤与回滚流程。稳定的接口与状态机定义见《华为云 CES 同步稳定契约》([../ops/huawei-ces-sync-contract.md](../ops/huawei-ces-sync-contract.md))；架构决策见 [adr-huawei-ces-sync.md](adr-huawei-ces-sync.md)。

## 1. 验收标准

### 1.1 单元测试

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

### 1.2 P0/P1 CES-only 集成验收

准备一个华为云账号：

- 授权 `CES ReadOnlyAccess`。
- 能在 CES 控制台看到目标 CES 资源分组（默认候选名“全部资源”，需预先创建）的资源总数。
- 平台账号配置对应 region/project_id。

验收步骤：

1. 在 CES 控制台记录目标资源分组口径（**不是** CES 总览“全部资源”聚合视图，避免拿错对账口径）：

```text
region                   = cn-south-1
project_id               = <对应 region 的 project_id>
enterprise_project_id    = <资源分组所属企业项目 ID，或 all_granted_eps>
resource_group_id        = <目标资源分组 ID>
resource_group_name      = <目标资源分组名称，如“全部资源”>
resource_group_total     = N
各资源类型数量            = map[type]count
```

2. 触发平台同步：

```http
POST /api/assets/sync
```

真实账号验收可使用 `scripts/e2e-asset-sync-real.ps1` 触发同步和打印对账摘要；脚本默认 `SyncTimeoutMs=2100000`（35 分钟），必须大于后端异步同步 30 分钟硬超时，避免脚本先进入清理而后台任务仍在写入。

3. 检查 batch：

```text
status = success 或 partial
summary 顶层（跨 scope 聚合）展示：
  - raw_fetched_count（进入映射流水线的原始行数，不含 max_resources 截断丢弃行）
  - mapped_count（进入映射并保留的资源数）
  - unique_discovered_count（最终进入待写入集合的资源数）
  - persisted_count（本批实际成功写入的资源数）
  - duplicate_count（按唯一键去重折损数）
  - persist_failed_count（映射成功但批量写入失败的资源数）
  - invalid_resource_count（无法转换为资产的非法资源数）
  - failed_scopes / query_failed_types / conversion_failed_types
  - enrichment_failed_types
summary.scopes[] 明细（每个 region/project/resource_group 组合）展示：
  - successful_types
  - resource_group_selection
```

`resource_group_selection` 不在顶层 `summary`，而在 `summary.scopes[]` 每项中，取值为 `specified_id` / `specified_name` / `default_name`。顶层 `resource_group_name`/`resource_group_id`/`regions`/`projects` 仅为兼容聚合字段，多区域排查必须读 `summary.scopes[]`（见契约 §15.3）。`partial` 只是部分成功，不能在前端或验收文案中直接等同失败。最小 JSON 示例：

```json
{
  "summary": {
    "sync_mode": "ces",
    "regions": ["cn-south-1"],
    "ces_total": 12,
    "raw_fetched_count": 12,
    "mapped_count": 12,
    "persisted_count": 12,
    "scopes": [
      {
        "region": "cn-south-1",
        "project_id": "0abc123",
        "sync_mode": "ces",
        "resource_group_id": "rg-001",
        "resource_group_name": "全部资源",
        "resource_group_selection": "specified_id",
        "ces_total": 12,
        "raw_fetched_count": 12,
        "mapped_count": 12,
        "persisted_count": 12
      }
    ]
  }
}
```

建议使用结构化对账，而不是单独用 `failed_count` 推导总量：

```text
raw_fetched_count        = mapped_count + invalid_resource_count
mapped_count             = unique_discovered_count + duplicate_count
unique_discovered_count  = persisted_count + persist_failed_count
```

> 权威定义见《华为云 CES 同步稳定契约》[§9.5](../ops/huawei-ces-sync-contract.md)（含 `mapped = unique + duplicate` 全部三条恒等式与字段语义）。本节公式与之等价，若有差异以契约为准。

其中：

- `persisted_count` 只统计本次真正写入 `asset_resource.sync_status=active` 的行；
- `invalid_resource_count` 统计不满足最小字段/格式的非法资源；
- `duplicate_count` 统计同一批次内按唯一键去重折损的资源；
- `persist_failed_count` 统计映射成功但批量落库失败的资源；
- `product_names_empty=true` 时，应明确标出是依赖白名单兜底而非按资源组/产品名过滤；
- `summary.scopes[].resource_group_selection` 需要说明实际采用的是 `specified_id`（按 resource_group_id）、`specified_name`（按显式 resource_group_name）还是 `default_name`（回落默认候选名）。

4. 检查数据库或资源列表：

```text
source = cloud_sync
integration_account_id = acc_xxx
sync_status = active
region = 目标 region
```

5. 对比类型分布：

```text
CES SYS.ECS 数量 == platform cloud_resource_type=ecs 数量
CES SYS.EVS 数量 == platform cloud_resource_type=evs 数量
CES SYS.VPC 拆分后子类型数量 == platform cloud_resource_type=eip/bandwidth/subnet/peering 数量
```

如果 `sync_mode=ces` 下目标 CES 资源分组 total（即上面记录的 `resource_group_total`）与平台 active 数量不一致，batch summary 必须能解释差异，例如：非法资源、unknown namespace、去重折损、max_resources reached、selected resource group mismatch、failed scopes。

### 1.3 P2 hybrid 增强验收

准备一个额外授予 ECS/RDS/ELB/EVS/VPC 等只读权限的账号：

1. 设置 `sync_mode=hybrid`。
2. 触发同步。
3. 验证基础 active 资源数仍与目标 CES 资源分组 total 一致。
4. 验证已授权类型出现增强字段，例如 IP、VPC、规格、磁盘关系。
5. 移除某个原生 API 权限后再次同步，基础资源数不下降，batch message 记录对应 enrichment warning。

### 1.4 native 兼容验收

1. 设置 `sync_mode=native`。
2. 验证仍能沿用旧 ECS/CCE/RDS/ELB 同步路径。
3. 验证文案和 batch message 不承诺与 CES 控制台数量一致。

### 1.5 回归验收

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

## 2. 发布策略

推荐灰度：

1. 先补 `sync_mode` 配置，但保留 legacy native 代码路径。
2. 新账号默认 `sync_mode=ces`；已有账号可灰度切换到 `ces`。
3. 对比目标 CES 资源分组 total 与平台 active 资源数（口径以 region/project_id/enterprise_project_id/resource_group_id 为准）。
4. 确认稳定后，将所有 `huawei_cloud + ak_sk` 账号默认同步模式切到 `ces`。
5. 对需要详情的账号启用 `hybrid`。
6. 保留 `native` 配置作为兼容回退，但不作为推荐模式。

## 3. 回滚 / 回退方式

- 配置 `sync_mode=native` 回到 ECS/CCE/RDS/ELB 旧路径。
- 已同步的 CES 资源不要物理删除，下一次 native sync 只对 native 成功 scope 标记 stale。

## 4. 风险与注意事项

- CES 数量口径随 region、project_id、企业项目、资源分组变化，易被误读成“CES 总览全部资源”；验收必须按“目标 CES 资源分组 total”对账，并记录 region/project_id/enterprise_project_id/resource_group_id/resource_group_name，避免排障时拿错总览口径。
- `product_names` 为空时，全量发现依赖内置 namespace 白名单，可能漏资源；需要在 batch message 中暴露。
- CES 资源维度可能不是稳定的云资源 ID，映射时必须保留 namespace 和 dim_name，避免跨类型冲突。
- `asset_resource` 唯一约束已由迁移 `0026` 补齐为含 `region` 的部分唯一索引，可区分多区域同类型同云 ID 资源。
- `asset_sync_batch` 已由迁移 `0030` 增加 `fencing_token`；续租必须按 `batch_id + fencing_token` 校验所有权，upsert/stale 写入前也必须确认仍持有 running 且未过期租约，避免旧任务在被 reap 后继续写入。
- `asset_sync_batch` 由迁移 `0033` 增加 `triggered_by`（触发用户 user_id）；崩溃批次被 reap 时 `sync_reaped` 审计 actor 取该字段，避免归因到当次请求用户。
- `/api/assets/sync` 已改为异步任务：立即返回 running batch，后台 goroutine 执行同步并续租；后续如需支撑更大账号，可评估 worker/队列化。
- CES API rate limit 需要退避重试；重试仍失败时标记 partial，不要阻塞其他 namespace。
- 审计必须始终是业务旁路：`sync_started` / `sync_finished` / `sync_reaped` 写失败只能 `warn`，不得回滚或反向影响 batch 终态；batch 状态更新失败才按 `error` 处理并作为业务失败收口。
- 涉及 `0026`、`0028`、`0030` 等数据库迁移时，执行方式以 `ops/migration-contract.md` 为准：生产 / 预发必须通过自研 runner（`cmd/migrate` 或 `go run ./cmd/migrate -config <path>`）显式执行，禁止手工 `psql`、手工执行 `manual_schema_migrations.sql` 或手工写入 `schema_migrations`。

### 4.1 同账号并发同步互斥

同账号并发同步互斥已解决（迁移 `0028`/`0030`/`0033`，不依赖 Redis）。互斥机制、租约续租、fencing token 校验、崩溃批次自愈与审计归因的稳定契约定义见《华为云 CES 同步稳定契约》§18.1。发布时需确认：

- `asset_sync_batch` 表已含 `lease_expires_at` / `fencing_token` / `triggered_by` 列。
- 部分唯一索引 `(integration_account_id) WHERE status='running'` 已建。
- 后台任务续租间隔 60 秒，租约有效期 5 分钟，硬超时 30 分钟。

## 5. 最终完成定义

满足以下条件才算实现完成：

- `sync_mode=ces` 是默认推荐模式。
- 使用仅 CES 只读权限的账号，可以同步指定 CES 资源分组（默认候选名“全部资源”，需预先创建）下可见的资源。
- 平台 active 资产数与指定资源分组总数一致，或 batch message 能解释所有差异。
- EVS/VPC/OBS/DCS/DMS/RDS/ELB/ECS 等截图中出现的类型能进入 `asset_resource`。
- `sync_mode=hybrid` 可以在指定资源分组发现基础上按权限补充已支持类型详情；EVS 因 CES 维度与原生资源匹配键不成立，详情增强尚未支持；增强失败不影响基础资源入库。
- Runbook 验收不再要求单独验证“磁盘关系”；该项属于原生详情增强能力，EVS 当前未支持。
- `sync_mode=native` 仅作为旧路径兼容，不作为完整性验收口径。
- 同步批次有审计、有失败摘要、有 stale 语义。
- 真实账号不会返回 fake 数据。
- P0 告警接入、资产匹配、Runbook 推荐、Execution 闭环不受影响。

## 6. 云同步应用 ID 与名称变更升级说明（迁移 0032 -> 0035 -> 0036 -> 0037 -> 0038 -> 0039 -> 0040 -> 0041 -> 0042）

> 适用场景：2026-06 之后使用新 `cloudApplicationID` 算法的版本。九条迁移共同收敛云同步应用 ID/名称，**必须按版本号顺序执行**：`0032` 破坏性删除旧格式 -> `0035` 修复字节/rune 截断 -> `0036` 修复应用名歧义 -> `0037` 修复合并安全性 -> `0038` 归一化反向格式应用名 -> `0039` 补全 0032 遗留的孤儿引用 -> `0040` 创建引用完整性诊断视图 -> `0041` legacy 应用收敛硬阻断守卫 -> `0042` 补建孤儿引用并将视图作为硬验收。其中 `0032` 为破坏性 DELETE（开发/未上线阶段"清空重来"策略），其余八条为无损改写/幂等更新/纯校验。生产/预发/开发统一适用。已跑过初版（启发式删除版）`0032` 的 dev/CI 库必须重建。

### 6.0 迁移链总览

| 顺序 | 迁移 | 职责 | 触发条件 | 依赖 |
| --- | --- | --- | --- | --- |
| 1 | `0032_cleanup_legacy_cloud_application_ids` | 破坏性 DELETE：按 `application_id = 'cloud-' \|\| trim(account_id)` 精确关联 `integration_account` 删除旧格式 `cloud-<account_id>` 应用及其关联 `asset_resource`/`asset_match_rule`（不处理 `alert_alert`/`inspection_policy`，由 `0039` 补全）；保留 `integration_account`，升级后需重新触发云同步 | 旧格式应用存在 | 无 |
| 2 | `0035_cloud_application_id_rune_truncation` | 把旧实现按**字节**截取的多字节账号 application_id 改写为按 **rune**（字符）截取的版本，合并子表引用 | 字节版 ≠ rune 版的应用存在 | pgcrypto |
| 3 | `0036_cloud_application_name_include_account` | 云同步应用名 `<provider>-cloud` 追加为 `<provider>-cloud-<account_id>`，避免多账号同名 | `name NOT LIKE '%-cloud-%'` 的 cloud 应用存在 | 无 |
| 4 | `0037_fix_huawei_ces_application_ids` | 修复 legacy/new 并存时的合并安全性：先迁移去重子表引用再删旧应用，仅 legacy 时安全重命名 | 仍存在 `cloud-<account_id>` 格式 legacy 应用 | pgcrypto |
| 5 | `0038_cloud_application_name_normalize` | 把反向格式应用名 `<provider>-<account_id>-cloud` 归一化为契约格式 `<provider>-cloud-<account_id>`，收敛 0036 之后由反向格式代码新建的应用 | `name = split_part(name,'-',1) || '-' || regexp_replace(description,'^Auto-created cloud sync application for account ','') || '-cloud'` 的 cloud 应用存在（从 description 提取 account_id 精确比对反向格式，不使用 LIKE 模糊排除） | 无 |
| 6 | `0039_cleanup_orphaned_application_refs` | 补全 0032 遗留的孤儿引用：将 `alert_alert.application_id` 和 `inspection_policy.scope.application_ids` 中仍残留的旧格式 `cloud-<account_id>` 改写为新格式 `cloud-<前缀>-<hash>` | 旧格式引用存在 | pgcrypto |
| 7 | `0040_application_ref_integrity_view` | 创建持久视图 `v_asset_app_ref_integrity`，暴露 `asset_resource`/`asset_match_rule`/`alert_alert`/`inspection_policy` 中指向不存在 `asset_application` 的孤儿引用，供运维和发布流水线查询验证 | 无（幂等 `CREATE OR REPLACE VIEW`） | 无 |
| 8 | `0041_legacy_app_id_convergence_guard` | 硬阻断守卫：若 `asset_application` 中仍存在 `cloud-<account_id>` 格式 legacy 应用，迁移失败终止 | legacy 应用存在 | 无 |
| 9 | `0042_backfill_orphaned_app_refs_and_guard` | 补建 0039 改写后仍被引用但不存在的新格式 cloud application ID 对应的 `asset_application` 记录（字段与 `ensureCloudApplication` 一致），并将 `v_asset_app_ref_integrity` 作为硬验收（`CHECK(n=0)`），补建后仍有孤儿则迁移失败 | 仍被引用但不存在的新格式 cloud 应用 | pgcrypto |

**关键依赖关系**：
- `0035` 的 rune 版目标 ID 与新格式一致（都按 `left(trim(account_id),17)` 截取），`0032` 先执行删除纯 legacy 格式应用后，重新同步将按新格式创建应用，`0035` 只需处理旧代码按字节截取创建的残留。
- `0037` 是 `0032` 之后对残留 legacy 应用的最终收敛与安全性补强；`0032` 从未在共享环境执行，当前精确匹配版本为开发期最终版，所有数据库须从零重建；`0037` 修复其合并路径中的安全性问题。
- `0036` 改 `name` 不改 `application_id`，与前三条互不冲突，但建议在 ID 收敛后执行，避免运维排查时名称与 ID 语义错位。
- `0038` 同样只改 `name` 不改 `application_id`，收敛 `ensureCloudApplication` 代码曾误用的反向格式 `<provider>-<account_id>-cloud`；它处理的是 `0036` 之后由代码新建的应用（`0036` 只归一化迁移前已存在的应用），因此必须排在 `0036` 之后；与 ID 类迁移（`0032`/`0035`/`0037`）互不冲突，建议放最后统一收敛名称。
- `0039` 补全 `0032` 破坏性 DELETE 遗留的 `alert_alert`/`inspection_policy` 孤儿引用；`0037` 的合并路径依赖旧应用仍在 `asset_application` 中，若 `0032` 已删除旧应用则 `0037` 跳过这两个表，`0039` 作为最终兜底改写。
- `0040` 创建引用完整性诊断视图 `v_asset_app_ref_integrity`，不修改数据，不阻断迁移；**重新同步后**查询该视图期望 0 行，若返回非 0 行说明存在孤儿引用（可能是 `0039` 改写后新格式应用尚未由同步创建）。
- `0041` 是 `0032`/`0037` 收敛后的硬阻断守卫：若 `asset_application` 中仍存在 `cloud-<account_id>` 格式 legacy 应用，迁移失败终止，runner 不记录版本号；排查修复后重试。`0040` 和 `0041` 不依赖 pgcrypto。
- `0042` 是 `0039`/`0040` 的收口迁移：`0039` 只改写 `alert_alert`/`inspection_policy` 中的旧格式引用为新格式，但不创建对应 `asset_application` 记录；若重新同步尚未执行，新格式应用不存在，引用仍为孤儿。`0042` 为这些"被引用但不存在"的新格式 cloud 应用补建 `asset_application` 记录（字段与 `ensureCloudApplication` 一致），随后将 `v_asset_app_ref_integrity` 作为硬验收：补建后视图仍返回非 0 行则 `CHECK(n=0)` 失败、迁移终止。补建幂等（`ON CONFLICT DO NOTHING`），依赖 pgcrypto `digest()`（与 `0039` 共用）。`0041` 校验 legacy 应用已收敛，`0042` 校验新格式应用引用完整，二者职责互补。
- 纯 ASCII 账号：字节版 = rune 版，`0035` 幂等无改写；`0032` 为破坏性 DELETE，不涉及字节/rune 截取。
- 其中 `0035`/`0037`/`0039`/`0042` 依赖 pgcrypto（`0032`/`0036`/`0038`/`0040`/`0041` 不依赖），生产由 DBA 确认 `CREATE EXTENSION IF NOT EXISTS pgcrypto` 可用。

### 6.1 迁移 0032：旧格式 application_id 收敛

#### 6.1.1 变更内容

`internal/asset/application/sync_service.go` 中 `cloudApplicationID` 从旧格式：

```text
cloud-<完整账号ID>
```

改为新格式：

```text
cloud-<账号前17位>-<sha1(账号)前12位>
```

例如账号 `1234567890123456789012345678` 生成 `cloud-12345678901234567-a1b2c3d4e5f6`。

变更原因：
- 原格式依赖账号长度，`application_id` 可能超过 `asset_application.application_id VARCHAR(36)` 限制；
- 多个账号前 28 位相同时会生成冲突的 ID；
- 新格式固定 ≤ 36 位并带哈希后缀，保证唯一性和可读性。

#### 6.1.2 升级影响

升级后若保留旧格式云同步应用，`ensureCloudApplication` 会按新 ID 创建新应用，旧资源继续挂在旧应用，新资源进入新应用，导致：
- 同一账号资产被拆散到两个应用；
- 匹配规则、Dashboard、告警匹配、Execution 关联应用 ID 全部失效；
- P0 资产匹配闭环被破坏。

#### 6.1.3 迁移策略

`0032_cleanup_legacy_cloud_application_ids`（已落地，破坏性 DELETE，开发/未上线阶段"清空重来"策略）：
- 按 `application_id = 'cloud-' || trim(integration_account.account_id)` 精确关联识别旧格式应用，删除 `asset_application`/`asset_resource`/`asset_match_rule` 中的旧格式数据；
- 覆盖 `account_id` 不含 `-`（如华为云纯数字账号）与含 `-`（如 `acc-<uuid>`）的所有情况，统一破坏性删除；
- 不处理 `alert_alert`/`inspection_policy` 中的旧格式引用（由 `0039` 补全清理）；
- 新格式应用 `cloud-<前缀>-<hash>` 比旧格式多一个 `-<hash>` 后缀，精确匹配不会误删；
- 保留 `integration_account`，升级后需重新触发云同步。

历史说明：`0032` 初版采用"连字符数量"启发式（`NOT LIKE 'cloud-%-%'`）识别旧格式，当 `account_id` 含连字符时旧格式 `cloud-<account_id>` 也含多个 `-`，导致启发式漏判、跳过删除，随后 `0037` 走无损迁移路径，与无连字符账号走 DELETE 路径产生两种完全不同的数据处理结果。因 `0032` 从未进入任何共享环境（含开发库），已直接修订为精确匹配 DELETE，统一所有账号的删除行为。修订后的 `0032` 为开发期最终版，所有数据库须从零重建，不得基于曾执行过旧版 `0032` 的数据库升级。

升级后需要：
1. 对已有接入账号重新触发云同步（迁移保留 `integration_account`，无需重新录入）；
2. 验证匹配规则、Dashboard、告警关联的应用 ID 已指向新格式应用（`alert_alert`/`inspection_policy` 中的旧格式引用由 `0039` 改写为新格式）。

### 6.2 迁移 0035：application_id 字节/rune 截断修复

#### 6.2.1 变更内容

`cloudApplicationID` 旧实现按**字节**截取 `account_id` 前 17 字节（Go `id[:17]`），而 `0032` 与新格式契约使用 PostgreSQL `left(...,17)` 按**字符**截取。对含多字节字符（中文等）且字节长度 > 17 的账号：

- 旧字节版可能截断 UTF-8 字符中部 → 非法 UTF-8，UTF8 编码库会拒绝写入（此类行不存在）；
- 当 17 字节边界恰好落在字符边界时，字节版产出有效但字符数 < 17 的前缀，可写入但与 `left()` 的 17 字符前缀不一致；
- 两种版本共享同一 sha1 后缀，但 application_id 不同 → 同账号云资源被拆散到两个应用，破坏资产匹配闭环。

修复后 Go 端改按 rune 截取（见 `internal/asset/application/sync_service.go:cloudApplicationID`），`0035` 把存量字节版 application_id 无损改写为 rune 版。纯 ASCII 账号字节版 = rune 版，无改写。

#### 6.2.2 迁移策略（无损精确改写）

`0035_cloud_application_id_rune_truncation`（已落地，生产/预发/开发统一适用）：
- 按 application_id 的 sha1 后缀（恒为 `'-' || substr(sha1hex(trim(account_id)),1,12)`，字节版与 rune 版一致）关联 `integration_account` 识别受影响应用；
- 对每个账号计算 rune 版新 ID（`'cloud-' || left(trim(account_id),17) || '-' || 后缀`）；
- **改写分支**（旧字节版存在且 rune 版新应用不存在）：改写 `asset_application`/`asset_resource`/`asset_match_rule`/`alert_alert`/`inspection_policy.scope.application_ids` 引用；
- **合并分支**（旧字节版存在且 rune 版新应用已存在）：按云资源唯一键合并资源/规则/告警引用到新应用，再删除空的旧应用；
- `asset_resource` 云资源唯一键 `(integration_account_id, cloud_resource_type, cloud_resource_id, region)` 不含 `application_id`；`asset_match_rule` 唯一约束仅在 `rule_id`；二者改写不会触发唯一冲突；
- 依赖 pgcrypto `digest()`。

#### 6.2.3 风险

- 仅影响 `cloud-` 前缀且 sha1 后缀匹配的应用；纯 ASCII 账号无改写，幂等。
- 合并分支会删除空的旧应用行，执行前必须备份 `asset_application`。
- 改写 `inspection_policy.scope.application_ids` 是 JSONB 数组逐元素替换，执行前确认巡检策略数据完整。

#### 6.2.4 验证 SQL

```sql
-- 0035 后：不应再存在"后缀匹配某账号 sha1 但前缀 ≠ left(account_id,17) rune 版"的 cloud- 应用。
-- 期望 0 行。
SELECT aa.application_id
FROM asset_application aa
JOIN integration_account ia
  ON right(aa.application_id, 13) = '-' || substr(encode(digest(trim(ia.account_id), 'sha1'), 'hex'), 1, 12)
WHERE aa.application_id LIKE 'cloud-%'
  AND aa.application_id <> 'cloud-' || left(trim(ia.account_id), 17) || '-' || substr(encode(digest(trim(ia.account_id), 'sha1'), 'hex'), 1, 12)
  AND aa.application_id <> 'cloud-' || trim(ia.account_id);
```

```sql
-- 0035 后：inspection_policy.scope.application_ids 数组不应含重复元素
-- （数组长度应等于去重后长度，防止旧字节版与新 rune 版替换后残留重复 ID）。期望 0 行。
SELECT ip.policy_id
FROM inspection_policy ip
WHERE jsonb_typeof(ip.scope->'application_ids') = 'array'
  AND jsonb_array_length(ip.scope->'application_ids')
      <> (SELECT count(DISTINCT e)
          FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(e));
```

#### 6.2.5 回滚注意事项

`0035_cloud_application_id_rune_truncation.down.sql` 仅作人工回滚参考，不由 runner 自动执行。回滚难点：
- 改写分支理论上可按同公式反推（rune 版 → 字节版），但字节版前缀取决于账号的 UTF-8 字节布局，且 rune 版应用也可能由修复后的新同步创建（并非 0035 改写而来），反改写会把这类应用一并改回字节版；
- 因此反改写必须限定在 0035 实际改写过的行——仅凭 SQL 无法区分，需对照升级前备份核对；
- 合并分支（旧应用资源/规则已改写指向新应用、旧应用行已删除）同样需按备份反改写 application_id 并重建旧 `asset_application` 行；
- 生产回滚应由 DBA 按审批流程显式执行。

### 6.3 迁移 0036：云同步应用名包含账号

#### 6.3.1 变更内容

`ensureCloudApplication` 之前用 `Name = "<provider>-cloud"`，所有同 provider 账号共用同一名称。`FindByNameEnv(name, env)` 取第一条，多账号场景告警默认匹配存在歧义。修复后 `Name` 改为 `<provider>-cloud-<account_id>`，保证每个账号的应用名称唯一。

`0036_cloud_application_name_include_account`（已落地）从 `description` 提取 `account_id`，追加到现有 `name`。幂等：`name NOT LIKE '%-cloud-%'` 确保不重复追加。

#### 6.3.2 风险

- 仅改 `asset_application.name`，不改 `application_id`，不影响匹配键与引用关系。
- 仅处理 `application_id LIKE 'cloud-%' AND environment='cloud'` 且 `description` 符合 `Auto-created cloud sync application for account %` 模式的行。
- 已追加过的行（`name LIKE '%-cloud-%'`）跳过，幂等。

#### 6.3.3 验证 SQL

```sql
-- 0036 后：cloud 同步应用不应再有未追加 account_id 的旧名称。
-- 期望 0 行。
SELECT application_id, name
FROM asset_application
WHERE application_id LIKE 'cloud-%'
  AND environment = 'cloud'
  AND description LIKE 'Auto-created cloud sync application for account %'
  AND name NOT LIKE '%-cloud-%';
```

#### 6.3.4 回滚注意事项

`0036` 的 down 可直接还原：provider 不含连字符（如 `huawei_cloud`、`aliyun_cloud`），取 `name` 前两段 `split_part` 即可还原为 `<provider>-cloud`：

```sql
-- 0036 回滚参考（人工执行，不由 runner 自动跑）：
UPDATE asset_application
SET name    = split_part(name, '-', 1) || '-' || split_part(name, '-', 2),
    updated_at = NOW()
WHERE application_id LIKE 'cloud-%'
  AND environment = 'cloud'
  AND name LIKE '%-cloud-%';
```

### 6.4 迁移 0037：application_id 安全合并修复

#### 6.4.1 变更内容

`0032` 从未在共享环境执行，当前为开发期最终版，所有数据库须从零重建；`0037` 修复历史数据迁移路径中的安全性问题。当 legacy `cloud-<account_id>` 与 new `cloud-<prefix>-<hash>` 应用并存时，`0037` 不再直接改写业务 ID，而是先把旧应用引用迁移到新应用、去重子表引用，再删除旧应用。覆盖四种状态：

1. **only legacy app**：把旧应用及其所有引用安全重命名为 new；
2. **only new app**：保持幂等，无操作；
3. **legacy + new 同时存在**：先迁移引用到 new，再删除 old；
4. **子表引用重复去重**：`asset_resource`/`asset_match_rule`/`alert_alert`/`inspection_policy.scope.application_ids` 在合并后去重。

#### 6.4.2 与 0032 的关系

`0032` 完成纯 legacy 收敛后，理论上不应再有 `cloud-<account_id>` 格式应用。`0037` 处理 `0032` 之后仍可能残留的 legacy 应用（例如 `0032` 执行时账号尚未接入、之后又出现 legacy 格式数据），并修复并存合并路径的安全性：`0032` 在并存时直接改写业务 ID，`0037` 改为先迁移去重子表引用再删旧应用，避免引用孤立。依赖 pgcrypto `digest()`。

#### 6.4.3 风险

- 合并分支会删除 legacy 应用行，执行前必须备份 `asset_application` 及子表。
- `inspection_policy.scope.application_ids` 合并后用 `DISTINCT` 去重并 `ORDER BY` 排序，执行前确认巡检策略 JSONB 数据完整。
- 仅 legacy 分支会重命名 `asset_application.application_id`，确认该账号没有并发同步在写入旧 ID。

#### 6.4.4 验证 SQL

```sql
-- 0037 后：不应再存在 cloud-<完整account_id> 格式的 legacy 应用。
-- 期望 0 行。
SELECT aa.application_id
FROM asset_application aa
JOIN integration_account ia
  ON aa.application_id = 'cloud-' || trim(ia.account_id)
WHERE aa.application_id LIKE 'cloud-%';
```

```sql
-- 0037 后：legacy 与 new 不应同时存在同一账号。
-- 期望 0 行。
SELECT s.account_id, s.old_app_id, s.new_app_id
FROM (
  SELECT ia.account_id,
         'cloud-' || trim(ia.account_id) AS old_app_id,
         'cloud-' || left(trim(ia.account_id), 17) || '-' || substr(encode(digest(trim(ia.account_id), 'sha1'), 'hex'), 1, 12) AS new_app_id
  FROM integration_account ia
) s
WHERE EXISTS (SELECT 1 FROM asset_application aa WHERE aa.application_id = s.old_app_id)
  AND EXISTS (SELECT 1 FROM asset_application aa WHERE aa.application_id = s.new_app_id);
```

#### 6.4.5 回滚注意事项

`0037_fix_huawei_ces_application_ids.down.sql` 仅作人工回滚参考，不由 runner 自动执行。与 `0032`/`0035` 类似，回滚需要对照升级前备份判断哪些 application_id 是本迁移改写而来。对于 new 与 legacy 并存被合并删除旧应用的场景，必须先恢复旧应用及引用，再回写业务数据。生产回滚应由 DBA 按审批流程显式执行。

### 6.5 迁移 0038：云同步应用名归一化（修复反向格式）

#### 6.5.1 变更内容

`ensureCloudApplication` 代码曾误用 `Name = fmt.Sprintf("%s-%s-cloud", provider, accountID)`，新建云同步应用名为反向格式 `<provider>-<account_id>-cloud`，与 `0036` 契约约定的 `<provider>-cloud-<account_id>` 不一致。`0036` 只归一化了“迁移前已存在”的应用；此 bug 代码上线后新建的应用均为反向格式，导致存量迁移后的应用名与新建应用名分裂。

`0038_cloud_application_name_normalize`（已落地）把反向格式 `<provider>-<account_id>-cloud` 改写为契约格式 `<provider>-cloud-<account_id>`，`account_id` 从 `description` 提取（沿用 `0036` 思路），保证 account_id 含连字符也正确。仅改 `asset_application.name`，不改 `application_id`，不影响匹配键与引用关系。

#### 6.5.2 风险

- 仅改 `asset_application.name`，不改 `application_id`，不影响 `asset_resource`/`asset_match_rule`/`alert_alert`/`inspection_policy.scope.application_ids` 等引用关系。
- 仅处理 `application_id LIKE 'cloud-%' AND environment='cloud'` 且 `description` 符合 `Auto-created cloud sync application for account %` 模式的行。
- 采用精确比对，从 `description` 提取 `account_id`（去掉前缀 `Auto-created cloud sync application for account `），与 `split_part(name,'-',1)` 拼成反向格式 `provider || '-' || account_id || '-cloud'`，仅当 `name` 精确等于该值时命中。
- 不使用 `LIKE '%-cloud' / NOT LIKE '%-cloud-%'` 模糊排除：当 `account_id` 含 `-cloud-` 或以 `-cloud` 结尾时，反向格式 name 同样会命中 `-cloud-` 子串，`NOT LIKE '%-cloud-%'` 会错误排除真实反向格式行；精确比对不受此影响。
- 契约格式 `<provider>-cloud-<account_id>` 与旧格式 `<provider>-cloud` 均不等于反向格式，不会被误改；仅 `account_id='cloud'` 时契约与反向格式相同，重写为同值无副作用。

#### 6.5.3 验证 SQL

```sql
-- 0038 后：cloud 同步应用不应再存在反向格式名称。
-- 期望 0 行。
SELECT application_id, name
FROM asset_application
WHERE application_id LIKE 'cloud-%'
  AND environment = 'cloud'
  AND description LIKE 'Auto-created cloud sync application for account %'
  AND name = split_part(name, '-', 1) || '-' ||
           regexp_replace(description, '^Auto-created cloud sync application for account ', '') ||
           '-cloud';
```

#### 6.5.4 回滚注意事项

`0038_cloud_application_name_normalize.down.sql` 仅作人工回滚参考，不由 runner 自动执行。`0038` 与 `0036` 产出相同的契约格式，仅凭当前数据无法区分哪些行是 `0038` 改写、哪些是 `0036` 改写，因此 down 脚本仅提供“契约格式 → 反向格式”的转换模板，人工回滚时必须对照 `0038` 执行前备份，仅挑选 `0038` 实际改写的行执行，否则会把 `0036` 归一化的行也错误还原为反向格式。生产回滚应由 DBA 按审批流程显式执行。

### 6.6 迁移 0039：补全孤儿引用

#### 6.6.1 变更内容

`0032` 是破坏性 DELETE，删除旧格式 `cloud-<account_id>` 应用及其关联的 `asset_resource`/`asset_match_rule`，但不处理 `alert_alert.application_id` 和 `inspection_policy.scope.application_ids` 中的旧格式引用。`0037` 修复路径依赖 `has_old`（旧应用仍在 `asset_application` 中），若 `0032` 已删除旧应用则 `has_old=false`，`0037` 跳过这两个表，留下孤儿引用。

`0039_cleanup_orphaned_application_refs`（已落地）补全这一缺口：按 `integration_account` 计算 old->new 映射，将 `alert_alert` 和 `inspection_policy` 中仍残留的旧格式 `application_id` 改写为新格式。幂等：已经是新格式或已由 `0037` 处理过的行不受影响。

#### 6.6.2 风险

- 仅改写 `alert_alert.application_id` 和 `inspection_policy.scope.application_ids` 中的旧格式引用，不影响 `asset_application`/`asset_resource`/`asset_match_rule`。
- `inspection_policy.scope.application_ids` 改写后用 `DISTINCT` 去重并 `ORDER BY` 排序，避免重复。
- 依赖 pgcrypto `digest()`。

#### 6.6.3 验证 SQL

```sql
-- 0039 后：alert_alert 不应再存在旧格式 cloud-<完整账号ID> 的 application_id 引用。
-- 期望 0 行。
SELECT al.application_id
FROM alert_alert al
JOIN integration_account ia
  ON al.application_id = 'cloud-' || trim(ia.account_id);
```

```sql
-- 0039 后：inspection_policy.scope.application_ids 不应含旧格式元素。
-- 期望 0 行。
SELECT ip.policy_id
FROM inspection_policy ip
WHERE jsonb_typeof(ip.scope->'application_ids') = 'array'
  AND EXISTS (
    SELECT 1
    FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(elem)
    JOIN integration_account ia ON elem = 'cloud-' || trim(ia.account_id)
  );
```

#### 6.6.4 回滚注意事项

`0039` 为幂等 DML 改写，down 脚本仅作人工回滚参考。回滚需对照升级前备份，将新格式 `application_id` 反向改写为旧格式。生产回滚应由 DBA 按审批流程显式执行。

### 6.7 迁移 0040：引用完整性诊断视图

#### 6.7.1 变更内容

`0032` 破坏性 DELETE 删除旧格式应用后，`0039` 将 `alert_alert`/`inspection_policy` 中旧格式引用改写为新格式 `cloud-<prefix>-<hash>`，但不校验新格式应用是否已在 `asset_application` 中存在。若重新同步尚未执行，新格式应用不存在，引用成为孤儿。

`0040_application_ref_integrity_view`（已落地）创建持久视图 `v_asset_app_ref_integrity`，暴露 `asset_resource`/`asset_match_rule`/`alert_alert`/`inspection_policy` 中指向不存在 `asset_application` 的孤儿引用。视图不修改数据，不阻断迁移执行。

#### 6.7.2 风险

- 仅创建视图，不修改任何业务数据。
- 视图查询在数据量大时可能有性能开销（`NOT EXISTS` 子查询），生产环境按需查询而非持续监控。

#### 6.7.3 验证 SQL

```sql
-- 0040 后：查询引用完整性视图，期望 0 行（重新同步后）。
-- 若返回非 0 行，说明存在孤儿引用，需排查 application_id 来源。
SELECT * FROM v_asset_app_ref_integrity;
```

#### 6.7.4 回滚注意事项

`0040` 的 down 脚本为 `DROP VIEW IF EXISTS v_asset_app_ref_integrity`，可直接回滚。

### 6.8 迁移 0041：legacy 应用收敛硬阻断守卫

#### 6.8.1 变更内容

`0032` 破坏性 DELETE 和 `0037` 安全合并/重命名完成后，`asset_application` 中不应再存在任何 `cloud-<account_id>` 格式的 legacy 应用。若仍存在，说明收敛失败或有代码路径仍在创建旧格式应用。

`0041_legacy_app_id_convergence_guard`（已落地）使用 `CHECK(n = 0)` 约束实现硬阻断：若 legacy 应用数 > 0，`INSERT` 违反约束导致迁移失败，runner 不记录版本号，下次运行会重试。兼容自研 SQL splitter（纯 DML，无 `$$` 美元引用）。

#### 6.8.2 风险

- 不修改任何业务数据，仅做校验。
- 若迁移失败，说明 `0032`/`0037` 收敛路径存在问题，必须排查修复后才能继续升级。

#### 6.8.3 验证 SQL

```sql
-- 0041 守卫逻辑等价查询：若返回 >0 行，0041 迁移会失败。
SELECT count(*)
FROM asset_application aa
JOIN integration_account ia
  ON aa.application_id = 'cloud-' || trim(ia.account_id);
```

#### 6.8.4 回滚注意事项

`0041` 不创建持久化对象（`_0041_guard` 为 `TEMP TABLE`），无需回滚。

### 6.9 迁移 0042：补建孤儿引用 + 引用完整性硬验收

#### 6.9.1 变更内容

`0039` 将 `alert_alert`/`inspection_policy` 中旧格式 `cloud-<account_id>` 引用改写为新格式 `cloud-<前缀>-<hash>`，但不创建对应 `asset_application` 记录；新格式应用仅在云同步（`ensureCloudApplication`）时创建。迁移完成到人工重新同步之间，这些引用成为孤儿，违反"应用层保证引用完整性"约定。`0040` 视图只诊断不修复，`0041` 只阻断 legacy 应用，均不覆盖此场景。

`0042_backfill_orphaned_app_refs_and_guard`（已落地）分两步收口：

1. **补建**：按 `integration_account` 计算新格式 `application_id`（`cloud-` + `left(trim(account_id),17)` + `-` + `substr(sha1(account_id),1,12)`，与 `cloudApplicationID`/`0032`/`0035` 一致），对"仍被 `alert_alert`/`inspection_policy`/`asset_resource`/`asset_match_rule` 引用但不存在于 `asset_application`"的新格式 cloud 应用补建记录。字段与 `ensureCloudApplication`（`internal/asset/application/sync_dto.go`）保持一致：`name = <provider>-cloud-<account_id>`、`environment = 'cloud'`、`namespace = ''`、`description = 'Auto-created cloud sync application for account <account_id>'`，`created_at`/`updated_at` 由迁移写入 `NOW()`（补建路径不走 GORM hook，与种子/回填迁移惯例一致）。
2. **硬验收**：补建后将 `v_asset_app_ref_integrity` 作为硬门控——若视图仍返回非 0 行（存在补建未覆盖的孤儿引用），`CHECK(n = 0)` 约束失败导致迁移终止，runner 不记录版本号。

幂等：补建使用 `INSERT ... ON CONFLICT (application_id) DO NOTHING`，已存在的应用不受影响；兼容自研 SQL splitter（仅普通 DML，无 `$$` 美元引用，语句以分号分隔）；依赖 pgcrypto `digest()`（`0039` 已 `CREATE EXTENSION IF NOT EXISTS`）。

#### 6.9.2 风险

- 补建记录的 `created_at`/`updated_at` 为迁移执行时刻，与真实云同步创建的应用时间戳不同；仅影响排查时的排序展示，不影响业务逻辑。
- 补建只覆盖"被引用但不存在"的新格式 cloud 应用；若某应用既不存在也未被任何表引用，`0042` 不会补建（无必要）。
- 若硬验收失败（视图返回非 0 行），说明存在 `0042` 补建逻辑未覆盖的孤儿来源（如 `integration_account` 已删除但引用残留、或引用格式不在补建映射范围内），必须排查修复后重试，不能跳过。
- 后续真实云同步触发 `ensureCloudApplication` 时，对这些已补建的应用走 `ON CONFLICT DO NOTHING`/upsert 语义，不会冲突。

#### 6.9.3 验证 SQL

```sql
-- 0042 硬验收等价查询：补建后引用完整性视图应返回 0 行。
-- 若返回非 0 行，0042 迁移会失败（CHECK(n=0)）。
SELECT count(*) AS orphan_refs FROM v_asset_app_ref_integrity;
```

```sql
-- 0042 补建应用应已存在：被引用但此前缺失的新格式 cloud 应用现已补建。
-- 列出 0042 可能补建过的应用（name/description 符合补建格式，供人工核对）。
SELECT application_id, name, environment, description, created_at
FROM asset_application
WHERE environment = 'cloud'
  AND description LIKE 'Auto-created cloud sync application for account %'
ORDER BY created_at DESC;
```

#### 6.9.4 回滚注意事项

`0042` 的 down 脚本为人工回滚参考，不由 runner 自动执行。补建的 `asset_application` 记录**不自动删除**：云同步可能已为其关联 `asset_resource`/`asset_match_rule`/`alert_alert`，删除会重新制造孤儿引用。如需回滚，请对照升级前备份核对 `asset_application` 表，仅删除由本迁移补建且确认无关联数据的记录。生产回滚应由 DBA 按审批流程显式执行。

### 6.10 生产/预发统一注意事项

除 `0032` 为破坏性 DELETE（开发/未上线阶段"清空重来"策略，不可自动恢复）外，其余八条（`0035`/`0036`/`0037`/`0038`/`0039`/`0040`/`0041`/`0042`）均为无损精确改写/幂等更新/纯校验，生产/预发升级与开发统一走同一套迁移，无需"单独依赖另一条迁移"：

- 执行前建议全量备份 `asset_application`、`asset_resource`、`asset_match_rule`、`alert_alert`、`inspection_policy`、`integration_account` 相关表（通用变更前备份实践）；
- 依赖 pgcrypto 扩展（`CREATE EXTENSION IF NOT EXISTS pgcrypto`），生产由 DBA 确认扩展可用；
- 在维护窗口执行，执行后重新触发同步验证资源全部写入新格式应用；
- 所有引用 `application_id` 的外部系统（CMDB、告警源、Runbook、Execution 历史）必须同步更新或建立兼容映射。

### 6.11 统一验证清单

升级到 `0042` 后，按顺序执行以下验证 SQL，**均期望 0 行**。可在 psql 中直接运行。

```sql
-- (1) 不应再存在旧格式 cloud-<完整账号ID> 的应用（0032/0037 收敛后）。
-- 期望 0 行。
SELECT aa.application_id
FROM asset_application aa
JOIN integration_account ia
  ON aa.application_id = 'cloud-' || trim(ia.account_id);
```

```sql
-- (2) 不应再存在字节版 ≠ rune 版的 cloud- 应用（0035 修复后）。
-- 期望 0 行。
SELECT aa.application_id
FROM asset_application aa
JOIN integration_account ia
  ON right(aa.application_id, 13) = '-' || substr(encode(digest(trim(ia.account_id), 'sha1'), 'hex'), 1, 12)
WHERE aa.application_id LIKE 'cloud-%'
  AND aa.application_id <> 'cloud-' || left(trim(ia.account_id), 17) || '-' || substr(encode(digest(trim(ia.account_id), 'sha1'), 'hex'), 1, 12)
  AND aa.application_id <> 'cloud-' || trim(ia.account_id);
```

```sql
-- (3) cloud 同步应用名应已追加 account_id（0036 修复后）。
-- 期望 0 行。
SELECT application_id, name
FROM asset_application
WHERE application_id LIKE 'cloud-%'
  AND environment = 'cloud'
  AND description LIKE 'Auto-created cloud sync application for account %'
  AND name NOT LIKE '%-cloud-%';
```

```sql
-- (4) 不应存在孤儿引用：asset_resource.application_id 在 asset_application 中不存在。
-- 期望 0 行。
SELECT ar.application_id
FROM asset_resource ar
LEFT JOIN asset_application aa ON aa.application_id = ar.application_id
WHERE ar.application_id LIKE 'cloud-%' AND aa.application_id IS NULL;
```

```sql
-- (5) 不应存在孤儿引用：asset_match_rule.application_id 在 asset_application 中不存在。
-- 期望 0 行。
SELECT am.application_id
FROM asset_match_rule am
LEFT JOIN asset_application aa ON aa.application_id = am.application_id
WHERE am.application_id LIKE 'cloud-%' AND aa.application_id IS NULL;
```

```sql
-- (6) 不应存在孤儿引用：alert_alert.application_id 在 asset_application 中不存在。
-- 期望 0 行。
SELECT al.application_id
FROM alert_alert al
LEFT JOIN asset_application aa ON aa.application_id = al.application_id
WHERE al.application_id LIKE 'cloud-%' AND aa.application_id IS NULL;
```

```sql
-- (7) inspection_policy.scope.application_ids 数组不应含重复元素
-- （数组长度应等于去重后长度，防止 0035 改写/合并后残留重复 ID）。期望 0 行。
SELECT ip.policy_id
FROM inspection_policy ip
WHERE jsonb_typeof(ip.scope->'application_ids') = 'array'
  AND jsonb_array_length(ip.scope->'application_ids')
      <> (SELECT count(DISTINCT e)
          FROM jsonb_array_elements_text(ip.scope->'application_ids') AS t(e));
```

```sql
-- (8) 不应再存在反向格式应用名 <provider>-<account_id>-cloud（0038 归一化后）。
-- 期望 0 行。
SELECT application_id, name
FROM asset_application
WHERE application_id LIKE 'cloud-%'
  AND environment = 'cloud'
  AND description LIKE 'Auto-created cloud sync application for account %'
  AND name LIKE '%-cloud'
  AND name NOT LIKE '%-cloud-%'
  AND split_part(name, '-', 2) <> 'cloud';
```

```sql
-- (9) 引用完整性诊断视图（0040 创建）：一次性检查所有表的孤儿引用。
-- 期望 0 行（重新同步后）。等价于上述 (4)(5)(6) 的合集，另含 inspection_policy。
-- 自 0042 起，该视图同时是迁移硬验收门控：补建后非 0 行则 0042 迁移失败。
SELECT * FROM v_asset_app_ref_integrity;
```

```sql
-- (10) 0042 补建应用应为引用完整性的兜底：列出补建过的应用供人工核对。
-- 不期望 0 行（列出补建记录），用于确认补建范围与人工核对，非阻断项。
SELECT application_id, name, description, created_at
FROM asset_application
WHERE environment = 'cloud'
  AND description LIKE 'Auto-created cloud sync application for account %'
ORDER BY created_at DESC;
```

功能验证：
- 升级后 `asset_application` 不再存在旧格式 `cloud-<账号>`；
- **重新触发已有账号同步**（发布流水线硬验收步骤，迁移完成后必须执行）；
- 重新同步后，资源全部写入新的 `cloud-<前缀>-<hash>` 应用；
- 告警/资产匹配规则指向新应用仍能命中资源；
- `alert_alert`/`inspection_policy` 中不再残留旧格式 `application_id` 引用；
- `v_asset_app_ref_integrity` 视图返回 0 行（无孤儿引用）；
- P0 验收脚本 `scripts/e2e-asset-sync.ps1` 通过。
