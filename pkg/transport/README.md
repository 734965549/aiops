# pkg/transport 模块说明（粤语版）

## 呢个模块做咩

`transport` 系平台同外界沟通嘅「交通总站」。而家主要系 HTTP，将来如果有 gRPC、消息队列入口，都可以放喺呢个方向。

## 调用关系

```text
internal/server
  -> pkg/transport/http
      -> response.go 统一响应
      -> health.go 健康检查
      -> middleware/* 中间件
```

## 入参

| 类型 | 说明 |
| --- | --- |
| HTTP 请求 | 前端、调用方发入嚟嘅请求 |
| Gin Context | 中间件同 Handler 共享嘅上下文 |
| 业务 data/error | Handler 调完 service 后交畀响应工具 |

## 出参

| 输出 | 说明 |
| --- | --- |
| JSON 响应 | 成功或失败统一格式 |
| trace_id | 方便前后端一齐查问题 |
| 中间件效果 | CORS、恢复 panic、日志、鉴权等 |

## 通俗比喻

`transport` 就似巴士总站：乘客由唔同路线入嚟，总站负责安排上车、验票、指路，最后送去正确目的地。