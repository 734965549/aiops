# Asset Sync 异步生命周期与租约续租设计

- 日期：2026-06-27
- 状态：历史设计记录（historical design record）
- 范围：P1 修复 - 云资源同步生命周期
- 关联契约：`ops/cloud-observability-contract.md` §5.5.1、`ops/migration-contract.md` 0028
- 关联设计：`ops/huawei-ces-sync-contract.md` §18.1

> **历史设计记录说明**：本文档记录了 asset sync 异步生命周期与租约续租的初始设计。代码已在后续迭代中演进，文中代码示例和部分描述与当前实现存在已知漂移（见下方「已知漂移」）。权威实现以源码和 `ops/*.md` 契约为准，本文档仅保留设计意图与流程参考价值。

## 0. 已知漂移

以下为文档与当前实现的主要差异（截至 2026-07-10 核查）：

1. **finalize 已非闭包**：文档称 `finalize` 是 `runSync` 内部闭包（§4.1）。实际实现已重构为 `syncRunState` struct 的方法 `st.finalize()`，状态由 struct 字段持有而非闭包捕获。参见 `internal/asset/application/sync_service.go` `syncRunState` 定义及 `finalize()` 方法。
2. **RenewLease SQL 缺少过期防护**：文档 §4.2 的 SQL 仅校验 `batch_id + fencing_token + status='running'`。实际实现额外包含 `lease_expires_at IS NOT NULL AND lease_expires_at >= now`，防止对已过期租约续租。参见 `internal/asset/infrastructure/persistence/sync_batch_repository.go` `RenewLease`。
3. **表结构已变更**：文档 §2 和 §7 称"不改 `asset_sync_batch` 表结构"。实际有 3 条迁移在 0028 之后修改了该表：0030 加 `fencing_token`、0031 加 `summary`、0033 加 `triggered_by`。参见 `migrations/0030_*`、`migrations/0031_*`、`migrations/0033_*`。
4. **改动范围远超单次 revert**：文档 §7 称改动集中在 4 处、可单次 git revert 回退。实际涉及 4 条迁移、8+ 仓储方法（跨 `SyncBatchRepository` 和 `ResourceRepository`）、独立文件 `internal/asset/application/sync_finalize.go`、domain 模型变更、多个契约文档与测试文件，无法单次 revert。

## 1. 背景与问题

当前 `POST /api/assets/sync` 把整个同步流程（discovery -> upsert -> stale 标记 -> 终态写入）放在 HTTP 请求 `ctx` 内同步执行，存在 4 个问题：

1. **前端全局超时 30s**：`web/src/api/request.ts` 全局 `timeout: 30_000`，`triggerAssetSync` 无独立超时。真实华为云账号单 region 全量同步常超 30s，前端报错但后端仍在执行。
2. **同步在 HTTP ctx 内执行**：`TriggerSync` 直接透传 `c.Request.Context()`；HTTP 请求取消（前端超时/用户离开页面）后，discovery 与 upsert 中途断开，资源可能只入库一部分。
3. **终态更新用已取消的 ctx**：`TriggerSync` 末尾的 `s.batches.Update(ctx, batch)` 仍使用已被取消的请求 ctx，导致批次无法落到终态，卡在 `running`，下次触发 409。
4. **15min 租约无续租 + stale 竞态**：`syncBatchLeaseTTL = 15min` 固定无续租。正常同步超过 15min 时，第二个请求会把仍在执行的批次 `ReapExpiredRunning` 为 `failed`，然后启动新批次；两个批次交错执行 `MarkStaleByAccountScopeExceptBatch` 会把对方刚 upsert 的资源错误标记为 `stale`，产生错误资产状态。

项目当前**无后台 worker 基础设施**（架构文档规划了 Worker/Scheduler 形态但未落地；Execution 模块也是同步模拟执行）。

## 2. 目标与非目标

### 目标

- `POST /api/assets/sync` 立即返回 `running` 批次，同步在后台 goroutine 执行。
- 后台 goroutine 用独立 context（不受 HTTP 请求生命周期影响）+ 硬超时。
- 引入 lease 续租心跳，正常同步不再因租约过期被 reap。
- 终态写入与审计使用独立短 context，保证即便 goroutine ctx 取消也能落终态。
- 前端触发后轮询批次到终态，不再受 30s 全局超时影响。

