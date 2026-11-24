# pkg/config 模块说明

`config` 用 Viper 加载平台配置：优先级为 **`AIOPS_` 环境变量 > `--config` YAML > 内置默认值**。加载后统一 `normalize()` 与 `Validate()`，校验失败在 bootstrap 最早阶段失败。

## 配置结构（`config.go` + 子文件）

| 配置段 | 来源 | 关键项 |
| --- | --- | --- |
| `app` | config.go | name/env/timezone/`bootstrap_timeout_s`(30) |
| `server` | config.go | host/port(8080)、`read_timeout_s`(15)、`read_header_timeout_s`(5)、`write_timeout_s`(30)、`shutdown_timeout_s`(10)、`max_header_bytes`(1MB)、`max_request_body_bytes`(10MB)、`trusted_proxies` |
| `logger` | config.go | level/format/output/file_path/滚动 |
| `database` | config.go | DSN（含时区）、连接池、`auto_migrate`、`migrate_timeout_s`、`log_level` |
| `redis` | config.go | addr/password/db/pool_size/`required` |
| `auth` | config.go + jwt_secret.go | JWT secret/issuer/TTL、登录限流、IP 白名单 |
| `identity.providers` | config.go | LDAP/OAuth2/OIDC 身份源 |
| `cors` | config.go | allow_origins/methods/credentials |
| `ai` | config.go | providers 种子、证据/回合/流式超时与 payload 上限 |
| `alert_sync` / `execution` / `integration` | alert_sync.go / execution.go / integration.go | CES 拉取 worker、Agent 租约与注册令牌、凭据加密 |

`AIConfig` 访问器统一兜底：`EvidenceTimeout`(5s)、`PostgresEvidenceTimeout`(15s)、`TurnBudgetTimeout`(600s)、`StreamIdleTimeout`(180s，下限 30s)、`EvidencePayloadLimit`(32KB)。

## 校验（`Validate()`）

- `validateLoggerConfig`：level/format 合法。
- `validateCORS`：`allow_credentials=true` 时禁止 `*` origin（CORS 中间件再防御一层）；dev 提供默认 origin。
- `validateIPAllowlist`：网段格式合法。
- `ValidateJWTSecret`（jwt_secret.go）：dev 全放行；**非 dev** 拒绝占位值与已知弱密钥列表，要求 ≥32 字节、≥16 个不同字符、最大重复 run ≤4、香农熵 ≥3.8（字符类 <3 时需熵 ≥4.5）。
- `materializeEnv` 把 `AIOPS_*` 环境变量物化为配置键；`normalizeCORSConfig` 做列表去空格与归一。

## 测试

`config_test.go`/`cors_test.go`/`jwt_secret_test.go`/`integration_test.go` 覆盖默认值、覆盖优先级与校验规则；修改默认值或校验阈值需同步 `ops/*` 契约与 `configs/config.example.yaml`。
