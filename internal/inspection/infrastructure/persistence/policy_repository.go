package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/inspection/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type policyModel struct {
	database.BaseModel
	PolicyID             string `gorm:"column:policy_id;type:varchar(64);uniqueIndex;not null"`
	Name                 string `gorm:"column:name;type:varchar(128);not null"`
	Enabled              bool   `gorm:"column:enabled;not null;default:true"`
	Schedule             string `gorm:"column:schedule;type:varchar(64);not null;default:''"`
	Scope                []byte `gorm:"column:scope;type:jsonb;not null"`
	Checks               []byte `gorm:"column:checks;type:jsonb;not null"`
	AgentProfile         string `gorm:"column:agent_profile;type:varchar(64);not null;default:'sre_default'"`
	NotificationPolicyID string `gorm:"column:notification_policy_id;type:varchar(64);not null;default:''"`
	Deleted              bool   `gorm:"column:deleted;not null;default:false"`
}

func (policyModel) TableName() string { return "inspection_policy" }

type PolicyRepository struct {
	db *gorm.DB
}

func NewPolicyRepository(db *gorm.DB) *PolicyRepository {
	return &PolicyRepository{db: db}
}

func (r *PolicyRepository) Create(ctx context.Context, policy *domain.InspectionPolicy) error {
	if r == nil || r.db == nil {
		return errors.New("policy repository is not configured")
	}
	if policy == nil {
		return domain.ErrInvalidArgument
	}
	m, err := toPolicyModel(policy)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	policy.CreatedAt = m.CreatedAt
	policy.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *PolicyRepository) Update(ctx context.Context, policy *domain.InspectionPolicy) error {
	if r == nil || r.db == nil {
		return errors.New("policy repository is not configured")
	}
	if policy == nil {
		return domain.ErrInvalidArgument
	}
	m, err := toPolicyModel(policy)
	if err != nil {
		return err
	}
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&policyModel{}).Where("policy_id = ? AND deleted = FALSE", policy.PolicyID).Updates(map[string]any{
		"name": m.Name, "enabled": m.Enabled, "schedule": m.Schedule, "scope": m.Scope,
		"checks": m.Checks, "agent_profile": m.AgentProfile, "notification_policy_id": m.NotificationPolicyID,
		"updated_at": now,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	policy.UpdatedAt = now
	return nil
}

func (r *PolicyRepository) GetByID(ctx context.Context, policyID string) (*domain.InspectionPolicy, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("policy repository is not configured")
	}
	var m policyModel
	err := r.db.WithContext(ctx).Where("policy_id = ? AND deleted = FALSE", policyID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return fromPolicyModel(&m)
}

func (r *PolicyRepository) List(ctx context.Context, filter domain.PolicyFilter) ([]domain.InspectionPolicy, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("policy repository is not configured")
	}
	q := r.db.WithContext(ctx).Model(&policyModel{}).Where("deleted = FALSE")
	q = applyPolicyFilter(q, filter)
	var rows []policyModel
	if err := q.Order("created_at DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.InspectionPolicy, 0, len(rows))
	for i := range rows {
		p, err := fromPolicyModel(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

func (r *PolicyRepository) Count(ctx context.Context, filter domain.PolicyFilter) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("policy repository is not configured")
	}
	q := r.db.WithContext(ctx).Model(&policyModel{}).Where("deleted = FALSE")
	q = applyPolicyFilter(q, filter)
	var total int64
	return total, q.Count(&total).Error
}

func (r *PolicyRepository) SoftDelete(ctx context.Context, policyID string) error {
	if r == nil || r.db == nil {
		return errors.New("policy repository is not configured")
	}
	res := r.db.WithContext(ctx).Model(&policyModel{}).Where("policy_id = ? AND deleted = FALSE", policyID).Updates(map[string]any{
		"deleted": true, "updated_at": time.Now(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func applyPolicyFilter(q *gorm.DB, filter domain.PolicyFilter) *gorm.DB {
	if filter.Enabled != nil {
		q = q.Where("enabled = ?", *filter.Enabled)
	}
	if kw := strings.TrimSpace(filter.Keyword); kw != "" {
		q = q.Where("name ILIKE ?", "%"+kw+"%")
	}
	return q
}

func toPolicyModel(p *domain.InspectionPolicy) (policyModel, error) {
	scope, err := marshalJSON(p.Scope)
	if err != nil {
		return policyModel{}, err
	}
	checks, err := marshalStringSlice(p.Checks)
	if err != nil {
		return policyModel{}, err
	}
	return policyModel{
		PolicyID: p.PolicyID, Name: p.Name, Enabled: p.Enabled, Schedule: p.Schedule,
		Scope: scope, Checks: checks, AgentProfile: p.AgentProfile,
		NotificationPolicyID: p.NotificationPolicyID, Deleted: p.Deleted,
	}, nil
}

func fromPolicyModel(m *policyModel) (*domain.InspectionPolicy, error) {
	var scope domain.PolicyScope
	if len(m.Scope) > 0 {
		_ = json.Unmarshal(m.Scope, &scope)
	}
	return &domain.InspectionPolicy{
		PolicyID: m.PolicyID, Name: m.Name, Enabled: m.Enabled, Schedule: m.Schedule,
		Scope: scope, Checks: unmarshalStringSlice(m.Checks), AgentProfile: m.AgentProfile,
		NotificationPolicyID: m.NotificationPolicyID, Deleted: m.Deleted,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}, nil
}
