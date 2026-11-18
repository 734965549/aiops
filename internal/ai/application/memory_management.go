package application

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// 管理端列表默认与上限，与知识条目列表保持一致。
const (
	MemoryCaseListDefaultLimit = 20
	MemoryCaseListMaxLimit     = 100
)

var _ MemoryAdminStore = (*MemoryService)(nil)

// ListCases 返回记忆条目分页列表（管理端，可按归属用户/状态/关键词检索）。
func (s *MemoryService) ListCases(ctx context.Context, in MemoryCaseListInput) ([]domain.MemoryCase, error) {
	if s == nil || s.repo == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai memory service is not enabled")
	}
	filter := domain.MemoryCaseListFilter{
		UserID:        strings.TrimSpace(in.UserID),
		ApplicationID: strings.TrimSpace(in.ApplicationID),
		ResourceID:    strings.TrimSpace(in.ResourceID),
		Keyword:       strings.TrimSpace(in.Keyword),
		Limit:         in.Limit,
		Offset:        in.Offset,
	}
	if filter.Limit <= 0 || filter.Limit > MemoryCaseListMaxLimit {
		filter.Limit = MemoryCaseListDefaultLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if raw := strings.TrimSpace(in.Status); raw != "" {
		status := domain.MemoryCaseStatus(strings.ToLower(raw))
		if !status.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid memory case status")
		}
		filter.Status = &status
	}
	cases, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list ai memory cases failed")
	}
	return cases, nil
}

// UpdateCaseStatus 手动停用/启用记忆条目；启停只影响后续检索，不改写案例内容。
func (s *MemoryService) UpdateCaseStatus(ctx context.Context, actorID, caseID, statusRaw string) (*domain.MemoryCase, error) {
	if s == nil || s.repo == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai memory service is not enabled")
	}
	caseID = strings.TrimSpace(caseID)
	if caseID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "case_id is required")
	}
	target := domain.MemoryCaseStatus(strings.ToLower(strings.TrimSpace(statusRaw)))
	if !target.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "status must be active or retired")
	}
	current, err := s.repo.Get(ctx, caseID)
	if err != nil {
		if errors.Is(err, domain.ErrMemoryCaseNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "memory case not found")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load ai memory case failed")
	}
	if current.Status == target {
		return current, nil
	}
	previous := current.Status
	now := s.now()
	if err := s.repo.UpdateStatus(ctx, caseID, target, now); err != nil {
		if errors.Is(err, domain.ErrMemoryCaseNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "memory case not found")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "update ai memory case status failed")
	}
	current.Status = target
	current.UpdatedAt = now
	s.recordMemoryAudit(ctx, actorID, caseID, map[string]any{
		"case_id":         caseID,
		"previous_status": string(previous),
		"status":          string(target),
		"result":          "success",
	})
	return current, nil
}

func (s *MemoryService) recordMemoryAudit(ctx context.Context, userID, caseID string, payload map[string]any) {
	if s == nil || s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "ai", ResourceID: caseID, Action: AuditMemoryReview, UserID: userID, Payload: payload,
	})
}
