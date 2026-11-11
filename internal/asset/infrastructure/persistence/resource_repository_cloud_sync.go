package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ResourceRepository) UpsertCloudSyncWithLease(ctx context.Context, res *domain.Resource, batchID, fencingToken string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("asset resource repository is not configured")
	}
	batchID = strings.TrimSpace(batchID)
	fencingToken = strings.TrimSpace(fencingToken)
	if batchID == "" || fencingToken == "" {
		return false, domain.ErrLeaseLost
	}
	var created bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch syncBatchModel
		if err := checkSyncLeaseOwnedForUpdate(tx, batchID, fencingToken, time.Now().UTC(), &batch); err != nil {
			return err
		}
		if res != nil && res.LastSyncedAt != nil && batch.StartedAt.After(*res.LastSyncedAt) {
			res.LastSyncedAt = &batch.StartedAt
		}
		var err error
		created, err = r.upsertCloudSync(ctx, tx, res)
		return err
	})
	return created, err
}

// UpsertCloudSyncBatchWithLease 在同一事务内校验一次租约后批量 upsert 云同步资源。
// 一个 chunk 仅一次 SELECT ... FOR UPDATE 租约校验 + 一次批量
// INSERT ... ON CONFLICT DO UPDATE，替代逐资源事务以支撑 max_resources=20000 场景。
// 通过 RETURNING (xmax = 0) 精确区分新增/更新计数，保留 batch.CreatedCount/UpdatedCount 语义。
// 批量失败时由调用方回退逐条 UpsertCloudSyncWithLease 以隔离坏数据，本方法不做回退。
func (r *ResourceRepository) UpsertCloudSyncBatchWithLease(ctx context.Context, resources []*domain.Resource, batchID, fencingToken string) (created, updated int, err error) {
	if r == nil || r.db == nil {
		return 0, 0, errors.New("asset resource repository is not configured")
	}
	batchID = strings.TrimSpace(batchID)
	fencingToken = strings.TrimSpace(fencingToken)
	if batchID == "" || fencingToken == "" {
		return 0, 0, domain.ErrLeaseLost
	}
	if len(resources) == 0 {
		return 0, 0, nil
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch syncBatchModel
		if err := checkSyncLeaseOwnedForUpdate(tx, batchID, fencingToken, time.Now().UTC(), &batch); err != nil {
			return err
		}
		// 用 batch.StartedAt 钳制 LastSyncedAt，与逐条 UpsertCloudSyncWithLease 语义一致。
		// 同时过滤 nil resource，避免 upsertCloudSyncBatch 直接解引用原始切片时 panic。
		cleaned := make([]*domain.Resource, 0, len(resources))
		for _, res := range resources {
			if res == nil {
				continue
			}
			if res.LastSyncedAt != nil && batch.StartedAt.After(*res.LastSyncedAt) {
				res.LastSyncedAt = &batch.StartedAt
			}
			cleaned = append(cleaned, res)
		}
		c, u, err := r.upsertCloudSyncBatch(ctx, tx, cleaned)
		if err != nil {
			return err
		}
		created, updated = c, u
		return nil
	})
	return created, updated, err
}

