package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
)

func TestResourceRepository_IntegrationFindBestMatchPod(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	resID := uniqueAssetResourceID(t)
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "order-service", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	if err := resRepo.Create(ctx, &domain.Resource{
		ID: resID, ApplicationID: appID, Name: "order-pod-1",
		ResourceType: "pod", Namespace: "order", Pod: "order-xxx-1",
	}); err != nil {
		t.Fatalf("create resource: %v", err)
	}

	matched, err := resRepo.FindBestMatch(ctx, domain.ResourceMatchQuery{
		ApplicationID: appID,
		Namespace:     "order",
		Pod:           "order-xxx-1",
	})
	if err != nil {
		t.Fatalf("find best match: %v", err)
	}
	if matched.ID != resID {
		t.Fatalf("unexpected resource id: %s", matched.ID)
	}

	if _, err := resRepo.FindBestMatch(ctx, domain.ResourceMatchQuery{
		ApplicationID: appID,
		Pod:           "missing-pod",
	}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestResourceRepository_IntegrationFindBestMatchInstance(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	resID := uniqueAssetResourceID(t)
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "metrics-app", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	if err := resRepo.Create(ctx, &domain.Resource{
		ID: resID, ApplicationID: appID, ResourceType: "host", Instance: "node-1",
	}); err != nil {
		t.Fatalf("create resource: %v", err)
	}

	matched, err := resRepo.FindBestMatch(ctx, domain.ResourceMatchQuery{
		ApplicationID: appID,
		Instance:      "node-1",
	})
	if err != nil {
		t.Fatalf("find by instance: %v", err)
	}
	if matched.ID != resID {
		t.Fatalf("unexpected resource id: %s", matched.ID)
	}
}

func TestResourceRepository_IntegrationListByApplicationID(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "list-res-app", Environment: "dev"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	if err := resRepo.Create(ctx, &domain.Resource{ID: uniqueAssetResourceID(t), ApplicationID: appID, Pod: "p1"}); err != nil {
		t.Fatalf("create resource 1: %v", err)
	}
	if err := resRepo.Create(ctx, &domain.Resource{ID: uniqueAssetResourceID(t), ApplicationID: appID, Pod: "p2"}); err != nil {
		t.Fatalf("create resource 2: %v", err)
	}

	items, err := resRepo.ListByApplicationID(ctx, appID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(items))
	}
}

// TestResourceLabelsRoundtrip 覆盖 labels 序列化/反序列化，确保 hybrid 增强的
// private_ip/flavor/vpc_id/az 等字段能正确落库与读回（无需 Postgres）。
func TestResourceLabelsRoundtrip(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]string
	}{
		{"nil", nil},
		{"empty", map[string]string{}},
		{"enriched", map[string]string{
			"namespace":  "SYS.ECS",
			"private_ip": "192.168.1.10",
			"flavor":     "s6.large.2",
			"vpc_id":     "vpc-xxx",
			"az":         "cn-north-4a",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := marshalResourceLabels(tc.in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if len(data) == 0 {
				t.Fatal("expected non-empty json")
			}
			out := unmarshalResourceLabels(data)
			if len(out) != len(tc.in) {
				t.Fatalf("expected %d labels, got %d", len(tc.in), len(out))
			}
			for k, v := range tc.in {
				if got := out[k]; got != v {
					t.Fatalf("expected %s=%q, got %q", k, v, got)
				}
			}
			// 模型互转应保留 labels。
			res := &domain.Resource{Labels: tc.in}
			m := toResourceModel(res)
			if len(m.Labels) == 0 {
				t.Fatal("expected model labels bytes")
			}
			back := toResourceDomain(&m)
			for k, v := range tc.in {
				if got := back.Labels[k]; got != v {
					t.Fatalf("roundtrip expected %s=%q, got %q", k, v, got)
				}
			}
		})
	}
}

