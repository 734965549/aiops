package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/execution/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type mediumModel struct {
	database.BaseModel
	MediumID          string `gorm:"column:medium_id;type:varchar(64);uniqueIndex;not null"`
	Name              string `gorm:"column:name;type:varchar(128);not null"`
	MediumType        string `gorm:"column:medium_type;type:varchar(32);not null"`
	Environment       string `gorm:"column:environment;type:varchar(64);not null;default:''"`
	Region            string `gorm:"column:region;type:varchar(64);not null;default:''"`
	NetworkZone       string `gorm:"column:network_zone;type:varchar(128);not null;default:''"`
	Capabilities      []byte `gorm:"column:capabilities;type:jsonb;not null;default:'[]'::jsonb"`
	AllowedCommandIDs []byte `gorm:"column:allowed_command_ids;type:jsonb;not null;default:'[]'::jsonb"`
	MaxRiskLevel      string `gorm:"column:max_risk_level;type:varchar(16);not null;default:'high'"`
	Enabled           bool   `gorm:"column:enabled;not null;default:true"`
	HealthStatus      string `gorm:"column:health_status;type:varchar(16);not null;default:'unknown'"`
	Description       string `gorm:"column:description;type:varchar(512);not null;default:''"`
}

func (mediumModel) TableName() string { return "exec_medium" }

type MediumRepository struct{ db *gorm.DB }

func NewMediumRepository(db *gorm.DB) *MediumRepository { return &MediumRepository{db: db} }

func (r *MediumRepository) Create(ctx context.Context, m *domain.ExecutionMedium) error {
	if r == nil || r.db == nil || m == nil {
		return errors.New("medium repository is not configured")
	}
	model, err := toMediumModel(m)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	fillMediumFromModel(m, model)
	return nil
}

func (r *MediumRepository) Update(ctx context.Context, m *domain.ExecutionMedium) error {
	if r == nil || r.db == nil || m == nil {
		return errors.New("medium repository is not configured")
	}
	model, err := toMediumModel(m)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&mediumModel{}).Where("medium_id = ?", m.MediumID).Updates(model)
	if res.Error != nil {
		return database.MapUniqueViolation(res.Error, domain.ErrAlreadyExists)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *MediumRepository) GetByID(ctx context.Context, mediumID string) (*domain.ExecutionMedium, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("medium repository is not configured")
	}
	var m mediumModel
	if err := r.db.WithContext(ctx).Where("medium_id = ?", strings.TrimSpace(mediumID)).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toMediumDomain(&m)
	return &out, nil
}

func (r *MediumRepository) List(ctx context.Context, filter domain.MediumFilter) ([]domain.ExecutionMedium, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("medium repository is not configured")
	}
	var rows []mediumModel
	if err := r.applyMediumFilter(r.db.WithContext(ctx), filter).Order("created_at DESC, id DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.ExecutionMedium, 0, len(rows))
	for _, row := range rows {
		out = append(out, toMediumDomain(&row))
	}
	return out, nil
}

func (r *MediumRepository) Count(ctx context.Context, filter domain.MediumFilter) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("medium repository is not configured")
	}
	var total int64
	if err := r.applyMediumFilter(r.db.WithContext(ctx), filter).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *MediumRepository) applyMediumFilter(q *gorm.DB, filter domain.MediumFilter) *gorm.DB {
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	if filter.Environment != "" {
		q = q.Where("environment = ?", filter.Environment)
	}
	if filter.MediumType != "" {
		q = q.Where("medium_type = ?", filter.MediumType)
	}
	if filter.Keyword != "" {
		kw := "%" + filter.Keyword + "%"
		q = q.Where("name ILIKE ? OR medium_id ILIKE ?", kw, kw)
	}
	return q
}

func toMediumModel(m *domain.ExecutionMedium) (*mediumModel, error) {
	caps, err := marshalStringSlice(m.Capabilities)
	if err != nil {
		return nil, err
	}
	allowed, err := marshalStringSlice(m.AllowedCommandIDs)
	if err != nil {
		return nil, err
	}
	return &mediumModel{
		MediumID: m.MediumID, Name: m.Name, MediumType: string(m.MediumType),
		Environment: m.Environment, Region: m.Region, NetworkZone: m.NetworkZone,
		Capabilities: caps, AllowedCommandIDs: allowed, MaxRiskLevel: string(m.MaxRiskLevel),
		Enabled: m.Enabled, HealthStatus: string(m.HealthStatus), Description: m.Description,
	}, nil
}

func toMediumDomain(m *mediumModel) domain.ExecutionMedium {
	return domain.ExecutionMedium{
		MediumID: m.MediumID, Name: m.Name, MediumType: domain.MediumType(m.MediumType),
		Environment: m.Environment, Region: m.Region, NetworkZone: m.NetworkZone,
		Capabilities: unmarshalStringSlice(m.Capabilities), AllowedCommandIDs: unmarshalStringSlice(m.AllowedCommandIDs),
		MaxRiskLevel: domain.RiskLevel(m.MaxRiskLevel), Enabled: m.Enabled,
		HealthStatus: domain.MediumHealthStatus(m.HealthStatus), Description: m.Description,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func fillMediumFromModel(m *domain.ExecutionMedium, model *mediumModel) {
	m.CreatedAt = model.CreatedAt
	m.UpdatedAt = model.UpdatedAt
}