// PatchCloudSyncLabelsBatchWithLease 在同一事务内校验一次租约后，按 cloud key 批量更新已落库云同步资源的 labels。
// 仅更新 source='cloud_sync' 且本轮已 upsert（last_synced_at >= batch.StartedAt）的 active 资源，
// 用于 hybrid 第二阶段增强 label 回写。sync_batch_id 在 FinalizeSuccess 才提升为当前批次，故这里用
// last_synced_at >= StartedAt 识别本轮资源（与 stale 标记的 syncedSince 阈值互补），见 ops/huawei-ces-sync-contract.md §8.2/§13.1。
// 注意：本方法直写 SQL 绕过 GORM 钩子，故显式维护 updated_at。只 patch labels 列，不触碰其它字段，
// 保证不影响 created/updated 计数与 stale 门控。
func (r *ResourceRepository) PatchCloudSyncLabelsBatchWithLease(ctx context.Context, patches []domain.CloudSyncLabelPatch, batchID, fencingToken string) (int, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset resource repository is not configured")
	}
	batchID = strings.TrimSpace(batchID)
	fencingToken = strings.TrimSpace(fencingToken)
	if batchID == "" || fencingToken == "" {
		return 0, domain.ErrLeaseLost
	}
	if len(patches) == 0 {
		return 0, nil
	}
	updated := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch syncBatchModel
		if err := checkSyncLeaseOwnedForUpdate(tx, batchID, fencingToken, time.Now().UTC(), &batch); err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, p := range patches {
			labels, mErr := marshalResourceLabels(p.Labels)
			if mErr != nil {
				return mErr
			}
			q := tx.WithContext(ctx).Model(&resourceModel{}).
				Where("source = ? AND integration_account_id = ? AND cloud_resource_type = ? AND cloud_resource_id = ? AND region = ? AND sync_status = ?",
					domain.ResourceSourceCloudSync,
					p.IntegrationAccountID, p.CloudResourceType, p.CloudResourceID, p.Region,
					domain.SyncStatusActive)
			if !batch.StartedAt.IsZero() {
				q = q.Where("last_synced_at >= ?", batch.StartedAt)
			}
			result := q.Updates(map[string]any{
				"labels":     labels,
				"updated_at": now,
			})
			if result.Error != nil {
				return result.Error
			}
			updated += int(result.RowsAffected)
		}
		return nil
	})
	return updated, err
}

// upsertCloudSyncBatch 用原生 SQL 执行批量 upsert。
// ON CONFLICT 推断 0026 部分唯一索引 (integration_account_id, cloud_resource_type,
// cloud_resource_id, region) WHERE source='cloud_sync' AND cloud_resource_id <> ''.
// DO UPDATE 的字段集合与 updateCloudSync 完全一致（不含 sync_batch_id、created_at、
// resource_id、source），保证批量与逐条语义一致。RETURNING (xmax = 0) 区分新增/更新。
// 注意：本方法直写 SQL，绕过 GORM 钩子，故显式维护 created_at/updated_at。
func (r *ResourceRepository) upsertCloudSyncBatch(ctx context.Context, tx *gorm.DB, resources []*domain.Resource) (created, updated int, err error) {
	const colsPerRow = 19
	n := len(resources)
	placeholders := make([]string, n)
	args := make([]any, 0, n*colsPerRow)
	now := time.Now().UTC()
	for i, res := range resources {
		labels, lErr := marshalResourceLabels(res.Labels)
		if lErr != nil {
			return 0, 0, fmt.Errorf("marshal labels for resource %s: %w", res.CloudResourceID, lErr)
		}
		base := i * colsPerRow
		placeholders[i] = fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9,
			base+10, base+11, base+12, base+13, base+14, base+15, base+16, base+17, base+18, base+19)
		args = append(args,
			res.ID, res.ApplicationID, res.Name, res.ResourceType, res.Namespace,
			res.Pod, res.Node, res.Instance, domain.ResourceSourceCloudSync,
			res.IntegrationAccountID, res.CloudResourceID, res.CloudResourceType, res.Region,
			res.SyncStatus, res.LastSyncedAt, res.SyncBatchID, labels, now, now,
		)
	}
	query := fmt.Sprintf(`INSERT INTO asset_resource (
    resource_id, application_id, name, resource_type, namespace, pod, node, instance, source,
    integration_account_id, cloud_resource_id, cloud_resource_type, region,
    sync_status, last_synced_at, sync_batch_id, labels, created_at, updated_at
) VALUES %s
ON CONFLICT (integration_account_id, cloud_resource_type, cloud_resource_id, region)
    WHERE source = 'cloud_sync' AND cloud_resource_id <> ''
DO UPDATE SET
    name = EXCLUDED.name,
    resource_type = EXCLUDED.resource_type,
    namespace = EXCLUDED.namespace,
    pod = EXCLUDED.pod,
    node = EXCLUDED.node,
    instance = EXCLUDED.instance,
    integration_account_id = EXCLUDED.integration_account_id,
    cloud_resource_id = EXCLUDED.cloud_resource_id,
    cloud_resource_type = EXCLUDED.cloud_resource_type,
    region = EXCLUDED.region,
    sync_status = EXCLUDED.sync_status,
    last_synced_at = EXCLUDED.last_synced_at,
    labels = EXCLUDED.labels,
    updated_at = EXCLUDED.updated_at
RETURNING (xmax = 0) AS inserted`, strings.Join(placeholders, ","))
	rows, err := tx.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var inserted bool
		if err := rows.Scan(&inserted); err != nil {
			return 0, 0, err
		}
		if inserted {
			created++
		} else {
			updated++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	return created, updated, nil
}