// TestResourceRepository_UpsertCloudSyncRegionKey 验证云资源唯一键含 region：
// 同 account+type+id 但不同 region 的资源应作为独立行入库，不互相覆盖。
// 需要 Postgres，不可用时跳过。
func TestResourceRepository_UpsertCloudSyncRegionKey(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	accountID := "acc-region-key-" + appID[:8]
	batchID := "batch-region-key-" + appID[4:12]
	fencingToken := "fence-region-key-" + appID[4:12]
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "region-key-app", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	createTestSyncBatch(t, db, accountID, batchID, fencingToken)

	base := domain.Resource{
		ApplicationID:        appID,
		IntegrationAccountID: accountID,
		CloudResourceType:    "ecs",
		CloudResourceID:      "i-same-id",
		Source:               domain.ResourceSourceCloudSync,
		SyncStatus:           domain.SyncStatusActive,
		Name:                 "ecs-demo",
		ResourceType:         "host",
	}

	// region A 入库。
	resA := base
	resA.ID = uniqueAssetResourceID(t)
	resA.Region = "cn-north-4"
	if created, err := resRepo.UpsertCloudSyncWithLease(ctx, &resA, batchID, fencingToken); err != nil || !created {
		t.Fatalf("upsert region A: created=%v err=%v", created, err)
	}

	// region B：同 account+type+id，不同 region，应为独立行而非覆盖 region A。
	resB := base
	resB.ID = uniqueAssetResourceID(t)
	resB.Region = "cn-south-1"
	createdB, err := resRepo.UpsertCloudSyncWithLease(ctx, &resB, batchID, fencingToken)
	if err != nil {
		t.Fatalf("upsert region B: %v", err)
	}
	if !createdB {
		t.Fatal("expected region B upsert to create a separate row, not overwrite region A")
	}

	// 按 region 分别查回，互不串扰。
	gotA, err := resRepo.FindByCloudKey(ctx, domain.CloudResourceKey{
		IntegrationAccountID: accountID, CloudResourceType: "ecs", CloudResourceID: "i-same-id", Region: "cn-north-4",
	})
	if err != nil {
		t.Fatalf("find region A: %v", err)
	}
	if gotA.Region != "cn-north-4" {
		t.Fatalf("expected region A row, got region %s", gotA.Region)
	}
	gotB, err := resRepo.FindByCloudKey(ctx, domain.CloudResourceKey{
		IntegrationAccountID: accountID, CloudResourceType: "ecs", CloudResourceID: "i-same-id", Region: "cn-south-1",
	})
	if err != nil {
		t.Fatalf("find region B: %v", err)
	}
	if gotB.Region != "cn-south-1" {
		t.Fatalf("expected region B row, got region %s", gotB.Region)
	}

	// region A 再次 upsert 应更新既有行（created=false），不新建第三行。
	resA2 := resA
	resA2.Name = "ecs-demo-updated"
	createdA2, err := resRepo.UpsertCloudSyncWithLease(ctx, &resA2, batchID, fencingToken)
	if err != nil {
		t.Fatalf("re-upsert region A: %v", err)
	}
	if createdA2 {
		t.Fatal("expected re-upsert region A to update existing, not create")
	}
}

