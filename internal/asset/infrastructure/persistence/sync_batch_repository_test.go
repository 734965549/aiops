package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// seedSyncBatchRow 通过仓储写入一个 running 批次，租约由 lease 指定（可为过去时间，模拟过期）。
func seedSyncBatchRow(t *testing.T, db *gorm.DB, accountID, batchID, token string, lease time.Time) {
	t.Helper()
	now := time.Now().UTC()
	batch := &domain.SyncBatch{
		BatchID:              batchID,
		IntegrationAccountID: accountID,
		Provider:             "huawei_ces",
		Status:               domain.SyncBatchStatusRunning,
		StartedAt:            now,
		FencingToken:         token,
		TriggeredBy:          "tester",
		LeaseExpiresAt:       &lease,
	}
	if err := NewSyncBatchRepository(db).Create(context.Background(), batch); err != nil {
		t.Fatalf("seed sync batch %s: %v", batchID, err)
	}
}

func deleteTestSyncBatches(t *testing.T, db *gorm.DB, batchIDs ...string) {
	t.Helper()
	if len(batchIDs) == 0 {
		return
	}
	if err := db.Exec("DELETE FROM asset_sync_batch WHERE batch_id IN ?", batchIDs).Error; err != nil {
		t.Fatalf("cleanup sync batches: %v", err)
	}
}

// TestSyncBatchRepo_ReapExpiredRunning 验证过期 running 批次被正确 reap 为 failed。
func TestSyncBatchRepo_ReapExpiredRunning(t *testing.T) {
	db := openAssetTestPostgres(t)
	repo := NewSyncBatchRepository(db)
	ctx := context.Background()

	accountID := "reap-basic-" + uuid.NewString()
	batchID := "batch-reap-basic-" + uuid.NewString()
	token := "token-reap-basic-" + uuid.NewString()
	t.Cleanup(func() { deleteTestSyncBatches(t, db, batchID) })

	now := time.Now().UTC()
	seedSyncBatchRow(t, db, accountID, batchID, token, now.Add(-time.Minute))

	reaped, err := repo.ReapExpiredRunning(ctx, accountID, now)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if len(reaped) != 1 {
		t.Fatalf("expected 1 reaped, got %d", len(reaped))
	}
	if reaped[0].BatchID != batchID {
		t.Fatalf("reaped batch id mismatch: %s", reaped[0].BatchID)
	}
	if reaped[0].TriggeredBy != "tester" {
		t.Fatalf("reaped triggered_by mismatch: %s", reaped[0].TriggeredBy)
	}
	if reaped[0].IntegrationAccountID != accountID {
		t.Fatalf("reaped account id mismatch: %s", reaped[0].IntegrationAccountID)
	}

	var row syncBatchModel
	if err := db.Where("batch_id = ?", batchID).First(&row).Error; err != nil {
		t.Fatalf("reload batch: %v", err)
	}
	if row.Status != domain.SyncBatchStatusFailed {
		t.Fatalf("expected status failed, got %s", row.Status)
	}
	if row.LeaseExpiresAt != nil {
		t.Fatalf("expected lease_expires_at NULL, got %v", row.LeaseExpiresAt)
	}
	if row.FinishedAt == nil {
		t.Fatalf("expected finished_at set")
	}
	if d := row.FinishedAt.Sub(now); d > time.Second || d < -time.Second {
		t.Fatalf("finished_at %v not near now (%v)", row.FinishedAt, now)
	}
	if row.Message != "lease expired; previous sync batch interrupted" {
		t.Fatalf("unexpected message: %s", row.Message)
	}
}

