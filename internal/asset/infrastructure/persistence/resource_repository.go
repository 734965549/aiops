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
)

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

// resourceSortColumns 是对外可排序字段白名单：key 为 API 的 sort_by，value 为数据库列名。
// 未命中白名单的值一律回退默认排序，绝不把外部字符串直接拼进 ORDER BY。
var resourceSortColumns = map[string]string{
	"name":                   "name",
	"resource_type":          "resource_type",
	"source":                 "source",
	"cloud_resource_type":    "cloud_resource_type",
	"integration_account_id": "integration_account_id",
	"cloud_resource_id":      "cloud_resource_id",
	"region":                 "region",
	"sync_status":            "sync_status",
	"last_synced_at":         "last_synced_at",
	"namespace":              "namespace",
	"pod":                    "pod",
	"instance":               "instance",
	"sync_batch_id":          "sync_batch_id",
	"created_at":             "created_at",
	"updated_at":             "updated_at",
}

// resourceOrderClause 返回安全的 ORDER BY 片段，固定追加 id ASC 保证分页结果稳定。
func resourceOrderClause(filter domain.ResourceFilter) string {
	column, ok := resourceSortColumns[strings.ToLower(strings.TrimSpace(filter.SortBy))]
	if !ok {
		return "created_at ASC, id ASC"
	}
	direction := "ASC"
	switch strings.ToLower(strings.TrimSpace(filter.SortOrder)) {
	case "desc", "descend", "descending":
		direction = "DESC"
	}
	return fmt.Sprintf("%s %s, id ASC", column, direction)
}

// escapeLikePattern 把用户输入里的 LIKE 通配符转义成字面量，避免 % / _ 变成通配。
func escapeLikePattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// ListByApplicationIDPaged 按应用 + 分页过滤返回资源列表与总数。
// 默认排序 created_at ASC, id ASC；传入合法 sort_by / sort_order 时改用对应字段。
func (r *ResourceRepository) ListByApplicationIDPaged(ctx context.Context, applicationID string, filter domain.ResourceFilter) ([]domain.Resource, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("asset resource repository is not configured")
	}
	q := r.db.WithContext(ctx).Model(&resourceModel{}).Where("application_id = ?", applicationID)
	if cloudType := strings.TrimSpace(filter.CloudResourceType); cloudType != "" {
		q = q.Where("cloud_resource_type = ?", cloudType)
	}
	if region := strings.TrimSpace(filter.Region); region != "" {
		q = q.Where("region = ?", region)
	}
	if syncStatus := strings.TrimSpace(filter.SyncStatus); syncStatus != "" {
		q = q.Where("sync_status = ?", syncStatus)
	}
	if source := strings.TrimSpace(filter.Source); source != "" {
		q = q.Where("source = ?", source)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		pattern := "%" + escapeLikePattern(keyword) + "%"
		q = q.Where(`(name ILIKE ? OR resource_type ILIKE ? OR namespace ILIKE ? OR pod ILIKE ? OR instance ILIKE ? OR integration_account_id ILIKE ? OR cloud_resource_id ILIKE ? OR cloud_resource_type ILIKE ? OR region ILIKE ?)`,
			pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern)
	}
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
	// Order 只挂在 Find 上：带 ORDER BY 的 COUNT(*) 在 PostgreSQL 下会报 GROUP BY 错误。
	if err := q.Order(resourceOrderClause(filter)).Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.Resource, 0, len(rows))
	for _, row := range rows {
		out = append(out, toResourceDomain(&row))
	}
	return out, total, nil
}

// ListFilterOptions 返回当前应用下 cloud_resource_type / region 的去重字典值。
// 只统计非空值并按字典序升序；这两列基数远小于资源数，不做分页。
func (r *ResourceRepository) ListFilterOptions(ctx context.Context, applicationID string) (domain.ResourceFilterOptions, error) {
	out := domain.ResourceFilterOptions{CloudResourceTypes: []string{}, Regions: []string{}}
	if r == nil || r.db == nil {
		return out, errors.New("asset resource repository is not configured")
	}
	appID := strings.TrimSpace(applicationID)
	if appID == "" {
		return out, nil
	}
	if err := r.db.WithContext(ctx).Model(&resourceModel{}).
		Where("application_id = ? AND cloud_resource_type <> ''", appID).
		Distinct().Order("cloud_resource_type ASC").
		Pluck("cloud_resource_type", &out.CloudResourceTypes).Error; err != nil {
		return out, err
	}
	if err := r.db.WithContext(ctx).Model(&resourceModel{}).
		Where("application_id = ? AND region <> ''", appID).
		Distinct().Order("region ASC").
		Pluck("region", &out.Regions).Error; err != nil {
		return out, err
	}
	return out, nil
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

// FindBestMatch 按 ops/alert-contract.md §9.1 优先级匹配：pod > node > instance > name。
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
	return r.findByCloudKey(ctx, r.db, key)
}

func (r *ResourceRepository) findByCloudKey(ctx context.Context, db *gorm.DB, key domain.CloudResourceKey) (*domain.Resource, error) {
	accountID := strings.TrimSpace(key.IntegrationAccountID)
	cloudType := strings.TrimSpace(key.CloudResourceType)
	cloudID := strings.TrimSpace(key.CloudResourceID)
	region := strings.TrimSpace(key.Region)
	if accountID == "" || cloudType == "" || cloudID == "" {
		return nil, domain.ErrNotFound
	}
	var row resourceModel
	err := db.WithContext(ctx).
		Where("source = ? AND integration_account_id = ? AND cloud_resource_type = ? AND cloud_resource_id = ? AND region = ?",
			domain.ResourceSourceCloudSync, accountID, cloudType, cloudID, region).
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