// TestResourceRepository_UpsertCloudSyncLabelsRoundtrip 验证云同步资源带 CES labels
// 经 GORM 写入 asset_resource.labels(jsonb) 后能按 region 唯一键读回，labels 不丢失；
// 并覆盖 update 路径整体覆盖 labels（旧 key 被清除）。见 ops/huawei-ces-sync-contract.md §9.1。需要 Postgres，不可用时跳过。
func TestResourceRepository_UpsertCloudSyncLabelsRoundtrip(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	accountID := "acc-labels-rt-" + appID[:8]
	batchID := "batch-labels-rt-" + appID[4:12]
	fencingToken := "fence-labels-rt-" + appID[4:12]
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "labels-rt-app", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	createTestSyncBatch(t, db, accountID, batchID, fencingToken)

	labels := map[string]string{
		"namespace":           "SYS.ECS",
		"dim_name":            "instance_id",
		"resource_group_id":   "rg001",
		"resource_group_name": "全部资源",
		"private_ip":          "192.168.1.10",
	}
	res := &domain.Resource{
		ID:                   uniqueAssetResourceID(t),
		ApplicationID:        appID,
		IntegrationAccountID: accountID,
		CloudResourceType:    "ecs",
		CloudResourceID:      "ecs-labels-1",
		Region:               "cn-north-4",
		Source:               domain.ResourceSourceCloudSync,
		SyncStatus:           domain.SyncStatusActive,
		Name:                 "ecs-labels-demo",
		ResourceType:         "host",
		Labels:               labels,
	}

	created, err := resRepo.UpsertCloudSyncWithLease(ctx, res, batchID, fencingToken)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if !created {
		t.Fatal("expected first upsert to create")
	}

	got, err := resRepo.FindByCloudKey(ctx, domain.CloudResourceKey{
		IntegrationAccountID: accountID, CloudResourceType: "ecs", CloudResourceID: "ecs-labels-1", Region: "cn-north-4",
	})
	if err != nil {
		t.Fatalf("find by cloud key: %v", err)
	}
	for k, v := range labels {
		if gotV := got.Labels[k]; gotV != v {
			t.Fatalf("expected label %s=%q, got %q", k, v, gotV)
		}
	}

	// update 路径应整体覆盖 labels：新 key 写入，旧 key（resource_group_id）清除。
	updated := *res
	updated.Labels = map[string]string{
		"namespace":           "SYS.ECS",
		"dim_name":            "instance_id",
		"resource_group_name": "全部资源",
		"private_ip":          "10.0.0.5",
		"flavor":              "s6.large.2",
	}
	created2, err := resRepo.UpsertCloudSyncWithLease(ctx, &updated, batchID, fencingToken)
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if created2 {
		t.Fatal("expected re-upsert to update, not create")
	}
	got2, err := resRepo.FindByCloudKey(ctx, domain.CloudResourceKey{
		IntegrationAccountID: accountID, CloudResourceType: "ecs", CloudResourceID: "ecs-labels-1", Region: "cn-north-4",
	})
	if err != nil {
		t.Fatalf("find after update: %v", err)
	}
	for k, v := range updated.Labels {
		if gotV := got2.Labels[k]; gotV != v {
			t.Fatalf("after update expected label %s=%q, got %q", k, v, gotV)
		}
	}
	if _, ok := got2.Labels["resource_group_id"]; ok {
		t.Fatal("expected resource_group_id to be removed after labels overwrite")
	}
}

// TestResourceRepository_MarkStaleByAccountRegionExceptTypes 验证反向 stale：
// account+region 下所有 active 的 cloud_sync 资源（排除当前批次）标记 stale，
// 但跳过 exceptTypes 中的类型，见 ops/huawei-ces-sync-contract.md §13.1。需要 Postgres，不可用时跳过。
func TestResourceRepository_MarkStaleByAccountRegionExceptTypes(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	accountID := "acc-neg-stale-" + appID[:8]
	oldBatchID := "sync-old-" + appID[4:12]
	newBatchID := "sync-new-" + appID[4:12]
	fencingToken := "fence-neg-stale-" + appID[4:12]
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "neg-stale-app", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	createTestSyncBatch(t, db, accountID, newBatchID, fencingToken)

	region := "cn-north-4"
	// 旧批次资源：ecs/evs（应被反向 stale），rds（在 exceptTypes，保持 active），
	// 另一 region 的 ecs（不应被触及）。
	seed := []struct {
		id, ctype, cid, reg string
	}{
		{"r-old-ecs", "ecs", "ecs-old", region},
		{"r-old-evs", "evs", "evs-old", region},
		{"r-old-rds", "rds", "rds-old", region},
		{"r-other-ecs", "ecs", "ecs-other", "cn-south-1"},
	}
	for _, s := range seed {
		r := domain.Resource{
			ID: s.id, ApplicationID: appID, IntegrationAccountID: accountID,
			CloudResourceType: s.ctype, CloudResourceID: s.cid, Region: s.reg,
			Source: domain.ResourceSourceCloudSync, SyncStatus: domain.SyncStatusActive,
			SyncBatchID: oldBatchID, Name: s.cid, ResourceType: "host",
		}
		if _, err := resRepo.UpsertCloudSyncWithLease(ctx, &r, newBatchID, fencingToken); err != nil {
			t.Fatalf("seed %s: %v", s.id, err)
		}
	}

	n, err := resRepo.MarkStaleByAccountRegionExceptTypesWithLease(ctx, accountID, region, []string{"rds"}, newBatchID, fencingToken)
	if err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows staled (ecs,evs), got %d", n)
	}

	statusOf := func(id string) string {
		got, err := resRepo.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		return got.SyncStatus
	}
	if st := statusOf("r-old-ecs"); st != domain.SyncStatusStale {
		t.Fatalf("expected ecs stale, got %s", st)
	}
	if st := statusOf("r-old-evs"); st != domain.SyncStatusStale {
		t.Fatalf("expected evs stale, got %s", st)
	}
	if st := statusOf("r-old-rds"); st != domain.SyncStatusActive {
		t.Fatalf("excepted rds must remain active, got %s", st)
	}
	if st := statusOf("r-other-ecs"); st != domain.SyncStatusActive {
		t.Fatalf("other region ecs must remain active, got %s", st)
	}
}

