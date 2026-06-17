package webhookidempotency

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var errWaitTimeout = errors.New("timeout waiting for webhook idempotency result")

// resolveWaitDeadline 优先使用 ctx deadline；否则用 cfg.DefaultMaxWait 兜底。
func resolveWaitDeadline(ctx context.Context, cfg Config) time.Time {
	if dl, ok := ctx.Deadline(); ok {
		return dl
	}
	return time.Now().Add(cfg.maxWaitDuration())
}

// waitForPoll 在下一轮轮询前等待；ctx 取消或超过 deadline 时返回错误。
func waitForPoll(ctx context.Context, deadline time.Time, ticker *time.Ticker) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("waiting for webhook idempotency result: %w", ctx.Err())
	case <-ticker.C:
		if time.Now().After(deadline) {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("waiting for webhook idempotency result: %w", err)
			}
			return errWaitTimeout
		}
		return nil
	}
}
