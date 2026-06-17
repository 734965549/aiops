package application

import (
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
)

// AlertDTO 对外 API 告警对象，字段与 ops/alert-contract.md §5.1 对齐；时间为 Unix 秒。
type AlertDTO struct {
	ID              string            `json:"id"`
	ExternalID      string            `json:"external_id,omitempty"`
	Source          string            `json:"source"`
	SourceID        string            `json:"source_id,omitempty"`
	SourceName      string            `json:"source_name,omitempty"`
	Fingerprint     string            `json:"fingerprint"`
	DedupKey        string            `json:"dedup_key"`
	Name            string            `json:"name"`
	Summary         string            `json:"summary,omitempty"`
	Description     string            `json:"description,omitempty"`
	Severity        string            `json:"severity"`
	Status          string            `json:"status"`
	RuleID          string            `json:"rule_id,omitempty"`
	RuleName        string            `json:"rule_name,omitempty"`
	BusinessLine    string            `json:"business_line,omitempty"`
	Environment     string            `json:"environment,omitempty"`
	ApplicationID   string            `json:"application_id,omitempty"`
	ApplicationName string            `json:"application_name,omitempty"`
	ResourceID      string            `json:"resource_id,omitempty"`
	ResourceType    string            `json:"resource_type,omitempty"`
	ResourceName    string            `json:"resource_name,omitempty"`
	OwnerUserID     string            `json:"owner_user_id,omitempty"`
	AssigneeUserID  string            `json:"assignee_user_id,omitempty"`
	Labels          map[string]string `json:"labels"`
	Annotations     map[string]string `json:"annotations"`
	OccurrenceCount int               `json:"occurrence_count"`
	FirstSeenAt     int64             `json:"first_seen_at"`
	LastSeenAt      int64             `json:"last_seen_at"`
	RecoveredAt     *int64            `json:"recovered_at,omitempty"`
	AcknowledgedAt  *int64            `json:"acknowledged_at,omitempty"`
	ClosedAt        *int64            `json:"closed_at,omitempty"`
	SilencedUntil   *int64            `json:"silenced_until,omitempty"`
	CreatedAt       int64             `json:"created_at"`
	UpdatedAt       int64             `json:"updated_at"`
}

// AlertEventDTO 时间线事件。
type AlertEventDTO struct {
	ID        string         `json:"id"`
	AlertID   string         `json:"alert_id"`
	EventType string         `json:"event_type"`
	ActorType string         `json:"actor_type"`
	ActorID   string         `json:"actor_id,omitempty"`
	ActorName string         `json:"actor_name,omitempty"`
	Message   string         `json:"message,omitempty"`
	Payload   map[string]any `json:"payload"`
	CreatedAt int64          `json:"created_at"`
}

// AlertDetailDTO 告警详情。
type AlertDetailDTO struct {
	Alert   AlertDTO        `json:"alert"`
	Events  []AlertEventDTO `json:"events"`
	Related map[string]any  `json:"related"`
}

// AlertSourceDTO 接入源（不含明文密钥）。
type AlertSourceDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Enabled      bool   `json:"enabled"`
	SecretMasked string `json:"secret_masked,omitempty"`
	Environment  string `json:"environment,omitempty"`
	BusinessLine string `json:"business_line,omitempty"`
	Description  string `json:"description,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

// IngestResultDTO Webhook 接入结果（§6.1 / §6.2 响应 data）。
type IngestResultDTO struct {
	Accepted  int `json:"accepted"`
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Recovered int `json:"recovered"`
	Ignored   int `json:"ignored"`
}

func ToAlertDTO(a domain.Alert) AlertDTO {
	labels := a.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	annotations := a.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}
	return AlertDTO{
		ID:              a.ID,
		ExternalID:      a.ExternalID,
		Source:          a.Source,
		SourceID:        a.SourceID,
		SourceName:      a.SourceName,
		Fingerprint:     a.Fingerprint,
		DedupKey:        a.DedupKey,
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
		FirstSeenAt:     a.FirstSeenAt.Unix(),
		LastSeenAt:      a.LastSeenAt.Unix(),
		RecoveredAt:     timePtrToUnix(a.RecoveredAt),
		AcknowledgedAt:  timePtrToUnix(a.AcknowledgedAt),
		ClosedAt:        timePtrToUnix(a.ClosedAt),
		SilencedUntil:   timePtrToUnix(a.SilencedUntil),
		CreatedAt:       a.CreatedAt.Unix(),
		UpdatedAt:       a.UpdatedAt.Unix(),
	}
}

func ToAlertEventDTO(e domain.AlertEvent) AlertEventDTO {
	payload := e.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	return AlertEventDTO{
		ID:        e.ID,
		AlertID:   e.AlertID,
		EventType: string(e.EventType),
		ActorType: string(e.ActorType),
		ActorID:   e.ActorID,
		ActorName: e.ActorName,
		Message:   e.Message,
		Payload:   payload,
		CreatedAt: e.CreatedAt.Unix(),
	}
}

func ToAlertSourceDTO(s domain.AlertSource, secretMasked string) AlertSourceDTO {
	return AlertSourceDTO{
		ID:           s.ID,
		Name:         s.Name,
		Type:         string(s.Type),
		Enabled:      s.Enabled,
		SecretMasked: secretMasked,
		Environment:  s.Environment,
		BusinessLine: s.BusinessLine,
		Description:  s.Description,
		CreatedAt:    s.CreatedAt.Unix(),
		UpdatedAt:    s.UpdatedAt.Unix(),
	}
}

func timePtrToUnix(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := t.Unix()
	return &v
}