// TestSyncBatchRepo_ReapExpiredRunning_SkipsActiveLease 验证租约未过期的 running 批次不会被 reap。
func TestSyncBatchRepo_ReapExpiredRunning_SkipsActiveLease(t *testing.T) {
	db := openAssetTestPostgres(t)
	repo := NewSyncBatchRepository(db)
	ctx := context.Background()

	accountID := "reap-active-" + uuid.NewString()
	batchID := "batch-reap-active-" + uuid.NewString()
	token := "token-reap-active-" + uuid.NewString()
	t.Cleanup(func() { deleteTestSyncBatches(t, db, batchID) })

	now := time.Now().UTC()
	seedSyncBatchRow(t, db, accountID, batchID, token, now.Add(10*time.Minute))

	reaped, err := repo.ReapExpiredRunning(ctx, accountID, now)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if len(reaped) != 0 {
		t.Fatalf("expected 0 reaped for active lease, got %d", len(reaped))
	}

	var row syncBatchModel
	if err := db.Where("batch_id = ?", batchID).First(&row).Error; err != nil {
		t.Fatalf("reload batch: %v", err)
	}
	if row.Status != domain.SyncBatchStatusRunning {
		t.Fatalf("expected status running, got %s", row.Status)
	}
	if row.LeaseExpiresAt == nil {
		t.Fatalf("expected lease_expires_at preserved")
	}
}

// TestSyncBatchRepo_ReapExpiredRunning_ConcurrentReapersDedup 验证并发 reaper
// 不会重复 reap 同一批次（即不会重复返回批次、不会重复写 sync_reaped 审计）。
func TestSyncBatchRepo_ReapExpiredRunning_ConcurrentReapersDedup(t *testing.T) {
	db := openAssetTestPostgres(t)
	repo := NewSyncBatchRepository(db)
	ctx := context.Background()

	accountID := "reap-dedup-" + uuid.NewString()
	batchID := "batch-reap-dedup-" + uuid.NewString()
	token := "token-reap-dedup-" + uuid.NewString()
	t.Cleanup(func() { deleteTestSyncBatches(t, db, batchID) })

	now := time.Now().UTC()
	seedSyncBatchRow(t, db, accountID, batchID, token, now.Add(-time.Minute))

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	var r1, r2 []domain.ReapedSyncBatch
	go func() {
		defer wg.Done()
		<-start
		r1, _ = repo.ReapExpiredRunning(ctx, accountID, time.Now().UTC())
	}()
	go func() {
		defer wg.Done()
		<-start
		r2, _ = repo.ReapExpiredRunning(ctx, accountID, time.Now().UTC())
	}()
	close(start)
	wg.Wait()

	total := len(r1) + len(r2)
	if total != 1 {
		t.Fatalf("expected exactly 1 reaped across concurrent reapers, got %d (r1=%d r2=%d)", total, len(r1), len(r2))
	}

	var row syncBatchModel
	if err := db.Where("batch_id = ?", batchID).First(&row).Error; err != nil {
		t.Fatalf("reload batch: %v", err)
	}
	if row.Status != domain.SyncBatchStatusFailed {
		t.Fatalf("expected status failed, got %s", row.Status)
	}
}

// TestSyncBatchRepo_RenewLease_RejectsExpiredLease 验证过期租约不可复活：
// 租约已过期时 RenewLease 必须返回 ErrLeaseLost，且不改动 lease_expires_at / status。
// 这是 Option A 的核心不变量——过期即死，只能由 ReapExpiredRunning 回收。
func TestSyncBatchRepo_RenewLease_RejectsExpiredLease(t *testing.T) {
	db := openAssetTestPostgres(t)
	repo := NewSyncBatchRepository(db)
	ctx := context.Background()

	accountID := "renew-exp-" + uuid.NewString()
	batchID := "batch-renew-exp-" + uuid.NewString()
	token := "token-renew-exp-" + uuid.NewString()
	t.Cleanup(func() { deleteTestSyncBatches(t, db, batchID) })

	now := time.Now().UTC()
	seedSyncBatchRow(t, db, accountID, batchID, token, now.Add(-time.Minute))

	err := repo.RenewLease(ctx, batchID, token, now, 5*time.Minute)
	if !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("expected ErrLeaseLost for expired lease, got %v", err)
	}

	var row syncBatchModel
	if err := db.Where("batch_id = ?", batchID).First(&row).Error; err != nil {
		t.Fatalf("reload batch: %v", err)
	}
	if row.Status != domain.SyncBatchStatusRunning {
		t.Fatalf("expected status unchanged (running), got %s", row.Status)
	}
	if row.LeaseExpiresAt == nil || row.LeaseExpiresAt.After(now) {
		t.Fatalf("expected lease_expires_at unchanged (still expired), got %v", row.LeaseExpiresAt)
	}
}

