package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/alert/domain"
	"github.com/734965549/aiops/internal/alert/infrastructure/ingest"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// SourceService 管理外部告警接入源 CRUD（§1 AlertSource、§5.3）。
// secret 仅存哈希，API 返回 secret_masked；HTTP 层经 httpx.OK/Fail 输出（§2）。
type SourceService struct {
	sources domain.AlertSourceRepository
	audit   AuditRecorder
}

// CreateSourceInput 创建接入源入参。
type CreateSourceInput struct {
	ID           string
	Name         string
	Type         string
	Enabled      *bool // 省略时默认 true
	Secret       string
	Environment  string
	BusinessLine string
	Description  string
}

// UpdateSourceInput 更新接入源入参。
// 指针字段：nil 表示省略不修改；非 nil 时写入（environment/business_line/description 可为空字符串表示清空）。
// secret 仍用普通 string：空字符串表示保留原密钥（§8.12.4）。
type UpdateSourceInput struct {
	Name         *string
	Type         *string
	Enabled      *bool
	Secret       string
	Environment  *string
	BusinessLine *string
	Description  *string
}

// NewSourceService 构造接入源服务。
func NewSourceService(sources domain.AlertSourceRepository, audit AuditRecorder) *SourceService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &SourceService{sources: sources, audit: audit}
}

func (s *SourceService) List(ctx context.Context) ([]AlertSourceDTO, error) {
	if s == nil || s.sources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert source service is not enabled")
	}
	rows, err := s.sources.List(ctx)
	if err != nil {
		return nil, wrapAlertOpError(err, "list alert sources failed")
	}
	out := make([]AlertSourceDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, ToAlertSourceDTO(row, ingest.MaskSecret("")))
	}
	return out, nil
}

func (s *SourceService) Get(ctx context.Context, sourceID string) (*AlertSourceDTO, error) {
	if s == nil || s.sources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert source service is not enabled")
	}
	src, err := s.sources.GetByID(ctx, strings.TrimSpace(sourceID))
	if err != nil {
		return nil, wrapAlertOpError(err, "load alert source failed")
	}
	dto := ToAlertSourceDTO(*src, ingest.MaskSecret(""))
	return &dto, nil
}

func (s *SourceService) Create(ctx context.Context, actor Actor, in CreateSourceInput) (*AlertSourceDTO, error) {
	if s == nil || s.sources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert source service is not enabled")
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "id is required")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "name is required")
	}
	secret := strings.TrimSpace(in.Secret)
	if secret == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "secret is required")
	}
	srcType := domain.AlertSourceType(strings.TrimSpace(in.Type))
	if srcType == "" {
		srcType = domain.SourcePrometheusAlertmanager
	} else if !srcType.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid alert source type")
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	src := &domain.AlertSource{
		ID:           id,
		Name:         name,
		Type:         srcType,
		Enabled:      enabled,
		SecretHash:   ingest.HashWebhookSecret(secret),
		Environment:  strings.TrimSpace(in.Environment),
		BusinessLine: strings.TrimSpace(in.BusinessLine),
		Description:  strings.TrimSpace(in.Description),
	}
	if err := s.sources.Create(ctx, src); err != nil {
		return nil, wrapAlertOpError(err, "create alert source failed")
	}
	s.recordSourceAudit(ctx, src.ID, actor.UserID, AuditSourceCreate, map[string]any{
		"name": src.Name, "type": string(src.Type), "enabled": src.Enabled, "result": "success",
	})
	dto := ToAlertSourceDTO(*src, ingest.MaskSecret(secret))
	return &dto, nil
}

func (s *SourceService) Update(ctx context.Context, sourceID string, actor Actor, in UpdateSourceInput) (*AlertSourceDTO, error) {
	if s == nil || s.sources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert source service is not enabled")
	}
	src, err := s.sources.GetByID(ctx, strings.TrimSpace(sourceID))
	if err != nil {
		return nil, wrapAlertOpError(err, "load alert source failed")
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "name cannot be empty")
		}
		src.Name = name
	}
	if in.Type != nil {
		t := strings.TrimSpace(*in.Type)
		if t == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "type cannot be empty")
		}
		srcType := domain.AlertSourceType(t)
		if !srcType.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid alert source type")
		}
		src.Type = srcType
	}
	if in.Enabled != nil {
		src.Enabled = *in.Enabled
	}
	if secret := strings.TrimSpace(in.Secret); secret != "" {
		src.SecretHash = ingest.HashWebhookSecret(secret)
	}
	if in.Environment != nil {
		src.Environment = strings.TrimSpace(*in.Environment)
	}
	if in.BusinessLine != nil {
		src.BusinessLine = strings.TrimSpace(*in.BusinessLine)
	}
	if in.Description != nil {
		src.Description = strings.TrimSpace(*in.Description)
	}
	if err := s.sources.Update(ctx, src); err != nil {
		return nil, wrapAlertOpError(err, "update alert source failed")
	}
	s.recordSourceAudit(ctx, src.ID, actor.UserID, AuditSourceUpdate, map[string]any{
		"name": src.Name, "enabled": src.Enabled, "result": "success",
	})
	masked := ingest.MaskSecret(in.Secret)
	dto := ToAlertSourceDTO(*src, masked)
	return &dto, nil
}

func (s *SourceService) Delete(ctx context.Context, sourceID string, actor Actor) error {
	if s == nil || s.sources == nil {
		return apperr.New(apperr.CodeUnavailable, "alert source service is not enabled")
	}
	if err := s.sources.Delete(ctx, strings.TrimSpace(sourceID)); err != nil {
		return wrapAlertOpError(err, "delete alert source failed")
	}
	s.recordSourceAudit(ctx, sourceID, actor.UserID, AuditSourceDelete, map[string]any{"result": "success"})
	return nil
}

func (s *SourceService) recordSourceAudit(ctx context.Context, sourceID, userID string, action AuditAction, payload map[string]any) {
	if s == nil || s.audit == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "alert_source",
		ResourceID:   sourceID,
		Action:       action,
		UserID:       userID,
		Payload:      payload,
	})
}
