# pkg 通用模块说明

`pkg` 是全项目共享的基础能力层：只承载与业务无关的原语与契约，业务专属逻辑留在 `internal/<context>`。各包均有独立 README；本文件是索引与跨包约定。

## 包清单

| 包 | 职责 | README |
| --- | --- | --- |
| `auth` | 密码哈希（bcrypt）、JWT 签发/校验、refresh token 会话存储、登录限流、IP 白名单、中间件适配器 | `pkg/auth/README.md` |
| `config` | Viper 配置加载、`AIOPS_` 环境变量覆盖、默认值、校验（JWT secret 强度/CORS/IP 白名单/日志） | `pkg/config/README.md` |
| `database` | PostgreSQL(GORM)/Redis 连接、`BaseModel` 钩子、PG 错误映射、GORM 日志桥接、自研迁移 runner | `pkg/database/README.md` |
| `errors` | 统一错误模型 `apperr.Error` 与 12 个错误码枚举 | `pkg/errors/README.md` |
| `logger` | zap 门面：全局单例、context 注入 trace 字段、文件滚动输出 | `pkg/logger/README.md` |
| `pagination` | 分页参数与 `PageData` 载荷（无独立 README，见下） | — |
| `transport/http` | 统一响应封装、UTF-8 绑定、trace 传递、健康检查结构、敏感路径脱敏 | `pkg/transport/http/README.md` |
| `transport/http/middleware` | Trace/Recovery/RequestLog/CORS/SecurityHeaders/RequestBodyLimit/Auth/Authorize | `pkg/transport/http/middleware/README.md` |

## pagination（无独立 README）

- `Query{page, page_size, keyword, order_by}`：`Normalize()` 修正 page≥1、page_size 缺省 20、上限 100；`Offset()/Limit()` 供 SQL。
- `Result[T]{items,total,page,page_size}` 即契约 `PageData<T>`（`ops/alert-contract.md` §2、`ops/identity-api-contract.md` §5.3），经 `httpx.OK` 写入 `data`。
- 本包无专项单元测试，行为由使用它的模块测试覆盖。

## 跨包约定

- 成功响应固定 `code="OK"`（字符串）、`message="ok"`、`trace_id` 必有（无 Trace 中间件时为空串不省略）；所有业务接口只经 `httpx.OK/Fail/FailWith` 输出。
- 业务错误一律 `apperr.New/Newf/Wrap` 构造；未包装的原始错误经 `FromError` 转为 `INTERNAL` 且对外固定文案 `internal server error`，底层细节只进日志。
- 业务代码只用 `logger.L()/From(ctx)` 与字段 helper，不直接 import zap；trace_id 由 middleware 写入标准 context，跨模块调用必须传递 `context.Context`。
- 迁移只允许 `pkg/database` 自研 runner；`*.down.sql` 仅为人工回滚参考。
- 共享包改动需跑对应包测试（项目宪章：修改 `pkg/*` 时跑或补对应包测试）。