### 非目标

- 不引入 job 队列、worker pool 或持久化调度框架（保持请求级 goroutine，最小侵入）。
- 不改变 409 账号级互斥语义与 `ReapExpiredRunning` 自愈机制。
- 不改 `asset_sync_batch` 表结构（0028 迁移的 `lease_expires_at` + 部分唯一索引已满足续租需求）。
- 不改 discovery/upsert/stale 的业务逻辑，仅改变其执行上下文与生命周期管理。

## 3. 方案概览

```text
POST /api/assets/sync
  ├─ [请求 ctx] 校验 -> reap 过期 -> 创建 running 批次(lease=now+5m) -> ensure cloud app
  │                -> 立即返回 running 批次 DTO（HTTP 200 / code=OK）
  └─ [detached goroutine ctx] runSync(runCtx, ...):
        ├─ 心跳 goroutine: 每 60s 用独立短 ctx 调 RenewLease(lease=now+5m)
        ├─ discovery + upsert + stale 标记（用 runCtx，可被取消）
        └─ finalize(): 用独立短 ctx(10s) 写终态 + 审计（不受 runCtx 取消影响）

关闭流程: rootCancel() -> goroutine 收到 runCtx 取消 -> finalize 落 failed -> WaitContext()

前端: triggerAssetSync -> 拿 running 批次 -> pollSyncBatch 每 2s 轮询 GetSyncBatch
      -> 终态展示（上限 10min，超时提示去批次页查看）
```

## 4. 详细设计

### 4.1 后端 `internal/asset/application/sync_service.go`

**常量调整**：

| 常量 | 旧值 | 新值 | 含义 |
|------|------|------|------|
| `syncBatchLeaseTTL` | 15min | 5min | 单次续租窗口；每次续到 `now+5min` |
| `syncLeaseRenewInterval` | - | 60s | 续租心跳间隔 |
| `syncHardTimeout` | - | 30min | goroutine 硬超时，兜底防止失控 |
| `syncTerminalCtxTimeout` | - | 10s | 终态写入/审计独立 ctx 超时 |
| `syncLeaseRenewCtxTimeout` | - | 5s | 单次续租 DB 调用超时 |

> 上述时长用 `var` 而非 `const`，测试中可缩短心跳间隔/硬超时以加速用例。

**SyncService 结构新增**：

- `shutdownCtx context.Context`：默认 `context.Background()`（`shutdownCtx == nil` 时兜底），由 `main.go` 通过 `NewSyncService` 构造器参数注入。`SetLifecycle(ctx)` 保留为向后兼容的辅助方法，会覆盖构造器已注入的值。
- `wg sync.WaitGroup`：跟踪在途同步 goroutine。`WaitContext(ctx)` 供关闭时带超时等待。

**TriggerSync 拆分**（同步前置阶段，仍在请求 ctx 内）：

1. 参数校验、`ResolveSyncAccount`、region 归一化（不变）。
2. `ReapExpiredRunning(ctx, accountID, now)` 清理过期批次（不变，仍用请求 ctx）。
3. `batches.Create` 插入 running 批次，`lease_expires_at = now + syncBatchLeaseTTL`（5min）。
4. 创建 running 批次后立即用请求 ctx 写 `sync_started` 审计（在 `ensureCloudApplication` 之前），保证前置阶段失败时 running 批次也有审计可追溯。
5. `ensureCloudApplication`（不变）。若失败，用 `context.WithoutCancel(ctx)` 派生的短 ctx 调 `finishBatchFailedDetached` 落终态，避免请求 ctx 取消导致卡 running。
6. **立即返回** running 批次 DTO（`status=running`，无 `finished_at`）。
7. `s.wg.Add(1); go s.runSync(runCtx, runCancel, actor, batch, appID, regions, provider, account)`。

`runCtx` 构造通过 `deriveSyncRunContext(ctx, s.shutdownCtx, syncHardTimeout)`：

