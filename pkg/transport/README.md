# pkg/transport 模块说明

`transport` 承载平台对外协议的通用封装。当前只有 HTTP 子包（响应结构、中间件、健康检查、trace 传递）；错误码到状态的映射也归传输层（`pkg/errors` 只保留 Code/Error）。

```text
internal/server
  -> pkg/transport/http（响应/健康/绑定/trace）
  -> pkg/transport/http/middleware（中间件链）
```

若将来新增 gRPC 或消息队列入口，在此目录下新增子包并复用 `pkg/errors` 的错误码，不把 HTTP 语义下沉到业务层。
