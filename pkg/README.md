# pkg 通用模块说明（粤语版）

## 呢个目录做咩

`pkg` 系全项目共用嘅「工具箱」。业务模块唔需要重复造轮子，统一喺呢度拎配置、数据库、日志、鉴权、HTTP 响应、错误码等能力。

## 包含模块

- `auth`：JWT、密码 hash、鉴权适配。
- `config`：配置读取同校验。
- `database`：PostgreSQL、Redis、数据库迁移。
- `errors`：统一错误码同业务错误。
- `logger`：zap 日志同 trace_id 支持。
- `pagination`：分页参数。
- `transport/http`：HTTP 响应、中间件、健康检查。

## 调用关系

```text
cmd/api + internal/*
  -> pkg/config
  -> pkg/logger
  -> pkg/database
  -> pkg/auth
  -> pkg/errors
  -> pkg/transport/http
```

## 入参 / 出参

| 工具 | 常见入参 | 常见出参 |
| --- | --- | --- |
| `auth` | 密码、JWT secret、token | hash、claims、token、error |
| `config` | 配置文件路径、环境变量 | `Config`、error |
| `database` | DB/Redis 配置、context | 连接对象、error |
| `errors` | 错误码、message、原始 error | 统一业务错误 |
| `logger` | 日志配置、字段 | logger 实例、日志输出 |
| `transport/http` | gin context、data/error | 统一 JSON 响应 |

## 通俗比喻

`pkg` 就似屋企工具箱：螺丝批、胶纸、电筒都放埋一齐。唔同房间要修嘢，都可以嚟工具箱攞，而唔系每间房自己买一套。