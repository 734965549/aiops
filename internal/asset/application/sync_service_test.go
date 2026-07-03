package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

func TestCloudApplicationIDUsesStableHashSuffix(t *testing.T) {
	first := "account-with-identical-prefix-000000000000000000000000000001"
	second := "account-with-identical-prefix-000000000000000000000000000002"

	firstID := cloudApplicationID(first)
	if firstID == cloudApplicationID(second) {
		t.Fatalf("expected different application ids for accounts with same prefix, got %q", firstID)
	}
	if firstID != cloudApplicationID(first) {
		t.Fatalf("expected stable application id for same account")
	}
	if len(firstID) > 36 {
		t.Fatalf("expected application id length <= 36, got %d: %q", len(firstID), firstID)
	}
	if !strings.HasPrefix(firstID, "cloud-account-with-iden-") {
		t.Fatalf("expected readable account prefix, got %q", firstID)
	}
}

// TestCloudApplicationIDNotLegacyFormat 验证新算法不会生成旧格式 cloud-<完整账号>，
// 从而避免 0032 迁移脚本把新 ID 误判为旧 ID 而误删。
func TestCloudApplicationIDNotLegacyFormat(t *testing.T) {
	cases := []string{
		"1234567890123456789012345678",
		"account-without-dash",
		"short",
		"acc-fake",
	}
	for _, accountID := range cases {
		id := cloudApplicationID(accountID)
		if isLegacyCloudApplicationID(id) {
			t.Fatalf("new cloudApplicationID %q for account %q looks like legacy format", id, accountID)
		}
		if !strings.HasPrefix(id, "cloud-") {
			t.Fatalf("expected cloud- prefix, got %q", id)
		}
	}
}

// isLegacyCloudApplicationID 与迁移 0032 的识别规则保持一致：
// 旧格式为 cloud-<完整账号>，只含一个 '-'；新格式至少含两个 '-'。
func isLegacyCloudApplicationID(id string) bool {
	return strings.HasPrefix(id, "cloud-") && !strings.Contains(id[len("cloud-"):], "-")
}

// TestLegacyCloudApplicationIDRecognition 验证迁移脚本对旧格式的识别逻辑。
// 注意：旧格式假设账号本身不含'-'；若旧账号含'-'，cloud-<账号> 会与 cloud-<前缀>-<hash> 无法区分，
// 这种有歧义的场景属于已知限制，现实中华为云账号一般为纯数字/字母，不会触发。
func TestLegacyCloudApplicationIDRecognition(t *testing.T) {
	legacyIDs := []string{
		"cloud-1234567890123456789012345678",
		"cloud-accfake",
	}
	for _, id := range legacyIDs {
		if !isLegacyCloudApplicationID(id) {
			t.Fatalf("expected %q to be recognized as legacy", id)
		}
	}
	newIDs := []string{
		cloudApplicationID("1234567890123456789012345678"),
		cloudApplicationID("account-without-dash"),
		cloudApplicationID("short"),
	}
	for _, id := range newIDs {
		if isLegacyCloudApplicationID(id) {
			t.Fatalf("expected %q to NOT be recognized as legacy", id)
		}
	}
}

type fakeSyncBatchRepo struct {
	mu   sync.Mutex
	rows []domain.SyncBatch
	// enforceRunningMutex 为 true 时模拟迁移 0028 的部分唯一索引：
	// 同一 integration_account_id 已有 running 批次时 Create 返回 ErrAlreadyExists。
	enforceRunningMutex bool
	// renewLeaseCount 记录 RenewLease 被成功调用的次数，供异步续租测试断言。
	renewLeaseCount int
	renewErrors     []error
	// promote 模拟 FinalizeSuccess 同一事务内提升资源的动作。
	// 设为 fakeResRepo.PromoteSuccessfulSyncBatch 即可保持“最近成功批次”断言。
	promote func(ctx context.Context, accountID, batchID string, syncedSince time.Time) (int64, error)
}

func (r *fakeSyncBatchRepo) appendRow(row domain.SyncBatch) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, row)
}

func (r *fakeSyncBatchRepo) renewLeaseCountValue() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.renewLeaseCount
}

func (r *fakeSyncBatchRepo) rowSnapshot(batchID string) (domain.SyncBatch, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.BatchID == batchID {
			return row, true
		}
	}
	return domain.SyncBatch{}, false
}

func (r *fakeSyncBatchRepo) runningCount(accountID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, row := range r.rows {
		if row.IntegrationAccountID == accountID && row.Status == domain.SyncBatchStatusRunning {
			count++
		}
	}
	return count
}

func (r *fakeSyncBatchRepo) setLeaseExpires(batchID string, leaseExpires *time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rows {
		if r.rows[i].BatchID == batchID {
			r.rows[i].LeaseExpiresAt = leaseExpires
			return
		}
	}
}

func (r *fakeSyncBatchRepo) failRunningBatches(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	finished := time.Now().UTC()
	for i := range r.rows {
		if r.rows[i].Status == domain.SyncBatchStatusRunning {
			r.rows[i].Status = domain.SyncBatchStatusFailed
			r.rows[i].FinishedAt = &finished
			r.rows[i].LeaseExpiresAt = nil
			r.rows[i].Message = message
		}
	}
}

func (r *fakeSyncBatchRepo) Create(_ context.Context, batch *domain.SyncBatch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.enforceRunningMutex {
		for _, row := range r.rows {
			if row.IntegrationAccountID == batch.IntegrationAccountID && row.Status == domain.SyncBatchStatusRunning {
				return domain.ErrAlreadyExists
			}
		}
	}
	if strings.TrimSpace(batch.FencingToken) == "" {
		batch.FencingToken = batch.BatchID
	}
	now := time.Now().UTC()
	batch.CreatedAt = now
	batch.UpdatedAt = now
	r.rows = append(r.rows, *batch)
	return nil
}

func (r *fakeSyncBatchRepo) Update(_ context.Context, batch *domain.SyncBatch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for i := range r.rows {
		row := &r.rows[i]
		if row.BatchID != batch.BatchID {
			continue
		}
		if batch.Status != domain.SyncBatchStatusRunning {
			if row.FencingToken != batch.FencingToken || row.Status != domain.SyncBatchStatusRunning || row.LeaseExpiresAt == nil || row.LeaseExpiresAt.Before(now) {
				return domain.ErrLeaseLost
			}
		}
		r.rows[i] = *batch
		// 对齐生产 SyncBatchRepository.Update：终态批次清空 lease，释放账号 running 槽位。
		if batch.Status != domain.SyncBatchStatusRunning {
			r.rows[i].LeaseExpiresAt = nil
		}
		return nil
	}
	return domain.ErrNotFound
}

func (r *fakeSyncBatchRepo) FinalizeSuccess(ctx context.Context, batch *domain.SyncBatch, accountID string, syncedSince time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for i := range r.rows {
		row := &r.rows[i]
		if row.BatchID != batch.BatchID {
			continue
		}
		if row.FencingToken != batch.FencingToken || row.Status != domain.SyncBatchStatusRunning || row.LeaseExpiresAt == nil || row.LeaseExpiresAt.Before(now) {
			return 0, domain.ErrLeaseLost
		}
		old := *row
		row.Status = domain.SyncBatchStatusSuccess
		row.LeaseExpiresAt = nil
		row.CreatedCount = batch.CreatedCount
		row.UpdatedCount = batch.UpdatedCount
		row.StaleCount = batch.StaleCount
		row.FailedCount = batch.FailedCount
		row.Message = batch.Message
		row.Summary = batch.Summary
		row.FinishedAt = batch.FinishedAt
		row.UpdatedAt = now
		var promoted int64
		if r.promote != nil {
			var err error
			promoted, err = r.promote(ctx, accountID, batch.BatchID, syncedSince)
			if err != nil {
				*row = old
				return 0, err
			}
		}
		*batch = *row
		return promoted, nil
	}
	return 0, domain.ErrNotFound
}

