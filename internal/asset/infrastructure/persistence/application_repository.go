// Package persistence 用 GORM 实现 Asset 仓储，映射 asset_* 表。
package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type applicationModel struct {
	database.BaseModel
	ApplicationID string `gorm:"column:application_id;type:varchar(36);uniqueIndex;not null"`
	Name          string `gorm:"column:name;type:varchar(128);not null;index:idx_asset_application_name_env,priority:1"`
	Environment   string `gorm:"column:environment;type:varchar(64);not null;default:'';index:idx_asset_application_name_env,priority:2"`
	Namespace     string `gorm:"column:namespace;type:varchar(128);not null;default:''"`
	Description   string `gorm:"column:description;type:varchar(255);not null;default:''"`
}

func (applicationModel) TableName() string { return "asset_application" }

// ApplicationRepository 应用 GORM 仓储。
type ApplicationRepository struct {
	db *gorm.DB
}

// NewApplicationRepository 创建应用仓储。
func NewApplicationRepository(db *gorm.DB) *ApplicationRepository {
	return &ApplicationRepository{db: db}
}

func (r *ApplicationRepository) Create(ctx context.Context, app *domain.Application) error {
	if r == nil || r.db == nil {
		return errors.New("asset application repository is not configured")
	}
	if app == nil {
		return errors.New("application is nil")
	}
	m := applicationModel{
		ApplicationID: app.ID,
		Name:          app.Name,
		Environment:   app.Environment,
		Namespace:     app.Namespace,
		Description:   app.Description,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	app.CreatedAt = m.CreatedAt
	app.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *ApplicationRepository) List(ctx context.Context) ([]domain.Application, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset application repository is not configured")
	}
	var rows []applicationModel
	if err := r.db.WithContext(ctx).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Application, 0, len(rows))
	for _, row := range rows {
		out = append(out, toApplicationDomain(&row))
	}
	return out, nil
}

// ListPaged 按分页过滤返回应用列表与总数，排序与 List 一致（created_at ASC, id ASC）。
func (r *ApplicationRepository) ListPaged(ctx context.Context, filter domain.ApplicationFilter) ([]domain.Application, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("asset application repository is not configured")
	}
	q := r.db.WithContext(ctx).Model(&applicationModel{})
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
	var rows []applicationModel
	if err := q.Order("created_at ASC, id ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domain.Application, 0, len(rows))
	for _, row := range rows {
		out = append(out, toApplicationDomain(&row))
	}
	return out, total, nil
}

func (r *ApplicationRepository) GetByID(ctx context.Context, id string) (*domain.Application, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset application repository is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, domain.ErrNotFound
	}
	var row applicationModel
	if err := r.db.WithContext(ctx).Where("application_id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toApplicationDomain(&row)
	return &out, nil
}

func (r *ApplicationRepository) Update(ctx context.Context, app *domain.Application) error {
	if r == nil || r.db == nil {
		return errors.New("asset application repository is not configured")
	}
	if app == nil || strings.TrimSpace(app.ID) == "" {
		return errors.New("application is nil")
	}
	res := r.db.WithContext(ctx).Model(&applicationModel{}).Where("application_id = ?", app.ID).Updates(map[string]any{
		"name": app.Name, "environment": app.Environment,
		"namespace": app.Namespace, "description": app.Description,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	var row applicationModel
	if err := r.db.WithContext(ctx).Where("application_id = ?", app.ID).First(&row).Error; err != nil {
		return err
	}
	app.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *ApplicationRepository) Delete(ctx context.Context, id string) error {
	if r == nil || r.db == nil {
		return errors.New("asset application repository is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.ErrNotFound
	}
	res := r.db.WithContext(ctx).Where("application_id = ?", id).Delete(&applicationModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ApplicationRepository) ExistsByID(ctx context.Context, id string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("asset application repository is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&applicationModel{}).Where("application_id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ApplicationRepository) Count(ctx context.Context) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("asset application repository is not configured")
	}
	var total int64
	if err := r.db.WithContext(ctx).Model(&applicationModel{}).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *ApplicationRepository) FindByNameEnv(ctx context.Context, name, environment string) (*domain.Application, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("asset application repository is not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.ErrNotFound
	}
	q := r.db.WithContext(ctx).Where("LOWER(name) = ?", strings.ToLower(name))
	environment = strings.TrimSpace(environment)
	if environment != "" {
		var exact applicationModel
		if err := q.Where("LOWER(environment) = ?", strings.ToLower(environment)).
			Order("id ASC").
			First(&exact).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		} else {
			out := toApplicationDomain(&exact)
			return &out, nil
		}

		var fallback applicationModel
		if err := q.Where("environment = ''").Order("id ASC").First(&fallback).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, domain.ErrNotFound
			}
			return nil, err
		}
		out := toApplicationDomain(&fallback)
		return &out, nil
	}

	var row applicationModel
	if err := q.Order("id ASC").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toApplicationDomain(&row)
	return &out, nil
}

func toApplicationDomain(m *applicationModel) domain.Application {
	if m == nil {
		return domain.Application{}
	}
	return domain.Application{
		ID:          m.ApplicationID,
		Name:        m.Name,
		Environment: m.Environment,
		Namespace:   m.Namespace,
		Description: m.Description,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}
