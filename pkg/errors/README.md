# pkg/errors 模块说明

`errors`（导入别名 `apperr`）是平台统一错误模型：12 个字符串错误码与 `Error` 结构。协议相关的状态码映射放在 `pkg/transport/http`，本包不感知传输层。

## 错误码（`code.go`，与 gRPC 命名一致）

`OK` / `INVALID_ARGUMENT` / `UNAUTHENTICATED` / `PERMISSION_DENIED` / `NOT_FOUND` / `ALREADY_EXISTS` / `FAILED_PRECONDITION` / `ABORTED` / `RESOURCE_EXHAUSTED` / `PAYLOAD_TOO_LARGE` / `UNAVAILABLE` / `INTERNAL`

## Error 模型（`errors.go`）

| API | 语义 |
| --- | --- |
| `New(code, message)` / `Newf` | 构造业务错误 |
| `Wrap(err, code, message)` | 包裹底层错误保留链路；err 为 nil 时返回 nil |
| `Error()/Unwrap()` | 支持 `errors.Is/As` 穿透底层错误 |
| `FromError(err)` | 已是 `*Error` 原样返回；否则包装为 `INTERNAL`，对外固定文案 `internal server error`，底层细节留在 cause |
| `CodeOf(err)` | nil → `OK`；未包装 → `INTERNAL` |
| `MessageOf(err)` | nil → `"ok"`；提取对外安全文案 |

分层约定：domain 只构造 `Error`；application 优先 `Wrap` 携带链路；interfaces 层只读 `Code/Message`，不再判定下层错误类型。

## HTTP 状态映射（`pkg/transport/http/status.go`）

| Code | HTTP |
| --- | --- |
| `OK` | 200 |
| `INVALID_ARGUMENT` | 400 |
| `UNAUTHENTICATED` | 401 |
| `PERMISSION_DENIED` | 403 |
| `NOT_FOUND` | 404 |
| `ALREADY_EXISTS` / `ABORTED` | 409 |
| `FAILED_PRECONDITION` | 412 |
| `RESOURCE_EXHAUSTED` | 429 |
| `PAYLOAD_TOO_LARGE` | 413 |
| `UNAVAILABLE` | 503 |
| 其它（含 `INTERNAL`） | 500 |

## 测试

`errors_test.go`、`sentinel_test.go` 覆盖构造/包裹/转换语义。