// bindFakePromote 把 fake 资源仓库的提升动作绑定到 fake 批次仓库，
// 让内存测试也能模拟“success 终态与资源提升原子完成”。
func bindFakePromote(batches *fakeSyncBatchRepo, resources *fakeResRepo) {
	if batches != nil && resources != nil {
		batches.promote = resources.PromoteSuccessfulSyncBatch
	}
}

func (r *fakeSyncBatchRepo) GetByID(_ context.Context, batchID string) (*domain.SyncBatch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rows {
		if r.rows[i].BatchID == batchID {
			cp := r.rows[i]
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

// ReapExpiredRunning 模拟迁移 0028 的租约自愈：把本账号下租约过期的 running 批次标记 failed。
func (r *fakeSyncBatchRepo) ReapExpiredRunning(_ context.Context, accountID string, now time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var reaped int64
	for i := range r.rows {
		row := &r.rows[i]
		if row.IntegrationAccountID != accountID || row.Status != domain.SyncBatchStatusRunning {
			continue
		}
		if row.LeaseExpiresAt == nil || !row.LeaseExpiresAt.Before(now) {
			continue
		}
		finished := now
		row.Status = domain.SyncBatchStatusFailed
		row.FinishedAt = &finished
		row.LeaseExpiresAt = nil
		row.Message = "lease expired; previous sync batch interrupted"
		row.UpdatedAt = now
		reaped++
	}
	return reaped, nil
}

// RenewLease 续租 running 批次，并按 fencing_token 校验租约所有权。
func (r *fakeSyncBatchRepo) RenewLease(_ context.Context, batchID, fencingToken string, now time.Time, ttl time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.renewErrors) > 0 {
		err := r.renewErrors[0]
		r.renewErrors = r.renewErrors[1:]
		if err != nil {
			return err
		}
	}
	for i := range r.rows {
		if r.rows[i].BatchID != batchID || r.rows[i].FencingToken != fencingToken || r.rows[i].Status != domain.SyncBatchStatusRunning {
			continue
		}
		expires := now.Add(ttl)
		r.rows[i].LeaseExpiresAt = &expires
		r.rows[i].UpdatedAt = now
		r.renewLeaseCount++
		return nil
	}
	return domain.ErrLeaseLost
}

func (r *fakeSyncBatchRepo) CheckLeaseOwned(_ context.Context, batchID, fencingToken string, _ time.Time) error {
	if r.isLeaseOwned(batchID, fencingToken) {
		return nil
	}
	return domain.ErrLeaseLost
}

func (r *fakeSyncBatchRepo) isLeaseOwned(batchID, fencingToken string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rows {
		row := &r.rows[i]
		if row.BatchID != batchID || row.FencingToken != fencingToken || row.Status != domain.SyncBatchStatusRunning {
			continue
		}
		if row.LeaseExpiresAt == nil {
			return false
		}
		return true
	}
	return false
}

func (r *fakeSyncBatchRepo) List(_ context.Context, filter domain.SyncBatchFilter) ([]domain.SyncBatch, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.SyncBatch, 0)
	for _, row := range r.rows {
		if filter.IntegrationAccountID != "" && row.IntegrationAccountID != filter.IntegrationAccountID {
			continue
		}
		out = append(out, row)
	}
	total := int64(len(out))
	if filter.Offset >= len(out) {
		return []domain.SyncBatch{}, total, nil
	}
	end := filter.Offset + filter.Limit
	if end > len(out) {
		end = len(out)
	}
	return out[filter.Offset:end], total, nil
}

type fakeDiscoveryPort struct {
	resources        []obsdomain.CloudResource
	errors           map[string]error
	fullSummary      *obsapp.CloudSyncSummary
	hasMoreTypes     map[string]bool
	beforeFullReturn func()
	fullSyncError    error
	// fullSyncUnsupported 为 true 时 ListAllResources 返回携带 ErrCapabilityUnsupported 的
	// FAILED_PRECONDITION，模拟 provider 未实现 CloudFullSyncPort，强制走 syncGeneric 通用路径。
	fullSyncUnsupported bool
	listResourceCalls   int
}

func (p *fakeDiscoveryPort) ListResources(_ context.Context, _ obsapp.Actor, q obsdomain.AssetDiscoveryQuery) (*obsapp.AssetDiscoveryResult, error) {
	p.listResourceCalls++
	if p.errors != nil {
		if err := p.errors[q.Region+"/"+q.ResourceType]; err != nil {
			return nil, err
		}
	}
	out := make([]obsdomain.CloudResource, 0)
	for _, item := range p.resources {
		if q.ResourceType != "" && item.Type != q.ResourceType {
			continue
		}
		if q.Region != "" && item.Region != q.Region {
			continue
		}
		out = append(out, item)
	}
	hasMore := false
	if p.hasMoreTypes != nil {
		hasMore = p.hasMoreTypes[q.ResourceType]
	}
	return &obsapp.AssetDiscoveryResult{Resources: out, EvidenceID: "ev-fake", HasMore: hasMore}, nil
}

func (p *fakeDiscoveryPort) ListAllResources(_ context.Context, _ obsapp.Actor, q obsapp.AssetFullSyncQuery) (*obsapp.AssetFullSyncResult, error) {
	if p.beforeFullReturn != nil {
		defer p.beforeFullReturn()
	}
	if p.fullSyncUnsupported {
		return nil, apperr.Wrap(obsdomain.ErrCapabilityUnsupported, apperr.CodeFailedPrecondition, "provider does not support full sync")
	}
	if p.fullSyncError != nil {
		return nil, p.fullSyncError
	}
	if q.MaxResources != 0 {
		return nil, errors.New("asset sync should let provider extra_config decide max_resources")
	}
	if p.errors != nil {
		if err := p.errors[q.Region+"/"]; err != nil {
			return nil, err
		}
	}
	out := make([]obsdomain.CloudResource, 0)
	for _, item := range p.resources {
		if q.Region != "" && item.Region != q.Region {
			continue
		}
		out = append(out, item)
	}
	summary := obsapp.CloudSyncSummary{Region: q.Region, Discovered: len(out)}
	if p.fullSummary != nil {
		summary = *p.fullSummary
		if summary.Region == "" {
			summary.Region = q.Region
		}
		if summary.Discovered == 0 {
			summary.Discovered = len(out)
		}
	}
	return &obsapp.AssetFullSyncResult{Resources: out, EvidenceID: "ev-fake", Summary: summary}, nil
}

type fakeIntegrationAccountPort struct {
	account *SyncAccountSnapshot
}

func (p *fakeIntegrationAccountPort) ResolveSyncAccount(_ context.Context, accountID string) (*SyncAccountSnapshot, error) {
	if p.account == nil || p.account.AccountID != accountID {
		return nil, domain.ErrNotFound
	}
	cp := *p.account
	return &cp, nil
}

func TestSyncService_TriggerSyncCreatesCloudResources(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	bindFakePromote(batches, resources)
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-1", Name: "ecs-demo-1", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-1"},
			{ResourceID: "res-fake-rds-1", Name: "rds-demo-1", Type: "rds", Region: "cn-north-4", Status: "ACTIVE", ProviderRef: "rds-demo-1"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	if out.Status != domain.SyncBatchStatusRunning {
		t.Fatalf("expected running batch immediately after trigger, got %+v", out)
	}
	// 异步同步：等待后台 goroutine 落终态后再断言。
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusSuccess {
		t.Fatalf("expected success batch, got %+v", final)
	}
	if final.CreatedCount < 2 {
		t.Fatalf("expected at least 2 created resources across types, got created=%d updated=%d", final.CreatedCount, final.UpdatedCount)
	}
	if len(resources.rows) < 2 {
		t.Fatalf("expected synced resources in repo, got %d", len(resources.rows))
	}
	for _, row := range resources.rows {
		if row.Source != domain.ResourceSourceCloudSync {
			t.Fatalf("expected cloud_sync source, got %s", row.Source)
		}
		if row.SyncStatus != domain.SyncStatusActive {
			t.Fatalf("expected active sync status, got %s", row.SyncStatus)
		}
		if row.SyncBatchID != out.BatchID {
			t.Fatalf("expected success batch to promote sync_batch_id to %s, got resource %s batch %q", out.BatchID, row.ID, row.SyncBatchID)
		}
	}
}

func TestSyncService_TriggerSyncMarksStale(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	batches := &fakeSyncBatchRepo{}
	resources := &fakeResRepo{leaseOwned: batches.isLeaseOwned, rows: []domain.Resource{
		{
			ID: "old-res", ApplicationID: appID, Name: "old-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
	}}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-2", Name: "ecs-demo-2", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-2"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.StaleCount < 1 {
		t.Fatalf("expected stale count >= 1, got %+v", final)
	}
	var staleFound bool
	for _, row := range resources.rows {
		if row.CloudResourceID == "gone-ecs" && row.SyncStatus == domain.SyncStatusStale {
			staleFound = true
		}
	}
	if !staleFound {
		t.Fatal("expected gone-ecs to be marked stale")
	}
}

func TestSyncService_TriggerSyncOnlyMarksStaleForSuccessfulScopes(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	batches := &fakeSyncBatchRepo{}
	resources := &fakeResRepo{leaseOwned: batches.isLeaseOwned, rows: []domain.Resource{
		{
			ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
		{
			ID: "old-rds", ApplicationID: appID, Name: "old-rds",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "rds", CloudResourceID: "keep-rds", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
	}}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-2", Name: "ecs-demo-2", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-2"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	// CES 全量同步整 region 成功；本批只发现 ecs，rds 未入库，rds 旧资源应保持 active。
	if final.Status != domain.SyncBatchStatusSuccess {
		t.Fatalf("expected success batch, got %+v", final)
	}
	statusByID := map[string]string{}
	for _, row := range resources.rows {
		statusByID[row.ID] = row.SyncStatus
	}
	if statusByID["old-ecs"] != domain.SyncStatusStale {
		t.Fatalf("expected old ecs stale, got %s", statusByID["old-ecs"])
	}
	if statusByID["old-rds"] != domain.SyncStatusActive {
		t.Fatalf("rds not discovered this batch must remain active, got %s", statusByID["old-rds"])
	}
}

func TestSyncService_SuccessPromotesExistingResourceSyncBatchID(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	batches := &fakeSyncBatchRepo{}
	resources := &fakeResRepo{leaseOwned: batches.isLeaseOwned, rows: []domain.Resource{
		{
			ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "res-fake-ecs-1", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
	}}
	bindFakePromote(batches, resources)
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-1", Name: "ecs-demo-1", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "res-fake-ecs-1"},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ResourceGroupName: "全部资源", CESTotal: 1, Discovered: 1,
			SuccessfulTypes: []string{"ecs"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusSuccess {
		t.Fatalf("expected success batch, got %+v", final)
	}
	if len(resources.rows) != 1 {
		t.Fatalf("expected existing resource updated, got %d resources", len(resources.rows))
	}
	if resources.rows[0].SyncBatchID != out.BatchID {
		t.Fatalf("success batch must promote updated resource sync_batch_id to %s, got %q", out.BatchID, resources.rows[0].SyncBatchID)
	}
}

func TestSyncService_HuaweiCESFailedScopesMakeBatchPartial(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-1", Name: "ecs-demo-1", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-1"},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ResourceGroupName: "全部资源", CESTotal: 2, Discovered: 1,
			SuccessfulTypes: []string{"ecs"},
			FailedScopes:    []string{"cn-north-4/SYS.RDS: provider unavailable"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusPartial {
		t.Fatalf("expected partial batch, got %+v", final)
	}
	if final.FailedCount != 1 {
		t.Fatalf("expected failed_count=1, got %+v", final)
	}
	if !strings.Contains(final.Message, "failed_scopes=1") || !strings.Contains(final.Message, "SYS.RDS") {
		t.Fatalf("expected failed scope in message, got %q", final.Message)
	}
	summary := unmarshalSyncBatchSummary(final.Summary)
	if summary == nil {
		t.Fatal("expected structured summary")
	}
	if summary.CESTotal != 2 || summary.DiscoveredCount != 1 || len(summary.FailedScopes) != 1 || !strings.Contains(summary.FailedScopes[0], "SYS.RDS") {
		t.Fatalf("unexpected structured summary: %+v", summary)
	}
	if len(resources.rows) != 1 {
		t.Fatalf("expected partial batch to persist discovered resource, got %d", len(resources.rows))
	}
	if resources.rows[0].SyncStatus != domain.SyncStatusActive {
		t.Fatalf("expected discovered resource active, got %s", resources.rows[0].SyncStatus)
	}
	if resources.rows[0].SyncBatchID != "" {
		t.Fatalf("partial batch must not become recent success batch, got sync_batch_id=%q", resources.rows[0].SyncBatchID)
	}
}

func TestSyncService_HuaweiCESSuccessfulEmptyScopeMarksStale(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	batches := &fakeSyncBatchRepo{}
	resources := &fakeResRepo{leaseOwned: batches.isLeaseOwned, rows: []domain.Resource{
		{
			ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "res-fake-ecs-1", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
	}}
	discovery := &fakeDiscoveryPort{
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ResourceGroupName: "全部资源", CESTotal: 0, Discovered: 0,
			SuccessfulTypes: []string{"ecs"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusSuccess {
		t.Fatalf("expected success batch, got %+v", final)
	}
	if final.StaleCount != 1 {
		t.Fatalf("expected stale_count=1, got %+v", final)
	}
	if resources.rows[0].SyncStatus != domain.SyncStatusStale {
		t.Fatalf("expected old ecs stale, got %s", resources.rows[0].SyncStatus)
	}
}

// TestSyncService_HuaweiCESAllUpsertFailSkipsStale 验证某类型 CES 查询成功但写库全部失败时，
// 该类型不进 stale scope，旧资产保持 active，见 docs/huawei-ces-asset-sync-plan.md §13（条件3：全部资源成功持久化）。
func TestSyncService_HuaweiCESAllUpsertFailSkipsStale(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	batches := &fakeSyncBatchRepo{}
	resources := &fakeResRepo{
		leaseOwned: batches.isLeaseOwned,
		rows: []domain.Resource{
			{
				ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
				Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
				CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
				SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
			},
		},
		upsertErr: errors.New("db unavailable"),
	}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-2", Name: "ecs-demo-2", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-2"},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ResourceGroupName: "全部资源", CESTotal: 1, Discovered: 1,
			SuccessfulTypes: []string{"ecs"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status == domain.SyncBatchStatusSuccess {
		t.Fatalf("expected non-success batch when all upserts failed, got %+v", final)
	}
	if final.StaleCount != 0 {
		t.Fatalf("expected stale_count=0 (persist incomplete), got %d", final.StaleCount)
	}
	if resources.rows[0].SyncStatus != domain.SyncStatusActive {
		t.Fatalf("old ecs must remain active when all upserts failed, got %s", resources.rows[0].SyncStatus)
	}
}

// TestSyncService_HuaweiCESConversionFailedSkipsStale 验证某类型查询成功但资源转换失败时，
// 该类型不进 stale scope，旧资产保持 active，见 §13（条件2：资源转换完整）。
func TestSyncService_HuaweiCESConversionFailedSkipsStale(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	batches := &fakeSyncBatchRepo{}
	resources := &fakeResRepo{leaseOwned: batches.isLeaseOwned, rows: []domain.Resource{
		{
			ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
	}}
	discovery := &fakeDiscoveryPort{
		// 转换失败的资源已被 provider 丢弃，不进入 Resources。
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ResourceGroupName: "全部资源", CESTotal: 1, Discovered: 0,
			SuccessfulTypes: []string{"ecs"}, ConversionFailedTypes: []string{"ecs"},
			InvalidResourceCount: 1,
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.StaleCount != 0 {
		t.Fatalf("expected stale_count=0 (conversion incomplete), got %d", final.StaleCount)
	}
	if resources.rows[0].SyncStatus != domain.SyncStatusActive {
		t.Fatalf("old ecs must remain active when conversion failed, got %s", resources.rows[0].SyncStatus)
	}
}

// TestSyncService_HuaweiCESPerTypePersistFailure 验证混合场景：ecs 持久化成功、rds 持久化失败时，
// 只有 ecs 进 stale scope，rds 旧资产保持 active，见 §13（条件3：全部资源成功持久化）。
func TestSyncService_HuaweiCESPerTypePersistFailure(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	resources := &fakeResRepo{
		rows: []domain.Resource{
			{
				ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
				Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
				CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
				SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
			},
			{
				ID: "old-rds", ApplicationID: appID, Name: "old-rds",
				Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
				CloudResourceType: "rds", CloudResourceID: "keep-rds", Region: "cn-north-4",
				SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
			},
		},
		upsertErrFor: map[string]error{"rds": errors.New("rds write failed")},
	}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-2", Name: "ecs-demo-2", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-2"},
			{ResourceID: "res-fake-rds-2", Name: "rds-demo-2", Type: "rds", Region: "cn-north-4", Status: "ACTIVE", ProviderRef: "rds-demo-2"},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ResourceGroupName: "全部资源", CESTotal: 2, Discovered: 2,
			SuccessfulTypes: []string{"ecs", "rds"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.StaleCount != 1 {
		t.Fatalf("expected stale_count=1 (only ecs scope eligible), got %d", final.StaleCount)
	}
	statusByID := map[string]string{}
	for _, row := range resources.rows {
		statusByID[row.ID] = row.SyncStatus
	}
	if statusByID["old-ecs"] != domain.SyncStatusStale {
		t.Fatalf("expected old ecs stale, got %s", statusByID["old-ecs"])
	}
	if statusByID["old-rds"] != domain.SyncStatusActive {
		t.Fatalf("expected old rds active (rds persist failed), got %s", statusByID["old-rds"])
	}
}

// TestSyncService_NativePathAllUpsertFailSkipsStale 验证通用（非 CES）同步路径同样遵循
// "全部资源成功持久化才执行 stale"：某类型查询成功但写库全失败时，旧资产保持 active，见 §13。
func TestSyncService_NativePathAllUpsertFailSkipsStale(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "aliyun_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	resources := &fakeResRepo{
		rows: []domain.Resource{
			{
				ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
				Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
				CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
				SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
			},
		},
		upsertErr: errors.New("db unavailable"),
	}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-2", Name: "ecs-demo-2", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-2"},
		},
		fullSyncUnsupported: true,
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "aliyun_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.StaleCount != 0 {
		t.Fatalf("expected stale_count=0 (ecs persist failed), got %d", final.StaleCount)
	}
	if resources.rows[0].SyncStatus != domain.SyncStatusActive {
		t.Fatalf("old ecs must remain active when all upserts failed, got %s", resources.rows[0].SyncStatus)
	}
}

// TestSyncService_GenericPathHasMoreSkipsStale 验证通用（非华为）同步路径 provider 返回 HasMore=true 时
// 跳过该类型 stale 标记，避免未返回资源被误标 stale，见 docs/huawei-ces-asset-sync-plan.md §13.1。
func TestSyncService_GenericPathHasMoreSkipsStale(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "aliyun_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	resources := &fakeResRepo{
		rows: []domain.Resource{
			{
				ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
				Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
				CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
				SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
			},
		},
	}
	batches := &fakeSyncBatchRepo{}
	// 通用路径每类 Limit=500：模拟云端资源数超过 limit，provider 标记 HasMore=true。
	// 返回 1 条新资源（已入库），但 old-ecs 不在返回中；由于 HasMore，old-ecs 不得被标 stale。
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "ecs-new", Name: "ecs-new", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-new"},
		},
		hasMoreTypes:        map[string]bool{"ecs": true},
		fullSyncUnsupported: true,
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "aliyun_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.StaleCount != 0 {
		t.Fatalf("expected stale_count=0 (ecs truncated via HasMore), got %d", final.StaleCount)
	}
	if resources.rows[0].SyncStatus != domain.SyncStatusActive {
		t.Fatalf("old ecs must remain active when provider reported HasMore, got %s", resources.rows[0].SyncStatus)
	}
}

func TestSyncService_FullSyncCapabilityUnsupportedFallsBackToGeneric(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "ecs-new", Name: "ecs-new", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-new"},
		},
		fullSyncUnsupported: true,
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "aliyun_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusSuccess {
		t.Fatalf("expected generic fallback success, got %s message=%q", final.Status, final.Message)
	}
	if discovery.listResourceCalls == 0 {
		t.Fatal("expected full sync unsupported to fall back to generic ListResources")
	}
}

func TestSyncService_FullSyncErrorDoesNotFallbackToGeneric(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{fullSyncError: errors.New("temporary lease check failed")}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusFailed {
		t.Fatalf("expected full sync error to fail without fallback, got %s message=%q", final.Status, final.Message)
	}
	if discovery.listResourceCalls != 0 {
		t.Fatalf("expected no generic fallback for full sync error, got %d ListResources calls", discovery.listResourceCalls)
	}
}

func TestSyncService_FullSyncFailedPreconditionDoesNotFallbackToGeneric(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		errors: map[string]error{
			"cn-north-4/": apperr.New(apperr.CodeFailedPrecondition, "huawei ak/sk credential is required"),
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusFailed {
		t.Fatalf("expected full sync config error to fail without fallback, got %s message=%q", final.Status, final.Message)
	}
	if discovery.listResourceCalls != 0 {
		t.Fatalf("expected no generic fallback for FAILED_PRECONDITION config error, got %d ListResources calls", discovery.listResourceCalls)
	}
	if !strings.Contains(final.Message, "huawei ak/sk credential is required") {
		t.Fatalf("expected config error in message, got %q", final.Message)
	}
}

// TestSyncService_HuaweiCESMaxResourcesSkipsStale 验证达到 max_resources 时：
// ① 批次状态为 partial；② 禁止该 region 执行 stale 标记，已有资源保持 active；
// ③ message 包含 max_resources_reached=true。见 docs/huawei-ces-asset-sync-plan.md §13。
func TestSyncService_HuaweiCESMaxResourcesSkipsStale(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	batches := &fakeSyncBatchRepo{}
	resources := &fakeResRepo{leaseOwned: batches.isLeaseOwned, rows: []domain.Resource{
		{
			ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
	}}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-1", Name: "ecs-demo-1", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-1"},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ResourceGroupName: "全部资源", CESTotal: 100, Discovered: 1,
			MaxResourcesReached: true,
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusPartial {
		t.Fatalf("expected partial batch, got %s", final.Status)
	}
	if final.StaleCount != 0 {
		t.Fatalf("expected stale_count=0 (max_resources must skip stale), got %d", final.StaleCount)
	}
	if resources.rows[0].SyncStatus != domain.SyncStatusActive {
		t.Fatalf("expected old ecs to remain active, got %s", resources.rows[0].SyncStatus)
	}
	if !strings.Contains(final.Message, "max_resources_reached=true") {
		t.Fatalf("expected max_resources_reached=true in message, got %q", final.Message)
	}
}

func TestSyncService_HuaweiHybridEnrichmentSummary(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-1", Name: "ecs-demo-1", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-1"},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ResourceGroupName: "全部资源", CESTotal: 1, Discovered: 1,
			SuccessfulTypes:       []string{"ecs"},
			EnrichedCount:         1,
			EnrichmentFailedTypes: []string{"rds"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if !strings.Contains(final.Message, "enriched=1") {
		t.Fatalf("expected enriched=1 in message, got %q", final.Message)
	}
	if !strings.Contains(final.Message, "enrichment_failed=rds") {
		t.Fatalf("expected enrichment_failed=rds in message, got %q", final.Message)
	}
}

// TestSyncService_TriggerSyncPersistsLabels 验证 hybrid 增强写入 CloudResource.Labels 的
// private_ip/flavor/vpc_id/az 等字段被同步层落库到 Resource.Labels，不再被丢弃。
func TestSyncService_TriggerSyncPersistsLabels(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{
				ResourceID: "res-fake-ecs-1", Name: "ecs-demo-1", Type: "ecs",
				Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-1",
				Labels: map[string]string{
					"namespace":  "SYS.ECS",
					"private_ip": "192.168.1.10",
					"flavor":     "s6.large.2",
					"vpc_id":     "vpc-xxx",
					"az":         "cn-north-4a",
				},
			},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ResourceGroupName: "全部资源", CESTotal: 1, Discovered: 1,
			SuccessfulTypes: []string{"ecs"}, EnrichedCount: 1,
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusSuccess {
		t.Fatalf("expected success batch, got %+v", final)
	}
	if len(resources.rows) != 1 {
		t.Fatalf("expected 1 synced resource, got %d", len(resources.rows))
	}
	row := resources.rows[0]
	want := map[string]string{
		"namespace":  "SYS.ECS",
		"private_ip": "192.168.1.10",
		"flavor":     "s6.large.2",
		"vpc_id":     "vpc-xxx",
		"az":         "cn-north-4a",
	}
	for k, v := range want {
		if got := row.Labels[k]; got != v {
			t.Fatalf("expected label %s=%q, got %q", k, v, got)
		}
	}
}

func TestSyncService_TriggerSyncAuditIncludesHuaweiSummary(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{{ResourceID: "res-1", Name: "ecs-1", Type: "ecs", Region: "cn-north-4", ProviderRef: "ecs-1"}},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ProjectID: "pid-north", SyncMode: "ces",
			ResourceGroupName: "全部资源", ResourceGroupID: "rg-1", ResourceGroupSelection: "max_total",
			CESTotal: 3, Discovered: 1, SuccessfulTypes: []string{"ecs"}, MaxResourcesReached: true,
		},
	}
	accounts := &fakeIntegrationAccountPort{account: &SyncAccountSnapshot{
		AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
	}}
	audit := &capturingAssetAudit{}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, audit)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusPartial {
		t.Fatalf("expected partial when max reached, got %+v", final)
	}
	if !strings.Contains(final.Message, "selected_resource_group=max_total") || !strings.Contains(final.Message, "max_resources_reached=true") {
		t.Fatalf("message missing summary flags: %q", final.Message)
	}
	if len(audit.rows) != 1 {
		t.Fatalf("expected audit row, got %d", len(audit.rows))
	}
	payload := audit.rows[0].Payload
	if payload["sync_mode"] != "ces" || payload["resource_group"] != "全部资源" || payload["ces_total"] != 3 || payload["discovered_count"] != 1 {
		t.Fatalf("unexpected audit payload: %+v", payload)
	}
}

