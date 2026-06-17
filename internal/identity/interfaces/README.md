# identity/interfaces 模块说明（粤语版）

## 呢个模块做咩

`interfaces` 系 Identity 对外嘅「窗口」。而家主要系 HTTP 窗口，负责收请求、拆参数、调用应用服务、包装响应。

## 调用关系

```text
客户端
  -> /api/identity/*
  -> interfaces/http Handler
  -> application AuthService / UserService / AccessControlService
  -> httpx.OK / httpx.Fail 统一响应
```

## 入参

| 路由 | 入参 | 说明 |
| --- | --- | --- |
| `POST /api/identity/login` | JSON：`username`、`password` | 登录 |
| `POST /api/identity/refresh` | JSON：`refresh_token` | 刷新 token |
| `GET /api/identity/me` | Header：`Authorization: Bearer ...` | 查当前用户 |
| `GET /api/identity/me/roles` | Header：`Authorization: Bearer ...` | 查当前用户绑定角色 |
| `GET /api/identity/roles` | Query：`page/page_size/status/is_system` | 查角色分页列表 |
| `GET /api/identity/permissions` | Query：`page/page_size/resource/action` | 查权限分页列表 |

## 出参

| 输出 | 说明 |
| --- | --- |
| 统一 JSON 成功响应 | `code/message/data/trace_id` |
| 统一 JSON 失败响应 | 参数错、未登录、服务异常等 |

## 通俗比喻

`interfaces` 就似政府办事大厅窗口：市民递表，窗口职员检查资料齐唔齐，然后交畀内部部门处理，最后盖章出结果。