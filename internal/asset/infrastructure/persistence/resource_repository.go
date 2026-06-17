package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type resourceModel struct {
	database.BaseModel
	ResourceID    string `gorm:"column:resource_id;type:varchar(36);uniqueIndex;not null"`
	ApplicationID string `gorm:"column:application_id;type:varchar(36);not null;index"`
	Name          string `gorm:"column:name;type:varchar(255);not null;default:''"`
	ResourceType  string `gorm:"column:resource_type;type:varchar(64);not null;default:''"`
	Namespace     string `gorm:"column:namespace;type:varchar(128);not null;default:'';index:idx_asset_resource_match,priority:2"`
	Pod           string `gorm:"column:pod;type:varchar(255);not null;default:'';index:idx_asset_resource_match,priority:3"`
	Node          string `gorm:"column:node;type:varchar(255);not null;default:'';index:idx_asset_resource_match,priority:4"`
	Instance      string `gorm:"column:instance;type:varchar(255);not null;default:'';index:idx_asset_resource_match,priority:5"`
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

func toResourceDomain(m *resourceModel) domain.Resource {
	if m == nil {
		return domain.Resource{}
	}
	return domain.Resource{
		ID:            m.ResourceID,
		ApplicationID: m.ApplicationID,
		Name:          m.Name,
		ResourceType:  m.ResourceType,
		Namespace:     m.Namespace,
		Pod:           m.Pod,
		Node:          m.Node,
		Instance:      m.Instance,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
}