func TestTruncateMessageKeepsUTF8(t *testing.T) {
	msg := strings.Repeat("中", syncBatchMessageMaxRunes+10)
	got := truncateMessage(msg)
	if len([]rune(got)) != syncBatchMessageMaxRunes {
		t.Fatalf("rune len = %d, want %d", len([]rune(got)), syncBatchMessageMaxRunes)
	}
	if strings.ContainsRune(got, '\ufffd') {
		t.Fatalf("message contains replacement rune after truncate")
	}
}

// TestMapCloudResourceToAssetFields 校验 cloud_resource_type -> resource_type 映射对齐
// docs/huawei-ces-asset-sync-plan.md §9.3，重点覆盖 SYS.RDS -> database。
func TestMapCloudResourceToAssetFields(t *testing.T) {
	cases := []struct {
		cloudType string
		want      string
	}{
		{"ecs", "host"},
		{"evs", "storage"},
		{"obs", "storage"},
		{"sfs", "storage"},
		{"vpc", "network"},
		{"vpcep", "network"},
		{"nat", "network"},
		{"rds", "database"},
		{"elb", "service"},
		{"cce", "service"},
		{"apm", "service"},
		{"dcs", "middleware"},
		{"dms", "middleware"},
		{"cbr", "backup"},
		{"ces", "monitor"},
		{"RDS", "database"},
		{"unknown_type", "service"},
	}
	for _, tc := range cases {
		t.Run(tc.cloudType, func(t *testing.T) {
			got, _ := mapCloudResourceToAssetFields(obsdomain.CloudResource{Type: tc.cloudType, ProviderRef: "ref-" + tc.cloudType})
			if got != tc.want {
				t.Fatalf("cloudType=%q: resource_type=%q, want %q", tc.cloudType, got, tc.want)
			}
		})
	}
}

