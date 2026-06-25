package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

// authAuditModel 对应 iam_auth_audit 表，保存认证入口的成功/失败事件。
type authAuditModel struct {
	database.BaseModel
	AuditID    string `gorm:"column:audit_id;type:varchar(36);uniqueIndex;not null"`
	UserID     string `gorm:"column:user_id;type:varchar(36);not null;default:'';index"`
	Username   string `gorm:"column:username;type:varchar(64);not null;default:'';index"`
	ProviderID string `gorm:"column:provider_id;type:varchar(64);not null;default:'';index"`
	Event      string `gorm:"column:event;type:varchar(32);not null;index"`
	Method     string `gorm:"column:method;type:varchar(32);not null;index"`
	Result     string `gorm:"column:result;type:varchar(32);not null;index"`
	IP         string `gorm:"column:ip;type:varchar(64);not null;default:''"`
	UserAgent  string `gorm:"column:user_agent;type:varchar(512);not null;default:''"`
	Reason     string `gorm:"column:reason;type:varchar(255);not null;default:''"`
}

func (authAuditModel) TableName() string { return "iam_auth_audit" }

// AuthAuditRepository 用 GORM 持久化认证审计资料。
type AuthAuditRepository struct {
	db *gorm.DB
}

// NewAuthAuditRepository 创建认证审计仓储实例。
func NewAuthAuditRepository(db *gorm.DB) *AuthAuditRepository {
	return &AuthAuditRepository{db: db}
}

// Create 写入一条认证审计，并回填数据库时间戳。
func (r *AuthAuditRepository) Create(ctx context.Context, audit *domain.AuthAudit) error {
	if r == nil || r.db == nil {
		return errors.New("auth audit repository is not configured")
	}
	if audit == nil {
		return errors.New("auth audit is nil")
	}
	m := authAuditModel{
		AuditID:    audit.ID,
		UserID:     audit.UserID,
		Username:   audit.Username,
		ProviderID: audit.ProviderID,
		Event:      string(audit.Event),
		Method:     string(audit.Method),
		Result:     string(audit.Result),
		IP:         audit.IP,
		UserAgent:  audit.UserAgent,
		Reason:     audit.Reason,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	audit.CreatedAt = m.CreatedAt
	audit.UpdatedAt = m.UpdatedAt
	return nil
}

// List 按筛选条件分页读取认证审计，默认按最新记录排前面。
func (r *AuthAuditRepository) List(ctx context.Context, filter domain.AuthAuditFilter) ([]domain.AuthAudit, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("auth audit repository is not configured")
	}
	var rows []authAuditModel
	q := applyAuthAuditFilter(r.db.WithContext(ctx).Model(&authAuditModel{}), filter)
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	if err := q.Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.AuthAudit, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAuthAuditDomain(&row))
	}
	return out, nil
}

// Count 统计符合筛选条件的认证审计数量。
func (r *AuthAuditRepository) Count(ctx context.Context, filter domain.AuthAuditFilter) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("auth audit repository is not configured")
	}
	var total int64
	if err := applyAuthAuditFilter(r.db.WithContext(ctx).Model(&authAuditModel{}), filter).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// applyAuthAuditFilter 将领域筛选条件转换成数据库查询条件。
func applyAuthAuditFilter(q *gorm.DB, filter domain.AuthAuditFilter) *gorm.DB {
	if userID := strings.TrimSpace(filter.UserID); userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if username := strings.TrimSpace(filter.Username); username != "" {
		q = q.Where("username = ?", username)
	}
	if providerID := strings.TrimSpace(filter.ProviderID); providerID != "" {
		q = q.Where("provider_id = ?", providerID)
	}
	if filter.Event != "" {
		q = q.Where("event = ?", string(filter.Event))
	}
	if filter.Result != "" {
		q = q.Where("result = ?", string(filter.Result))
	}
	return q
}

// toAuthAuditDomain 将数据库模型转换成领域对象，避免上层依赖 GORM 结构。
func toAuthAuditDomain(m *authAuditModel) domain.AuthAudit {
	if m == nil {
		return domain.AuthAudit{}
	}
	return domain.AuthAudit{
		ID:         m.AuditID,
		UserID:     m.UserID,
		Username:   m.Username,
		ProviderID: m.ProviderID,
		Event:      domain.AuthAuditEvent(m.Event),
		Method:     domain.AuthAuditMethod(m.Method),
		Result:     domain.AuthAuditResult(m.Result),
		IP:         m.IP,
		UserAgent:  m.UserAgent,
		Reason:     m.Reason,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}
