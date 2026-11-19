package huawei

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCallWithContextReturnsOnDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	release := make(chan struct{})
	start := time.Now()
	_, err := callWithContext(ctx, func() (string, error) {
		<-release
		return "late", nil
	})
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("callWithContext blocked %s on a hanging call", elapsed)
	}
}

func TestCallWithContextPassesResult(t *testing.T) {
	value, err := callWithContext(context.Background(), func() (string, error) {
		return "ok", nil
	})
	if err != nil || value != "ok" {
		t.Fatalf("value=%q err=%v", value, err)
	}
}

func TestCallWithContextSkipsCallWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	_, err := callWithContext(ctx, func() (string, error) {
		called = true
		return "", nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
	if called {
		t.Fatal("call must not start when ctx is already done")
	}
}
