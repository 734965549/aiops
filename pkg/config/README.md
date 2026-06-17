# pkg/config 模块说明（粤语版）

## 呢个模块做咩

`config` 系平台嘅「说明书同调味表」。程序启动之前，要知道监听边个端口、数据库喺边、JWT secret 系咩、日志点写。

## 调用关系

```text
bootstrap.Init
  -> config.Load(configPath)
  -> Config.Validate()
  -> 其他模块读取 cfg.App / cfg.Server / cfg.Database / cfg.Redis / cfg.Auth / cfg.Logger
```

## 入参

| 来源 | 参数 | 说明 |
| --- | --- | --- |
| 配置文件 | YAML | 默认配置，例如端口、数据库、Redis |
| 环境变量 | `AIOPS_*` 或项目约定变量 | 覆盖敏感或环境相关配置 |
| 命令行 | `-config` | 指定配置文件路径 |

## 出参

| 输出 | 说明 |
| --- | --- |
| `Config` | 全项目统一配置对象 |
| `error` | 文件唔存在、格式错、必填项缺失、数值非法 |
| TTL 方法 | access/refresh token 有效期转换成 `time.Duration` |

## 通俗比喻

`config` 就似煲汤食谱：几多水、几多盐、煲几耐都写清楚。唔同环境可以换唔同食谱，开发环境淡啲，生产环境严谨啲。