# identity/application 模块说明（粤语版）

## 呢个模块做咩

`application` 系 Identity 嘅「办事柜台」。HTTP 层将参数交畀佢，佢负责安排流程：查用户、验密码、签 token、查当前用户，以及查询角色、权限、数据范围、AI 工具权限等 Identity 权限模型。

## 调用关系

```text
interfaces/http Handler
  -> AuthService.Login / Refresh / EnsureBootstrapUser
  -> UserService.GetCurrentUser
  -> AccessControlService.ListRoles / CountRoles / ListPermissions / CountPermissions / ListUserRoles
  -> domain.UserRepository / AccessControlRepository
  -> pkg/auth 密码同 JWT 工具
```

## 入参

| 方法 | 入参 | 说明 |
| --- | --- | --- |
| `Login` | `LoginInput{Username, Password}` | 登录资料 |
| `Refresh` | `refreshToken string` | 刷新令牌 |
| `GetCurrentUser` | `userID string` | 从鉴权中间件解析出嚟嘅用户 ID |
| `EnsureBootstrapUser` | `username/password/displayName` | 默认管理员资料 |
| `ListRoles` / `CountRoles` | `RoleFilter` | 查询角色列表与总数 |
| `ListPermissions` / `CountPermissions` | `PermissionFilter` | 查询权限列表与总数 |
| `ListUserRoles` | `userID string` | 查询当前用户或指定用户绑定角色 |
| `ListRolePermissions` | `roleID string` | 查询角色绑定权限 |
| `ListDataScopes` | `DataScopeFilter` | 查询数据范围定义 |
| `ListAIToolPermissions` / `CountAIToolPermissions` | `AIToolPermissionFilter` | 查询 AI 工具权限列表与总数 |

## 出参

| 方法 | 出参 | 说明 |
| --- | --- | --- |
| `Login` | `TokenPair` | access token、refresh token、用户资料 |
| `Refresh` | `TokenPair` | 新嘅 token 对 |
| `GetCurrentUser` | `CurrentUserDTO` | 当前用户资料 |
| `ListRoles` / `ListPermissions` / `ListUserRoles` 等 | 权限模型列表 | 角色、权限、数据范围、AI 工具权限等只读查询结果 |
| `CountRoles` / `CountPermissions` / `CountAIToolPermissions` | `int64` | 分页列表总数 |
| 全部方法 | `error` | 参数错、未认证、数据库错、服务未配置 |

## 通俗比喻

`application` 就似银行柜员：客户话「我要登录/续证/查资料」，柜员会按固定流程查系统、核身份、出结果，而唔会畀客户直接入金库。