// TestSyncService_TriggerSyncRejectsConcurrentRunning 校验账号级互斥：
// 同一账号已有 running 批次时，第二次 TriggerSync 返回 ALREADY_EXISTS(409)，
// 不会创建第二个批次，避免并发批次交错互相标记 stale。见 docs/huawei-ces-asset-sync-plan.md §P1。
func TestSyncService_TriggerSyncRejectsConcurrentRunning(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{enforceRunningMutex: true}
	// 预置一个仍 running 的批次（租约未过期），模拟并发场景。
	lease := time.Now().UTC().Add(syncBatchLeaseTTL)
	batches.appendRow(domain.SyncBatch{
		BatchID: "sync-existing", IntegrationAccountID: "acc-fake", Provider: "huawei_cloud",
		Status: domain.SyncBatchStatusRunning, StartedAt: time.Now().UTC(), LeaseExpiresAt: &lease,
	})
	discovery := &fakeDiscoveryPort{}
	accounts := &fakeIntegrationAccountPort{account: &SyncAccountSnapshot{
		AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
	}}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)

	_, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err == nil {
		t.Fatalf("expected conflict error, got nil")
	}
	if apperr.CodeOf(err) != apperr.CodeAlreadyExists {
		t.Fatalf("expected ALREADY_EXISTS, got %v", err)
	}
	// 仅预置的那一个 running 批次存在，没有新批次被创建。
	if runningCount := batches.runningCount("acc-fake"); runningCount != 1 {
		t.Fatalf("expected exactly 1 running batch, got %d", runningCount)
	}
}

