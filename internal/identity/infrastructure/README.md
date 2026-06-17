# identity/infrastructure 模块说明（粤语版）

## 呢个模块做咩

`infrastructure` 系 Identity 嘅「水电煤同仓库」。业务规则唔写喺呢度，呢度主要负责接驳外部世界，例如数据库、缓存、第三方接口。

目前主要有：

- `persistence`：用 GORM 实现用户仓储与权限模型查询仓储，读写 PostgreSQL。
- 已覆盖 Identity 核心表：`iam_user`、`iam_role`、`iam_permission`、`iam_user_role`、`iam_role_permission`、`iam_data_scope`、`iam_role_data_scope`、`iam_ai_tool_permission`、`iam_role_ai_tool_permission`。

## 调用关系

```text
application 层
  -> domain.UserRepository / AccessControlRepository 接口
  -> infrastructure/persistence.UserRepository / AccessControlRepository 实现
  -> GORM
  -> PostgreSQL iam_user / iam_role / iam_permission / iam_data_scope / iam_ai_tool_permission 等表
```

## 入参

| 方法 | 入参 | 说明 |
| --- | --- | --- |
| `NewUserRepository` | `*gorm.DB` | 数据库连接 |
| `FindByID` | `ctx`、`id` | 按 ID 查用户 |
| `FindByUsername` | `ctx`、`username` | 按用户名查用户 |
| `Create` | `ctx`、`*domain.User` | 新增用户 |
| `NewAccessControlRepository` | `*gorm.DB` | 构造权限模型查询仓储 |
| `ListRoles` / `CountRoles` | `ctx`、`RoleFilter` | 查询角色列表与总数 |
| `ListPermissions` / `CountPermissions` | `ctx`、`PermissionFilter` | 查询权限列表与总数 |
| `ListUserRoles` / `ListRolePermissions` | `ctx`、业务 ID | 查询用户角色与角色权限关联 |
| `ListDataScopes` / `ListRoleDataScopes` | `ctx`、过滤器或角色 ID | 查询数据范围定义与角色数据范围 |
| `ListAIToolPermissions` / `CountAIToolPermissions` | `ctx`、`AIToolPermissionFilter` | 查询 AI 工具权限列表与总数 |
| `ListRoleAIToolPermissions` | `ctx`、`roleID` | 查询角色绑定 AI 工具权限 |

## 出参

| 输出 | 说明 |
| --- | --- |
| `domain.User` | 将数据库 model 转返领域对象 |
| `nil` | 查唔到时返回 nil 用户 |
| `error` | 数据库连接、SQL、唯一约束等错误 |

## 通俗比喻

如果 `application` 系柜员，`infrastructure` 就系柜员背后用嘅电脑系统同资料仓。柜员讲「查呢个会员」，电脑系统负责真真正正入数据库搵。