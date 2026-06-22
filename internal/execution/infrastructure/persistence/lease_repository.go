package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/execution/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type leaseModel struct {
	database.BaseModel
	LeaseID    string     `gorm:"column:lease_id;type:varchar(64);uniqueIndex;not null"`
	TaskID     string     `gorm:"column:task_id;type:varchar(36);not null;index"`
	StepID     string     `gorm:"column:step_id;type:varchar(36);not null"`
	AgentID    string     `gorm:"column:agent_id;type:varchar(64);not null;index"`
	MediumID   string     `gorm:"column:medium_id;type:varchar(64);not null"`
	Status     string     `gorm:"column:status;type:varchar(16);not null;default:'active'"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;not null"`
	ReleasedAt *time.Time `gorm:"column:released_at"`
}

func (leaseModel) TableName() string { return "exec_lease" }

type LeaseRepository struct{ db *gorm.DB }

func NewLeaseRepository(db *gorm.DB) *LeaseRepository { return &LeaseRepository{db: db} }

func (r *LeaseRepository) Create(ctx context.Context, lease *domain.ExecutionLease) error {
	if r == nil || r.db == nil || lease == nil {
		return errors.New("lease repository is not configured")
	}
	m := toLeaseModel(lease)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	fillLeaseFromModel(lease, m)
	return nil
}

func (r *LeaseRepository) Update(ctx context.Context, lease *domain.ExecutionLease) error {
	if r == nil || r.db == nil || lease == nil {
		return errors.New("lease repository is not configured")
	}
	m := toLeaseModel(lease)
	res := r.db.WithContext(ctx).Model(&leaseModel{}).Where("lease_id = ?", lease.LeaseID).Updates(m)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *LeaseRepository) GetByID(ctx context.Context, leaseID string) (*domain.ExecutionLease, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("lease repository is not configured")
	}
	var m leaseModel
	if err := r.db.WithContext(ctx).Where("lease_id = ?", strings.TrimSpace(leaseID)).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toLeaseDomain(&m)
	return &out, nil
}

func (r *LeaseRepository) GetActiveByTask(ctx context.Context, taskID string) (*domain.ExecutionLease, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("lease repository is not configured")
	}
	var m leaseModel
	if err := r.db.WithContext(ctx).
		Where("task_id = ? AND status = ?", strings.TrimSpace(taskID), string(domain.LeaseActive)).
		Order("created_at DESC").
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toLeaseDomain(&m)
	return &out, nil
}

func toLeaseModel(lease *domain.ExecutionLease) *leaseModel {
	return &leaseModel{
		LeaseID: lease.LeaseID, TaskID: lease.TaskID, StepID: lease.StepID,
		AgentID: lease.AgentID, MediumID: lease.MediumID, Status: string(lease.Status),
		ExpiresAt: lease.ExpiresAt, ReleasedAt: lease.ReleasedAt,
	}
}

func toLeaseDomain(m *leaseModel) domain.ExecutionLease {
	return domain.ExecutionLease{
		LeaseID: m.LeaseID, TaskID: m.TaskID, StepID: m.StepID, AgentID: m.AgentID,
		MediumID: m.MediumID, Status: domain.LeaseStatus(m.Status),
		ExpiresAt: m.ExpiresAt, ReleasedAt: m.ReleasedAt,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func fillLeaseFromModel(lease *domain.ExecutionLease, m *leaseModel) {
	lease.CreatedAt = m.CreatedAt
	lease.UpdatedAt = m.UpdatedAt
}