// TestSyncService_TriggerSyncReapsExpiredLease 校验租约自愈：
// 同一账号存在租约已过期的 running 批次时，下一次 TriggerSync 先 reap 为 failed，
// 再正常创建新批次完成同步，不会因崩溃批次永久 409。
func TestSyncService_TriggerSyncReapsExpiredLease(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{enforceRunningMutex: true}
	// 预置一个租约已过期的 running 批次（模拟进程崩溃遗留）。
	expired := time.Now().UTC().Add(-1 * time.Minute)
	batches.appendRow(domain.SyncBatch{
		BatchID: "sync-stale", IntegrationAccountID: "acc-fake", Provider: "huawei_cloud",
		Status: domain.SyncBatchStatusRunning, StartedAt: time.Now().UTC().Add(-20 * time.Minute), LeaseExpiresAt: &expired,
	})
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{{ResourceID: "res-1", Name: "ecs-1", Type: "ecs", Region: "cn-north-4", ProviderRef: "ecs-1"}},
	}
	accounts := &fakeIntegrationAccountPort{account: &SyncAccountSnapshot{
		AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
	}}
	audit := &capturingAssetAudit{}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, audit)

	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusSuccess {
		t.Fatalf("expected success after reap, got %+v", final)
	}
	// 旧批次应已被 reap 为 failed。
	stale, ok := batches.rowSnapshot("sync-stale")
	if !ok {
		t.Fatalf("expected reaped stale batch to remain in repo")
	}
	if stale.Status != domain.SyncBatchStatusFailed {
		t.Fatalf("expected reaped batch status=failed, got %s", stale.Status)
	}
	if stale.LeaseExpiresAt != nil {
		t.Fatalf("expected reaped batch lease cleared, got %v", stale.LeaseExpiresAt)
	}
	if len(audit.rows) != 2 {
		t.Fatalf("expected reap audit and final sync audit, got %d", len(audit.rows))
	}
	reapAudit := audit.rows[0]
	if reapAudit.ResourceType != "asset_sync_batch" || reapAudit.ResourceID != "acc-fake" || reapAudit.Action != AuditAssetSync || reapAudit.UserID != "u1" {
		t.Fatalf("unexpected reap audit metadata: %+v", reapAudit)
	}
	if reapAudit.Payload["event"] != "reap_expired_running" || reapAudit.Payload["account_id"] != "acc-fake" || reapAudit.Payload["reaped_count"] != int64(1) || reapAudit.Payload["result"] != "success" {
		t.Fatalf("unexpected reap audit payload: %+v", reapAudit.Payload)
	}
}