func promoteSuccessfulSyncBatch(ctx context.Context, db *gorm.DB, accountID, batchID string, syncedSince, now time.Time) (int64, error) {
	result := db.WithContext(ctx).Model(&resourceModel{}).
		Where("source = ? AND integration_account_id = ? AND sync_status = ? AND last_synced_at >= ?",
			domain.ResourceSourceCloudSync, accountID, domain.SyncStatusActive, syncedSince).
		Updates(map[string]any{
			"sync_batch_id": batchID,
			"updated_at":    now,
		})
	return result.RowsAffected, result.Error
}

func (r *ResourceRepository) upsertCloudSync(ctx context.Context, db *gorm.DB, res *domain.Resource) (bool, error) {
	if res == nil {
		return false, errors.New("resource is nil")
	}
	key := domain.CloudResourceKey{
		IntegrationAccountID: res.IntegrationAccountID,
		CloudResourceType:    res.CloudResourceType,
		CloudResourceID:      res.CloudResourceID,
		Region:               res.Region,
	}
	existing, err := r.findByCloudKey(ctx, db, key)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return false, err
	}
	if existing != nil {
		res.ID = existing.ID
		res.Source = domain.ResourceSourceCloudSync
		res.CreatedAt = existing.CreatedAt
		res.SyncBatchID = existing.SyncBatchID
		if err := r.updateCloudSync(ctx, db, res); err != nil {
			return false, err
		}
		return false, nil
	}
	res.Source = domain.ResourceSourceCloudSync
	if strings.TrimSpace(res.ID) == "" {
		return false, errors.New("resource id is required for cloud sync create")
	}
	if err := r.createCloudSync(ctx, db, res); err != nil {
		return false, err
	}
	return true, nil
}

func (r *ResourceRepository) createCloudSync(ctx context.Context, db *gorm.DB, res *domain.Resource) error {
	m := toResourceModel(res)
	if err := db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	res.CreatedAt = m.CreatedAt
	res.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *ResourceRepository) updateCloudSync(ctx context.Context, db *gorm.DB, res *domain.Resource) error {
	now := time.Now().UTC()
	labels, _ := marshalResourceLabels(res.Labels)
	result := db.WithContext(ctx).Model(&resourceModel{}).Where("resource_id = ?", res.ID).Updates(map[string]any{
		"name": res.Name, "resource_type": res.ResourceType, "namespace": res.Namespace,
		"pod": res.Pod, "node": res.Node, "instance": res.Instance,
		"integration_account_id": res.IntegrationAccountID,
		"cloud_resource_id":      res.CloudResourceID,
		"cloud_resource_type":    res.CloudResourceType,
		"region":                 res.Region,
		"sync_status":            res.SyncStatus,
		"last_synced_at":         res.LastSyncedAt,
		"labels":                 labels,
		"updated_at":             now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	res.UpdatedAt = now
	return nil
}

func (r *ResourceRepository) MarkStaleByAccountScopeExceptBatchWithLease(ctx context.Context, accountID, region, cloudResourceType, batchID, fencingToken string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset resource repository is not configured")
	}
	batchID = strings.TrimSpace(batchID)
	fencingToken = strings.TrimSpace(fencingToken)
	if batchID == "" || fencingToken == "" {
		return 0, domain.ErrLeaseLost
	}
	var staleCount int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch syncBatchModel
		if err := checkSyncLeaseOwnedForUpdate(tx, batchID, fencingToken, time.Now().UTC(), &batch); err != nil {
			return err
		}
		var err error
		staleCount, err = markStaleByAccountScopeExceptBatch(ctx, tx, accountID, region, cloudResourceType, batch.StartedAt)
		return err
	})
	return staleCount, err
}

