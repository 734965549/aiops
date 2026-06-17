# pkg/logger 模块说明（粤语版）

## 呢个模块做咩

`logger` 系平台嘅「闭路电视同记事簿」。系统发生咩事、边个请求入嚟、边度出错，都要有记录，方便之后排查。

## 调用关系

```text
bootstrap.Init
  -> logger.Init(options)

server middleware
  -> 为请求生成 trace_id
  -> logger.From(ctx).Info/Warn/Error

各业务模块
  -> logger.L() 或 logger.From(ctx)
```

## 入参

| 参数 | 说明 |
| --- | --- |
| `Level` | 日志级别，例如 debug/info/warn/error |
| `Format` | JSON 或 console |
| `Output` | stdout 或文件 |
| `FilePath/MaxSizeMB/MaxBackups/MaxAgeDays` | 文件日志滚动配置 |
| `context.Context` | 从请求上下文带出 trace_id |

## 出参

| 输出 | 说明 |
| --- | --- |
| 日志行 | 包含时间、级别、message、trace_id、字段 |
| 全局 logger | `logger.L()` 可直接使用 |
| 上下文 logger | `logger.From(ctx)` 自动带 trace 信息 |

## 通俗比喻

`logger` 就似商场 CCTV：平时没人专门盯住，但一出事就可以翻查「几时、边个入口、发生咩」。trace_id 就似录像编号，可以由头追到尾。