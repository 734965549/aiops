# Asset Sync 异步生命周期与租约续租设计

- 日期：2026-06-27
- 状态：待审核
- 范围：P1 修复 — 云资源同步生命周期
- 关联契约：`ops/cloud-observability-contract.md` §5.5.1、`ops/migration-contract.md` 0028
- 关联设计：`docs/huawei-ces-asset-sync-plan.md` §P1

## 1. 背景与问题

当前 `POST /api/assets/sync` 把整个同步流程（discovery → upsert → stale 标记 → 终态写入）放在 HTTP 请求 `ctx` 内同步执行，存在 4 个问题：

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
  ├─ [请求 ctx] 校验 → reap 过期 → 创建 running 批次(lease=now+5m) → ensure cloud app
  │                → 立即返回 running 批次 DTO（HTTP 200 / code=OK）
  └─ [detached goroutine ctx] runSync(runCtx, ...):
        ├─ 心跳 goroutine: 每 60s 用独立短 ctx 调 RenewLease(lease=now+5m)
        ├─ discovery + upsert + stale 标记（用 runCtx，可被取消）
        └─ defer finalizeBatch: 用独立短 ctx(10s) 写终态 + 审计（不受 runCtx 取消影响）

关闭流程: rootCancel() → goroutine 收到 runCtx 取消 → finalize 落 failed → wg.Wait()

前端: triggerAssetSync → 拿 running 批次 → pollSyncBatch 每 2s 轮询 GetSyncBatch
      → 终态展示（上限 10min，超时提示去批次页查看）
