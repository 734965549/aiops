package auth

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/734965549/aiops/pkg/errors"
)

func TestMemoryLoginAttemptLimiterBlocksIPWindow(t *testing.T) {
	limiter := newMemoryLoginAttemptLimiter(LoginRateLimitConfig{
		Enabled:             true,
		IPRequestsPerWindow: 2,
		IPWindowS:           60,
	})
	ctx := context.Background()
	if err := limiter.Allow(ctx, "1.1.1.1", ""); err != nil {
		t.Fatalf("first allow: %v", err)
	}
	if err := limiter.Allow(ctx, "1.1.1.1", ""); err != nil {
		t.Fatalf("second allow: %v", err)
	}
	err := limiter.Allow(ctx, "1.1.1.1", "")
	if err == nil {
		t.Fatal("expected IP window limit")
	}
	var app *apperr.Error
	if !errors.As(err, &app) || app.Code != apperr.CodeResourceExhausted {
		t.Fatalf("expected resource exhausted, got %v", err)
	}
}

func TestMemoryLoginAttemptLimiterLocksUsernameAfterFailures(t *testing.T) {
	limiter := newMemoryLoginAttemptLimiter(LoginRateLimitConfig{
		Enabled:                       true,
		IPRequestsPerWindow:           100,
		UsernameFailuresBeforeLockout: 2,
		LockoutS:                      60,
	})
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := limiter.RecordFailure(ctx, "1.1.1.1", "alice"); err != nil {
			t.Fatalf("record failure: %v", err)
		}
	}
	if err := limiter.Allow(ctx, "1.1.1.1", "alice"); err == nil {
		t.Fatal("expected username lockout")
	}
}

func TestMemoryLoginAttemptLimiterClearsUsernameFailuresOnSuccess(t *testing.T) {
	limiter := newMemoryLoginAttemptLimiter(LoginRateLimitConfig{
		Enabled:                       true,
		IPRequestsPerWindow:           100,
		UsernameFailuresBeforeLockout: 2,
		LockoutS:                      60,
	})
	ctx := context.Background()
	_ = limiter.RecordFailure(ctx, "1.1.1.1", "alice")
	if err := limiter.RecordSuccess(ctx, "1.1.1.1", "alice"); err != nil {
		t.Fatalf("record success: %v", err)
	}
	_ = limiter.RecordFailure(ctx, "1.1.1.1", "alice")
	if err := limiter.Allow(ctx, "1.1.1.1", "alice"); err != nil {
		t.Fatalf("expected allow after single failure post-success, got %v", err)
	}
}

func TestNoopLoginAttemptLimiter(t *testing.T) {
	var limiter LoginAttemptLimiter = NoopLoginAttemptLimiter{}
	ctx := context.Background()
	if err := limiter.Allow(ctx, "1.1.1.1", "alice"); err != nil {
		t.Fatalf("noop allow: %v", err)
	}
}
