package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/redis/go-redis/v9"
)

// LoginRateLimitConfig 控制登录/刷新接口的 IP 与用户名维度限流。
type LoginRateLimitConfig struct {
	Enabled                       bool
	IPRequestsPerWindow           int
	IPWindowS                     int
	IPFailuresBeforeLockout       int
	UsernameFailuresBeforeLockout int
	LockoutS                      int
}

func (c LoginRateLimitConfig) normalized() LoginRateLimitConfig {
	out := c
	if out.IPRequestsPerWindow <= 0 {
		out.IPRequestsPerWindow = 30
	}
	if out.IPWindowS <= 0 {
		out.IPWindowS = 60
	}
	if out.IPFailuresBeforeLockout <= 0 {
		out.IPFailuresBeforeLockout = 20
	}
	if out.UsernameFailuresBeforeLockout <= 0 {
		out.UsernameFailuresBeforeLockout = 5
	}
	if out.LockoutS <= 0 {
		out.LockoutS = 900
	}
	return out
}

// LoginAttemptLimiter 在登录/刷新前检查是否被限流，并在认证失败后累计计数。
type LoginAttemptLimiter interface {
	Allow(ctx context.Context, ip, username string) error
	RecordFailure(ctx context.Context, ip, username string) error
	RecordSuccess(ctx context.Context, ip, username string) error
}

// NoopLoginAttemptLimiter 不做限流。
type NoopLoginAttemptLimiter struct{}

func (NoopLoginAttemptLimiter) Allow(context.Context, string, string) error         { return nil }
func (NoopLoginAttemptLimiter) RecordFailure(context.Context, string, string) error { return nil }
func (NoopLoginAttemptLimiter) RecordSuccess(context.Context, string, string) error { return nil }

// NewLoginAttemptLimiter 按配置构造限流器；未启用或 client 为 nil 时返回 Noop。
func NewLoginAttemptLimiter(cfg LoginRateLimitConfig, client *redis.Client) LoginAttemptLimiter {
	if !cfg.Enabled {
		return NoopLoginAttemptLimiter{}
	}
	cfg = cfg.normalized()
	if client != nil {
		return &redisLoginAttemptLimiter{cfg: cfg, client: client}
	}
	return newMemoryLoginAttemptLimiter(cfg)
}

type redisLoginAttemptLimiter struct {
	cfg    LoginRateLimitConfig
	client *redis.Client
}

func (l *redisLoginAttemptLimiter) Allow(ctx context.Context, ip, username string) error {
	if err := l.checkLock(ctx, ipLockKey(ip)); err != nil {
		return err
	}
	if username = normalizeLoginUsername(username); username != "" {
		if err := l.checkLock(ctx, usernameLockKey(username)); err != nil {
			return err
		}
	}
	key := ipWindowKey(ip)
	n, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "login rate limit check failed")
	}
	if n == 1 {
		_ = l.client.Expire(ctx, key, time.Duration(l.cfg.IPWindowS)*time.Second).Err()
	}
	if int(n) > l.cfg.IPRequestsPerWindow {
		return apperr.New(apperr.CodeResourceExhausted, "too many login attempts from this IP")
	}
	return nil
}

func (l *redisLoginAttemptLimiter) RecordFailure(ctx context.Context, ip, username string) error {
	if err := l.incrFailure(ctx, ipFailKey(ip), ipLockKey(ip)); err != nil {
		return err
	}
	if username = normalizeLoginUsername(username); username != "" {
		if err := l.incrFailure(ctx, usernameFailKey(username), usernameLockKey(username)); err != nil {
			return err
		}
	}
	return nil
}

func (l *redisLoginAttemptLimiter) RecordSuccess(ctx context.Context, ip, username string) error {
	if username = normalizeLoginUsername(username); username != "" {
		_ = l.client.Del(ctx, usernameFailKey(username)).Err()
	}
	return nil
}

func (l *redisLoginAttemptLimiter) checkLock(ctx context.Context, key string) error {
	ok, err := l.client.Exists(ctx, key).Result()
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "login rate limit check failed")
	}
	if ok > 0 {
		return apperr.New(apperr.CodeResourceExhausted, "login temporarily blocked due to repeated failures")
	}
	return nil
}

func (l *redisLoginAttemptLimiter) incrFailure(ctx context.Context, counterKey, lockKey string) error {
	n, err := l.client.Incr(ctx, counterKey).Result()
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "login failure counter update failed")
	}
	if n == 1 {
		_ = l.client.Expire(ctx, counterKey, time.Duration(l.cfg.LockoutS)*time.Second).Err()
	}
	threshold := l.cfg.IPFailuresBeforeLockout
	if strings.HasPrefix(counterKey, "auth:login:fail:user:") {
		threshold = l.cfg.UsernameFailuresBeforeLockout
	}
	if int(n) >= threshold {
		_ = l.client.Set(ctx, lockKey, "1", time.Duration(l.cfg.LockoutS)*time.Second).Err()
	}
	return nil
}

