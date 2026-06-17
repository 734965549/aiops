package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// mapDomainError 将 alert domain 哨兵错误映射为 pkg/errors 统一错误码。
func mapDomainError(err error) error {
	return apperr.MapSentinels(err, "alert operation failed",
		apperr.Sentinel{Err: domain.ErrNotFound, Code: apperr.CodeNotFound},
		apperr.Sentinel{Err: domain.ErrAlreadyExists, Code: apperr.CodeAlreadyExists},
		apperr.Sentinel{Err: domain.ErrInvalidTransition, Code: apperr.CodeInvalidArgument},
	)
}

func newEventID() string {
	return uuid.NewString()
}

func newAlertID() string {
	return uuid.NewString()
}

// recordEvent 写入一条时间线事件；events 未配置时静默跳过。
func (s *AlertService) recordEvent(ctx context.Context, alertID string, eventType domain.AlertEventType, actorType domain.ActorType, actorID, actorName, message string, payload map[string]any) error {
	if s == nil || s.events == nil {
		return nil
	}
	if payload == nil {
		payload = map[string]any{}
	}
	ev := &domain.AlertEvent{
		ID:        newEventID(),
		AlertID:   alertID,
		EventType: eventType,
		ActorType: actorType,
		ActorID:   strings.TrimSpace(actorID),
		ActorName: strings.TrimSpace(actorName),
		Message:   strings.TrimSpace(message),
		Payload:   payload,
	}
	if err := s.events.Create(ctx, ev); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "record alert event failed")
	}
	return nil
}

// recordAudit 调用审计预留接口，失败不影响主流程。
func (s *AlertService) recordAudit(ctx context.Context, alertID, userID string, action AuditAction, payload map[string]any) {
	if s == nil || s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "alert",
		ResourceID:   alertID,
		Action:       action,
		UserID:       userID,
		Payload:      payload,
	})
}

// wrapAlertOpError 先尝试 domain 哨兵映射，否则 Wrap 为 INTERNAL。
func wrapAlertOpError(err error, op string) error {
	if err == nil {
		return nil
	}
	if mapped := mapDomainError(err); apperr.FromError(mapped).Code != apperr.CodeInternal {
		return mapped
	}
	return apperr.Wrap(err, apperr.CodeInternal, op)
}

func limitString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