```

## 4. 详细设计

### 4.1 后端 `internal/asset/application/sync_service.go`

**常量调整**：

| 常量 | 旧值 | 新值 | 含义 |
|------|------|------|------|
| `syncBatchLeaseTTL` | 15min | 5min | 单次续租窗口；每次续到 `now+5min` |
| `syncLeaseRenewInterval` | — | 60s | 续租心跳间隔 |
| `syncHardTimeout` | — | 30min | goroutine 硬超时，兜底防止失控 |
| `syncTerminalCtxTimeout` | — | 10s | 终态写入/审计独立 ctx 超时 |

**SyncService 结构新增**：

- `shutdownCtx context.Context`：默认 `context.Background()`，由 `main.go` 通过 `SetLifecycle(ctx)` 注入 root ctx。goroutine 的 `runCtx` 派生自它。
- `wg sync.WaitGroup`：跟踪在途同步 goroutine。`Wait()` 供关闭时等待。

**TriggerSync 拆分**（同步前置阶段，仍在请求 ctx 内）：

1. 参数校验、`ResolveSyncAccount`、region 归一化（不变）。
2. `ReapExpiredRunning(ctx, accountID, now)` 清理过期批次（不变，仍用请求 ctx）。
3. `batches.Create` 插入 running 批次，`lease_expires_at = now + syncBatchLeaseTTL`（5min）。
4. `ensureCloudApplication`（不变）。
5. **立即返回** running 批次 DTO（`status=running`，无 `finished_at`）。
6. `s.wg.Add(1); go s.runSync(runCtx, actor, account, batch, appID, regions, provider)`。

`runCtx` 构造：`context.WithTimeout(s.shutdownCtx, syncHardTimeout)`。携带 `trace_id`/`user_id` 的 logger 从请求 ctx 提取后注入 runCtx（用 `logger.From(reqCtx)` 取出字段再 attach 到 runCtx，避免请求 ctx 取消后 logger 失效）。

**runSync（goroutine 主体）**：

```go
func (s *SyncService) runSync(runCtx context.Context, actor Actor, account Account, batch *domain.SyncBatch, appID string, regions []string, provider string) {
    defer s.wg.Done()
    // 心跳：周期续租，独立短 ctx，runCtx 取消时停止
    leaseCtx, leaseCancel := context.WithCancel(context.Background())
    defer leaseCancel()
    go s.leaseHeartbeat(leaseCtx, batch.BatchID)
    // 业务执行
    defer s.finalizeBatch(runCtx, actor, account, batch, appID, regions)
    // discovery/upsert/stale 复用现有 syncCloudFullSync / syncGeneric 逻辑
    // （迁入 runSync，使用 runCtx；runCtx 取消时 discovery 返回 ctx 错误）
}
```

**leaseHeartbeat**：

```go
func (s *SyncService) leaseHeartbeat(ctx context.Context, batchID string) {
    ticker := time.NewTicker(syncLeaseRenewInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            renewCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            err := s.batches.RenewLease(renewCtx, batchID, time.Now().UTC(), syncBatchLeaseTTL)
            cancel()
            if err != nil {
                // 批次已终态或 DB 异常：停止续租，runSync 仍会继续但不再续租
                return
            }
        }
    }
}
```

**finalizeBatch（defer 兜底，修复问题 #3/#4）**：

`finalizeBatch` 是 `runSync` 内部的闭包（非独立方法），捕获 `runSync` 局部变量 `successScopes`/`maxResourcesReached`/`partialErrs` 等，保证终态判定所需中间状态可访问：

```go
func (s *SyncService) finalizeBatch(runCtx context.Context, actor Actor, account Account, batch *domain.SyncBatch, appID string, regions []string) {
    // 终态用独立 ctx，不受 runCtx 取消影响
    termCtx, cancel := context.WithTimeout(context.Background(), syncTerminalCtxTimeout)
    defer cancel()
    finished := time.Now().UTC()
    batch.FinishedAt = &finished
    switch {
    case runCtx.Err() != nil:
        batch.Status = domain.SyncBatchStatusFailed
        batch.Message = "sync cancelled or timed out"
    case batch.FailedCount > 0 && batch.CreatedCount+batch.UpdatedCount == 0 && len(successScopes) == 0:
        batch.Status = domain.SyncBatchStatusFailed
        // ...
    case batch.FailedCount > 0 || maxResourcesReached:
        batch.Status = domain.SyncBatchStatusPartial
    default:
        batch.Status = domain.SyncBatchStatusSuccess
    }
    _ = s.batches.Update(termCtx, batch)  // 终态清空 lease_expires_at
    _ = s.audit.Record(termCtx, AuditRecord{...})  // 审计
}
```

> 注：现有 `TriggerSync` 内的 `syncCloudFullSync`/`syncGeneric`/stale 循环/终态判定逻辑整体迁移到 `runSync` + `finalizeBatch`，业务规则不变。`batch.successScopes`/`maxResourcesReached` 等中间状态通过闭包或 runSync 局部变量在 finalize 中可访问（finalize 作为 runSync 内的闭包捕获这些变量）。

**新增 SetLifecycle / Wait**：

```go
func (s *SyncService) SetLifecycle(ctx context.Context) { s.shutdownCtx = ctx }
func (s *SyncService) Wait() { s.wg.Wait() }
```

### 4.2 domain / persistence

**`internal/asset/domain/repository.go`**：`SyncBatchRepository` 接口新增

```go
// RenewLease 续租 running 批次：把 lease_expires_at 置为 now+ttl，updated_at=now。
// 仅当 status='running' 时续租；批次已终态（RowsAffected=0）返回 domain.ErrNotFound，
// 调用方据此停止心跳。
RenewLease(ctx context.Context, batchID string, now, ttl time.Time) error
```

**`internal/asset/infrastructure/persistence/sync_batch_repository.go`**：

```go
func (r *SyncBatchRepository) RenewLease(ctx context.Context, batchID string, now, ttl time.Time) error {
    expires := now.Add(ttl)
    result := r.db.WithContext(ctx).Model(&syncBatchModel{}).
        Where("batch_id = ? AND status = ?", batchID, domain.SyncBatchStatusRunning).
        Updates(map[string]any{
            "lease_expires_at": expires,
            "updated_at":       now,
        })
    if result.Error != nil { return result.Error }
    if result.RowsAffected == 0 { return domain.ErrNotFound }
    return nil
}
```

### 4.3 `cmd/api/main.go` 装配

- 构造 `rootCtx, rootCancel := context.WithCancel(context.Background())`；`defer rootCancel()`。
- `assetSyncSvc.SetLifecycle(rootCtx)`（紧跟 `NewSyncService` 之后）。
- 关闭顺序调整为：

```go
case s := <-sigCh:
    logger.L().Info("signal received, shutting down", logger.String("signal", s.String()))
