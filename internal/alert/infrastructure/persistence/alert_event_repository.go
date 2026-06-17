package persistence

import (
	"context"
	"errors"

	"github.com/734965549/aiops/internal/alert/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

// alertEventModel 对应 alert_event 表。
type alertEventModel struct {
	database.BaseModel
	EventID   string `gorm:"column:event_id;type:varchar(36);uniqueIndex;not null"`
	AlertID   string `gorm:"column:alert_id;type:varchar(36);not null;index"`
	EventType string `gorm:"column:event_type;type:varchar(64);not null"`
	ActorType string `gorm:"column:actor_type;type:varchar(32);not null"`
	ActorID   string `gorm:"column:actor_id;type:varchar(64);not null;default:''"`
	ActorName string `gorm:"column:actor_name;type:varchar(128);not null;default:''"`
	Message   string `gorm:"column:message;type:varchar(1024);not null;default:''"`
	Payload   []byte `gorm:"column:payload;type:jsonb;not null;default:'{}'::jsonb"`
}

func (alertEventModel) TableName() string { return "alert_event" }

// AlertEventRepository 告警时间线 GORM 仓储。
type AlertEventRepository struct {
	db *gorm.DB
}

// NewAlertEventRepository 创建时间线仓储。
func NewAlertEventRepository(db *gorm.DB) *AlertEventRepository {
	return &AlertEventRepository{db: db}
}

func (r *AlertEventRepository) Create(ctx context.Context, event *domain.AlertEvent) error {
	if r == nil || r.db == nil {
		return errors.New("alert event repository is not configured")
	}
	if event == nil {
		return errors.New("alert event is nil")
	}
	payload, err := marshalAnyMap(event.Payload)
	if err != nil {
		return err
	}
	m := alertEventModel{
		EventID:   event.ID,
		AlertID:   event.AlertID,
		EventType: string(event.EventType),
		ActorType: string(event.ActorType),
		ActorID:   event.ActorID,
		ActorName: event.ActorName,
		Message:   event.Message,
		Payload:   payload,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	event.CreatedAt = m.CreatedAt
	event.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *AlertEventRepository) ListByAlertID(ctx context.Context, alertID string) ([]domain.AlertEvent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("alert event repository is not configured")
	}
	var rows []alertEventModel
	if err := r.db.WithContext(ctx).
		Where("alert_id = ?", alertID).
		Order("created_at ASC, id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.AlertEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAlertEventDomain(&row))
	}
	return out, nil
}

func toAlertEventDomain(m *alertEventModel) domain.AlertEvent {
	if m == nil {
		return domain.AlertEvent{}
	}
	return domain.AlertEvent{
		ID:        m.EventID,
		AlertID:   m.AlertID,
		EventType: domain.AlertEventType(m.EventType),
		ActorType: domain.ActorType(m.ActorType),
		ActorID:   m.ActorID,
		ActorName: m.ActorName,
		Message:   m.Message,
		Payload:   unmarshalAnyMap(m.Payload),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
