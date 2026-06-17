// Package logger 提供平台统一的 zap 日志门面。
//
// 设计要点：
//   - New 构造独立实例（库/测试注入）；Init 设置进程级全局单例（atomic 并发安全），业务侧用 logger.L() 获取。
//   - ServiceName 写入固定字段 service，便于集中日志区分微服务。
//   - context 注入：中间件可在 ctx 中放入带 trace_id 的子 logger，
//     业务通过 logger.From(ctx) 获取后续日志会自动带上 trace_id。
//   - 后续接入集中式日志（Loki / 华为云 LTS）时，只需在此包替换 Core，
//     上层调用方无需改动。
package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Options 控制 logger 初始化。
type Options struct {
	Level       string // debug/info/warn/error
	Format      string // json/console
	Output      string // stdout / stderr / file
	AppEnv      string // dev/test/prod，用于在 dev 环境强制 console 格式
	ServiceName string // 微服务标识，写入固定字段 service（如 aiops-api、alert-service）
	FilePath    string
	MaxSizeMB   int
	MaxBackups  int
	MaxAgeDays  int
	// Writer 非空时写入此目标，忽略 Output；供测试或自定义 sink 注入。
	Writer io.Writer
}

var (
	globalLogger atomic.Value // *zap.Logger
	initialized  atomic.Bool
)

func init() {
	globalLogger.Store(zap.NewNop())
}

func loadGlobal() *zap.Logger {
	return globalLogger.Load().(*zap.Logger)
}

// New 根据配置构造独立 logger，适合库侧注入或测试；不修改全局单例。
func New(opt Options) (*zap.Logger, error) {
	level := zap.InfoLevel
	if err := level.UnmarshalText([]byte(opt.Level)); err != nil && opt.Level != "" {
		return nil, fmt.Errorf("parse log level %q: %w", opt.Level, err)
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	encCfg.EncodeDuration = zapcore.MillisDurationEncoder

	format := opt.Format
	if format == "" {
		format = "json"
	}
	if opt.AppEnv == "dev" {
		// 本地开发更友好：console 编码 + 彩色 level。
		format = "console"
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	var encoder zapcore.Encoder
	switch format {
	case "console":
		encoder = zapcore.NewConsoleEncoder(encCfg)
	default:
		encoder = zapcore.NewJSONEncoder(encCfg)
	}

	ws, err := openWriter(opt)
	if err != nil {
		return nil, err
	}

	core := zapcore.NewCore(encoder, ws, level)
	l := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	if name := opt.ServiceName; name != "" {
		l = l.With(zap.String("service", name))
	}
	return l, nil
}

// Init 根据配置构造全局 logger。重复调用会替换全局实例。
func Init(opt Options) error {
	l, err := New(opt)
	if err != nil {
		return err
	}
	globalLogger.Store(l)
	initialized.Store(true)
	return nil
}

func openWriter(opt Options) (zapcore.WriteSyncer, error) {
	if opt.Writer != nil {
		return zapcore.AddSync(opt.Writer), nil
	}
	if opt.Output == "" || opt.Output == "stdout" {
		return zapcore.AddSync(os.Stdout), nil
	}
	if opt.Output == "stderr" {
		return zapcore.AddSync(os.Stderr), nil
	}

	if opt.Output == "file" {
		if opt.FilePath == "" {
			opt.FilePath = "logs/aiops.log"
		}
		l := &lumberjack.Logger{
			Filename:   opt.FilePath,
			MaxSize:    opt.MaxSizeMB,
			MaxBackups: opt.MaxBackups,
			MaxAge:     opt.MaxAgeDays,
			Compress:   true,
		}
		return zapcore.AddSync(l), nil
	}

	// 兼容老代码传文件路径作为 Output 的情况
	f, err := os.OpenFile(opt.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", opt.Output, err)
	}
	return zapcore.AddSync(f), nil
}

// L 返回全局 logger。未 Init 时返回 NopLogger，避免业务侧 panic。
func L() *zap.Logger { return loadGlobal() }

// Initialized 报告全局 logger 是否已通过 Init 成功构造。
func Initialized() bool { return initialized.Load() }

// ReportError 写入 logger；若尚未 Init（仍是 NopLogger）则降级到 stderr，供 CLI 入口在 bootstrap 前可靠输出错误。
func ReportError(message string, err error) {
	if err == nil {
		return
	}
	if initialized.Load() {
		loadGlobal().Error(message, zap.Error(err))
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
}

// With 在全局 logger 基础上追加字段。
func With(fields ...zap.Field) *zap.Logger { return loadGlobal().With(fields...) }

// Sync 在程序退出前 flush 日志缓冲。建议在 main 中 defer 调用。
func Sync() { _ = loadGlobal().Sync() }

type ctxKey struct{}

// WithContext 把 logger 写入 ctx，供 From 取出。
func WithContext(ctx context.Context, l *zap.Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKey{}, l)
}

// From 从 ctx 取出 logger；若不存在则返回全局 logger。
func From(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return loadGlobal()
	}
	if l, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok && l != nil {
		return l
	}
	return loadGlobal()
}
