# configs 模块说明（粤语版）

## 呢个目录做咩

`configs` 放配置样例，等开发、测试、部署时知道要填咩。可以将佢想象成「菜单模板」：店铺未开之前，先睇模板改成自己环境嘅价钱同菜式。

## 调用关系

```text
cmd/api -config 指定配置文件（可选；唔传则 env + 默认值）
  -> pkg/config.Load
  -> internal/bootstrap 使用配置初始化 logger/database/redis
  -> cmd/api 读取 auth.bootstrap_*、ai.providers 等做启动期装配
```

默认容器镜像**唔内置** YAML；`docker-compose.yml` 主文件纯 env 启动。本地 dev 可叠加 `docker-compose.dev.yml` 挂载 `config.example.yaml` 为 `/app/configs/config.yaml`。

## 入参

| 配置块 | 说明 |
| --- | --- |
| `app` | 应用名、环境、时区 |
| `server` | 监听地址、端口、超时 |
| `database` | PostgreSQL 连接资料、是否自动迁移（默认 `false`） |
| `redis` | Redis 地址、密码、库号 |
| `auth` | JWT secret、issuer、token 过期、bootstrap 默认管理员（username/password/display_name） |
| `logger` | 日志级别、格式、输出位置 |
| `cors` | 跨域 origin 列表、是否允许凭证 |
| `ai.providers` | AI 工具提供方列表；启动时载入内存注册表，亦可用 API 维护 |
| `integration` | Integration 接入账号凭据加密密钥与密钥版本 |

## 出参

配置本身唔直接输出嘢，但会影响：

- HTTP 服务监听端口。
- 数据库同 Redis 连接。
- token 签发同过期时间。
- Integration 接入账号 AK/SK、Token 等凭据的加密与解密。
- 日志格式同保存位置。
- 启动期默认管理员同 AI provider 初始列表。

## Integration 凭据加密密钥

`integration.credential_encryption_key` 用于加密 `integration_credential_ref` 中的接入账号凭据，环境变量为 `AIOPS_INTEGRATION__CREDENTIAL_ENCRYPTION_KEY`。从 `aiops-api:1.2` 起，非 dev 环境会在启动阶段校验该值：不能为空，不能使用 `dev-integration-credential-key-change-me` 等占位值，不能与 `auth.jwt_secret` 相同，并需要满足长度、字符多样性和熵要求。

生产和测试集群应通过 Secret / CI 注入该项，不要写入 Git、ConfigMap、镜像层或前端环境变量。生成示例：

```bash
openssl rand -base64 32
```

## AI providers 说明

- `ai.providers` 系 YAML 数组，**唔支持**用单个 env 键整段 JSON 注入。
- 启动时 `cmd/api` 会将配置中嘅 provider 写入内存注册表。
- 若纯 env 启动、无 YAML，可启动后调用 `POST /api/ai/providers` 手动创建（需登录同 `app:ai.providers:write` 权限）。
- 详见 `ops/ai-contract.md` 同 `config.example.yaml` 示例块。

## 通俗比喻

配置文件就似遥控器。程序代码系电视机，遥控器决定开边个频道、声音几大、画面模式点样。