// blockingDiscoveryPort 让 ListAllResources 在 delay 后返回；ctx 取消时立即返回 ctx 错误。
// 用于异步生命周期测试：制造足够长的同步耗时以观察续租/取消/硬超时行为。
type blockingDiscoveryPort struct {
	resources []obsdomain.CloudResource
	delay     time.Duration
}

func (p *blockingDiscoveryPort) ListResources(_ context.Context, _ obsapp.Actor, _ obsdomain.AssetDiscoveryQuery) (*obsapp.AssetDiscoveryResult, error) {
	return &obsapp.AssetDiscoveryResult{Resources: p.resources}, nil
}

func (p *blockingDiscoveryPort) ListAllResources(ctx context.Context, _ obsapp.Actor, q obsapp.AssetFullSyncQuery) (*obsapp.AssetFullSyncResult, error) {
	timer := time.NewTimer(p.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	out := make([]obsdomain.CloudResource, 0, len(p.resources))
	for _, item := range p.resources {
		if q.Region != "" && item.Region != q.Region {
			continue
		}
		out = append(out, item)
	}
	return &obsapp.AssetFullSyncResult{Resources: out, Summary: obsapp.CloudSyncSummary{Region: q.Region, Discovered: len(out)}}, nil
}

// TestSyncService_AsyncRenewsLeaseDuringSync 验证后台同步期间心跳续租 lease_expires_at，
// 避免正常同步超过 TTL 被 reap。用短心跳间隔 + 阻塞 discovery 制造续租窗口。
func TestSyncService_AsyncRenewsLeaseDuringSync(t *testing.T) {
	origInterval := syncLeaseRenewInterval
	syncLeaseRenewInterval = 5 * time.Millisecond
	defer func() { syncLeaseRenewInterval = origInterval }()

	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &blockingDiscoveryPort{
		resources: []obsdomain.CloudResource{{ResourceID: "res-1", Name: "ecs-1", Type: "ecs", Region: "cn-north-4", ProviderRef: "ecs-1"}},
		delay:     40 * time.Millisecond, // > 2 个心跳周期，保证续租被触发
	}
	accounts := &fakeIntegrationAccountPort{account: &SyncAccountSnapshot{
		AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
	}}
	audit := &capturingAssetAudit{}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, audit)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	if out.Status != domain.SyncBatchStatusRunning {
		t.Fatalf("expected running immediately, got %s", out.Status)
	}
	svc.Wait()
	if batches.renewLeaseCountValue() < 1 {
		t.Fatalf("expected at least 1 lease renewal during sync, got %d", batches.renewLeaseCountValue())
	}
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final: %v", err)
	}
	if final.Status != domain.SyncBatchStatusSuccess {
		t.Fatalf("expected success after async sync, got %s", final.Status)
	}
	if final.LeaseExpiresAt != nil {
		t.Fatalf("terminal batch should clear lease, got %v", final.LeaseExpiresAt)
	}
}

func TestSyncService_WaitContextReturnsFalseWhenContextCancelled(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &blockingDiscoveryPort{delay: 5 * time.Second}
	accounts := &fakeIntegrationAccountPort{account: &SyncAccountSnapshot{
		AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
	}}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)

	if _, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"}); err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if svc.WaitContext(waitCtx) {
		t.Fatal("expected WaitContext to return false after timeout")
	}
}

// TestSyncService_CancelledReachesTerminal 验证 runCtx 取消（进程关闭）时，
// finalize 用独立短 context 仍能把批次落为 failed，不卡 running。
func TestSyncService_CancelledReachesTerminal(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &blockingDiscoveryPort{delay: 5 * time.Second} // 长阻塞，等取消
	accounts := &fakeIntegrationAccountPort{account: &SyncAccountSnapshot{
		AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
	}}
	ctx, cancel := context.WithCancel(context.Background())
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	svc.SetLifecycle(ctx)

	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		cancel()
		t.Fatalf("TriggerSync: %v", err)
	}
	// 取消 shutdownCtx，模拟进程关闭：runCtx 随之取消，discovery 返回 ctx 错误，finalize 落 failed。
	cancel()
	svc.Wait()

	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final: %v", err)
	}
	if final.Status != domain.SyncBatchStatusFailed {
		t.Fatalf("expected failed after cancel, got %s", final.Status)
	}
	if final.FinishedAt == nil {
		t.Fatal("expected finished_at set on cancelled batch")
	}
	if final.LeaseExpiresAt != nil {
		t.Fatalf("terminal batch should clear lease, got %v", final.LeaseExpiresAt)
	}
}

// TestSyncService_HardTimeoutFails 验证 goroutine 硬超时触发时批次落 failed，
// 防止失控 goroutine 无限占用账号 running 槽位。
func TestSyncService_HardTimeoutFails(t *testing.T) {
	origTimeout := syncHardTimeout
	syncHardTimeout = 20 * time.Millisecond
	defer func() { syncHardTimeout = origTimeout }()

	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &blockingDiscoveryPort{delay: 5 * time.Second} // 远超硬超时
	accounts := &fakeIntegrationAccountPort{account: &SyncAccountSnapshot{
		AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
	}}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)

	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()

	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final: %v", err)
	}
	if final.Status != domain.SyncBatchStatusFailed {
		t.Fatalf("expected failed after hard timeout, got %s", final.Status)
	}
	if final.FinishedAt == nil {
		t.Fatal("expected finished_at set on timed-out batch")
	}
}

