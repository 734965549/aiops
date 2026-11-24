# internal/server 模块说明

`server` 装配 HTTP 引擎与全局中间件、注册系统探针路由，并包装 `http.Server` 提供阻塞 `Run` 与优雅 `Shutdown`。各业务限界上下文通过 `RouteRegistrar` 接口把路由挂到 `/api` 分组，server 不反向依赖业务模块。

## 中间件链（顺序固定）

```text
SecurityHeaders -> Trace -> Recovery -> RequestLog -> CORS -> RequestBodyLimit(可选)
  -> /api 分组：public 挂 AuthOptional，authed 挂 AuthRequired
```

- `Trace` 在外层，保证后续日志与响应都带 trace_id；`RequestLog` 里的敏感路径（如 `/api/ai/chat/shares/<token>`）经 `RedactSensitivePath` 脱敏。
- `RequestBodyLimit` 仅在 `server.max_request_body_bytes > 0` 时启用。
- 非 dev 环境 `gin.SetMode(ReleaseMode)`；`server.trusted_proxies` 为空时显式置 nil（不信任任何代理）。
- `NoRoute` 兜底返回平台统一 `code/message/trace_id` 的 404。

## 系统路由

| 路径 | 语义 |
| --- | --- |
| `GET /healthz` | liveness：仅进程存活，顶层 `data.status=ok` |
| `GET /readyz` | readiness：`process/config/migration/db/redis` 五个子项；仅顶层 `ready` 才可接流量 |
| `GET /version` | 返回 `app/env/version`（ldflags 注入的 Version/Commit/BuildAt） |

`/readyz` 子项语义：

- `config`：重新执行 `cfg.Validate()`，失败 `down` 并阻塞。
- `migration`：读取迁移状态；有 pending 或 **checksum drift** 时子项 `degraded`，顶层随之置 `not_ready`（config/migration/db 任一非 OK 都阻塞就绪），details 携带 dir/latest/applied/pending 与漂移明细。
- `db`：2s 超时 ping；details 含连接池统计（open/in-use/idle/wait 等）。
- `redis`：2s 超时 ping；`required=true` 时异常为 `down` 并阻塞顶层，optional 时为 `degraded` 不阻塞。

## RouteRegistrar

```go
type RouteRegistrar interface { RegisterRoutes(groups RouteGroups) }
type RouteGroups struct { Public, Authed *gin.RouterGroup }
```

- `Public` 组挂 `AuthOptional`（登录、webhook 等匿名可达）；`Authed` 组挂 `AuthRequired`（无合法 Bearer 直接 401）。
- `cmd/api/main.go` 按 identity/ai/asset/audit/alert/execution/runbook/dashboard/integration/observability/inspection 顺序注入 11 个 registrar；nil 注册器跳过。

## http.Server 参数

- `addr = host:port`；`read_header_timeout` 缺省 5s；`max_header_bytes` 缺省 1MB。
- `Run` 阻塞直到 `Shutdown` 或监听错误（`http.ErrServerClosed` 视为正常）；超时值来自 `server.*` 配置，SSE 场景必须 ≥ `ai.turn_budget_s + 60`（启动时有告警核对）。

## 验证覆盖

- `router_test.go`（236 行）覆盖中间件链、系统探针与注册器分组行为。
- 就绪子项的降级组合（checksum drift、redis optional/required）在 `ops/health-contract.md` 有契约口径，修改时需同步契约。
