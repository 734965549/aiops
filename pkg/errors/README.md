# pkg/errors 模块说明（粤语版）

## 呢个模块做咩

`errors` 系平台嘅「统一错误翻译机」。业务里面可能出现好多种错误：参数错、未登录、数据库错、服务未配置。如果每个地方自己乱写，前端就好难处理。所以呢度统一错误码同错误结构。

## 调用关系

```text
application / infrastructure
  -> apperr.New(code, message)
  -> apperr.Wrap(err, code, message)
  -> interfaces/http
  -> httpx.Fail(c, err)
  -> 统一 JSON 错误响应
```

## 入参

| 方法 | 入参 | 说明 |
| --- | --- | --- |
| `New` | 错误码、message | 创建业务错误 |
| `Wrap` | 原始 error、错误码、message | 包装底层错误 |
| `CodeOf` | error | 提取统一错误码 |

## 出参

| 输出 | 说明 |
| --- | --- |
| 业务错误对象 | 带 code、message、cause |
| 错误码 | 例如 `INVALID_ARGUMENT`、`UNAUTHENTICATED`、`INTERNAL` |
| HTTP 响应依据 | HTTP 层可根据错误码返回合适状态同 JSON |

## 通俗比喻

`errors` 就似客服话术表：无论后厨发生咩，面对客人都要讲清楚「资料唔齐」「未登录」「系统忙」。唔会将厨房入面嘅复杂报错原封不动丢畀客人。