```go
func deriveSyncRunContext(ctx context.Context, shutdownCtx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
    detached := context.WithoutCancel(ctx)              // 保留请求 ctx 的 trace_id/user_id/logger values
    runCtx, runCancel := context.WithTimeout(detached, timeout)
    stop := context.AfterFunc(shutdownCtx, runCancel)   // 进程关闭时取消 runCtx
    return runCtx, func() {
        stop()
        runCancel()
    }
}
```

> 与初始设计不同：`runCtx` 从**请求 ctx**（经 `WithoutCancel` 脱离取消信号但保留 values）派生，再叠加硬超时；`shutdownCtx` 通过 `AfterFunc` 桥接关闭取消，而非直接作为 `WithTimeout` 的父 context。这样 trace/logger 链路不会断。

**runSync（goroutine 主体）**：

```go
func (s *SyncService) runSync(
    runCtx context.Context,
    runCancel context.CancelFunc,
    actor Actor,
    batch *domain.SyncBatch,
    appID string,
    regions []string,
    provider string,
    account *SyncAccountSnapshot,
) {
    defer s.wg.Done()
    defer runCancel()

    detachedCtx := context.WithoutCancel(runCtx)  // 终态 ctx 的父，不受 runCtx 取消影响

    // 心跳：周期续租，受 runCtx 硬超时控制，finalize 时停止。leaseDone 在心跳退出后关闭，
    // finalize 等待它以确保终态 Update 清空 lease 后不会再被心跳写回（避免竞态）。
    leaseCtx, leaseCancel := context.WithCancel(runCtx)
    defer leaseCancel()
    leaseDone := make(chan struct{})
    go s.leaseHeartbeat(leaseCtx, runCancel, batch.BatchID, batch.FencingToken, leaseDone)

    // discovery/upsert/stale 复用现有 syncCloudFullSync / syncGeneric 逻辑（使用 runCtx）
    // ... 中间状态变量 successScopes/maxResourcesReached/partialErrs/syncSummaries 等在此阶段产出 ...

    finalize()  // 直接调用（非 defer），显式停止心跳后落终态
}
```

**leaseHeartbeat**：

```go
func (s *SyncService) leaseHeartbeat(ctx context.Context, runCancel context.CancelFunc, batchID, fencingToken string, done chan<- struct{}) {
    defer close(done)  // 退出时关闭，供 finalize 等待心跳彻底停止
    ticker := time.NewTicker(syncLeaseRenewInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            renewCtx, cancel := context.WithTimeout(ctx, syncLeaseRenewCtxTimeout)
            err := s.batches.RenewLease(renewCtx, batchID, fencingToken, time.Now().UTC(), syncBatchLeaseTTL)
            cancel()
            if err == nil {
                continue
            }
            if errors.Is(err, domain.ErrLeaseLost) {
                // 租约所有权丢失（fencing_token 不匹配或批次已终态）：取消主任务
                logger.From(ctx).Warn("asset sync lease lost", logger.String("batch_id", batchID))
                runCancel()
                return
            }
            // DB 临时错误：记录并下个 tick 重试（不停止心跳）
            logger.From(ctx).Warn("renew asset sync lease failed, will retry", logger.String("batch_id", batchID), logger.Error(err))
        }
    }
}
```

> 与初始设计不同：`leaseCtx` 派生自 `runCtx`（非 `context.Background()`），续租超时用 `syncLeaseRenewCtxTimeout`（5s）。引入 `fencingToken` 做租约所有权校验，`ErrLeaseLost` 时主动 `runCancel()` 取消主任务。`done` channel 供 finalize 同步等待心跳停止，避免终态清空 lease 后被心跳写回。

**finalize（runSync 内部闭包，直接调用）**：

`finalize` 是 `runSync` 内部的闭包（非独立方法），捕获 `runSync` 局部变量 `successScopes`/`maxResourcesReached`/`partialErrs`/`summaryLines`/`syncSummaries`/`negativeScopes`/`productNamesEmpty` 以及 `leaseCancel`/`leaseDone`/`detachedCtx`/`runCtx`/`batch` 等，保证终态判定所需中间状态可访问：

