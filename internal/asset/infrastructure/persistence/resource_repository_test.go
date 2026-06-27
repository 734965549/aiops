package persistence

import (
	"context"
	"errors"
	"testing"

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
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "region-key-app", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}

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
	if created, err := resRepo.UpsertCloudSync(ctx, &resA); err != nil || !created {
		t.Fatalf("upsert region A: created=%v err=%v", created, err)
	}

	// region B：同 account+type+id，不同 region，应为独立行而非覆盖 region A。
	resB := base
	resB.ID = uniqueAssetResourceID(t)
	resB.Region = "cn-south-1"
	createdB, err := resRepo.UpsertCloudSync(ctx, &resB)
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
	createdA2, err := resRepo.UpsertCloudSync(ctx, &resA2)
	if err != nil {
		t.Fatalf("re-upsert region A: %v", err)
	}
	if createdA2 {
		t.Fatal("expected re-upsert region A to update existing, not create")
	}
}

// TestResourceRepository_UpsertCloudSyncLabelsRoundtrip 验证云同步资源带 CES labels
// 经 GORM 写入 asset_resource.labels(jsonb) 后能按 region 唯一键读回，labels 不丢失；
// 并覆盖 update 路径整体覆盖 labels（旧 key 被清除）。见 §9.1。需要 Postgres，不可用时跳过。
func TestResourceRepository_UpsertCloudSyncLabelsRoundtrip(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	accountID := "acc-labels-rt-" + appID[:8]
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "labels-rt-app", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}

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

	created, err := resRepo.UpsertCloudSync(ctx, res)
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
	created2, err := resRepo.UpsertCloudSync(ctx, &updated)
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
// 但跳过 exceptTypes 中的类型，见 docs/huawei-ces-asset-sync-plan.md §13.1。需要 Postgres，不可用时跳过。
func TestResourceRepository_MarkStaleByAccountRegionExceptTypes(t *testing.T) {
	db := openAssetTestPostgres(t)
	appRepo := NewApplicationRepository(db)
	resRepo := NewResourceRepository(db)
	ctx := context.Background()

	appID := uniqueAssetAppID(t)
	accountID := "acc-neg-stale-" + appID[:8]
	t.Cleanup(func() { deleteTestApplications(t, db, appID) })

	if err := appRepo.Create(ctx, &domain.Application{ID: appID, Name: "neg-stale-app", Environment: "prod"}); err != nil {
		t.Fatalf("create app: %v", err)
	}

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
			SyncBatchID: "sync-old", Name: s.cid, ResourceType: "host",
		}
		if _, err := resRepo.UpsertCloudSync(ctx, &r); err != nil {
			t.Fatalf("seed %s: %v", s.id, err)
		}
	}

	n, err := resRepo.MarkStaleByAccountRegionExceptTypes(ctx, accountID, region, []string{"rds"}, "sync-new")
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
