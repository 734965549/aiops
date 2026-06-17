// Package persistence 用 GORM 实现 Audit 仓储，映射 audit_operation 表。
package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/audit/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type operationModel struct {
	database.BaseModel
	AuditID      string `gorm:"column:audit_id;type:varchar(36);uniqueIndex;not null"`
	UserID       string `gorm:"column:user_id;type:varchar(36);not null;default:'';index"`
	ResourceType string `gorm:"column:resource_type;type:varchar(64);not null;index:idx_audit_operation_resource,priority:1"`
	ResourceID   string `gorm:"column:resource_id;type:varchar(128);not null;index:idx_audit_operation_resource,priority:2"`
	Action       string `gorm:"column:action;type:varchar(64);not null;index"`
	Payload      []byte `gorm:"column:payload;type:jsonb;not null;default:'{}'::jsonb"`
	IP           string `gorm:"column:ip;type:varchar(64);not null;default:''"`
	UserAgent    string `gorm:"column:user_agent;type:varchar(512);not null;default:''"`
}

func (operationModel) TableName() string { return "audit_operation" }

// OperationAuditRepository GORM 仓储。
type OperationAuditRepository struct {
	db *gorm.DB
}

// NewOperationAuditRepository 创建仓储。
func NewOperationAuditRepository(db *gorm.DB) *OperationAuditRepository {
	return &OperationAuditRepository{db: db}
}

func (r *OperationAuditRepository) Create(ctx context.Context, audit *domain.OperationAudit) error {
	if r == nil || r.db == nil {
		return errors.New("audit repository is not configured")
	}
	if audit == nil {
		return errors.New("audit is nil")
	}
	payload, err := marshalPayload(audit.Payload)
	if err != nil {
		return err
	}
	m := operationModel{
		AuditID:      audit.ID,
		UserID:       audit.UserID,
		ResourceType: audit.ResourceType,
		ResourceID:   audit.ResourceID,
		Action:       audit.Action,
		Payload:      payload,
		IP:           audit.IP,
		UserAgent:    audit.UserAgent,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	audit.CreatedAt = m.CreatedAt
	audit.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *OperationAuditRepository) List(ctx context.Context, filter domain.OperationAuditFilter) ([]domain.OperationAudit, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("audit repository is not configured")
	}
	var rows []operationModel
	q := applyAuditFilter(r.db.WithContext(ctx), filter)
	if err := q.Order("created_at DESC, id DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.OperationAudit, 0, len(rows))
	for _, row := range rows {
		out = append(out, toOperationDomain(&row))
	}
	return out, nil
}

func (r *OperationAuditRepository) Count(ctx context.Context, filter domain.OperationAuditFilter) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("audit repository is not configured")
	}
	var n int64
	q := applyAuditFilter(r.db.WithContext(ctx).Model(&operationModel{}), filter)
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func applyAuditFilter(q *gorm.DB, filter domain.OperationAuditFilter) *gorm.DB {
	if v := strings.TrimSpace(filter.ResourceType); v != "" {
		q = q.Where("resource_type = ?", v)
	}
	if v := strings.TrimSpace(filter.ResourceID); v != "" {
		q = q.Where("resource_id = ?", v)
	}
	if v := strings.TrimSpace(filter.UserID); v != "" {
		q = q.Where("user_id = ?", v)
	}
	if v := strings.TrimSpace(filter.Action); v != "" {
		q = q.Where("action = ?", v)
	}
	return q
}

func marshalPayload(payload map[string]any) ([]byte, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	return json.Marshal(payload)
}

func toOperationDomain(m *operationModel) domain.OperationAudit {
	if m == nil {
		return domain.OperationAudit{}
	}
	out := domain.OperationAudit{
		ID: m.AuditID, UserID: m.UserID, ResourceType: m.ResourceType,
		ResourceID: m.ResourceID, Action: m.Action, IP: m.IP, UserAgent: m.UserAgent,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	_ = json.Unmarshal(m.Payload, &out.Payload)
	if out.Payload == nil {
		out.Payload = map[string]any{}
	}
	return out
}