// TestResourceRepository_UpsertCloudSyncBatchWithLease 验证批量 upsert：
//   - 纯新增 chunk 返回 created 计数且资源可按 cloud key 查回；
//   - 纯更新 chunk 返回 updated 计数且字段被覆盖；
//   - 新增+更新混合 chunk 精确区分计数；
//   - 同 account+type+id 不同 region 的资源作为独立行（0026 部分唯一索引含 region）；
//   - 租约丢失返回 ErrLeaseLost 且不写入；
//   - 空切片直接返回 0,0,nil。
//
// 需要 Postgres，不可用时跳过。
func TestResourceRepository_UpsertCloudSyncBatchWithLease(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	accountID := "acc-batch-" + appID[:8]
	batchID := "batch-upsert-batch-" + appID[4:12]
	fencingToken := "fence-upsert-batch-" + appID[4:12]
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "batch-upsert-app", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	createTestSyncBatch(t, db, accountID, batchID, fencingToken)

	now := time.Now().UTC()

	// 空切片应直接返回。
	c, u, err := resRepo.UpsertCloudSyncBatchWithLease(ctx, nil, batchID, fencingToken)
	if err != nil || c != 0 || u != 0 {
		t.Fatalf("empty batch: created=%d updated=%d err=%v", c, u, err)
	}

	// 1) 纯新增：3 条 ecs（不同 region 独立行）+ 1 条 rds。
	batch1 := []*domain.Resource{
		{ID: uniqueAssetResourceID(t), ApplicationID: appID, IntegrationAccountID: accountID, CloudResourceType: "ecs", CloudResourceID: "ecs-b1", Region: "cn-north-4", Source: domain.ResourceSourceCloudSync, SyncStatus: domain.SyncStatusActive, LastSyncedAt: &now, Name: "ecs-b1", ResourceType: "host"},
		{ID: uniqueAssetResourceID(t), ApplicationID: appID, IntegrationAccountID: accountID, CloudResourceType: "ecs", CloudResourceID: "ecs-b1", Region: "cn-south-1", Source: domain.ResourceSourceCloudSync, SyncStatus: domain.SyncStatusActive, LastSyncedAt: &now, Name: "ecs-b1-south", ResourceType: "host"},
		{ID: uniqueAssetResourceID(t), ApplicationID: appID, IntegrationAccountID: accountID, CloudResourceType: "rds", CloudResourceID: "rds-b1", Region: "cn-north-4", Source: domain.ResourceSourceCloudSync, SyncStatus: domain.SyncStatusActive, LastSyncedAt: &now, Name: "rds-b1", ResourceType: "db"},
	}
	created, updated, err := resRepo.UpsertCloudSyncBatchWithLease(ctx, batch1, batchID, fencingToken)
	if err != nil {
		t.Fatalf("batch1 upsert: %v", err)
	}
	if created != 3 || updated != 0 {
		t.Fatalf("batch1 expected created=3 updated=0, got created=%d updated=%d", created, updated)
	}
	// 按 cloud key 查回，验证 region 独立行。
	for _, r := range batch1 {
		got, err := resRepo.FindByCloudKey(ctx, domain.CloudResourceKey{
			IntegrationAccountID: accountID, CloudResourceType: r.CloudResourceType, CloudResourceID: r.CloudResourceID, Region: r.Region,
		})
		if err != nil {
			t.Fatalf("find after batch1 %s/%s: %v", r.CloudResourceType, r.CloudResourceID, err)
		}
		if got.Name != r.Name {
			t.Fatalf("expected name %s, got %s", r.Name, got.Name)
		}
	}

	// 2) 纯更新：复用 batch1 的 cloud key，改 Name，应为 updated=3 created=0。
	batch2 := make([]*domain.Resource, len(batch1))
	for i, r := range batch1 {
		cp := *r
		cp.ID = uniqueAssetResourceID(t) // 新 UUID，但 ON CONFLICT 应保留旧 resource_id
		cp.Name = r.Name + "-updated"
		batch2[i] = &cp
	}
	created, updated, err = resRepo.UpsertCloudSyncBatchWithLease(ctx, batch2, batchID, fencingToken)
	if err != nil {
		t.Fatalf("batch2 upsert: %v", err)
	}
	if created != 0 || updated != 3 {
		t.Fatalf("batch2 expected created=0 updated=3, got created=%d updated=%d", created, updated)
	}
	// 验证 Name 被覆盖且 resource_id 不变（仍是 batch1 的 ID）。
	for i, r := range batch1 {
		got, err := resRepo.FindByCloudKey(ctx, domain.CloudResourceKey{
			IntegrationAccountID: accountID, CloudResourceType: r.CloudResourceType, CloudResourceID: r.CloudResourceID, Region: r.Region,
		})
		if err != nil {
			t.Fatalf("find after batch2: %v", err)
		}
		if got.Name != batch2[i].Name {
			t.Fatalf("expected updated name %s, got %s", batch2[i].Name, got.Name)
		}
		if got.ID != r.ID {
			t.Fatalf("resource_id must be preserved on update, expected %s got %s", r.ID, got.ID)
		}
	}

	// 3) 混合：1 条新增（evs）+ 1 条更新（ecs-b1 cn-north-4）。
	batch3 := []*domain.Resource{
		{ID: uniqueAssetResourceID(t), ApplicationID: appID, IntegrationAccountID: accountID, CloudResourceType: "evs", CloudResourceID: "evs-b3", Region: "cn-north-4", Source: domain.ResourceSourceCloudSync, SyncStatus: domain.SyncStatusActive, LastSyncedAt: &now, Name: "evs-b3", ResourceType: "volume"},
		{ID: uniqueAssetResourceID(t), ApplicationID: appID, IntegrationAccountID: accountID, CloudResourceType: "ecs", CloudResourceID: "ecs-b1", Region: "cn-north-4", Source: domain.ResourceSourceCloudSync, SyncStatus: domain.SyncStatusActive, LastSyncedAt: &now, Name: "ecs-b1-mixed", ResourceType: "host"},
	}
	created, updated, err = resRepo.UpsertCloudSyncBatchWithLease(ctx, batch3, batchID, fencingToken)
	if err != nil {
		t.Fatalf("batch3 upsert: %v", err)
	}
	if created != 1 || updated != 1 {
		t.Fatalf("batch3 expected created=1 updated=1, got created=%d updated=%d", created, updated)
	}

	// 4) 租约丢失：错误 fencingToken 应返回 ErrLeaseLost 且不写入。
	leasedBefore, _ := resRepo.FindByCloudKey(ctx, domain.CloudResourceKey{
		IntegrationAccountID: accountID, CloudResourceType: "evs", CloudResourceID: "evs-lease", Region: "cn-north-4",
	})
	batchLease := []*domain.Resource{
		{ID: uniqueAssetResourceID(t), ApplicationID: appID, IntegrationAccountID: accountID, CloudResourceType: "evs", CloudResourceID: "evs-lease", Region: "cn-north-4", Source: domain.ResourceSourceCloudSync, SyncStatus: domain.SyncStatusActive, LastSyncedAt: &now, Name: "evs-lease", ResourceType: "volume"},
	}
	_, _, err = resRepo.UpsertCloudSyncBatchWithLease(ctx, batchLease, batchID, "wrong-token")
	if !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("expected ErrLeaseLost, got %v", err)
	}
	leasedAfter, _ := resRepo.FindByCloudKey(ctx, domain.CloudResourceKey{
		IntegrationAccountID: accountID, CloudResourceType: "evs", CloudResourceID: "evs-lease", Region: "cn-north-4",
	})
	if leasedBefore == nil && leasedAfter != nil {
		t.Fatal("lease-lost batch must not write any resource")
	}
}

