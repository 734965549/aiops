package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

// alertModel 对应 alert_alert 表；业务主键为 alert_id，与自增 id 分离。
type alertModel struct {
	database.BaseModel
	AlertID         string     `gorm:"column:alert_id;type:varchar(36);uniqueIndex;not null"`
	ExternalID      string     `gorm:"column:external_id;type:varchar(128);not null;default:''"`
	Source          string     `gorm:"column:source;type:varchar(64);not null"`
	SourceID        string     `gorm:"column:source_id;type:varchar(64);not null;default:'';index:idx_alert_source_external,priority:1"`
	SourceName      string     `gorm:"column:source_name;type:varchar(128);not null;default:''"`
	Fingerprint     string     `gorm:"column:fingerprint;type:varchar(128);not null;default:''"`
	DedupKey        string     `gorm:"column:dedup_key;type:varchar(128);not null"`
	LifecycleSeq    int        `gorm:"column:lifecycle_seq;not null;default:1"`
	Name            string     `gorm:"column:name;type:varchar(255);not null"`
	Summary         string     `gorm:"column:summary;type:varchar(512);not null;default:''"`
	Description     string     `gorm:"column:description;type:text;not null;default:''"`
	Severity        string     `gorm:"column:severity;type:varchar(16);not null;index:idx_alert_status_severity_last_seen,priority:2"`
	Status          string     `gorm:"column:status;type:varchar(32);not null;index:idx_alert_status_severity_last_seen,priority:1"`
	RuleID          string     `gorm:"column:rule_id;type:varchar(128);not null;default:''"`
	RuleName        string     `gorm:"column:rule_name;type:varchar(255);not null;default:''"`
	BusinessLine    string     `gorm:"column:business_line;type:varchar(128);not null;default:''"`
	Environment     string     `gorm:"column:environment;type:varchar(64);not null;default:''"`
	ApplicationID   string     `gorm:"column:application_id;type:varchar(36);not null;default:''"`
	ApplicationName string     `gorm:"column:application_name;type:varchar(128);not null;default:''"`
	ResourceID      string     `gorm:"column:resource_id;type:varchar(36);not null;default:''"`
	ResourceType    string     `gorm:"column:resource_type;type:varchar(64);not null;default:''"`
	ResourceName    string     `gorm:"column:resource_name;type:varchar(255);not null;default:''"`
	OwnerUserID     string     `gorm:"column:owner_user_id;type:varchar(36);not null;default:''"`
	AssigneeUserID  string     `gorm:"column:assignee_user_id;type:varchar(36);not null;default:''"`
	Labels          []byte     `gorm:"column:labels;type:jsonb;not null;default:'{}'::jsonb"`
	Annotations     []byte     `gorm:"column:annotations;type:jsonb;not null;default:'{}'::jsonb"`
	OccurrenceCount int        `gorm:"column:occurrence_count;not null;default:1"`
	FirstSeenAt     time.Time  `gorm:"column:first_seen_at;not null"`
	LastSeenAt      time.Time  `gorm:"column:last_seen_at;not null;index:idx_alert_status_severity_last_seen,priority:3"`
	RecoveredAt     *time.Time `gorm:"column:recovered_at"`
	AcknowledgedAt  *time.Time `gorm:"column:acknowledged_at"`
	ClosedAt        *time.Time `gorm:"column:closed_at"`
	SilencedUntil   *time.Time `gorm:"column:silenced_until"`
}

func (alertModel) TableName() string { return "alert_alert" }

// AlertRepository 告警主记录 GORM 仓储。
type AlertRepository struct {
	db *gorm.DB
}

// NewAlertRepository 创建告警仓储。
func NewAlertRepository(db *gorm.DB) *AlertRepository {
	return &AlertRepository{db: db}
}

// Create 插入告警并回填 created_at/updated_at。
func (r *AlertRepository) Create(ctx context.Context, alert *domain.Alert) error {
	if r == nil || r.db == nil {
		return errors.New("alert repository is not configured")
	}
	if alert == nil {
		return errors.New("alert is nil")
	}
	m, err := toAlertModel(alert)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	fillAlertFromModel(alert, m)
	return nil
}

