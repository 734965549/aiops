package application

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
)

// AlertSyncWorker 按源间隔调度启用的拉取源；默认由部署配置显式启用，且仅应在一个 API 实例启动。
type AlertSyncWorker struct {
	sources interface {
		List(ctx context.Context) ([]domain.AlertSource, error)
	}
	sync         *AlertSyncService
	scanInterval time.Duration
	mu           sync.Mutex
	lastAttempt  map[string]time.Time
	wg           sync.WaitGroup
	cancel       context.CancelFunc
}

// NewAlertSyncWorker 构造调度器；scanInterval<=0 时使用 30 秒。
func NewAlertSyncWorker(sources interface {
	List(ctx context.Context) ([]domain.AlertSource, error)
}, syncSvc *AlertSyncService, scanInterval time.Duration) *AlertSyncWorker {
	if scanInterval <= 0 {
		scanInterval = 30 * time.Second
	}
	return &AlertSyncWorker{
		sources: sources, sync: syncSvc, scanInterval: scanInterval,
		lastAttempt: map[string]time.Time{},
	}
}

// Start 启动单 goroutine 调度循环。
func (w *AlertSyncWorker) Start(parent context.Context) {
	if w == nil || w.sync == nil || w.sources == nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.scanInterval)
		defer ticker.Stop()
		w.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.tick(ctx)
			}
		}
	}()
}

// WaitContext 等待调度循环退出；超时返回 false。
func (w *AlertSyncWorker) WaitContext(ctx context.Context) bool {
	if w == nil {
		return true
	}
	if w.cancel != nil {
		w.cancel()
	}
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

func (w *AlertSyncWorker) tick(ctx context.Context) {
	rows, err := w.sources.List(ctx)
	if err != nil {
		logger.From(ctx).Warn("alert sync worker list sources failed", logger.Error(err))
		return
	}
	now := time.Now()
	for _, src := range rows {
		if !src.Enabled || !src.IsPullSource() {
			continue
		}
		interval := src.PullConfig.EffectivePullInterval()
		if interval <= 0 {
			continue
		}
		if !w.shouldRun(src.ID, now, interval) {
			continue
		}
		if _, err := w.sync.Sync(ctx, src.ID, Actor{}); err != nil {
			logger.From(ctx).Warn("alert sync worker round failed",
				logger.String("source_id", src.ID),
				logger.String("error_code", string(apperrCode(err))),
				logger.Error(err),
			)
		}
	}
}

func (w *AlertSyncWorker) shouldRun(sourceID string, now time.Time, interval time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	last, ok := w.lastAttempt[sourceID]
	if ok && now.Sub(last) < interval {
		return false
	}
	w.lastAttempt[sourceID] = now
	return true
}

func apperrCode(err error) string {
	if err == nil {
		return ""
	}
	return strings.ToUpper(string(apperr.CodeOf(err)))
}