case err := <-errCh:
    ...
}
rootCancel()                      // 1. 取消所有在途同步 goroutine 的 runCtx
if err := srv.Shutdown(shutdownCtx); err != nil { ... }  // 2. 停 HTTP
assetSyncSvc.Wait()               // 3. 等待同步 goroutine 落终态收尾
```

> `Wait()` 不额外设超时：`runCtx` 已有 30min 硬超时 + finalize 用 10s 独立 ctx，关闭时 in-flight 同步会很快进入 finalize；若担心极端情况，可在 `Wait` 前后记录日志。shutdown 超时仍由 `srv.Shutdown(shutdownCtx)` 控制 HTTP 侧。

### 4.4 前端 `web/src/api/asset.ts`

- `triggerAssetSync` 签名不变，返回的 DTO 现在 `status=running`（已支持）。
- 新增轮询 helper：

```ts
export class SyncStillRunningError extends Error {
  constructor(public batchId: string) {
    super('同步仍在进行，可在同步批次页查看')
    this.name = 'SyncStillRunningError'
  }
}

export async function pollSyncBatch(
  batchId: string,
  opts: { intervalMs?: number; timeoutMs?: number; shouldStop?: () => boolean } = {}
): Promise<SyncBatch> {
  const interval = opts.intervalMs ?? 2000
  const deadline = Date.now() + (opts.timeoutMs ?? 600_000)
  while (true) {
    if (opts.shouldStop?.()) throw new Error('polling cancelled')
    if (Date.now() > deadline) throw new SyncStillRunningError(batchId)
    const batch = await getSyncBatch(batchId)
    if (batch.status !== 'running') return batch
    await new Promise((r) => setTimeout(r, interval))
  }
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
| `docs/huawei-ces-asset-sync-plan.md` §P1 | 生命周期改为异步 + 续租，更新 §18 互斥/租约描述 |
| `scripts/e2e-asset-sync.ps1` | 检查是否假设同步响应为终态；若需调整，改为触发后轮询 |

## 6. 测试与验收

### 后端单元测试（`sync_service_test.go`）

- **现有同步完成测试**：改为「TriggerSync 立即返回 running → 断言后台执行后 fake repo 中批次到终态」。由于 goroutine 异步，测试中用 `svc.Wait()` 等待收尾后再断言终态。
- **新增 `TestSyncService_RenewLeaseCalled`**：注入 fake RenewLease，断言 runSync 期间被调用 ≥1 次。
- **新增 `TestSyncService_CancelledReachesTerminal`**：用可取消 ctx，取消后 `Wait()` 等待，断言批次落 `failed`（验证 finalize 用独立 ctx）。
- **新增 `TestSyncService_HardTimeoutFails`**：模拟 discovery 阻塞 > hardTimeout，断言批次落 `failed`。
- **保留**：`TestSyncService_TriggerSyncRejectsConcurrentRunning`、`TestSyncService_TriggerSyncReapsExpiredLease`（409/reap 仍生效）。

### 仓储测试

- `RenewLease`：running 续租成功；终态批次续租返回 `ErrNotFound`。

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
| 关闭时在途同步阻塞关闭 | `rootCancel` 触发 runCtx 取消 → discovery 返回 ctx 错误 → finalize 落 failed（10s）→ `Wait()` 收尾；不阻塞超过 shutdown 超时 |
| 续租期间批次被他人 reap | 不可能：reap 仅作用于 `lease_expires_at < now`，续租保持 `lease > now`；且部分唯一索引保证同账号只有一个 running |
| API 契约变更（status=running） | 仅 status 语义变化，409 不变；前端轮询为新增能力；E2E 脚本同步调整 |
| 旧前端版本收到 running 不轮询 | 仅本仓库前端，无外部消费者；同批发布 |

**回滚**：所有改动集中在 asset application + 1 个仓储方法 + main.go 装配 + 前端 2 文件；git revert 单提交可回退。表结构无变更。

## 8. 参数小结

| 参数 | 值 | 依据 |
|------|------|------|
| lease TTL（单次窗口） | 5min | 续租间隔 60s，留 4min 容错；reap 仅在续租彻底失败 5min 后触发 |
| 续租间隔 | 60s | 续租是轻量 UPDATE，60s 足够；DB 短暂抖动有 5min 缓冲 |
| goroutine 硬超时 | 30min | 单账号多 region 全量同步上限；超过视为异常强制 failed |
| 终态 ctx 超时 | 10s | 终态 Update + 审计单次写入，10s 足够 |
| 前端轮询间隔 | 2s | 平衡及时性与请求量 |
| 前端轮询上限 | 10min | 覆盖绝大多数同步；超时提示去批次页查看（不阻塞用户） |