// TestSyncBatchRepo_RenewLease_RenewsValidLease 验证未过期租约可续租：
// RenewLease 成功，lease_expires_at 推进到 now+ttl，updated_at 刷新。
func TestSyncBatchRepo_RenewLease_RenewsValidLease(t *testing.T) {
	db := openAssetTestPostgres(t)
	repo := NewSyncBatchRepository(db)
	ctx := context.Background()

	accountID := "renew-ok-" + uuid.NewString()
	batchID := "batch-renew-ok-" + uuid.NewString()
	token := "token-renew-ok-" + uuid.NewString()
	t.Cleanup(func() { deleteTestSyncBatches(t, db, batchID) })

	now := time.Now().UTC()
	seedSyncBatchRow(t, db, accountID, batchID, token, now.Add(time.Minute))

	ttl := 5 * time.Minute
	if err := repo.RenewLease(ctx, batchID, token, now, ttl); err != nil {
		t.Fatalf("renew valid lease: %v", err)
	}

	var row syncBatchModel
	if err := db.Where("batch_id = ?", batchID).First(&row).Error; err != nil {
		t.Fatalf("reload batch: %v", err)
	}
	if row.Status != domain.SyncBatchStatusRunning {
		t.Fatalf("expected status running, got %s", row.Status)
	}
	if row.LeaseExpiresAt == nil {
		t.Fatalf("expected lease_expires_at set")
	}
	want := now.Add(ttl)
	if d := row.LeaseExpiresAt.Sub(want); d > time.Second || d < -time.Second {
		t.Fatalf("lease_expires_at %v not near now+ttl %v", row.LeaseExpiresAt, want)
	}
}

