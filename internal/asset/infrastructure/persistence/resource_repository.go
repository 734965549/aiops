package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type resourceModel struct {
	database.BaseModel
	ResourceID           string     `gorm:"column:resource_id;type:varchar(36);uniqueIndex;not null"`
	ApplicationID        string     `gorm:"column:application_id;type:varchar(36);not null;index"`
	Name                 string     `gorm:"column:name;type:varchar(255);not null;default:''"`
	ResourceType         string     `gorm:"column:resource_type;type:varchar(64);not null;default:''"`
	Namespace            string     `gorm:"column:namespace;type:varchar(128);not null;default:'';index:idx_asset_resource_match,priority:2"`
	Pod                  string     `gorm:"column:pod;type:varchar(255);not null;default:'';index:idx_asset_resource_match,priority:3"`
	Node                 string     `gorm:"column:node;type:varchar(255);not null;default:'';index:idx_asset_resource_match,priority:4"`
	Instance             string     `gorm:"column:instance;type:varchar(255);not null;default:'';index:idx_asset_resource_match,priority:5"`
	Source               string     `gorm:"column:source;type:varchar(32);not null;default:manual"`
	IntegrationAccountID string     `gorm:"column:integration_account_id;type:varchar(64);not null;default:'';index:idx_asset_resource_cloud_key,priority:1"`
	CloudResourceID      string     `gorm:"column:cloud_resource_id;type:varchar(255);not null;default:'';index:idx_asset_resource_cloud_key,priority:3"`
	CloudResourceType    string     `gorm:"column:cloud_resource_type;type:varchar(64);not null;default:'';index:idx_asset_resource_cloud_key,priority:2"`
	Region               string     `gorm:"column:region;type:varchar(64);not null;default:''"`
	SyncStatus           string     `gorm:"column:sync_status;type:varchar(32);not null;default:''"`
	LastSyncedAt         *time.Time `gorm:"column:last_synced_at"`
	SyncBatchID          string     `gorm:"column:sync_batch_id;type:varchar(64);not null;default:''"`
}

func (resourceModel) TableName() string { return "asset_resource" }

// ResourceRepository 资源 GORM 仓储。
type ResourceRepository struct {
	db *gorm.DB
}

// NewResourceRepository 创建资源仓储。
func NewResourceRepository(db *gorm.DB) *ResourceRepository {
	return &ResourceRepository{db: db}
}

func (r *ResourceRepository) Create(ctx context.Context, res *domain.Resource) error {
	if r == nil || r.db == nil {
		return errors.New("asset resource repository is not configured")
	}
	if res == nil {
		return errors.New("resource is nil")
	}
	m := resourceModel{
		ResourceID:    res.ID,
		ApplicationID: res.ApplicationID,
		Name:          res.Name,
		ResourceType:  res.ResourceType,
		Namespace:     res.Namespace,
		Pod:           res.Pod,
		Node:          res.Node,
		Instance:      res.Instance,
		Source:        defaultSource(res.Source),
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	res.CreatedAt = m.CreatedAt
	res.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *ResourceRepository) ListByApplicationID(ctx context.Context, applicationID string) ([]domain.Resource, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset resource repository is not configured")
	}
	var rows []resourceModel
	if err := r.db.WithContext(ctx).Where("application_id = ?", applicationID).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Resource, 0, len(rows))
	for _, row := range rows {
		out = append(out, toResourceDomain(&row))
	}
	return out, nil
}

