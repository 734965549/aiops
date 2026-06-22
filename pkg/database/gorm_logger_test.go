package database

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/734965549/aiops/pkg/logger"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestGormLoggerTraceUsesProjectLoggerWithContext(t *testing.T) {
	var buf bytes.Buffer
	l, err := logger.New(logger.Options{
		Level:       "warn",
		Format:      "json",
		Writer:      &buf,
		ServiceName: "aiops-test",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := logger.WithContext(context.Background(), l.With(logger.String("trace_id", "trace-1")))
	gl := newGormLogger(gormlogger.Warn)
	gl.Trace(ctx, time.Now().Add(-defaultSlowQueryThreshold-time.Millisecond), func() (string, int64) {
		return "select * from iam_identity_provider where api_key = 'secret'", 1
	}, nil)
	_ = l.Sync()

	var fields map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &fields); err != nil {
		t.Fatalf("unmarshal log line: %v line=%q", err, buf.String())
	}
	if got := fields["level"]; got != "warn" {
		t.Fatalf("expected warn level, got %v", got)
	}
	if got := fields["trace_id"]; got != "trace-1" {
		t.Fatalf("expected trace_id, got %v", got)
	}
	if got := fields["component"]; got != "gorm" {
		t.Fatalf("expected component=gorm, got %v", got)
	}
	if _, ok := fields["sql"]; ok {
		t.Fatal("warn/error GORM logs must not include raw SQL")
	}
}

func TestGormLoggerTraceIgnoresRecordNotFound(t *testing.T) {
	var buf bytes.Buffer
	l, err := logger.New(logger.Options{Level: "error", Format: "json", Writer: &buf})
	if err != nil {
		t.Fatal(err)
	}
	ctx := logger.WithContext(context.Background(), l)

	called := false
	gl := newGormLogger(gormlogger.Error)
	gl.Trace(ctx, time.Now(), func() (string, int64) {
		called = true
		return "select 1", 0
	}, gorm.ErrRecordNotFound)

	if called {
		t.Fatal("record-not-found path should not evaluate SQL callback")
	}
	if got := strings.TrimSpace(buf.String()); got != "" {
		t.Fatalf("expected no log for record not found, got %q", got)
	}
}
