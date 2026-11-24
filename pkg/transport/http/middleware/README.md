# HTTP middleware 模块说明

`middleware` 收敛平台 HTTP 通用中间件。`internal/server.NewEngine` 按固定顺序装配：

```text
SecurityHeaders -> Trace -> Recovery -> RequestLog -> CORS -> RequestBodyLimit(可选)
  -> /api/public 挂 AuthOptional、/api/authed 挂 AuthRequired
  -> 业务路由内再按需挂 AuthorizeStatic/AuthorizeDynamic
```

## 各中间件语义

| 中间件 | 语义 |
| --- | --- |
| `SecurityHeaders` | 每个响应注入 `X-Content-Type-Options: nosniff`、`X-Frame-Options: DENY`、`Referrer-Policy: strict-origin-when-cross-origin`、`Permissions-Policy`（geolocation/microphone/camera 全禁）。不含 CSP |
| `Trace` | 优先取合法 UUID 且 ≤128 字符的上游 `X-Trace-Id`，否则生成 UUID；写响应头 + Gin key + 标准 context，并把带 `trace_id` 字段的 logger 注入 ctx（`logger.From(ctx)` 自动携带） |
| `Recovery` | 兜底 panic：Error 日志（含 stack）后返回统一 `INTERNAL`；业务错误应走 `*apperr.Error`，不经此路径 |
| `RequestLog` | 访问日志 method/path/route/status/latency/client_ip/user_agent；路径经 `RedactSensitivePath` 脱敏；≥500 记 Error、≥400 记 Warn、其余 Info |
| `CORS` | 按 `config.CORSConfig` 生成；再防御 `allow_credentials=true` 与 `*` 并存（过滤后为空则不注册 CORS 头，适配同源反代）；AllowMethods 固定 GET/POST/PUT/PATCH/DELETE/OPTIONS，AllowHeaders 含 Authorization 与 X-Trace-Id |
| `RequestBodyLimit` | 用 `http.MaxBytesReader` 包裹请求体；`ShouldBindJSON` 触发的 `MaxBytesError` 在响应未写出时中止并返回 `PAYLOAD_TOO_LARGE`（413）。仅 `server.max_request_body_bytes > 0` 时注册 |
| `AuthRequired` | 无 Authenticator 直接 503；缺 Bearer 或校验失败统一 401（不区分缺失/过期，避免信息差）；成功后写入 `user_id/username/roles` 并把身份注入请求 logger |
| `AuthOptional` | 解析存在的 token 但不强制；无 token 或校验失败都放行（公共接口共享链路） |

## 统一授权（`authorization.go`）

- `AuthorizationInput`：resource/action + 对象字段（owner/dept/team/region/tags）+ 可选 tool_code/confirmed + `RequiredPermission` + `SkipDataScope`（静态路由只查 RBAC，不查数据范围）。
- `AuthorizationService.Authorize` 由 Identity 装配注入；结果写 `CtxKeyAuthDecision` 供后续复用。
- `AuthorizeStatic`：固定 resource/action + `SkipDataScope=true`；`AuthorizeDynamic`：由路由自行解析输入（如直接工具调用的对象字段）。svc 未配置 503、resolver 失败 400、`allowed=false` 403（带 reason）、Authorize 返回 error 走 `Fail`。
- 注意：中间件授权与 application 层二次校验是纵深防御的两层，不能只依赖其中一层（见 `ops/ai-contract.md` 工具网关三条调用链）。

## 测试

`auth` 相关由 Identity 集成测试覆盖；`authorization_test.go`/`cors_test.go`/`security_test.go`/`trace_test.go` 覆盖本包语义。
