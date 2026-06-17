package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
	"testing"

	"go.uber.org/zap"
)

func resetGlobal() {
	globalLogger.Store(zap.NewNop())
	initialized.Store(false)
}

func assertServiceField(t *testing.T, logLine []byte, want string) {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal(logLine, &fields); err != nil {
		t.Fatalf("unmarshal log line: %v line=%q", err, string(logLine))
	}
	got, ok := fields["service"].(string)
	if !ok || got != want {
		t.Fatalf("expected service=%q in log, got fields=%v", want, fields)
	}
}

func TestReportErrorBeforeInit(t *testing.T) {
	resetGlobal()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
		_ = w.Close()
	}()

	ReportError("migrate failed", errors.New("load config: boom"))

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()

	got := buf.String()
	if got != "migrate failed: load config: boom\n" {
		t.Fatalf("unexpected stderr: %q", got)
	}
}

func TestNew_InjectsServiceNameIntoJSON(t *testing.T) {
	var buf bytes.Buffer
	l, err := New(Options{
		Level:       "info",
		Format:      "json",
		Writer:      &buf,
		ServiceName: "aiops-api",
	})
	if err != nil {
		t.Fatal(err)
	}

	l.Info("hello")
	_ = l.Sync()

	assertServiceField(t, bytes.TrimSpace(buf.Bytes()), "aiops-api")
}

func TestInit_InjectsServiceNameIntoJSON(t *testing.T) {
	resetGlobal()
	defer resetGlobal()

	var buf bytes.Buffer
	if err := Init(Options{
		Level:       "info",
		Format:      "json",
		Writer:      &buf,
		ServiceName: "alert-service",
	}); err != nil {
		t.Fatal(err)
	}

	L().Info("started")
	_ = L().Sync()

	assertServiceField(t, bytes.TrimSpace(buf.Bytes()), "alert-service")
}

func TestReportErrorAfterInit(t *testing.T) {
	resetGlobal()
	defer resetGlobal()

	if err := Init(Options{Level: "error", Format: "console", Output: "stdout", AppEnv: "test"}); err != nil {
		t.Fatal(err)
	}

	// Should not panic; output goes to stdout (not asserted here).
	ReportError("migrate failed", errors.New("ping postgres: down"))
}

func TestGlobalLoggerConcurrentInitAndRead(t *testing.T) {
	resetGlobal()
	defer resetGlobal()

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers * 2)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_ = Init(Options{
				Level:       "info",
				Format:      "json",
				Output:      "stdout",
				ServiceName: "aiops-api",
			})
		}()
		go func() {
			defer wg.Done()
			_ = L().Sync()
			_ = Initialized()
			_ = From(nil)
			With(zap.String("k", "v"))
			ReportError("probe", errors.New("x"))
		}()
	}

	wg.Wait()
}
