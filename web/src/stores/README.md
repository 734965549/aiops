# web/src/stores 模块说明

## 职责

`stores` 存放跨页面共享状态。当前已实现 `auth` store，负责登录态、JWT 与当前用户缓存。

## auth store（`auth.ts`）

### 状态

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `token` | `ref<string>` | access token，写入 `Authorization: Bearer` |
| `refreshToken` | `ref<string>` | refresh token，仅用于续期与登出吊销 |
| `user` | `ref<CurrentUser \| null>` | 当前用户摘要 |

本地持久化键名：`aiops_token`、`aiops_refresh_token`、`aiops_user`。

### 方法

| 方法 | 说明 |
| --- | --- |
| `login(username, password)` | 调用 `POST /api/identity/login`，保存 TokenPair |
| `refresh()` | 调用 `POST /api/identity/refresh`，轮换 token 对；并发 401 时复用同一 in-flight Promise，避免重复轮换 |
| `logout()` | 清除本地状态，并尝试 `POST /api/identity/logout` 吊销 refresh token |
| `loadFromStorage()` | 从 localStorage 恢复 token / user（路由守卫、刷新页面时使用） |
| `applyTokenPair(pair)` | 登录/刷新成功后统一写入 token、refreshToken、user |
| `setToken` / `setRefreshToken` / `setUser` | 细粒度更新（如 `GET /api/identity/me` 后刷新 user） |

### 调用关系

```text
views/login
  -> auth.login()
  -> api/identity.login()
  -> auth.applyTokenPair()
  -> router -> /dashboard

api/request.ts（401 拦截）
  -> auth.refresh() 重试原请求
  -> 失败则 auth.logout() + 跳转 /login

layouts/BasicLayout
  -> fetchCurrentUser() -> auth.setUser()
  -> auth.logout() -> /login
```

契约见 `ops/auth-contract.md`；`refresh()` 通常由 `request.ts` 拦截器自动触发，页面无需直接调用。

## 扩展约定

新增 store 时：

1. 使用 Pinia `defineStore` + composition API（与 `auth.ts` 一致）。
2. 在本 README 补充状态表与方法表。
3. 涉及 API 的 store 方法应调用 `web/src/api/*`，勿在 store 内直接 axios。
