# HTTP middleware 模块说明（粤语版）

## 呢个模块做咩

`middleware` 系 HTTP 请求入门前嘅「保安通道」。每个请求真正到业务 Handler 之前，都要经过一层层检查同记录。

## 当前中间件

- `trace`：为每个请求生成或接收 `trace_id`。
- `recovery`：防止 panic 直接打爆服务。
- `request_log`：记录请求方法、路径、耗时、状态码。
- `cors`：处理浏览器跨域。
- `auth`：检查 `Authorization: Bearer <token>`。

## 调用关系

```text
客户端请求
  -> Trace
  -> Recovery
  -> RequestLog
  -> CORS
  -> Auth（受保护路由）
  -> 业务 Handler
```

## 入参

| 中间件 | 入参 | 说明 |
| --- | --- | --- |
| Trace | Header `X-Trace-ID` 或空 | 有就沿用，冇就生成 |
| Recovery | Gin Context | 捕获 panic |
| RequestLog | 请求方法、路径、IP、耗时 | 写访问日志 |
| CORS | Origin、Method、Headers | 判断是否允许跨域 |
| Auth | Bearer access token | 验证登录身份 |

## 出参

| 输出 | 说明 |
| --- | --- |
| `trace_id` | 放入 context 同响应，方便排查 |
| 日志 | 记录请求同错误 |
| 用户信息 | Auth 通过后写入 user_id、username |
| 错误响应 | token 缺失、过期、无效时返回未认证 |

## 通俗比喻

中间件就似机场安检：先睇登机牌，再过安检，再记录登机口。通过之后，你先可以去真正嘅航班柜台。