// TestSyncService_HuaweiCESDroppedTypeMarkedStale 验证 CES 权威 scope 反向 stale：
// 资源组从 ECS+EVS 改为仅 ECS 后，EVS 不再出现在本轮 product_names scope，
// 历史 EVS 资产应被标记 stale，而不是永久保持 active，见 §13.1。
func TestSyncService_LeaseRenewDBErrorRetries(t *testing.T) {
	origInterval := syncLeaseRenewInterval
	syncLeaseRenewInterval = 5 * time.Millisecond
	defer func() { syncLeaseRenewInterval = origInterval }()

	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{renewErrors: []error{errors.New("temporary db error")}}
	discovery := &blockingDiscoveryPort{
		resources: []obsdomain.CloudResource{{ResourceID: "res-1", Name: "ecs-1", Type: "ecs", Region: "cn-north-4", ProviderRef: "ecs-1"}},
		delay:     40 * time.Millisecond,
	}
	accounts := &fakeIntegrationAccountPort{account: &SyncAccountSnapshot{
		AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
	}}
	audit := &capturingAssetAudit{}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, audit)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	if batches.renewLeaseCountValue() < 1 {
		t.Fatalf("expected renewal retry to eventually succeed, got %d", batches.renewLeaseCountValue())
	}
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final: %v", err)
	}
	if final.Status != domain.SyncBatchStatusSuccess {
		t.Fatalf("expected success after transient renew error, got %s message=%q", final.Status, final.Message)
	}
}

func TestSyncService_LeaseLostCancelsSync(t *testing.T) {
	origInterval := syncLeaseRenewInterval
	syncLeaseRenewInterval = 5 * time.Millisecond
	defer func() { syncLeaseRenewInterval = origInterval }()

	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{renewErrors: []error{domain.ErrLeaseLost}}
	discovery := &blockingDiscoveryPort{
		resources: []obsdomain.CloudResource{{ResourceID: "res-1", Name: "ecs-1", Type: "ecs", Region: "cn-north-4", ProviderRef: "ecs-1"}},
		delay:     5 * time.Second,
	}
	accounts := &fakeIntegrationAccountPort{account: &SyncAccountSnapshot{
		AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
	}}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final: %v", err)
	}
	if final.Status != domain.SyncBatchStatusFailed {
		t.Fatalf("expected failed after lease lost, got %s", final.Status)
	}
	if !strings.Contains(final.Message, "sync cancelled or timed out") {
		t.Fatalf("expected cancelled message, got %q", final.Message)
	}
}

func TestSyncService_FencingBlocksStaleAfterLeaseReaped(t *testing.T) {
	origInterval := syncLeaseRenewInterval
	syncLeaseRenewInterval = time.Hour
	defer func() { syncLeaseRenewInterval = origInterval }()

	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	leaseExpires := time.Now().UTC().Add(syncBatchLeaseTTL)
	batches := &fakeSyncBatchRepo{}
	resources := &fakeResRepo{leaseOwned: batches.isLeaseOwned, rows: []domain.Resource{
		{
			ID: "new-batch-res", ApplicationID: appID, Name: "new-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "new-ecs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-new", LastSyncedAt: &syncedAt,
		},
	}}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-old", Name: "old-ecs", Type: "ecs", Region: "cn-north-4", ProviderRef: "old-ecs"},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ResourceGroupName: "全部资源", CESTotal: 1, Discovered: 1,
			SuccessfulTypes: []string{"ecs"},
		},
		beforeFullReturn: func() {
			batches.failRunningBatches("lease expired; previous sync batch interrupted")
		},
	}
	accounts := &fakeIntegrationAccountPort{account: &SyncAccountSnapshot{
		AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
	}}
	audit := &capturingAssetAudit{}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, audit)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	batches.setLeaseExpires(out.BatchID, &leaseExpires)
	svc.Wait()
	if resources.rows[0].SyncStatus != domain.SyncStatusActive {
		t.Fatalf("new batch resource must remain active when old task loses fence, got %s", resources.rows[0].SyncStatus)
	}
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final: %v", err)
	}
	if final.Status != domain.SyncBatchStatusFailed {
		t.Fatalf("expected failed batch after fence lost, got %+v", final)
	}
	if final.StaleCount != 0 {
		t.Fatalf("expected stale_count=0 after fence lost, got %d", final.StaleCount)
	}
	if len(audit.rows) != 0 {
		t.Fatalf("expected no final sync audit after lease lost, got %d", len(audit.rows))
	}
}

