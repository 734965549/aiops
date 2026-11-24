# pkg/transport/http 模块说明

`http` 是平台 HTTP 层统一封装：响应结构、请求绑定、trace 传递、健康检查结构与敏感路径脱敏。所有业务接口必须经 `OK/Fail/FailWith` 输出。

## 统一响应（`response.go`）

```go
type Response struct {
    Code    apperr.Code `json:"code"`     // 成功固定 "OK"（字符串）
    Message string      `json:"message"`  // 成功固定 "ok"
    TraceID string      `json:"trace_id"` // 固定字段，无 Trace 时输出 "" 不省略
    Data    any         `json:"data,omitempty"`
}
```

- `OK(c, data)`：HTTP 200 成功。
- `Fail(c, err)`：`*apperr.Error` 按 `HTTPStatus(code)` 输出；未包装错误经 `FromError` 转为 `INTERNAL`（对外固定文案，底层细节不外泄）。仅在「非业务错误」或「INTERNAL 且带 cause」时写 Error 日志（path 经脱敏）。
- `FailWith(c, code, message)`：直接以 code+message 输出。

## 错误码 → HTTP 状态（`status.go`）

400 `INVALID_ARGUMENT` / 401 `UNAUTHENTICATED` / 403 `PERMISSION_DENIED` / 404 `NOT_FOUND` / 409 `ALREADY_EXISTS`+`ABORTED` / 412 `FAILED_PRECONDITION` / 413 `PAYLOAD_TOO_LARGE` / 429 `RESOURCE_EXHAUSTED` / 503 `UNAVAILABLE` / 500 其余。

## 请求绑定（`bind.go`）

`BindJSON` 在**解码前**校验原始字节 UTF-8 有效性（`encoding/json` 会把非法字节静默替换为 U+FFFD，绑定后检查永远通过）；非法字节直接 `INVALID_ARGUMENT`，不做转码猜测。AI 契约 §10.1 的 UTF-8 约束由本函数承载。

## trace 传递（`trace_helper.go` + middleware/trace.go）

- 头部 `X-Trace-Id`；Gin key 与标准 context 同值。
- `TraceIDFrom(c)` 优先标准 context，其次 Gin keys；`TraceIDFromCtx(ctx)` 供非 HTTP 路径（定时任务/消费）使用；`ContextWithTraceID` 显式写入。
- 上游 trace_id 必须是合法 UUID 且 ≤128 字符，否则生成新 UUID（`normalizeTraceID`）。

## 健康检查结构（`health.go`）

`HealthStatus`（ok/down/degraded + 顶层 ready/not_ready）、`HealthCheck{Name,Status,Error,Details}`、迁移漂移（`HealthMigrationDetails` 含 checksum_drifts）、DB 连接池统计与 Redis 连接信息等结构化 details。语义口径见 `ops/health-contract.md` 与 `internal/server/README.md`。

## 敏感路径脱敏（`redact.go`）

`RedactSensitivePath` 把 `/api/ai/chat/shares/<token>` 的凭证段替换为 `<redacted>`；响应失败日志与访问日志中间件都使用。

## 测试

`response_test.go`/`bind_test.go`/`redact_test.go` 与 middleware 各测试覆盖主路径。