```go
finalize := func() {
    leaseCancel()  // 停止心跳，终态不再续租
    <-leaseDone    // 等待心跳完全停止，避免终态清空 lease 后被写回
    termCtx, termCancel := context.WithTimeout(detachedCtx, syncTerminalCtxTimeout)  // 独立短 ctx，不受 runCtx 取消影响
    defer termCancel()
    finished := time.Now().UTC()
    batch.FinishedAt = &finished
    // ... 组装 message/summary ...
    switch {
    case runCtx.Err() != nil:
        batch.Status = domain.SyncBatchStatusFailed
        // ... Update(termCtx, batch) + 审计；ErrLeaseLost 时跳过 ...
    case batch.FailedCount > 0 && batch.CreatedCount+batch.UpdatedCount == 0 && !hasSuccessfulDiscovery:
        batch.Status = domain.SyncBatchStatusFailed
        // ... Update + 审计 ...
    case batch.FailedCount > 0 || maxResourcesReached || productNamesEmpty || enrichmentFailed:
        batch.Status = domain.SyncBatchStatusPartial
        // ... Update + 审计 ...
    default:
        batch.Status = domain.SyncBatchStatusSuccess
        // 用 FinalizeSuccess(termCtx, batch, ...) 推进成功批次并清空 lease
        // FinalizeSuccess 失败时降级为 partial
    }
}
finalize()  // runSync 末尾直接调用
```

> 与初始设计不同：`finalize` 是闭包而非独立方法 `finalizeBatch`；终态 ctx 用 `context.WithTimeout(detachedCtx, ...)`（`detachedCtx = WithoutCancel(runCtx)`）而非 `context.Background()`，保留 trace/logger 链路；调用方式是直接 `finalize()` 而非 `defer`；增加 `leaseCancel()`+`<-leaseDone` 同步、`ErrLeaseLost` 跳过终态、success 路径用 `FinalizeSuccess`、`hasSuccessfulDiscovery` 判定等。

**SetLifecycle / WaitContext**：

```go
// SetLifecycle 向后兼容辅助方法，会覆盖构造器已注入的 shutdownCtx。生产装配已通过 NewSyncService 注入，无需调用。
func (s *SyncService) SetLifecycle(ctx context.Context) {
    if ctx != nil {
        s.shutdownCtx = ctx
    }
}

// WaitContext 等待所有在途同步 goroutine 收尾；ctx 取消时返回 false，避免关闭流程无限阻塞。
func (s *SyncService) WaitContext(ctx context.Context) bool {
    done := make(chan struct{})
    go func() {
        s.wg.Wait()
        close(done)
    }()
    select {
    case <-done:
        return true
    case <-ctx.Done():
        return false
    }
}
```

### 4.2 domain / persistence

**`internal/asset/domain/repository.go`**：`SyncBatchRepository` 接口新增

```go
// RenewLease 续租 running 批次：按 batch_id + fencing_token 校验租约所有权，
// 把 lease_expires_at 置为 now+ttl，updated_at=now。
// 仅当 status='running' 且 fencing_token 匹配时续租；批次已终态或 token 不匹配
// （RowsAffected=0）返回 domain.ErrLeaseLost，调用方据此停止心跳并取消主任务。
RenewLease(ctx context.Context, batchID, fencingToken string, now time.Time, ttl time.Duration) error
```

**`internal/asset/infrastructure/persistence/sync_batch_repository.go`**：

```go
func (r *SyncBatchRepository) RenewLease(ctx context.Context, batchID, fencingToken string, now time.Time, ttl time.Duration) error {
    expires := now.Add(ttl)
    result := r.db.WithContext(ctx).Model(&syncBatchModel{}).
        Where("batch_id = ? AND fencing_token = ? AND status = ?", batchID, fencingToken, domain.SyncBatchStatusRunning).
        Updates(map[string]any{
            "lease_expires_at": expires,
            "updated_at":       now,
        })
    if result.Error != nil { return result.Error }
    if result.RowsAffected == 0 { return domain.ErrLeaseLost }
    return nil
}
```

> 与初始设计不同：`RenewLease` 增加 `fencingToken` 参数做租约所有权校验；续租失败（批次已终态或 token 不匹配）返回 `domain.ErrLeaseLost` 而非 `domain.ErrNotFound`。

