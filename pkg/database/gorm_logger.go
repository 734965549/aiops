package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/734965549/aiops/pkg/logger"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const defaultSlowQueryThreshold = 200 * time.Millisecond

type gormZapLogger struct {
	level         gormlogger.LogLevel
	slowThreshold time.Duration
}

func newGormLogger(level gormlogger.LogLevel) gormlogger.Interface {
	return gormZapLogger{
		level:         level,
		slowThreshold: defaultSlowQueryThreshold,
	}
}

func (l gormZapLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	l.level = level
	return l
}

func (l gormZapLogger) Info(ctx context.Context, msg string, args ...any) {
	if l.level < gormlogger.Info {
		return
	}
	logger.From(ctx).Info("gorm info", logger.String("component", "gorm"), logger.String("message", fmt.Sprintf(msg, args...)))
}

func (l gormZapLogger) Warn(ctx context.Context, msg string, args ...any) {
	if l.level < gormlogger.Warn {
		return
	}
	logger.From(ctx).Warn("gorm warning", logger.String("component", "gorm"), logger.String("message", fmt.Sprintf(msg, args...)))
}

func (l gormZapLogger) Error(ctx context.Context, msg string, args ...any) {
	if l.level < gormlogger.Error {
		return
	}
	logger.From(ctx).Error("gorm error", logger.String("component", "gorm"), logger.String("message", fmt.Sprintf(msg, args...)))
}

func (l gormZapLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	baseFields := []logger.Field{
		logger.String("component", "gorm"),
		logger.Duration("elapsed", elapsed),
	}

	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound) && l.level >= gormlogger.Error:
		logger.From(ctx).Error("gorm query failed", append(baseFields, logger.Error(err))...)
	case l.slowThreshold > 0 && elapsed > l.slowThreshold && l.level >= gormlogger.Warn:
		logger.From(ctx).Warn("gorm slow query", append(baseFields, logger.Duration("slow_threshold", l.slowThreshold))...)
	case l.level >= gormlogger.Info:
		sql, rows := fc()
		fields := append(baseFields, logger.String("sql", sql))
		if rows >= 0 {
			fields = append(fields, logger.Int64("rows", rows))
		}
		logger.From(ctx).Info("gorm query", fields...)
	}
}
