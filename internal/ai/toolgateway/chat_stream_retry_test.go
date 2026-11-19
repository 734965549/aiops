package toolgateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apperr "github.com/734965549/aiops/pkg/errors"
)

func withFastRetries(t *testing.T) {
	t.Helper()
	oldSleep := chatRetrySleep
	chatRetrySleep = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { chatRetrySleep = oldSleep })
}

func withStreamTimeouts(t *testing.T, firstChunk, idle time.Duration) {
	t.Helper()
	oldFirst := chatStreamFirstChunkTimeout
	oldIdle := chatStreamIdleTimeout
	chatStreamFirstChunkTimeout = firstChunk
	chatStreamIdleTimeout = idle
	t.Cleanup(func() {
		chatStreamFirstChunkTimeout = oldFirst
		chatStreamIdleTimeout = oldIdle
	})
}

func writeSSE(w http.ResponseWriter, lines ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, line := range lines {
		_, _ = io.WriteString(w, line+"\n\n")
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func TestCompleteStreamRetriesOnFirstChunkTimeout(t *testing.T) {
	withFastRetries(t)
	withStreamTimeouts(t, 60*time.Millisecond, 2*time.Second)

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			// 不写响应头，等待客户端首块超时后放弃；用有限睡眠避免 httptest 关闭挂起。
			time.Sleep(300 * time.Millisecond)
			return
		}
		writeSSE(w,
			`data: {"choices":[{"delta":{"content":"ok"}}]}`,
			`data: [DONE]`,
		)
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	result, err := registry.CompleteStream(context.Background(), "ark", "sys", "user", func(string, string) error { return nil })
	if err != nil {
		t.Fatalf("complete stream: %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
	if result.Content != "ok" {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestCompleteStreamIdleTimeoutRetainsPartial(t *testing.T) {
	withFastRetries(t)
	withStreamTimeouts(t, 2*time.Second, 60*time.Millisecond)

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		writeSSE(w, `data: {"choices":[{"delta":{"content":"部分"}}]}`)
		// 发出部分内容后保持连接；客户端空闲超时后返回 Partial，服务端有限睡眠后退出。
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	result, err := registry.CompleteStream(context.Background(), "ark", "sys", "user", func(string, string) error { return nil })
	if err != nil {
		t.Fatalf("complete stream: %v", err)
	}
	if !result.Partial || result.PartialReason != "stream_idle_timeout" {
		t.Fatalf("result = %+v", result)
	}
	if result.Content != "部分" {
		t.Fatalf("content = %q", result.Content)
	}
	if attempts.Load() != 1 {
		t.Fatalf("partial stream must not retry, attempts = %d", attempts.Load())
	}
}

func TestCompleteStreamRetriesOn429WithRetryAfter(t *testing.T) {
	withFastRetries(t)
	var retryDelay time.Duration
	oldSleep := chatRetrySleep
	chatRetrySleep = func(_ context.Context, delay time.Duration) error {
		retryDelay = delay
		return nil
	}
	t.Cleanup(func() { chatRetrySleep = oldSleep })

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeSSE(w,
			`data: {"choices":[{"delta":{"content":"ok"}}]}`,
			`data: [DONE]`,
		)
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	result, err := registry.CompleteStream(context.Background(), "ark", "sys", "user", nil)
	if err != nil {
		t.Fatalf("complete stream: %v", err)
	}
	if attempts.Load() != 2 || result.Content != "ok" {
		t.Fatalf("attempts=%d result=%+v", attempts.Load(), result)
	}
	if retryDelay < 2*time.Second {
		t.Fatalf("Retry-After not honored: %v", retryDelay)
	}
}

func TestCompleteStreamDoesNotRetryNonRetryableStatus(t *testing.T) {
	withFastRetries(t)
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	_, err := registry.CompleteStream(context.Background(), "ark", "sys", "user", nil)
	if err == nil || apperr.CodeOf(err) != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("400 must not retry, attempts = %d", attempts.Load())
	}
}

func TestCompleteStreamEmptyResponseRetriesThenFails(t *testing.T) {
	withFastRetries(t)
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		writeSSE(w, `data: [DONE]`)
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	_, err := registry.CompleteStream(context.Background(), "ark", "sys", "user", nil)
	if err == nil || apperr.CodeOf(err) != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	if attempts.Load() != chatStreamMaxAttempts {
		t.Fatalf("attempts = %d, want %d", attempts.Load(), chatStreamMaxAttempts)
	}
}

func TestCompleteRetriesOn5xxWithBackoff(t *testing.T) {
	withFastRetries(t)
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(chatCompletionResponse(`{"summary":"ok"}`))
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	result, err := registry.Complete(context.Background(), "ark", "sys", "user")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if attempts.Load() != 2 || result.Content != `{"summary":"ok"}` {
		t.Fatalf("attempts=%d result=%+v", attempts.Load(), result)
	}
}

func TestParseQuotaResetHint(t *testing.T) {
	cases := map[string]time.Duration{
		"quota resets in 5h":         5 * time.Hour,
		"please try again in 30m":    30 * time.Minute,
		"quotaResetDelay: 1h2m":      time.Hour,
		"rate limit exceeded":        0,
		"resets in 45 seconds":       45 * time.Second,
		"no numbers in this message": 0,
	}
	for raw, want := range cases {
		if got := parseQuotaResetHint(raw); got != want {
			t.Fatalf("parseQuotaResetHint(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestCompleteStreamRateLimitExhaustedCarriesResetHint(t *testing.T) {
	withFastRetries(t)
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "1")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"quota resets in 2h"}}`)
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	_, err := registry.CompleteStream(context.Background(), "ark", "sys", "user", nil)
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
	if rateErr.StatusCode != http.StatusTooManyRequests || rateErr.RetryAfter != time.Second || rateErr.ResetAfter != 2*time.Hour {
		t.Fatalf("rate error = %+v", rateErr)
	}
	if attempts.Load() != chatStreamMaxAttempts {
		t.Fatalf("attempts = %d, want %d", attempts.Load(), chatStreamMaxAttempts)
	}
}

func TestCompleteRateLimitCarriesResetHint(t *testing.T) {
	withFastRetries(t)
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"quota resets in 2h"}}`)
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	_, err := registry.Complete(context.Background(), "ark", "sys", "user")
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) || rateErr.ResetAfter != 2*time.Hour {
		t.Fatalf("expected ResetAfter=2h, got %+v (%v)", rateErr, err)
	}
	if attempts.Load() != chatMaxAttempts {
		t.Fatalf("attempts = %d, want %d", attempts.Load(), chatMaxAttempts)
	}
}

func TestParseRetryAfterDuration(t *testing.T) {
	if got := parseRetryAfterDuration("5"); got != 5*time.Second {
		t.Fatalf("seconds = %v", got)
	}
	if got := parseRetryAfterDuration(""); got != 0 {
		t.Fatalf("empty = %v", got)
	}
	if got := parseRetryAfterDuration("not-a-date"); got != 0 {
		t.Fatalf("garbage = %v", got)
	}
	future := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfterDuration(future); got <= 0 || got > 31*time.Second {
		t.Fatalf("http date = %v", got)
	}
}

func TestChatStreamRetryDelayGrowth(t *testing.T) {
	first := chatRetryDelay(0)
	second := chatRetryDelay(1)
	if first < time.Second || first >= 2*time.Second {
		t.Fatalf("first delay = %v", first)
	}
	if second < 2*time.Second || second >= 4*time.Second {
		t.Fatalf("second delay = %v", second)
	}
	if !strings.Contains(first.String(), "s") {
		t.Fatalf("unexpected delay format: %v", first)
	}
}