// Update 按 alert_id 更新告警字段。
func (r *AlertRepository) Update(ctx context.Context, alert *domain.Alert) error {
	if r == nil || r.db == nil {
		return errors.New("alert repository is not configured")
	}
	if alert == nil {
		return errors.New("alert is nil")
	}
	m, err := toAlertModel(alert)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&alertModel{}).Where("alert_id = ?", alert.ID).Updates(m)
	if res.Error != nil {
		return database.MapUniqueViolation(res.Error, domain.ErrAlreadyExists)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	row, err := r.getModelByID(ctx, alert.ID)
	if err != nil {
		return err
	}
	fillAlertFromModel(alert, row)
	return nil
}

func (r *AlertRepository) GetByID(ctx context.Context, alertID string) (*domain.Alert, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alert repository is not configured")
	}
	row, err := r.getModelByID(ctx, alertID)
	if err != nil {
		return nil, err
	}
	out := toAlertDomain(row)
	return &out, nil
}

// FindActiveByDedupKey 查找 status <> closed 的最新 lifecycle 记录。
func (r *AlertRepository) FindActiveByDedupKey(ctx context.Context, sourceID, dedupKey string) (*domain.Alert, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alert repository is not configured")
	}
	var row alertModel
	err := r.db.WithContext(ctx).
		Where("source_id = ? AND dedup_key = ? AND status <> ?", sourceID, dedupKey, string(domain.StatusClosed)).
		Order("lifecycle_seq DESC").
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	out := toAlertDomain(&row)
	return &out, nil
}

func (r *AlertRepository) MaxLifecycleSeq(ctx context.Context, dedupKey string) (int, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("alert repository is not configured")
	}
	var maxSeq int
	err := r.db.WithContext(ctx).Model(&alertModel{}).
		Where("dedup_key = ?", dedupKey).
		Select("COALESCE(MAX(lifecycle_seq), 0)").
		Scan(&maxSeq).Error
	return maxSeq, err
}

func (r *AlertRepository) List(ctx context.Context, filter domain.AlertFilter) ([]domain.Alert, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alert repository is not configured")
	}
	var rows []alertModel
	q := applyAlertFilter(r.db.WithContext(ctx).Model(&alertModel{}), filter)
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	if err := q.Order("last_seen_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Alert, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAlertDomain(&row))
	}
	return out, nil
}

