package application

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
)

type fakeAppRepo struct {
	mu   sync.Mutex
	apps map[string]domain.Application
}

func (r *fakeAppRepo) Create(_ context.Context, app *domain.Application) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.apps[app.Name] = *app
	return nil
}

func (r *fakeAppRepo) List(_ context.Context) ([]domain.Application, error) { return nil, nil }

func (r *fakeAppRepo) ListPaged(_ context.Context, filter domain.ApplicationFilter) ([]domain.Application, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := make([]domain.Application, 0, len(r.apps))
	for _, app := range r.apps {
		all = append(all, app)
	}
	total := int64(len(all))
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	out := make([]domain.Application, 0)
	for i := offset; i < len(all) && len(out) < limit; i++ {
		out = append(out, all[i])
	}
	return out, total, nil
}

func (r *fakeAppRepo) Count(_ context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.apps)), nil
}

func (r *fakeAppRepo) FindByNameEnv(_ context.Context, name, environment string) (*domain.Application, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	app, ok := r.apps[name]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if environment != "" && app.Environment != "" && app.Environment != environment {
		return nil, domain.ErrNotFound
	}
	cp := app
	return &cp, nil
}

func (r *fakeAppRepo) ExistsByID(_ context.Context, id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, app := range r.apps {
		if app.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeAppRepo) GetByID(_ context.Context, id string) (*domain.Application, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, app := range r.apps {
		if app.ID == id {
			cp := app
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeAppRepo) Update(_ context.Context, app *domain.Application) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, row := range r.apps {
		if row.ID == app.ID {
			r.apps[name] = *app
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeAppRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, app := range r.apps {
		if app.ID == id {
			delete(r.apps, name)
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeResRepo struct {
	mu           sync.Mutex
	rows         []domain.Resource
	upsertErr    error            // 非 nil 时所有 UpsertCloudSync 失败
	upsertErrFor map[string]error // 按 CloudResourceType 注入 upsert 失败
	leaseOwned   func(batchID, fencingToken string) bool
	syncStarted  map[string]time.Time
}

func (r *fakeResRepo) rowsSnapshot() []domain.Resource {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Resource, len(r.rows))
	copy(out, r.rows)
	return out
}

func (r *fakeResRepo) syncStatusAt(index int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if index < 0 || index >= len(r.rows) {
		return ""
	}
	return r.rows[index].SyncStatus
}

func (r *fakeResRepo) Create(_ context.Context, res *domain.Resource) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, *res)
	return nil
}

func (r *fakeResRepo) Count(_ context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.rows)), nil
}

func (r *fakeResRepo) ListByApplicationID(_ context.Context, applicationID string) ([]domain.Resource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Resource, 0)
	for _, row := range r.rows {
		if row.ApplicationID == applicationID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeResRepo) ListByApplicationIDPaged(_ context.Context, applicationID string, filter domain.ResourceFilter) ([]domain.Resource, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	matched := make([]domain.Resource, 0)
	for _, row := range r.rows {
		if row.ApplicationID != applicationID {
			continue
		}
		if filter.CloudResourceType != "" && row.CloudResourceType != filter.CloudResourceType {
			continue
		}
		if filter.Region != "" && row.Region != filter.Region {
			continue
		}
		if filter.SyncStatus != "" && row.SyncStatus != filter.SyncStatus {
			continue
		}
		matched = append(matched, row)
	}
	total := int64(len(matched))
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	out := make([]domain.Resource, 0)
	for i := offset; i < len(matched) && len(out) < limit; i++ {
		out = append(out, matched[i])
	}
	return out, total, nil
}

func (r *fakeResRepo) FindBestMatch(_ context.Context, q domain.ResourceMatchQuery) (*domain.Resource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rows {
		row := r.rows[i]
		if row.ApplicationID != q.ApplicationID {
			continue
		}
		if q.Pod != "" && row.Pod == q.Pod {
			cp := row
			return &cp, nil
		}
		if q.Instance != "" && row.Instance == q.Instance {
			cp := row
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeResRepo) GetByID(_ context.Context, id string) (*domain.Resource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rows {
		if r.rows[i].ID == id {
			cp := r.rows[i]
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeResRepo) Update(_ context.Context, res *domain.Resource) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.updateLocked(res)
}

func (r *fakeResRepo) updateLocked(res *domain.Resource) error {
	for i := range r.rows {
		if r.rows[i].ID == res.ID {
			r.rows[i] = *res
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeResRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rows {
		if r.rows[i].ID == id {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeResRepo) CountByApplicationID(_ context.Context, applicationID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, row := range r.rows {
		if row.ApplicationID == applicationID {
			n++
		}
	}
	return n, nil
}

func (r *fakeResRepo) FindByCloudKey(_ context.Context, key domain.CloudResourceKey) (*domain.Resource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.findByCloudKeyLocked(key)
}

func (r *fakeResRepo) findByCloudKeyLocked(key domain.CloudResourceKey) (*domain.Resource, error) {
	for i := range r.rows {
		row := r.rows[i]
		if row.IntegrationAccountID == key.IntegrationAccountID &&
			row.CloudResourceType == key.CloudResourceType &&
			row.CloudResourceID == key.CloudResourceID &&
			row.Region == key.Region {
			cp := row
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeResRepo) UpsertCloudSync(_ context.Context, res *domain.Resource) (bool, error) {
	return r.upsertCloudSync(res)
}

func (r *fakeResRepo) UpsertCloudSyncWithLease(_ context.Context, res *domain.Resource, batchID, fencingToken string) (bool, error) {
	if r.leaseOwned != nil && !r.leaseOwned(batchID, fencingToken) {
		return false, domain.ErrLeaseLost
	}
	return r.upsertCloudSyncWithBatch(res, batchID)
}

func (r *fakeResRepo) UpsertCloudSyncBatchWithLease(_ context.Context, resources []*domain.Resource, batchID, fencingToken string) (int, int, error) {
	if r.leaseOwned != nil && !r.leaseOwned(batchID, fencingToken) {
		return 0, 0, domain.ErrLeaseLost
	}
	created, updated := 0, 0
	for _, res := range resources {
		if res == nil {
			continue
		}
		if ok, err := r.upsertCloudSyncWithBatch(res, batchID); err != nil {
			return created, updated, err
		} else if ok {
			created++
		} else {
			updated++
		}
	}
	return created, updated, nil
}

func (r *fakeResRepo) PromoteSuccessfulSyncBatch(_ context.Context, accountID, batchID string, syncedSince time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for i := range r.rows {
		row := &r.rows[i]
		if row.Source == domain.ResourceSourceCloudSync &&
			row.IntegrationAccountID == accountID &&
			row.SyncStatus == domain.SyncStatusActive &&
			row.LastSyncedAt != nil && !row.LastSyncedAt.Before(syncedSince) {
			row.SyncBatchID = batchID
			n++
		}
	}
	return n, nil
}

func (r *fakeResRepo) upsertCloudSync(res *domain.Resource) (bool, error) {
	return r.upsertCloudSyncWithBatch(res, "")
}

func (r *fakeResRepo) upsertCloudSyncWithBatch(res *domain.Resource, batchID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.upsertErr != nil {
		return false, r.upsertErr
	}
	if r.upsertErrFor != nil {
		if err := r.upsertErrFor[res.CloudResourceType]; err != nil {
			return false, err
		}
	}
	if batchID != "" && res.LastSyncedAt != nil {
		if r.syncStarted == nil {
			r.syncStarted = map[string]time.Time{}
		}
		if _, ok := r.syncStarted[batchID]; !ok {
			r.syncStarted[batchID] = *res.LastSyncedAt
		}
	}
	key := domain.CloudResourceKey{
		IntegrationAccountID: res.IntegrationAccountID,
		CloudResourceType:    res.CloudResourceType,
		CloudResourceID:      res.CloudResourceID,
		Region:               res.Region,
	}
	if existing, err := r.findByCloudKeyLocked(key); err == nil && existing != nil {
		res.ID = existing.ID
		res.SyncBatchID = existing.SyncBatchID
		return false, r.updateLocked(res)
	}
	r.rows = append(r.rows, *res)
	return true, nil
}

func (r *fakeResRepo) MarkStaleByAccountScopeExceptBatch(_ context.Context, accountID, region, cloudResourceType, batchID string) (int64, error) {
	return r.markStaleByAccountScopeExceptBatch(accountID, region, cloudResourceType, batchID)
}

func (r *fakeResRepo) MarkStaleByAccountScopeExceptBatchWithLease(_ context.Context, accountID, region, cloudResourceType, batchID, fencingToken string) (int64, error) {
	if r.leaseOwned != nil && !r.leaseOwned(batchID, fencingToken) {
		return 0, domain.ErrLeaseLost
	}
	return r.markStaleByAccountScopeExceptBatch(accountID, region, cloudResourceType, batchID)
}

func (r *fakeResRepo) markStaleByAccountScopeExceptBatch(accountID, region, cloudResourceType, batchID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var startedAt time.Time
	if r.syncStarted != nil {
		startedAt = r.syncStarted[batchID]
	}
	var n int64
	for i := range r.rows {
		row := &r.rows[i]
		if row.Source == domain.ResourceSourceCloudSync &&
			row.IntegrationAccountID == accountID &&
			row.Region == region &&
			row.CloudResourceType == cloudResourceType &&
			(startedAt.IsZero() || row.LastSyncedAt == nil || row.LastSyncedAt.Before(startedAt)) &&
			row.SyncStatus == domain.SyncStatusActive {
			row.SyncStatus = domain.SyncStatusStale
			n++
		}
	}
	return n, nil
}

// MarkStaleByAccountRegionExceptTypes 模拟反向 stale：account+region 下所有 active 的 cloud_sync
// 资源（排除当前批次）标记 stale，但跳过 exceptTypes 中的类型，见 §13.1。
func (r *fakeResRepo) MarkStaleByAccountRegionExceptTypes(_ context.Context, accountID, region string, exceptTypes []string, batchID string) (int64, error) {
	return r.markStaleByAccountRegionExceptTypes(accountID, region, exceptTypes, batchID)
}

func (r *fakeResRepo) MarkStaleByAccountRegionExceptTypesWithLease(_ context.Context, accountID, region string, exceptTypes []string, batchID, fencingToken string) (int64, error) {
	if r.leaseOwned != nil && !r.leaseOwned(batchID, fencingToken) {
		return 0, domain.ErrLeaseLost
	}
	return r.markStaleByAccountRegionExceptTypes(accountID, region, exceptTypes, batchID)
}

func (r *fakeResRepo) PatchCloudSyncLabelsBatchWithLease(_ context.Context, patches []domain.CloudSyncLabelPatch, batchID, fencingToken string) (int, error) {
	if r.leaseOwned != nil && !r.leaseOwned(batchID, fencingToken) {
		return 0, domain.ErrLeaseLost
	}
	return 0, nil
}

func (r *fakeResRepo) markStaleByAccountRegionExceptTypes(accountID, region string, exceptTypes []string, batchID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	except := make(map[string]struct{}, len(exceptTypes))
	for _, t := range exceptTypes {
		except[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}
	var startedAt time.Time
	if r.syncStarted != nil {
		startedAt = r.syncStarted[batchID]
	}
	var n int64
	for i := range r.rows {
		row := &r.rows[i]
		if row.Source == domain.ResourceSourceCloudSync &&
			row.IntegrationAccountID == accountID &&
			row.Region == region &&
			(startedAt.IsZero() || row.LastSyncedAt == nil || row.LastSyncedAt.Before(startedAt)) &&
			row.SyncStatus == domain.SyncStatusActive {
			if _, skip := except[strings.ToLower(strings.TrimSpace(row.CloudResourceType))]; skip {
				continue
			}
			row.SyncStatus = domain.SyncStatusStale
			n++
		}
	}
	return n, nil
}

type fakeRuleRepo struct {
	rules []domain.MatchRule
}

func (r *fakeRuleRepo) Create(_ context.Context, rule *domain.MatchRule) error {
	r.rules = append(r.rules, *rule)
	return nil
}

func (r *fakeRuleRepo) List(_ context.Context) ([]domain.MatchRule, error) { return r.rules, nil }

func (r *fakeRuleRepo) ListPaged(_ context.Context, filter domain.MatchRuleFilter) ([]domain.MatchRule, int64, error) {
	total := int64(len(r.rules))
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	out := make([]domain.MatchRule, 0)
	for i := offset; i < len(r.rules) && len(out) < limit; i++ {
		out = append(out, r.rules[i])
	}
	return out, total, nil
}

func (r *fakeRuleRepo) ListEnabledByPriority(_ context.Context) ([]domain.MatchRule, error) {
	out := make([]domain.MatchRule, 0)
	for _, rule := range r.rules {
		if rule.Enabled {
			out = append(out, rule)
		}
	}
	return out, nil
}

func (r *fakeRuleRepo) GetByID(_ context.Context, id string) (*domain.MatchRule, error) {
	for i := range r.rules {
		if r.rules[i].ID == id {
			cp := r.rules[i]
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeRuleRepo) Update(_ context.Context, rule *domain.MatchRule) error {
	for i := range r.rules {
		if r.rules[i].ID == rule.ID {
			r.rules[i] = *rule
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeRuleRepo) Delete(_ context.Context, id string) error {
	for i := range r.rules {
		if r.rules[i].ID == id {
			r.rules = append(r.rules[:i], r.rules[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeRuleRepo) CountByApplicationID(_ context.Context, applicationID string) (int64, error) {
	var n int64
	for _, rule := range r.rules {
		if rule.ApplicationID == applicationID {
			n++
		}
	}
	return n, nil
}

func (r *fakeRuleRepo) CountByResourceID(_ context.Context, resourceID string) (int64, error) {
	var n int64
	for _, rule := range r.rules {
		if rule.ResourceID == resourceID {
			n++
		}
	}
	return n, nil
}

func TestNormalizeAssetPage(t *testing.T) {
	page, pageSize := normalizeAssetPage(0, 0)
	if page != 1 || pageSize != 20 {
		t.Fatalf("expected defaults page=1 page_size=20, got page=%d page_size=%d", page, pageSize)
	}
	page, pageSize = normalizeAssetPage(2, 200)
	if page != 2 || pageSize != 100 {
		t.Fatalf("expected capped page_size=100, got page=%d page_size=%d", page, pageSize)
	}
}

func TestMatcherService_MatchApplicationAndPod(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment-service": {ID: "app-1", Name: "payment-service", Environment: "prod"},
	}}
	resources := &fakeResRepo{rows: []domain.Resource{
		{ID: "res-1", ApplicationID: "app-1", Pod: "payment-xxx-1", Namespace: "payment"},
	}}
	svc := NewMatcherService(apps, resources, nil)
	out, err := svc.Match(context.Background(), MatchInput{
		ApplicationName: "payment-service",
		Environment:     "prod",
		Labels:          map[string]string{"pod": "payment-xxx-1", "namespace": "payment"},
	})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "app-1" || out.ResourceID != "res-1" {
		t.Fatalf("unexpected match result: %+v", out)
	}
}

func TestMatcherService_MatchByServiceLabel(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"checkout-service": {ID: "app-2", Name: "checkout-service", Environment: "prod"},
	}}
	svc := NewMatcherService(apps, &fakeResRepo{}, nil)
	out, err := svc.Match(context.Background(), MatchInput{
		Environment: "prod",
		Labels:      map[string]string{"service": "checkout-service"},
	})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "app-2" || out.ResourceID != "" {
		t.Fatalf("unexpected match result: %+v", out)
	}
}

func TestMatcherService_MatchInstanceWhenNoPod(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"metrics-app": {ID: "app-3", Name: "metrics-app", Environment: "prod"},
	}}
	resources := &fakeResRepo{rows: []domain.Resource{
		{ID: "res-2", ApplicationID: "app-3", Instance: "node-1"},
	}}
	svc := NewMatcherService(apps, resources, nil)
	out, err := svc.Match(context.Background(), MatchInput{
		ApplicationName: "metrics-app",
		Environment:     "prod",
		Labels:          map[string]string{"instance": "node-1"},
	})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "app-3" || out.ResourceID != "res-2" {
		t.Fatalf("unexpected match result: %+v", out)
	}
}

func TestMatcherService_NoMatchReturnsEmpty(t *testing.T) {
	svc := NewMatcherService(&fakeAppRepo{apps: map[string]domain.Application{}}, &fakeResRepo{}, nil)
	out, err := svc.Match(context.Background(), MatchInput{ApplicationName: "missing"})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if out.ApplicationID != "" || out.ResourceID != "" {
		t.Fatalf("expected empty match, got %+v", out)
	}
}

func TestAssetService_CreateResourceApplicationNotFound(t *testing.T) {
	svc := NewAssetService(&fakeAppRepo{apps: map[string]domain.Application{}}, &fakeResRepo{}, nil, nil, NoopAuditRecorder{})
	_, err := svc.CreateResource(context.Background(), Actor{}, CreateResourceInput{
		ApplicationID: "missing-app-id",
		Name:          "pod-1",
		ResourceType:  "pod",
	})
	if err == nil {
		t.Fatal("expected error for missing application")
	}
}