type memoryLoginAttemptLimiter struct {
	cfg LoginRateLimitConfig
	mu  sync.Mutex
	// ipWindow: ip -> window start + count
	ipWindow map[string]ipWindowState
	failures map[string]failureState
	locks    map[string]time.Time
}

type ipWindowState struct {
	start time.Time
	count int
}

type failureState struct {
	count int
	start time.Time
}

func newMemoryLoginAttemptLimiter(cfg LoginRateLimitConfig) *memoryLoginAttemptLimiter {
	cfg = cfg.normalized()
	return &memoryLoginAttemptLimiter{
		cfg:      cfg,
		ipWindow: make(map[string]ipWindowState),
		failures: make(map[string]failureState),
		locks:    make(map[string]time.Time),
	}
}

func (l *memoryLoginAttemptLimiter) Allow(_ context.Context, ip, username string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.purgeExpired(now)
	if l.isLocked(now, ipLockKey(ip)) {
		return apperr.New(apperr.CodeResourceExhausted, "login temporarily blocked due to repeated failures")
	}
	if username = normalizeLoginUsername(username); username != "" {
		if l.isLocked(now, usernameLockKey(username)) {
			return apperr.New(apperr.CodeResourceExhausted, "login temporarily blocked due to repeated failures")
		}
	}
	window := l.ipWindow[ip]
	windowDur := time.Duration(l.cfg.IPWindowS) * time.Second
	if window.start.IsZero() || now.Sub(window.start) >= windowDur {
		window = ipWindowState{start: now, count: 0}
	}
	window.count++
	l.ipWindow[ip] = window
	if window.count > l.cfg.IPRequestsPerWindow {
		return apperr.New(apperr.CodeResourceExhausted, "too many login attempts from this IP")
	}
	return nil
}

func (l *memoryLoginAttemptLimiter) RecordFailure(_ context.Context, ip, username string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.purgeExpired(now)
	l.bumpFailure(now, ipFailKey(ip), ipLockKey(ip), l.cfg.IPFailuresBeforeLockout)
	if username = normalizeLoginUsername(username); username != "" {
		l.bumpFailure(now, usernameFailKey(username), usernameLockKey(username), l.cfg.UsernameFailuresBeforeLockout)
	}
	return nil
}

func (l *memoryLoginAttemptLimiter) RecordSuccess(_ context.Context, _, username string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if username = normalizeLoginUsername(username); username != "" {
		delete(l.failures, usernameFailKey(username))
	}
	return nil
}

func (l *memoryLoginAttemptLimiter) purgeExpired(now time.Time) {
	for key, until := range l.locks {
		if !now.Before(until) {
			delete(l.locks, key)
		}
	}
	lockout := time.Duration(l.cfg.LockoutS) * time.Second
	for key, st := range l.failures {
		if !st.start.IsZero() && now.Sub(st.start) >= lockout {
			delete(l.failures, key)
		}
	}
	windowDur := time.Duration(l.cfg.IPWindowS) * time.Second
	for ip, st := range l.ipWindow {
		if !st.start.IsZero() && now.Sub(st.start) >= windowDur {
			delete(l.ipWindow, ip)
		}
	}
}

func (l *memoryLoginAttemptLimiter) isLocked(now time.Time, key string) bool {
	until, ok := l.locks[key]
	return ok && now.Before(until)
}

func (l *memoryLoginAttemptLimiter) bumpFailure(now time.Time, counterKey, lockKey string, threshold int) {
	lockout := time.Duration(l.cfg.LockoutS) * time.Second
	st := l.failures[counterKey]
	if st.start.IsZero() || now.Sub(st.start) >= lockout {
		st = failureState{start: now, count: 0}
	}
	st.count++
	l.failures[counterKey] = st
	if st.count >= threshold {
		l.locks[lockKey] = now.Add(lockout)
	}
}

func normalizeLoginUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func ipWindowKey(ip string) string    { return fmt.Sprintf("auth:login:ip:%s:window", ip) }
func ipFailKey(ip string) string      { return fmt.Sprintf("auth:login:fail:ip:%s", ip) }
func ipLockKey(ip string) string      { return fmt.Sprintf("auth:login:lock:ip:%s", ip) }
func usernameFailKey(u string) string { return fmt.Sprintf("auth:login:fail:user:%s", u) }
func usernameLockKey(u string) string { return fmt.Sprintf("auth:login:lock:user:%s", u) }