### 4.3 `cmd/api/main.go` 装配

- 构造 `rootCtx, rootCancel := context.WithCancel(context.Background())`；`defer rootCancel()`。
- `assetSyncSvc := assetapp.NewSyncService(..., rootCtx)`——`shutdownCtx` 通过构造器参数注入，**无需再调用 `SetLifecycle`**。
- 关闭顺序调整为：

```go
// 1. 取消进程级 context：在途后台同步 goroutine 收到 runCtx 取消，
//    discovery 循环退出，finalize 用独立短 ctx 落终态。
rootCancel()
// 2. 停止接收新 HTTP 请求并处理在途请求。
httpShutdownCtx, cancelHTTP := context.WithTimeout(context.Background(), shutdownTimeout)
defer cancelHTTP()
if err := srv.Shutdown(httpShutdownCtx); err != nil { ... }
// 3. 后台同步等待使用独立预算，避免 HTTP drain 吃掉 finalize 时间。
syncShutdownCtx, cancelSync := context.WithTimeout(context.Background(), shutdownTimeout)
defer cancelSync()
if !assetSyncSvc.WaitContext(syncShutdownCtx) {
    logger.L().Warn("asset sync shutdown wait timed out")
}
```

> 与初始设计不同：HTTP drain 与 sync finalize 使用**独立超时预算**（各一个 `shutdownTimeout`），避免 HTTP drain 耗尽时间导致 finalize 来不及落终态；`WaitContext(syncShutdownCtx)` 带超时返回，避免关闭流程无限阻塞。

### 4.4 前端 `web/src/api/asset.ts`

- `triggerAssetSync` 签名不变，返回的 DTO 现在 `status=running`（已支持）。
- 新增轮询 helper：

```ts
export class SyncStillRunningError extends Error {
  batchId: string
  constructor(batchId: string) {
    super('同步仍在进行，可在同步批次页查看')
    this.name = 'SyncStillRunningError'
    this.batchId = batchId
  }
}

export async function pollSyncBatch(
  batchId: string,
  opts: { intervalMs?: number; timeoutMs?: number; shouldStop?: () => boolean } = {}
): Promise<SyncBatch> {
  const interval = opts.intervalMs ?? 2000
  const deadline = Date.now() + (opts.timeoutMs ?? 600_000)
  while (Date.now() <= deadline) {
    if (opts.shouldStop?.()) {
      throw new Error('polling cancelled')
    }
    const batch = await getSyncBatch(batchId)
    if (batch.status !== 'running') {
      return batch
    }
    await new Promise((resolve) => setTimeout(resolve, interval))
  }
  throw new SyncStillRunningError(batchId)
}
```

### 4.5 前端 `web/src/views/integrations/index.vue`

`onSyncAssets` 改为：

```ts
async function onSyncAssets(accountId: string) {
  syncingId.value = accountId
  let cancelled = false
  onBeforeUnmount(() => { cancelled = true })
  try {
    const batch = await triggerAssetSync(accountId)   // 瞬时调用，返回 running
    const result = await pollSyncBatch(batch.batch_id, { shouldStop: () => cancelled })
    Message.success(`同步完成：新建 ${result.created_count}，更新 ${result.updated_count}，stale ${result.stale_count}（${result.status}）`)
  } catch (err) {
    if (isAssetSyncInProgressError(err)) {
      Message.warning('该账号正在同步，请稍后重试'); return
    }
    if (err instanceof SyncStillRunningError) {
      Message.info(err.message); return   // 提示去批次页查看
    }
    Message.error(getApiError(err)?.message || '资源同步失败')
  } finally {
    syncingId.value = ''
  }
}
```

> 全局 30s 超时不再影响同步：触发是瞬时 POST，轮询是快速 GET，均远低于 30s。`shouldStop` 用组件卸载 flag 取消轮询，避免泄漏。

## 5. 契约与文档同步

