# pkg/transport/http 模块说明（粤语版）

## 呢个模块做咩

`pkg/transport/http` 系 HTTP 层嘅「统一包装纸」。无论边个业务模块返回结果，都尽量包成同一种 JSON 格式；同时提供健康检查同请求辅助函数。

## 调用关系

```text
internal/server
  -> health handler
  -> middleware

identity/interfaces/http
  -> httpx.OK(c, data)
  -> httpx.Fail(c, err)
  -> httpx.FailWith(c, code, message)
```

## 入参

| 方法 / 功能 | 入参 | 说明 |
| --- | --- | --- |
| `OK` | Gin Context、data | 成功响应 |
| `Fail` | Gin Context、error | 按统一错误码失败响应 |
| `FailWith` | Gin Context、code、message | 直接返回指定业务错误 |
| health | 依赖检查结果 | 健康/就绪接口使用 |

## 出参

| 输出 | 说明 |
| --- | --- |
| 成功 JSON | 包含 `code/message/data/trace_id` |
| 失败 JSON | 包含错误码、错误说明、trace_id |
| 健康检查 JSON | 告诉外部服务而家可唔可以访问 |

## 通俗比喻

HTTP 响应就似外卖包装。如果每间店包装唔同，骑手同客人都会乱。`http` 模块就系统一餐盒尺寸同标签，令前端一睇就明。