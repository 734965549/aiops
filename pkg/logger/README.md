# pkg/logger 模块说明

`logger` 是平台唯一日志门面（zap 封装）。项目宪章要求：业务代码只用 `logger.L()/From(ctx)` 与字段 helper，不直接 import `go.uber.org/zap`，也不引入第二套日志门面。

## 初始化与全局单例

- `New(opt)` 构造独立实例（库/测试注入用，不改全局）；`Init(opt)` 设置进程级全局单例（`atomic.Value`，重复调用会替换）。
- `Options`：`level`（debug/info/warn/error，空缺省 info）、`format`（json/console）、`output`（stdout/stderr/file）、`service_name`（写入固定字段 `service`）、`file_path/max_size_mb/max_backups/max_age_days`（file 模式经 lumberjack 滚动并压缩，路径缺省 `logs/aiops.log`）。
- **dev 环境强制 console 编码 + 彩色 level**（忽略 format=json）。
- 未 `Init` 时全局为 NopLogger，业务调用不会 panic；`Initialized()` 报告是否已初始化。
- `ReportError(message, err)`：已初始化写 Error 日志，否则降级输出 stderr——供 CLI 入口在 bootstrap 前可靠报错（`cmd/api`/`cmd/migrate` 使用）。

## context 注入与字段

- `WithContext(ctx, l)` / `WithContextFields(ctx, fields...)`：把带字段的子 logger 写入标准 context；`From(ctx)` 取出，不存在时回退全局。
- Trace 中间件注入的 logger 自带 `trace_id` 字段，业务侧 `logger.From(ctx).Info(...)` 自动携带。
- 字段 helper：`String/Strings/Int/Int64/Bool/Duration/Time/Any/ByteString/Error`；调用方不直接构造 `zap.Field`。
- `Sync()` 供 main 退出前 flush；`L()` 全局 logger；`With(...)` 追加全局字段。

## 输出约定

- 字段只放业务 ID、trace、状态、计数与脱敏信息；凭据、Token、原始 Authorization header、provider api key、数据库密码不得入日志（项目宪章）。SQL 默认不出原始语句（见 `pkg/database` 的 GORM 日志桥接）。
- 级别语义：正常关键节点 `Info`，可恢复降级/拒绝 `Warn`，系统错误 `Error`，启动致命 `Fatal`。

## 测试

`logger_test.go` 覆盖初始化、级别解析与 context 注入。
