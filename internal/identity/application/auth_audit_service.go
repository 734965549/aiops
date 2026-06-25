package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/identity/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// AuthAuditService 负责整理同写入认证审计，避免接口层直接处理持久化细节。
type AuthAuditService struct {
	repo domain.AuthAuditRepository
}

// NewAuthAuditService 创建认证审计服务；repo 可以由不同存储实现接上。
func NewAuthAuditService(repo domain.AuthAuditRepository) *AuthAuditService {
	return &AuthAuditService{repo: repo}
}

// Record 记录一次认证审计；如果审计仓储未配置，就静默略过，不影响登录主流程。
func (s *AuthAuditService) Record(ctx context.Context, audit domain.AuthAudit) error {
	if s == nil || s.repo == nil {
		return nil
	}
	audit.ID = strings.TrimSpace(audit.ID)
	if audit.ID == "" {
		audit.ID = uuid.NewString()
	}
	audit.UserID = strings.TrimSpace(audit.UserID)
	audit.Username = limitString(strings.TrimSpace(audit.Username), 64)
	audit.ProviderID = limitString(strings.TrimSpace(audit.ProviderID), 64)
	audit.IP = limitString(strings.TrimSpace(audit.IP), 64)
	audit.UserAgent = limitString(strings.TrimSpace(audit.UserAgent), 512)
	audit.Reason = limitString(strings.TrimSpace(audit.Reason), 255)
	if audit.Event == "" {
		audit.Event = domain.AuthAuditEventLogin
	}
	if audit.Method == "" {
		audit.Method = domain.AuthAuditMethodLocal
	}
	if audit.Result == "" {
		audit.Result = domain.AuthAuditResultFailure
	}
	if err := s.repo.Create(ctx, &audit); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "record auth audit failed")
	}
	return nil
}

// List 按筛选条件返回认证审计列表，主要俾管理员后台分页查询用。
func (s *AuthAuditService) List(ctx context.Context, filter domain.AuthAuditFilter) ([]domain.AuthAudit, error) {
	if s == nil || s.repo == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "auth audit is not enabled")
	}
	rows, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list auth audits failed")
	}
	return rows, nil
}

// Count 统计符合筛选条件的认证审计数量，用于配合分页。
func (s *AuthAuditService) Count(ctx context.Context, filter domain.AuthAuditFilter) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, apperr.New(apperr.CodeUnavailable, "auth audit is not enabled")
	}
	n, err := s.repo.Count(ctx, filter)
	if err != nil {
		return 0, apperr.Wrap(err, apperr.CodeInternal, "count auth audits failed")
	}
	return n, nil
}

// limitString 截断外部输入，防止 UA、失败原因等长文本撑爆字段。
func limitString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
