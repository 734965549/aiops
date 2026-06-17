// Package application 实现平台操作审计用例（Alert §9.4 写入点）。
package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/audit/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// RecordInput 写入一条操作审计。
type RecordInput struct {
	UserID       string
	ResourceType string
	ResourceID   string
	Action       string
	Payload      map[string]any
	IP           string
	UserAgent    string
}

// OperationAuditService 平台操作审计服务。
type OperationAuditService struct {
	repo domain.OperationAuditRepository
}

// NewOperationAuditService 构造服务。
func NewOperationAuditService(repo domain.OperationAuditRepository) *OperationAuditService {
	return &OperationAuditService{repo: repo}
}

// Record 写入审计；失败返回 error，由调用方决定是否忽略（Alert 侧忽略）。
func (s *OperationAuditService) Record(ctx context.Context, in RecordInput) error {
	if s == nil || s.repo == nil {
		return nil
	}
	payload := in.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	audit := domain.OperationAudit{
		ID:           uuid.NewString(),
		UserID:       strings.TrimSpace(in.UserID),
		ResourceType: strings.TrimSpace(in.ResourceType),
		ResourceID:   strings.TrimSpace(in.ResourceID),
		Action:       strings.TrimSpace(in.Action),
		Payload:      payload,
		IP:           limitString(strings.TrimSpace(in.IP), 64),
		UserAgent:    limitString(strings.TrimSpace(in.UserAgent), 512),
	}
	if audit.ResourceType == "" || audit.ResourceID == "" || audit.Action == "" {
		return apperr.New(apperr.CodeInvalidArgument, "resource_type, resource_id and action are required")
	}
	if err := s.repo.Create(ctx, &audit); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "record operation audit failed")
	}
	return nil
}

// ListQuery 审计列表查询。
type ListQuery struct {
	Page         int
	PageSize     int
	ResourceType string
	ResourceID   string
	UserID       string
	Action       string
}

// OperationAuditDTO 对外审计对象。
type OperationAuditDTO struct {
	ID           string         `json:"id"`
	UserID       string         `json:"user_id,omitempty"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Action       string         `json:"action"`
	Payload      map[string]any `json:"payload"`
	IP           string         `json:"ip,omitempty"`
	UserAgent    string         `json:"user_agent,omitempty"`
	CreatedAt    int64          `json:"created_at"`
}

func (s *OperationAuditService) List(ctx context.Context, q ListQuery) ([]OperationAuditDTO, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "audit service is not enabled")
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	if q.PageSize > 100 {
		q.PageSize = 100
	}
	filter := domain.OperationAuditFilter{
		ResourceType: strings.TrimSpace(q.ResourceType),
		ResourceID:   strings.TrimSpace(q.ResourceID),
		UserID:       strings.TrimSpace(q.UserID),
		Action:       strings.TrimSpace(q.Action),
		Limit:        q.PageSize,
		Offset:       (q.Page - 1) * q.PageSize,
	}
	rows, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, apperr.Wrap(err, apperr.CodeInternal, "list audits failed")
	}
	total, err := s.repo.Count(ctx, filter)
	if err != nil {
		return nil, 0, apperr.Wrap(err, apperr.CodeInternal, "count audits failed")
	}
	out := make([]OperationAuditDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAuditDTO(row))
	}
	return out, total, nil
}

func toAuditDTO(a domain.OperationAudit) OperationAuditDTO {
	payload := a.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	return OperationAuditDTO{
		ID: a.ID, UserID: a.UserID, ResourceType: a.ResourceType,
		ResourceID: a.ResourceID, Action: a.Action, Payload: payload,
		IP: a.IP, UserAgent: a.UserAgent, CreatedAt: a.CreatedAt.Unix(),
	}
}

func limitString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
