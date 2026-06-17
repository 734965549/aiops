# identity/domain 模块说明（粤语版）

## 呢个模块做咩

`domain` 系 Identity 嘅「规则核心」。佢唔关心 HTTP 点传、数据库点查，只关心用户、角色、权限、数据范围、AI 工具权限本身应该系点：有咩字段、咩状态算可用、业务层要用咩仓储接口。

## 调用关系

```text
application.AuthService / UserService / AccessControlService
  -> domain.UserRepository / AccessControlRepository 接口
  -> domain.User 实体方法，例如 IsActive()
  -> domain.Role / Permission / DataScope / AIToolPermission 权限模型
```

真正查数据库嘅代码喺 `infrastructure/persistence`，但 application 只认 `domain.UserRepository`，就好似老板只讲「帮我查会员」，唔需要知道员工系翻纸簿定查电脑。

## 入参

| 对象 | 入参 | 说明 |
| --- | --- | --- |
| `FindByID` | `ctx`、`id` | 按用户 ID 查用户 |
| `FindByUsername` | `ctx`、`username` | 按用户名查用户 |
| `Create` | `ctx`、`*User` | 创建用户 |
| `User.IsActive` | 无 | 判断用户状态是否 active |
| `ListRoles` / `CountRoles` | `ctx`、`RoleFilter` | 查询角色列表与总数 |
| `ListPermissions` / `CountPermissions` | `ctx`、`PermissionFilter` | 查询权限列表与总数 |
| `ListUserRoles` | `ctx`、`userID` | 查询用户绑定角色 |
| `ListRolePermissions` | `ctx`、`roleID` | 查询角色绑定权限 |
| `ListDataScopes` / `ListRoleDataScopes` | `ctx`、过滤器或 `roleID` | 查询数据范围定义与角色数据范围 |
| `ListAIToolPermissions` / `CountAIToolPermissions` | `ctx`、`AIToolPermissionFilter` | 查询 AI 工具权限列表与总数 |
| `ListRoleAIToolPermissions` | `ctx`、`roleID` | 查询角色绑定 AI 工具权限 |

## 出参

| 输出 | 说明 |
| --- | --- |
| `*User` | 用户领域对象，可能为 nil |
| `error` | 仓储错误、数据库错误会一路传返上层 |
| `bool` | `IsActive()` 返回账号是否可用 |

## 通俗比喻

`domain` 就似会员制度嘅章程：会员有会员号、姓名、状态；停用会员唔可以入场。至于会员资料放喺 Excel 定数据库，章程唔理。