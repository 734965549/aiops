# cmd/api 模块说明

`cmd/api` 是平台主进程入口：准备基础组件、装配全部 11 个业务限界上下文与可选后台任务、启动 HTTP 服务并编排优雅关机。装配层不做业务判断，只负责把仓储、应用服务、HTTP handler 与路由注册器按依赖顺序连起来。

## 启动模式与前置检查

```text
go run ./cmd/api [-config configs/config.yaml]
go run ./cmd/api -migrate            # 仅执行迁移后退出（生产流水线推荐，compose prod 方式 A）
```

- `-migrate` 模式先加载配置取迁移超时（`database.migrate_timeout_s`，缺省 5 分钟），调用 `bootstrap.Migrate` 后返回，不启动 HTTP。
- 正常启动使用 `app.bootstrap_timeout_s`（缺省 30s）作为 bootstrap 超时。
- 启动日志打印版本信息（ldflags 注入）；若 `server.read_timeout_s`/`write_timeout_s` 小于 `ai.turn_budget_s + 60`，仅告警不阻断——SSE 回合可能被截断。

## 装配顺序（main.go）

```text
bootstrap.Init（配置 -> 日志 -> PG[+auto_migrate] -> Redis）
  -> JWT 管理器与 Authenticator
  -> Identity：仓储 -> 用户/认证/访问控制/授权服务；Redis 不可用时：
       redis.required=true -> Fatal；否则 dev 降级（refresh 轮换禁用、幂等/会话退内存，均不允许进 prod）
     EnsureBootstrapUser + admin 角色绑定（失败 Fatal）
  -> Integration 凭据 Vault -> AI 密钥加密封装
  -> AI 工具网关：出站策略（白名单 + dev 允许 loopback）-> a/b/c 三执行器 -> Provider 注册表
     providerSvc.Bootstrap 仅在空表时写入 YAML 种子
  -> Audit 服务 + alert/asset/ai/identity 四个 recorder
  -> Asset / Alert（Redis 幂等窗口 = max(write_timeout+60s, 90s)）/ Execution / Runbook / Dashboard
  -> Integration / Observability（huawei ak_sk 走真实 CES，其余能力 fake）
  -> AI 端口注入：指标证据、快照存储、Postgres 采集、规则与规则体检（含资源发现兜底）、
     账号发现、资源读取/发现、回合预算、记忆、只读分享、知识
  -> CES 告警拉取：租约、账号读取/数据范围、非 prod 允许 fake 账号、实例 ID=api:<host>:<pid>
  -> Asset 云同步：数据范围授权 + 进程级 rootCtx 生命周期
  -> 可选 AlertSyncWorker（alert_sync.worker_enabled，默认 false）
  -> Inspection
  -> 11 个 RouteRegistrar -> server.NewEngine -> server.New
```

## 关机顺序（SIGINT/SIGTERM 或 HTTP 退出）

```text
1. rootCancel()            取消进程级 context，后台同步 goroutine 收到取消
2. srv.Shutdown            HTTP drain（server.shutdown_timeout_s，缺省 10s）
3. assetSyncSvc.WaitContext 独立预算等待在途同步 finalize（不吃 HTTP drain 时间）
4. alertSyncWorker.WaitContext 可选 worker 等待
5. app.Close()             Redis/PG 连接与日志 Sync（defer，幂等）
```

## 验证覆盖

- `main_test.go` 仅 1 个装配冒烟用例（系统探针）；装配顺序与降级分支依赖各模块自身测试与人工验收。
- 关机顺序无专项测试；修改 rootCtx/后台任务生命周期时需人工核对「取消 -> drain -> finalize」顺序与超时预算。
