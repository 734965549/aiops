# 健康检查契约

本文档定义平台对外健康检查接口的稳定契约，供平台团队、联调方、前端与运维系统统一参考。

## 接口

- `GET /healthz`
  - 语义：liveness，仅确认进程存活。
  - 依赖：只检查 `process`，不访问数据库或 Redis。
  - 用途：存活探针、进程级可用性监测。
  - 顶层 `data.status`：`ok` 表示进程存活（**不使用** `ready` / `not_ready`）。

- `GET /readyz`
  - 语义：readiness，确认实例是否可以接收流量。
  - 依赖：`process`、`config`、`migration`、`db`、`redis`。
  - 顶层 `data.status`（与 `/healthz` 区分）：
    - `ready`：全部子项均为 `ok`，可接流量；
    - `not_ready`：任一子项非 `ok`（含 `down` 或 `degraded`），不应加入流量池。
  - 子项 `checks[*].status`（**仅出现在 checks 数组内，不会作为顶层 status**）：
    - `ok`：该项检查通过；
    - `down`：依赖未注入或连接/查询失败；
    - `degraded`：可观测风险，常见于迁移未追平（`migration` 存在 pending）。

## 响应结构

两个端点都使用统一响应封装 `httpx.OK`，返回结构如下。

### `/healthz` 示例

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "...",
  "data": {
    "status": "ok",
    "checks": [
      { "name": "process", "status": "ok" }
    ],
    "uptime_ms": 12345
  }
}
```

### `/readyz` 示例（迁移未追平时）

顶层 `status` 为 `not_ready`；子项 `migration.status` 可为 `degraded`，但**不会**出现顶层 `degraded`。

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "...",
  "data": {
    "status": "not_ready",
    "checks": [
      { "name": "process", "status": "ok" },
      { "name": "config", "status": "ok" },
      {
        "name": "migration",
        "status": "degraded",
        "error": "pending migrations exist",
        "details": {
          "dir": "./migrations",
          "latest_version": "0002",
          "applied_version": "0001",
          "pending_count": 1,
          "up_to_date": false
        }
      },
      { "name": "db", "status": "ok" },
      { "name": "redis", "status": "ok" }
    ],
    "uptime_ms": 12345
  }
}
```

### `/readyz` 示例（全部就绪）

```json
{
  "code": "OK",
  "message": "ok",
  "trace_id": "...",
  "data": {
    "status": "ready",
    "checks": [
      { "name": "process", "status": "ok" },
      { "name": "config", "status": "ok" },
      { "name": "migration", "status": "ok", "details": { "up_to_date": true } },
      { "name": "db", "status": "ok" },
      { "name": "redis", "status": "ok" }
    ],
    "uptime_ms": 12345
  }
}
```

## 子项说明

- `process`
  - 仅表示进程本身存活。

- `config`
  - 校验平台基础配置是否可用，避免无效配置进入运行态。

- `migration`
  - 反映迁移是否追平。
  - `details` 用于展示迁移目录、最新版本、已应用版本、待执行数量。

- `db`
  - 反映 PostgreSQL 连接与 ping 状态。
  - `details` 可用于排查连接池与超时情况。

- `redis`
  - 反映 Redis 连接与 ping 状态。
  - `details` 可用于排查地址、池大小和 ping 延迟。
  - 当 `redis.required=false` 时，客户端未初始化或 ping 失败记为 `degraded`，**不**将顶层 `status` 置为 `not_ready`；`redis.required=true` 时 `down` 或非 `ok` 状态会阻塞就绪。

## 判读规则

- `/healthz`：顶层 `data.status == "ok"` 表示进程存活。
- `/readyz`：顶层 `data.status == "ready"` 才视为可接流量；`not_ready` 一律视为未就绪。
- 子项 `checks[*].status` 为 `degraded`（如 `migration` 有待执行脚本）时，顶层仍为 `not_ready`，**不得**将子项 `degraded` 当作顶层已就绪。
- 子项 `down` 表示依赖未注入（如 `main` 未向 `NewEngine` 传入 DB/Redis）或 ping/查询失败，需结合 `error` 排查。
- 调试与联调时，再结合 `checks[*].error` 和 `checks[*].details` 定位具体子项。

## 约束

- `healthz` 不得引入外部依赖检查。
- `readyz` 必须反映数据库、缓存、配置与迁移状态。
- 所有健康检查响应必须闭合 `trace_id`。