// ListByApplicationIDPaged 按应用 + 分页过滤返回资源列表与总数，排序与 ListByApplicationID 一致。
func (r *ResourceRepository) ListByApplicationIDPaged(ctx context.Context, applicationID string, filter domain.ResourceFilter) ([]domain.Resource, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("asset resource repository is not configured")
	}
	q := r.db.WithContext(ctx).Model(&resourceModel{}).Where("application_id = ?", applicationID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var rows []resourceModel
	if err := q.Order("created_at ASC, id ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.Resource, 0, len(rows))
	for _, row := range rows {
		out = append(out, toResourceDomain(&row))
	}
	return out, total, nil
}

func (r *ResourceRepository) GetByID(ctx context.Context, id string) (*domain.Resource, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset resource repository is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, domain.ErrNotFound
	}
	var row resourceModel
	if err := r.db.WithContext(ctx).Where("resource_id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toResourceDomain(&row)
	return &out, nil
}

func (r *ResourceRepository) Update(ctx context.Context, res *domain.Resource) error {
	if r == nil || r.db == nil {
		return errors.New("asset resource repository is not configured")
	}
	if res == nil || strings.TrimSpace(res.ID) == "" {
		return errors.New("resource is nil")
	}
	result := r.db.WithContext(ctx).Model(&resourceModel{}).Where("resource_id = ?", res.ID).Updates(map[string]any{
		"name": res.Name, "resource_type": res.ResourceType,
		"namespace": res.Namespace, "pod": res.Pod,
		"node": res.Node, "instance": res.Instance,
		"updated_at": time.Now().UTC(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	var row resourceModel
	if err := r.db.WithContext(ctx).Where("resource_id = ?", res.ID).First(&row).Error; err != nil {
		return err
	}
	res.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *ResourceRepository) Delete(ctx context.Context, id string) error {
	if r == nil || r.db == nil {
		return errors.New("asset resource repository is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ErrNotFound
	}
	res := r.db.WithContext(ctx).Where("resource_id = ?", id).Delete(&resourceModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ResourceRepository) CountByApplicationID(ctx context.Context, applicationID string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset resource repository is not configured")
	}
	var total int64
	if err := r.db.WithContext(ctx).Model(&resourceModel{}).Where("application_id = ?", strings.TrimSpace(applicationID)).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *ResourceRepository) Count(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset resource repository is not configured")
	}
	var total int64
	if err := r.db.WithContext(ctx).Model(&resourceModel{}).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// FindBestMatch 按 §9.1 优先级匹配：pod > node > instance > name。
func (r *ResourceRepository) FindBestMatch(ctx context.Context, q domain.ResourceMatchQuery) (*domain.Resource, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset resource repository is not configured")
	}
	appID := strings.TrimSpace(q.ApplicationID)
	if appID == "" {
		return nil, domain.ErrNotFound
	}
	var rows []resourceModel
	if err := r.db.WithContext(ctx).Where("application_id = ?", appID).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, domain.ErrNotFound
	}
	pod := strings.TrimSpace(q.Pod)
	node := strings.TrimSpace(q.Node)
	instance := strings.TrimSpace(q.Instance)
	name := strings.TrimSpace(q.Name)
	namespace := strings.TrimSpace(q.Namespace)

	matchField := func(row resourceModel, field string, want string) bool {
		want = strings.TrimSpace(want)
		if want == "" {
			return false
		}
		switch field {
		case "pod":
			return strings.EqualFold(row.Pod, want)
		case "node":
			return strings.EqualFold(row.Node, want)
		case "instance":
			return strings.EqualFold(row.Instance, want)
		case "name":
			return strings.EqualFold(row.Name, want)
		case "namespace":
			return row.Namespace == "" || strings.EqualFold(row.Namespace, want)
		default:
			return false
		}
	}

	tryOrder := []struct {
		field string
		value string
	}{
		{"pod", pod},
		{"node", node},
		{"instance", instance},
		{"name", name},
	}
	for _, step := range tryOrder {
		if step.value == "" {
			continue
		}
		for i := range rows {
			row := rows[i]
			if namespace != "" && row.Namespace != "" && !strings.EqualFold(row.Namespace, namespace) {
				continue
			}
			if matchField(row, step.field, step.value) {
				out := toResourceDomain(&row)
				return &out, nil
			}
		}
	}
	return nil, domain.ErrNotFound
}

func (r *ResourceRepository) FindByCloudKey(ctx context.Context, key domain.CloudResourceKey) (*domain.Resource, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset resource repository is not configured")
	}
	accountID := strings.TrimSpace(key.IntegrationAccountID)
	cloudType := strings.TrimSpace(key.CloudResourceType)
	cloudID := strings.TrimSpace(key.CloudResourceID)
	if accountID == "" || cloudType == "" || cloudID == "" {
		return nil, domain.ErrNotFound
	}
	var row resourceModel
	err := r.db.WithContext(ctx).
		Where("source = ? AND integration_account_id = ? AND cloud_resource_type = ? AND cloud_resource_id = ?",
			domain.ResourceSourceCloudSync, accountID, cloudType, cloudID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toResourceDomain(&row)
	return &out, nil
}

func (r *ResourceRepository) UpsertCloudSync(ctx context.Context, res *domain.Resource) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("asset resource repository is not configured")
	}
	if res == nil {
		return false, errors.New("resource is nil")
	}
	key := domain.CloudResourceKey{
		IntegrationAccountID: res.IntegrationAccountID,
		CloudResourceType:    res.CloudResourceType,
		CloudResourceID:      res.CloudResourceID,
	}
	existing, err := r.FindByCloudKey(ctx, key)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return false, err
	}
	if existing != nil {
		res.ID = existing.ID
		res.Source = domain.ResourceSourceCloudSync
		res.CreatedAt = existing.CreatedAt
		if err := r.updateCloudSync(ctx, res); err != nil {
			return false, err
		}
		return false, nil
	}
	res.Source = domain.ResourceSourceCloudSync
	if strings.TrimSpace(res.ID) == "" {
		return false, errors.New("resource id is required for cloud sync create")
	}
	if err := r.createCloudSync(ctx, res); err != nil {
		return false, err
	}
	return true, nil
}