// TestResourceRepository_PatchCloudSyncLabelsBatchWithLease 验证 hybrid 第二阶段增强 label 回写：
//   - 命中本轮 active 资源时整体替换 labels，且不改变 name 等非 label 字段；
//   - 租约丢失返回 ErrLeaseLost；
//   - 空 patches 直接返回 0。
//
// 需要 Postgres，不可用时跳过。
func TestResourceRepository_PatchCloudSyncLabelsBatchWithLease(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	accountID := "acc-patch-" + appID[:8]
	batchID := "batch-patch-" + appID[4:12]
	fencingToken := "fence-patch-" + appID[4:12]
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "patch-app", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}
	createTestSyncBatch(t, db, accountID, batchID, fencingToken)

	now := time.Now().UTC()
	// 先 upsert 一条基础资源（仅 CES labels）。
	base := []*domain.Resource{
		{ID: uniqueAssetResourceID(t), ApplicationID: appID, IntegrationAccountID: accountID, CloudResourceType: "ecs", CloudResourceID: "ecs-p1", Region: "cn-north-4", Source: domain.ResourceSourceCloudSync, SyncStatus: domain.SyncStatusActive, LastSyncedAt: &now, Name: "ecs-p1", ResourceType: "host", Labels: map[string]string{"namespace": "SYS.ECS"}},
	}
	if _, _, err := resRepo.UpsertCloudSyncBatchWithLease(ctx, base, batchID, fencingToken); err != nil {
		t.Fatalf("upsert base: %v", err)
	}

	// 空 patches 直接返回。
	if n, err := resRepo.PatchCloudSyncLabelsBatchWithLease(ctx, nil, batchID, fencingToken); err != nil || n != 0 {
		t.Fatalf("empty patches: n=%d err=%v", n, err)
	}

	// 回写增强 labels（合并后整体替换）。
	patches := []domain.CloudSyncLabelPatch{{
		CloudResourceKey: domain.CloudResourceKey{IntegrationAccountID: accountID, CloudResourceType: "ecs", CloudResourceID: "ecs-p1", Region: "cn-north-4"},
		Labels:           map[string]string{"namespace": "SYS.ECS", "flavor": "s6.large.2", "private_ip": "10.0.0.1"},
	}}
	n, err := resRepo.PatchCloudSyncLabelsBatchWithLease(ctx, patches, batchID, fencingToken)
	if err != nil {
		t.Fatalf("patch labels: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 updated, got %d", n)
	}
	got, err := resRepo.FindByCloudKey(ctx, domain.CloudResourceKey{IntegrationAccountID: accountID, CloudResourceType: "ecs", CloudResourceID: "ecs-p1", Region: "cn-north-4"})
	if err != nil {
		t.Fatalf("find after patch: %v", err)
	}
	if got.Labels["flavor"] != "s6.large.2" || got.Labels["private_ip"] != "10.0.0.1" {
		t.Fatalf("enriched labels not written back: %+v", got.Labels)
	}
	if got.Labels["namespace"] != "SYS.ECS" {
		t.Fatalf("basic label namespace lost: %+v", got.Labels)
	}
	if got.Name != "ecs-p1" {
		t.Fatalf("patch must not change non-label fields, name=%s", got.Name)
	}

	// 租约丢失：错误 fencingToken。
	if _, err := resRepo.PatchCloudSyncLabelsBatchWithLease(ctx, patches, batchID, "wrong-token"); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("expected ErrLeaseLost, got %v", err)
	}
}
