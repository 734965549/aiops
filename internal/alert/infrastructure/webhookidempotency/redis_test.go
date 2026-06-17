package webhookidempotency_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/alert/infrastructure/webhookidempotency"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedisStore(t *testing.T, cfg webhookidempotency.Config) (*miniredis.Miniredis, *webhookidempotency.RedisStore) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return mr, webhookidempotency.NewRedisStore(client, cfg)
}

func testRedisKey(t *testing.T, logicalKey string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(logicalKey))
	return "aiops:alert:webhook_idem:" + hex.EncodeToString(sum[:])
}

func TestRedisStore_ConcurrentDo(t *testing.T) {
	mr, store := newTestRedisStore(t, webhookidempotency.Config{})
	defer mr.Close()

	const workers = 16
	var runs int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			raw, err := store.Do(context.Background(), "src\x00req-redis", time.Minute, func() ([]byte, error) {
				atomic.AddInt32(&runs, 1)
				return []byte(`{"created":1}`), nil
			})
			if err != nil {
				t.Errorf("do: %v", err)
				return
			}
			if string(raw) != `{"created":1}` {
				t.Errorf("unexpected result %q", string(raw))
			}
		}()
	}
	wg.Wait()
	if runs != 1 {
		t.Fatalf("expected fn to run once across concurrent callers, got %d", runs)
	}
}

func TestRedisStore_ReplayCached(t *testing.T) {
	mr, store := newTestRedisStore(t, webhookidempotency.Config{})
	defer mr.Close()
	key := "src\x00req-replay"

	first, err := store.Do(context.Background(), key, time.Minute, func() ([]byte, error) {
		return []byte(`{"created":1}`), nil
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := store.Do(context.Background(), key, time.Minute, func() ([]byte, error) {
		t.Fatal("fn should not run on replay")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("replay mismatch: %q vs %q", first, second)
	}
}

func TestRedisStore_SlowLeaderSecondWaitsForResult(t *testing.T) {
	// 首个 fn 慢于旧版 6s 固定窗口时，第二个同 key 请求应跟 ctx 等到结果，而非提前 timeout。
	mr, store := newTestRedisStore(t, webhookidempotency.Config{
		PollInterval: 20 * time.Millisecond,
	})
	defer mr.Close()

	key := "src\x00req-slow"
	leaderStarted := make(chan struct{})
	const leaderDelay = 8 * time.Second
	want := []byte(`{"created":1}`)

	var wg sync.WaitGroup
	wg.Add(2)
	errCh := make(chan error, 2)

	go func() {
		defer wg.Done()
		_, err := store.Do(context.Background(), key, time.Minute, func() ([]byte, error) {
			close(leaderStarted)
			time.Sleep(leaderDelay)
			return want, nil
		})
		if err != nil {
			errCh <- fmt.Errorf("leader: %w", err)
		}
	}()

	go func() {
		defer wg.Done()
		<-leaderStarted
		waitCtx, cancel := context.WithTimeout(context.Background(), leaderDelay+5*time.Second)
		defer cancel()
		got, err := store.Do(waitCtx, key, time.Minute, func() ([]byte, error) {
			return nil, fmt.Errorf("follower fn should not run")
		})
		if err != nil {
			errCh <- fmt.Errorf("follower wait failed (should outlive old 6s cap): %w", err)
			return
		}
		if string(got) != string(want) {
			errCh <- fmt.Errorf("follower result=%q want=%q", got, want)
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestRedisStore_StoreResultFailureReturnsErrorAndShortensMarker(t *testing.T) {
	mr, store := newTestRedisStore(t, webhookidempotency.Config{
		FailedStoreMarkerTTL: 150 * time.Millisecond,
	})
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	key := "src\x00req-store-fail"
	want := []byte(`{"created":1}`)
	var runs int32

	first, err := store.Do(context.Background(), key, time.Minute, func() ([]byte, error) {
		atomic.AddInt32(&runs, 1)
		mr.SetError("simulated redis set failure")
		return want, nil
	})
	if apperr.FromError(err).Code != apperr.CodeUnavailable {
		t.Fatalf("expected UNAVAILABLE, got %v", apperr.FromError(err).Code)
	}
	if first != nil {
		t.Fatalf("expected nil result on cache failure, got %q", first)
	}
	if runs != 1 {
		t.Fatalf("expected fn once on first attempt, got %d", runs)
	}

	mr.SetError("")
	if err := client.Del(context.Background(), testRedisKey(t, key)).Err(); err != nil {
		t.Fatalf("clear processing marker after cache failure: %v", err)
	}

	second, err := store.Do(context.Background(), key, time.Minute, func() ([]byte, error) {
		atomic.AddInt32(&runs, 1)
		return want, nil
	})
	if err != nil {
		t.Fatalf("retry after marker expiry: %v", err)
	}
	if string(second) != string(want) {
		t.Fatalf("retry result=%q want=%q", second, want)
	}
	if runs != 2 {
		t.Fatalf("expected fn twice after marker expiry without cached result, got %d", runs)
	}
}

func TestRedisStore_StoreResultUsesIndependentTimeoutAfterSlowFn(t *testing.T) {
	mr, store := newTestRedisStore(t, webhookidempotency.Config{})
	defer mr.Close()

	key := "src\x00req-expired-ctx"
	want := []byte(`{"created":1}`)
	var runs int32

	reqCtx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	first, err := store.Do(reqCtx, key, time.Minute, func() ([]byte, error) {
		atomic.AddInt32(&runs, 1)
		time.Sleep(70 * time.Millisecond)
		return want, nil
	})
	if err != nil {
		t.Fatalf("leader should cache result even when request ctx is nearly expired: %v", err)
	}
	if string(first) != string(want) {
		t.Fatalf("leader result=%q want=%q", first, want)
	}

	second, err := store.Do(context.Background(), key, time.Minute, func() ([]byte, error) {
		atomic.AddInt32(&runs, 1)
		t.Fatal("fn should not run when cached result exists")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if string(second) != string(want) {
		t.Fatalf("replay result=%q want=%q", second, want)
	}
	if runs != 1 {
		t.Fatalf("expected fn once, got %d", runs)
	}
}

func TestRedisStore_WaitRespectsContextDeadline(t *testing.T) {
	mr, store := newTestRedisStore(t, webhookidempotency.Config{PollInterval: 20 * time.Millisecond})
	defer mr.Close()

	key := "src\x00req-ctx-timeout"
	leaderStarted := make(chan struct{})

	go func() {
		_, _ = store.Do(context.Background(), key, time.Minute, func() ([]byte, error) {
			close(leaderStarted)
			time.Sleep(2 * time.Second)
			return []byte(`{"created":1}`), nil
		})
	}()
	<-leaderStarted

	shortCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := store.Do(shortCtx, key, time.Minute, func() ([]byte, error) {
		t.Error("fn should not run")
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}