func (r *ResourceRepository) createCloudSync(ctx context.Context, res *domain.Resource) error {
	m := toResourceModel(res)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	res.CreatedAt = m.CreatedAt
	res.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *ResourceRepository) updateCloudSync(ctx context.Context, res *domain.Resource) error {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&resourceModel{}).Where("resource_id = ?", res.ID).Updates(map[string]any{
		"name": res.Name, "resource_type": res.ResourceType, "namespace": res.Namespace,
		"pod": res.Pod, "node": res.Node, "instance": res.Instance,
		"integration_account_id": res.IntegrationAccountID,
		"cloud_resource_id":      res.CloudResourceID,
		"cloud_resource_type":    res.CloudResourceType,
		"region":                 res.Region,
		"sync_status":            res.SyncStatus,
		"last_synced_at":         res.LastSyncedAt,
		"sync_batch_id":          res.SyncBatchID,
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

func (r *ResourceRepository) MarkStaleByAccountScopeExceptBatch(ctx context.Context, accountID, region, cloudResourceType, batchID string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset resource repository is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	region = strings.TrimSpace(region)
	cloudResourceType = strings.TrimSpace(cloudResourceType)
	batchID = strings.TrimSpace(batchID)
	if accountID == "" || region == "" || cloudResourceType == "" || batchID == "" {
		return 0, nil
	}
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&resourceModel{}).
		Where("source = ? AND integration_account_id = ? AND region = ? AND cloud_resource_type = ? AND sync_batch_id <> ? AND sync_status = ?",
			domain.ResourceSourceCloudSync, accountID, region, cloudResourceType, batchID, domain.SyncStatusActive).
		Updates(map[string]any{
			"sync_status": domain.SyncStatusStale,
			"updated_at":  now,
		})
	return result.RowsAffected, result.Error
}

func defaultSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return domain.ResourceSourceManual
	}
	return source
}

func toResourceModel(res *domain.Resource) resourceModel {
	if res == nil {
		return resourceModel{}
	}
	return resourceModel{
		ResourceID:           res.ID,
		ApplicationID:        res.ApplicationID,
		Name:                 res.Name,
		ResourceType:         res.ResourceType,
		Namespace:            res.Namespace,
		Pod:                  res.Pod,
		Node:                 res.Node,
		Instance:             res.Instance,
		Source:               defaultSource(res.Source),
		IntegrationAccountID: res.IntegrationAccountID,
		CloudResourceID:      res.CloudResourceID,
		CloudResourceType:    res.CloudResourceType,
		Region:               res.Region,
		SyncStatus:           res.SyncStatus,
		LastSyncedAt:         res.LastSyncedAt,
		SyncBatchID:          res.SyncBatchID,
	}
}

func toResourceDomain(m *resourceModel) domain.Resource {
	if m == nil {
		return domain.Resource{}
	}
	return domain.Resource{
		ID:                   m.ResourceID,
		ApplicationID:        m.ApplicationID,
		Name:                 m.Name,
		ResourceType:         m.ResourceType,
		Namespace:            m.Namespace,
		Pod:                  m.Pod,
		Node:                 m.Node,
		Instance:             m.Instance,
		Source:               defaultSource(m.Source),
		IntegrationAccountID: m.IntegrationAccountID,
		CloudResourceID:      m.CloudResourceID,
		CloudResourceType:    m.CloudResourceType,
		Region:               m.Region,
		SyncStatus:           m.SyncStatus,
		LastSyncedAt:         m.LastSyncedAt,
		SyncBatchID:          m.SyncBatchID,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}