func markStaleByAccountScopeExceptBatch(ctx context.Context, db *gorm.DB, accountID, region, cloudResourceType string, syncedSince time.Time) (int64, error) {
	accountID = strings.TrimSpace(accountID)
	region = strings.TrimSpace(region)
	cloudResourceType = strings.TrimSpace(cloudResourceType)
	if accountID == "" || region == "" || cloudResourceType == "" {
		return 0, nil
	}
	now := time.Now().UTC()
	query := db.WithContext(ctx).Model(&resourceModel{}).
		Where("source = ? AND integration_account_id = ? AND region = ? AND cloud_resource_type = ? AND sync_status = ?",
			domain.ResourceSourceCloudSync, accountID, region, cloudResourceType, domain.SyncStatusActive)
	if !syncedSince.IsZero() {
		query = query.Where("(last_synced_at IS NULL OR last_synced_at < ?)", syncedSince)
	}
	result := query.Updates(map[string]any{
		"sync_status": domain.SyncStatusStale,
		"updated_at":  now,
	})
	return result.RowsAffected, result.Error
}

func (r *ResourceRepository) MarkStaleByAccountRegionExceptTypesWithLease(ctx context.Context, accountID, region string, exceptTypes []string, batchID, fencingToken string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset resource repository is not configured")
	}
	batchID = strings.TrimSpace(batchID)
	fencingToken = strings.TrimSpace(fencingToken)
	if batchID == "" || fencingToken == "" {
		return 0, domain.ErrLeaseLost
	}
	var staleCount int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch syncBatchModel
		if err := checkSyncLeaseOwnedForUpdate(tx, batchID, fencingToken, time.Now().UTC(), &batch); err != nil {
			return err
		}
		var err error
		staleCount, err = markStaleByAccountRegionExceptTypes(ctx, tx, accountID, region, exceptTypes, batch.StartedAt)
		return err
	})
	return staleCount, err
}

func markStaleByAccountRegionExceptTypes(ctx context.Context, db *gorm.DB, accountID, region string, exceptTypes []string, syncedSince time.Time) (int64, error) {
	accountID = strings.TrimSpace(accountID)
	region = strings.TrimSpace(region)
	if accountID == "" || region == "" {
		return 0, nil
	}
	// 归一化 exceptTypes：小写去重去空。
	except := make([]string, 0, len(exceptTypes))
	seen := make(map[string]struct{}, len(exceptTypes))
	for _, t := range exceptTypes {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			except = append(except, t)
		}
	}
	now := time.Now().UTC()
	query := db.WithContext(ctx).Model(&resourceModel{}).
		Where("source = ? AND integration_account_id = ? AND region = ? AND sync_status = ?",
			domain.ResourceSourceCloudSync, accountID, region, domain.SyncStatusActive)
	if !syncedSince.IsZero() {
		query = query.Where("(last_synced_at IS NULL OR last_synced_at < ?)", syncedSince)
	}
	if len(except) > 0 {
		query = query.Where("cloud_resource_type NOT IN ?", except)
	}
	result := query.Updates(map[string]any{
		"sync_status": domain.SyncStatusStale,
		"updated_at":  now,
	})
	return result.RowsAffected, result.Error
}

func checkSyncLeaseOwnedForUpdate(db *gorm.DB, batchID, fencingToken string, now time.Time, out ...*syncBatchModel) error {
	var row syncBatchModel
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("batch_id = ? AND fencing_token = ? AND status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at >= ?",
			batchID, fencingToken, domain.SyncBatchStatusRunning, now).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrLeaseLost
		}
		return err
	}
	if len(out) > 0 && out[0] != nil {
		*out[0] = row
	}
	return nil
}