func (r *AlertRepository) Count(ctx context.Context, filter domain.AlertFilter) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("alert repository is not configured")
	}
	var total int64
	if err := applyAlertFilter(r.db.WithContext(ctx).Model(&alertModel{}), filter).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *AlertRepository) getModelByID(ctx context.Context, alertID string) (*alertModel, error) {
	var row alertModel
	err := r.db.WithContext(ctx).Where("alert_id = ?", alertID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func applyAlertFilter(q *gorm.DB, filter domain.AlertFilter) *gorm.DB {
	if len(filter.Statuses) > 0 {
		vals := make([]string, 0, len(filter.Statuses))
		for _, s := range filter.Statuses {
			vals = append(vals, string(s))
		}
		q = q.Where("status IN ?", vals)
	}
	if len(filter.Severities) > 0 {
		vals := make([]string, 0, len(filter.Severities))
		for _, s := range filter.Severities {
			vals = append(vals, string(s))
		}
		q = q.Where("severity IN ?", vals)
	}
	if v := strings.TrimSpace(filter.Source); v != "" {
		q = q.Where("source = ?", v)
	}
	if v := strings.TrimSpace(filter.SourceID); v != "" {
		q = q.Where("source_id = ?", v)
	}
	if v := strings.TrimSpace(filter.BusinessLine); v != "" {
		q = q.Where("business_line = ?", v)
	}
	if v := strings.TrimSpace(filter.Environment); v != "" {
		q = q.Where("environment = ?", v)
	}
	if v := strings.TrimSpace(filter.ApplicationID); v != "" {
		q = q.Where("application_id = ?", v)
	}
	if v := strings.TrimSpace(filter.ResourceID); v != "" {
		q = q.Where("resource_id = ?", v)
	}
	if v := strings.TrimSpace(filter.AssigneeUserID); v != "" {
		q = q.Where("assignee_user_id = ?", v)
	}
	if filter.ActiveOnly {
		q = q.Where("status <> ?", string(domain.StatusClosed))
	}
	if filter.FromFirstSeen != nil {
		q = q.Where("first_seen_at >= ?", *filter.FromFirstSeen)
	}
	if filter.ToFirstSeen != nil {
		q = q.Where("first_seen_at <= ?", *filter.ToFirstSeen)
	}
	if kw := strings.TrimSpace(filter.Keyword); kw != "" {
		pattern := "%" + kw + "%"
		q = q.Where("name ILIKE ? OR summary ILIKE ? OR resource_name ILIKE ?", pattern, pattern, pattern)
	}
	return q
}

func toAlertModel(a *domain.Alert) (*alertModel, error) {
	labels, err := marshalStringMap(a.Labels)
	if err != nil {
		return nil, err
	}
	annotations, err := marshalStringMap(a.Annotations)
	if err != nil {
		return nil, err
	}
	return &alertModel{
		AlertID:         a.ID,
		ExternalID:      a.ExternalID,
		Source:          a.Source,
		SourceID:        a.SourceID,
		SourceName:      a.SourceName,
		Fingerprint:     a.Fingerprint,
		DedupKey:        a.DedupKey,
		LifecycleSeq:    a.LifecycleSeq,
		Name:            a.Name,
		Summary:         a.Summary,
		Description:     a.Description,
		Severity:        string(a.Severity),
		Status:          string(a.Status),
		RuleID:          a.RuleID,
		RuleName:        a.RuleName,
		BusinessLine:    a.BusinessLine,
		Environment:     a.Environment,
		ApplicationID:   a.ApplicationID,
		ApplicationName: a.ApplicationName,
		ResourceID:      a.ResourceID,
		ResourceType:    a.ResourceType,
		ResourceName:    a.ResourceName,
		OwnerUserID:     a.OwnerUserID,
		AssigneeUserID:  a.AssigneeUserID,
		Labels:          labels,
		Annotations:     annotations,
		OccurrenceCount: a.OccurrenceCount,
		FirstSeenAt:     a.FirstSeenAt,
		LastSeenAt:      a.LastSeenAt,
		RecoveredAt:     a.RecoveredAt,
		AcknowledgedAt:  a.AcknowledgedAt,
		ClosedAt:        a.ClosedAt,
		SilencedUntil:   a.SilencedUntil,
	}, nil
}

func toAlertDomain(m *alertModel) domain.Alert {
	if m == nil {
		return domain.Alert{}
	}
	return domain.Alert{
		ID:              m.AlertID,
		ExternalID:      m.ExternalID,
		Source:          m.Source,
		SourceID:        m.SourceID,
		SourceName:      m.SourceName,
		Fingerprint:     m.Fingerprint,
		DedupKey:        m.DedupKey,
		LifecycleSeq:    m.LifecycleSeq,
		Name:            m.Name,
		Summary:         m.Summary,
		Description:     m.Description,
		Severity:        domain.AlertSeverity(m.Severity),
		Status:          domain.AlertStatus(m.Status),
		RuleID:          m.RuleID,
		RuleName:        m.RuleName,
		BusinessLine:    m.BusinessLine,
		Environment:     m.Environment,
		ApplicationID:   m.ApplicationID,
		ApplicationName: m.ApplicationName,
		ResourceID:      m.ResourceID,
		ResourceType:    m.ResourceType,
		ResourceName:    m.ResourceName,
		OwnerUserID:     m.OwnerUserID,
		AssigneeUserID:  m.AssigneeUserID,
		Labels:          unmarshalStringMap(m.Labels),
		Annotations:     unmarshalStringMap(m.Annotations),
		OccurrenceCount: m.OccurrenceCount,
		FirstSeenAt:     m.FirstSeenAt,
		LastSeenAt:      m.LastSeenAt,
		RecoveredAt:     m.RecoveredAt,
		AcknowledgedAt:  m.AcknowledgedAt,
		ClosedAt:        m.ClosedAt,
		SilencedUntil:   m.SilencedUntil,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

func fillAlertFromModel(a *domain.Alert, m *alertModel) {
	a.CreatedAt = m.CreatedAt
	a.UpdatedAt = m.UpdatedAt
}