func TestSyncService_HuaweiCESDroppedTypeMarkedStale(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	batches := &fakeSyncBatchRepo{}
	resources := &fakeResRepo{leaseOwned: batches.isLeaseOwned, rows: []domain.Resource{
		{
			ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
		{
			ID: "old-evs", ApplicationID: appID, Name: "old-evs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "evs", CloudResourceID: "gone-evs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
	}}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-2", Name: "ecs-demo-2", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-2"},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", SyncMode: "ces", ResourceGroupName: "全部资源",
			CESTotal: 1, Discovered: 1, SuccessfulTypes: []string{"ecs"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	statusByID := map[string]string{}
	for _, row := range resources.rows {
		statusByID[row.ID] = row.SyncStatus
	}
	// old-ecs 不在本批（gone-ecs），应 stale；old-evs 已从资源组移除，应 stale。
	if statusByID["old-ecs"] != domain.SyncStatusStale {
		t.Fatalf("expected old ecs stale, got %s", statusByID["old-ecs"])
	}
	if statusByID["old-evs"] != domain.SyncStatusStale {
		t.Fatalf("expected dropped evs stale, got %s", statusByID["old-evs"])
	}
	if final.StaleCount != 2 {
		t.Fatalf("expected stale_count=2, got %d", final.StaleCount)
	}
	if final.Status != domain.SyncBatchStatusSuccess {
		t.Fatalf("expected success batch, got %+v", final)
	}
}

// TestSyncService_HuaweiCESNegativeMarkingExcludesUncertainTypes 验证 CES 反向 stale 的
// exceptTypes 门控：查询失败(QueryFailedTypes)、转换失败(ConversionFailedTypes)的类型保持 active，
// 其余类型（含已移除类型）标记 stale，见 §13.1。
func TestSyncService_HuaweiCESNegativeMarkingExcludesUncertainTypes(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	batches := &fakeSyncBatchRepo{}
	resources := &fakeResRepo{leaseOwned: batches.isLeaseOwned, rows: []domain.Resource{
		{
			ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
		{
			ID: "old-evs", ApplicationID: appID, Name: "old-evs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "evs", CloudResourceID: "keep-evs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
		{
			ID: "old-rds", ApplicationID: appID, Name: "old-rds",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "rds", CloudResourceID: "keep-rds", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
		{
			ID: "old-vpc", ApplicationID: appID, Name: "old-vpc",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "vpc", CloudResourceID: "gone-vpc", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
	}}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-2", Name: "ecs-demo-2", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-2"},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", SyncMode: "ces", ResourceGroupName: "全部资源",
			CESTotal: 1, Discovered: 1, SuccessfulTypes: []string{"ecs"},
			QueryFailedTypes:      []string{"rds"},
			ConversionFailedTypes: []string{"evs"},
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	statusByID := map[string]string{}
	for _, row := range resources.rows {
		statusByID[row.ID] = row.SyncStatus
	}
	// ecs 不在本批 → stale；vpc 已从资源组移除且非不确定 → stale。
	if statusByID["old-ecs"] != domain.SyncStatusStale {
		t.Fatalf("expected old ecs stale, got %s", statusByID["old-ecs"])
	}
	if statusByID["old-vpc"] != domain.SyncStatusStale {
		t.Fatalf("expected dropped vpc stale, got %s", statusByID["old-vpc"])
	}
	// rds 查询失败、evs 转换失败：不确定，保持 active。
	if statusByID["old-rds"] != domain.SyncStatusActive {
		t.Fatalf("query-failed rds must remain active, got %s", statusByID["old-rds"])
	}
	if statusByID["old-evs"] != domain.SyncStatusActive {
		t.Fatalf("conversion-failed evs must remain active, got %s", statusByID["old-evs"])
	}
	if final.StaleCount != 2 {
		t.Fatalf("expected stale_count=2, got %d", final.StaleCount)
	}
}

// TestSyncService_HuaweiCESProductNamesEmptyMarksPartial 验证 product_names 为空时使用兜底白名单，
// 批次至少标记 partial，提示同步可能不完整；同时禁止 CES/hybrid 反向 stale，
// 只允许完整成功的类型逐类型 stale，白名单外历史资产保持 active，见 §8.5、§13.1。
func TestSyncService_HuaweiCESProductNamesEmptyMarksPartial(t *testing.T) {
	appID := cloudApplicationID("acc-fake")
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"cloud": {ID: appID, Name: "huawei_cloud-cloud", Environment: "cloud"},
	}}
	syncedAt := time.Now().UTC().Add(-time.Hour)
	batches := &fakeSyncBatchRepo{}
	resources := &fakeResRepo{leaseOwned: batches.isLeaseOwned, rows: []domain.Resource{
		{
			ID: "old-ecs", ApplicationID: appID, Name: "old-ecs",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "ecs", CloudResourceID: "gone-ecs", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
		{
			ID: "old-apm", ApplicationID: appID, Name: "old-apm",
			Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-fake",
			CloudResourceType: "apm", CloudResourceID: "keep-apm", Region: "cn-north-4",
			SyncStatus: domain.SyncStatusActive, SyncBatchID: "sync-old", LastSyncedAt: &syncedAt,
		},
	}}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-fake-ecs-1", Name: "ecs-demo-1", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-demo-1"},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", SyncMode: "ces", ResourceGroupName: "全部资源",
			CESTotal: 1, Discovered: 1, SuccessfulTypes: []string{"ecs"},
			ProductNamesEmpty: true,
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusPartial {
		t.Fatalf("expected partial batch when product_names empty, got %+v", final)
	}
	if !strings.Contains(final.Message, "product_names_empty=true") {
		t.Fatalf("expected product_names_empty note in message, got %q", final.Message)
	}
	statusByID := map[string]string{}
	for _, row := range resources.rows {
		statusByID[row.ID] = row.SyncStatus
	}
	if statusByID["old-ecs"] != domain.SyncStatusStale {
		t.Fatalf("complete ecs scope should still mark old ecs stale, got %s", statusByID["old-ecs"])
	}
	if statusByID["old-apm"] != domain.SyncStatusActive {
		t.Fatalf("product_names_empty must not negative-mark whitelist-excluded apm stale, got %s", statusByID["old-apm"])
	}
	if final.StaleCount != 1 {
		t.Fatalf("expected stale_count=1 (only complete ecs scope), got %d", final.StaleCount)
	}
}

// TestSyncService_FullSyncAllRegionsFailedSummaryCoversRegions 验证全部 region 失败时，
// 终态 summary 不为空且覆盖所有失败 region，见 ops/cloud-observability-contract.md §5.5.1。
func TestSyncService_FullSyncAllRegionsFailedSummaryCoversRegions(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		errors: map[string]error{
			"cn-north-4/": apperr.New(apperr.CodeUnavailable, "provider api unavailable"),
			"cn-south-1/": apperr.New(apperr.CodeUnavailable, "provider api unavailable"),
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4", "cn-south-1"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusFailed {
		t.Fatalf("expected failed batch, got %s message=%q", final.Status, final.Message)
	}
	dto := toSyncBatchDTO(*final)
	if dto.Summary == nil {
		t.Fatalf("expected summary to cover failed regions, got nil")
	}
	if !sliceContainsString(dto.Summary.Regions, "cn-north-4") || !sliceContainsString(dto.Summary.Regions, "cn-south-1") {
		t.Fatalf("expected summary.regions to include both failed regions, got %v", dto.Summary.Regions)
	}
	if len(dto.Summary.FailedScopes) == 0 || !strings.Contains(dto.Summary.FailedScopes[0], "provider api unavailable") {
		t.Fatalf("expected summary.failed_scopes to include region errors, got %v", dto.Summary.FailedScopes)
	}
}

// TestSyncService_FullSyncPartialRegionsFailedSummaryCoversAll 验证部分 region 失败时，
// 成功 region 的资源组信息保留，失败 region 也进入 summary.failed_scopes。
func TestSyncService_FullSyncPartialRegionsFailedSummaryCoversAll(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{}}
	resources := &fakeResRepo{}
	batches := &fakeSyncBatchRepo{}
	discovery := &fakeDiscoveryPort{
		resources: []obsdomain.CloudResource{
			{ResourceID: "res-1", Name: "ecs-1", Type: "ecs", Region: "cn-north-4", Status: "running", ProviderRef: "ecs-1"},
		},
		fullSummary: &obsapp.CloudSyncSummary{
			Region: "cn-north-4", ProjectID: "pid-north", SyncMode: "ces",
			ResourceGroupName: "全部资源", ResourceGroupID: "rg-001",
			CESTotal: 1, Discovered: 1, SuccessfulTypes: []string{"ecs"},
		},
		errors: map[string]error{
			"cn-south-1/": apperr.New(apperr.CodeUnavailable, "south api unavailable"),
		},
	}
	accounts := &fakeIntegrationAccountPort{
		account: &SyncAccountSnapshot{
			AccountID: "acc-fake", Provider: "huawei_cloud", Regions: []string{"cn-north-4", "cn-south-1"}, Enabled: true,
		},
	}
	svc := NewSyncService(apps, resources, batches, discovery, accounts, nil)
	out, err := svc.TriggerSync(context.Background(), Actor{UserID: "u1"}, TriggerSyncInput{AccountID: "acc-fake"})
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
	svc.Wait()
	final, err := batches.GetByID(context.Background(), out.BatchID)
	if err != nil {
		t.Fatalf("load final batch: %v", err)
	}
	if final.Status != domain.SyncBatchStatusPartial {
		t.Fatalf("expected partial batch, got %s message=%q", final.Status, final.Message)
	}
	dto := toSyncBatchDTO(*final)
	if dto.Summary == nil {
		t.Fatalf("expected summary to cover all regions, got nil")
	}
	if !sliceContainsString(dto.Summary.Regions, "cn-north-4") || !sliceContainsString(dto.Summary.Regions, "cn-south-1") {
		t.Fatalf("expected summary.regions to include both regions, got %v", dto.Summary.Regions)
	}
	if dto.Summary.ResourceGroupName != "全部资源" || dto.Summary.ResourceGroupID != "rg-001" {
		t.Fatalf("expected summary to keep successful region group info, got %+v", dto.Summary)
	}
	if len(dto.Summary.FailedScopes) == 0 || !strings.Contains(dto.Summary.FailedScopes[0], "south api unavailable") {
		t.Fatalf("expected summary.failed_scopes to include failed region error, got %v", dto.Summary.FailedScopes)
	}
}

func sliceContainsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}