// TestSyncBatchRepo_ReapExpiredRunning_RenewRaceInvariant 验证 RenewLease 与
// ReapExpiredRunning 并发时的不变量：绝不会出现“心跳续租成功(nil) 且 批次被 reap”。
//
// Option A（过期不可复活）后，RenewLease 只在租约未过期时命中。续租成功会把 lease
// 推到未来，reaper 的 WHERE lease_expires_at < now 不再命中，批次不会被误杀——这正是
// 旧“先无锁 SELECT 再按 batch_id UPDATE”实现的 TOCTOU 缺陷（心跳在两条语句间完成续租后，
// reaper 仍按 batch_id 把它打成 failed）所违反、原子 UPDATE 后恒成立的不变量。
//
// 每轮用一个有效租约播种并并发续租/回收：续租必成功、回收必为 0。
// “过期→续租失败”路径由 TestSyncBatchRepo_RenewLease_RejectsExpiredLease 确定性覆盖。
func TestSyncBatchRepo_ReapExpiredRunning_RenewRaceInvariant(t *testing.T) {
	db := openAssetTestPostgres(t)
	repo := NewSyncBatchRepository(db)
	ctx := context.Background()

	accountID := "reap-race-" + uuid.NewString()
	batchID := "batch-reap-race-" + uuid.NewString()
	token := "token-reap-race-" + uuid.NewString()
	t.Cleanup(func() { deleteTestSyncBatches(t, db, batchID) })

	seedNow := time.Now().UTC()
	seedSyncBatchRow(t, db, accountID, batchID, token, seedNow.Add(time.Minute))

	const iterations = 200
	for i := 0; i < iterations; i++ {
		// 每轮重置为有效 running 租约：lease = now + 1min。
		base := time.Now().UTC()
		valid := base.Add(time.Minute)
		if err := db.Exec(`UPDATE asset_sync_batch
SET status = ?, lease_expires_at = ?, finished_at = NULL, message = '', fencing_token = ?, updated_at = ?
WHERE batch_id = ?`,
			domain.SyncBatchStatusRunning, valid, token, base, batchID).Error; err != nil {
			t.Fatalf("reset batch iter %d: %v", i, err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var renewErr error
		var reaped []domain.ReapedSyncBatch
		go func() {
			defer wg.Done()
			<-start
			renewErr = repo.RenewLease(ctx, batchID, token, time.Now().UTC(), 10*time.Minute)
		}()
		go func() {
			defer wg.Done()
			<-start
			reaped, _ = repo.ReapExpiredRunning(ctx, accountID, time.Now().UTC())
		}()
		close(start)
		wg.Wait()

		renewOK := renewErr == nil
		reapedCount := len(reaped)

		// 核心不变量：续租成功与被 reap 互斥；有效租约下续租必成功、回收必为 0。
		if !renewOK {
			t.Fatalf("iter %d: expected renew to succeed on valid lease, got %v", i, renewErr)
		}
		if reapedCount != 0 {
			t.Fatalf("iter %d: invariant violated - valid lease reaped (reaped=%d)", i, reapedCount)
		}
	}
}

// TestSyncBatchRepo_ReapAllExpiredRunning 验证跨账号全量 reap：所有账号下租约过期的 running 批次
// 都被标记为 failed，未过期的批次不受影响。
func TestSyncBatchRepo_ReapAllExpiredRunning(t *testing.T) {
	db := openAssetTestPostgres(t)
	repo := NewSyncBatchRepository(db)
	ctx := context.Background()

	accountA := "reap-all-a-" + uuid.NewString()
	accountB := "reap-all-b-" + uuid.NewString()
	accountActive := "reap-all-active-" + uuid.NewString()
	batchA := "batch-reap-all-a-" + uuid.NewString()
	batchB := "batch-reap-all-b-" + uuid.NewString()
	batchActive := "batch-reap-all-active-" + uuid.NewString()
	t.Cleanup(func() { deleteTestSyncBatches(t, db, batchA, batchB, batchActive) })

	now := time.Now().UTC()
	seedSyncBatchRow(t, db, accountA, batchA, "tok-a", now.Add(-time.Minute))
	seedSyncBatchRow(t, db, accountB, batchB, "tok-b", now.Add(-time.Minute))
	seedSyncBatchRow(t, db, accountActive, batchActive, "tok-active", now.Add(5*time.Minute))

	reaped, err := repo.ReapAllExpiredRunning(ctx, now)
	if err != nil {
		t.Fatalf("reap all: %v", err)
	}

	reapedIDs := map[string]bool{}
	for _, r := range reaped {
		reapedIDs[r.BatchID] = true
	}
	if !reapedIDs[batchA] || !reapedIDs[batchB] {
		t.Fatalf("expected batchA and batchB reaped, got %v", reapedIDs)
	}
	if reapedIDs[batchActive] {
		t.Fatalf("active lease batch should not be reaped")
	}

	// 验证 batchA/batchB 终态为 failed，batchActive 仍 running。
	for _, bid := range []string{batchA, batchB} {
		var row syncBatchModel
		if err := db.Where("batch_id = ?", bid).First(&row).Error; err != nil {
			t.Fatalf("reload batch %s: %v", bid, err)
		}
		if row.Status != domain.SyncBatchStatusFailed {
			t.Fatalf("expected batch %s status failed, got %s", bid, row.Status)
		}
	}
	var activeRow syncBatchModel
	if err := db.Where("batch_id = ?", batchActive).First(&activeRow).Error; err != nil {
		t.Fatalf("reload active batch: %v", err)
	}
	if activeRow.Status != domain.SyncBatchStatusRunning {
		t.Fatalf("expected active batch still running, got %s", activeRow.Status)
	}
}
