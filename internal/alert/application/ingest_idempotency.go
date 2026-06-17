package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/alert/infrastructure/webhookidempotency"
	apperr "github.com/734965549/aiops/pkg/errors"
	"golang.org/x/sync/singleflight"
)

// webhookIdempotencyTTL 是 X-Request-ID 短期幂等缓存时长（ops/alert-contract.md §3.2、§7.2）。
const webhookIdempotencyTTL = 5 * time.Minute

// webhookIdempotencyExecutor 封装 Redis 跨 Pod 幂等 + 本 Pod singleflight 合并并发。
type webhookIdempotencyExecutor struct {
	store    webhookidempotency.Store
	inflight singleflight.Group
}

func (e *webhookIdempotencyExecutor) do(ctx context.Context, key string, fn func() (*IngestResultDTO, error)) (*IngestResultDTO, error) {
	if key == "" {
		return fn()
	}
	if e == nil || e.store == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "webhook idempotency store is not configured")
	}
	v, err, _ := e.inflight.Do(key, func() (any, error) {
		raw, err := e.store.Do(ctx, key, webhookIdempotencyTTL, func() ([]byte, error) {
			r, err := fn()
			if err != nil {
				return nil, err
			}
			return json.Marshal(r)
		})
		if err != nil {
			return nil, err
		}
		var out IngestResultDTO
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, apperr.Wrap(err, apperr.CodeInternal, "decode webhook idempotency result failed")
		}
		return &out, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*IngestResultDTO), nil
}

func ingestIdempotencyKey(sourceID, requestID string) string {
	return webhookIdempotencyKey(sourceID, requestID)
}

func webhookIdempotencyKey(sourceID, requestID string) string {
	sourceID = strings.TrimSpace(sourceID)
	requestID = strings.TrimSpace(requestID)
	if sourceID == "" || requestID == "" {
		return ""
	}
	return sourceID + "\x00" + requestID
}