| 文档 | 改动 |
|------|------|
| `ops/cloud-observability-contract.md` §5.5.1 | `POST /api/assets/sync` 立即返回 `status=running` 批次；客户端轮询 `GET /batches/:batch_id` 到终态；lease TTL 5min + 每 60s 续租；硬超时 30min |
| `web/src/api/README.md` | `triggerAssetSync` 语义改为返回 running；新增 `pollSyncBatch`/`SyncStillRunningError` |
| `ops/huawei-ces-sync-contract.md` §18.1 | 生命周期改为异步 + 续租，更新互斥/租约描述 |
| `scripts/e2e-asset-sync.ps1` | 检查是否假设同步响应为终态；若需调整，改为触发后轮询 |

## 6. 测试与验收

### 后端单元测试（`sync_service_test.go`）

- **现有同步完成测试**：改为「TriggerSync 立即返回 running -> 断言后台执行后 fake repo 中批次到终态」。由于 goroutine 异步，测试中用 `svc.WaitContext(ctx)` 等待收尾后再断言终态。
- **新增 `TestSyncService_RenewLeaseCalled`**：注入 fake RenewLease，断言 runSync 期间被调用 ≥1 次。
- **新增 `TestSyncService_CancelledReachesTerminal`**：用可取消 ctx，取消后 `WaitContext(ctx)` 等待，断言批次落 `failed`（验证 finalize 用独立 ctx）。
- **新增 `TestSyncService_HardTimeoutFails`**：模拟 discovery 阻塞 > hardTimeout，断言批次落 `failed`。
- **保留**：`TestSyncService_TriggerSyncRejectsConcurrentRunning`、`TestSyncService_TriggerSyncReapsExpiredLease`（409/reap 仍生效）。

### 仓储测试

- `RenewLease`：running 续租成功；终态批次或 fencing_token 不匹配续租返回 `ErrLeaseLost`。

### 验收脚本

```powershell
go test ./internal/asset/... ./pkg/...
cd web && npm run build
.\scripts\e2e-asset-sync.ps1
```

## 7. 风险与回滚

| 风险 | 缓解 |
|------|------|
| goroutine 失控 | hard timeout 30min + finalize 独立 ctx 兜底；进程崩溃靠 5min lease 自愈（reap 为 failed） |
| 关闭时在途同步阻塞关闭 | `rootCancel` 触发 runCtx 取消 -> discovery 返回 ctx 错误 -> finalize 落 failed（10s）-> `WaitContext(syncShutdownCtx)` 带独立超时预算收尾；不阻塞超过 shutdown 超时 |
| 续租期间批次被他人 reap | 不可能：reap 仅作用于 `lease_expires_at < now`，续租保持 `lease > now`；且部分唯一索引保证同账号只有一个 running |
| 续租时 fencing_token 不匹配（并发批次） | `RenewLease` 按 `batch_id + fencing_token` 校验，不匹配返回 `ErrLeaseLost`，心跳停止并取消主任务 |
| API 契约变更（status=running） | 仅 status 语义变化，409 不变；前端轮询为新增能力；E2E 脚本同步调整 |
| 旧前端版本收到 running 不轮询 | 仅本仓库前端，无外部消费者；同批发布 |

**回滚**：所有改动集中在 asset application + 1 个仓储方法 + main.go 装配 + 前端 2 文件；git revert 单提交可回退。表结构无变更。

## 8. 参数小结

| 参数 | 值 | 依据 |
|------|------|------|
| lease TTL（单次窗口） | 5min | 续租间隔 60s，留 4min 容错；reap 仅在续租彻底失败 5min 后触发 |
| 续租间隔 | 60s | 续租是轻量 UPDATE，60s 足够；DB 短暂抖动有 5min 缓冲 |
| 单次续租 DB 超时 | 5s | 续租是单行 UPDATE，5s 足够；超时后下个 tick 重试 |
| goroutine 硬超时 | 30min | 单账号多 region 全量同步上限；超过视为异常强制 failed |
| 终态 ctx 超时 | 10s | 终态 Update + 审计单次写入，10s 足够 |
| 前端轮询间隔 | 2s | 平衡及时性与请求量 |
| 前端轮询上限 | 10min | 覆盖绝大多数同步；超时提示去批次页查看（不阻塞用户） |
