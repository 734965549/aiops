package application

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// SourceService 管理外部告警接入源 CRUD（§1 AlertSource、§5.3、CES 拉取源 §4）。
// secret 仅存哈希，API 返回 secret_masked；HTTP 层经 httpx.OK/Fail 输出（§2）。
type SourceService struct {
	sources  domain.AlertSourceRepository
	accounts IntegrationAccountReader
	audit    AuditRecorder
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
	// AccountID / PullConfig 为拉取式接入扩展；Webhook 源留空。
	AccountID  string
	PullConfig domain.PullConfig
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
	AccountID    *string
	PullConfig   *domain.PullConfig
}

// NewSourceService 构造接入源服务。
func NewSourceService(sources domain.AlertSourceRepository, audit AuditRecorder) *SourceService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &SourceService{sources: sources, audit: audit}
}

// SetAccountReader 注入 integration 账号读取端口；装配完成后调用。
func (s *SourceService) SetAccountReader(reader IntegrationAccountReader) {
	if s != nil {
		s.accounts = reader
	}
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
		out = append(out, ToAlertSourceDTO(row, domain.MaskSecret("")))
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
	dto := ToAlertSourceDTO(*src, domain.MaskSecret(""))
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
	srcType := domain.AlertSourceType(strings.TrimSpace(in.Type))
	if srcType == "" {
		srcType = domain.SourcePrometheusAlertmanager
	} else if !srcType.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid alert source type")
	}
	accountID := strings.TrimSpace(in.AccountID)
	mode, err := resolveDeliveryMode(srcType, accountID, in.PullConfig.DeliveryMode)
	if err != nil {
		return nil, err
	}
	secret := strings.TrimSpace(in.Secret)
	if mode == domain.DeliveryModePull {
		if err := s.validatePullAccount(ctx, accountID); err != nil {
			return nil, err
		}
	} else {
		if secret == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "secret is required")
		}
		if accountID != "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "account_id is only supported for pull delivery")
		}
	}
	pullConfig, err := domain.NormalizePullConfig(domain.PullConfig{
		DeliveryMode:        mode,
		PullIntervalSeconds: in.PullConfig.PullIntervalSeconds,
		SeverityMap:         in.PullConfig.SeverityMap,
		RegionFilter:        in.PullConfig.RegionFilter,
	})
	if err != nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid pull_config")
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
		SecretHash:   hashOptionalSecret(secret),
		Environment:  strings.TrimSpace(in.Environment),
		BusinessLine: strings.TrimSpace(in.BusinessLine),
		Description:  strings.TrimSpace(in.Description),
		AccountID:    accountID,
		PullConfig:   pullConfig,
	}
	if err := s.sources.Create(ctx, src); err != nil {
		return nil, wrapAlertOpError(err, "create alert source failed")
	}
	s.recordSourceAudit(ctx, src.ID, actor.UserID, AuditSourceCreate, map[string]any{
		"name": src.Name, "type": string(src.Type), "enabled": src.Enabled,
		"delivery_mode": string(pullConfig.DeliveryMode), "account_id": src.AccountID, "result": "success",
	})
	dto := ToAlertSourceDTO(*src, domain.MaskSecret(secret))
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
	oldMode := src.PullConfig.DeliveryMode
	oldAccountID := src.AccountID
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
	accountID := src.AccountID
	if in.AccountID != nil {
		accountID = strings.TrimSpace(*in.AccountID)
	}
	cfg := src.PullConfig
	if in.PullConfig != nil {
		cfg = *in.PullConfig
	}
	requestedMode := cfg.DeliveryMode
	mode, err := resolveDeliveryMode(src.Type, accountID, requestedMode)
	if err != nil {
		return nil, err
	}
	if mode == domain.DeliveryModePull {
		if err := s.validatePullAccount(ctx, accountID); err != nil {
			return nil, err
		}
	} else if accountID != "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "account_id is only supported for pull delivery")
	}
	secret := strings.TrimSpace(in.Secret)
	if secret != "" {
		src.SecretHash = domain.HashWebhookSecret(secret)
	}
	if mode == domain.DeliveryModeWebhook && strings.TrimSpace(src.SecretHash) == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "secret is required for webhook delivery")
	}
	pullConfig, err := domain.NormalizePullConfig(domain.PullConfig{
		DeliveryMode:        mode,
		PullIntervalSeconds: cfg.PullIntervalSeconds,
		SeverityMap:         cfg.SeverityMap,
		RegionFilter:        cfg.RegionFilter,
	})
	if err != nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid pull_config")
	}
	src.AccountID = accountID
	src.PullConfig = pullConfig
	if oldMode != mode || oldAccountID != accountID {
		src.LastSyncAt = nil
		src.LastSyncStatus = ""
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
		"name": src.Name, "enabled": src.Enabled,
		"delivery_mode": string(pullConfig.DeliveryMode), "account_id": src.AccountID, "result": "success",
	})
	masked := domain.MaskSecret(in.Secret)
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

func (s *SourceService) validatePullAccount(ctx context.Context, accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return apperr.New(apperr.CodeInvalidArgument, "account_id is required for pull delivery")
	}
	if s == nil || s.accounts == nil {
		return apperr.New(apperr.CodeUnavailable, "integration account reader is not configured")
	}
	acc, err := s.accounts.GetAccount(ctx, accountID)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) {
			return err
		}
		return apperr.Wrap(err, apperr.CodeInternal, "resolve integration account failed")
	}
	if acc == nil {
		return apperr.New(apperr.CodeNotFound, "integration account not found")
	}
	if acc.Provider != "huawei_cloud" {
		return apperr.New(apperr.CodeInvalidArgument, "account provider must be huawei_cloud for huawei_ces pull")
	}
	if !acc.Enabled {
		return apperr.New(apperr.CodeInvalidArgument, "integration account is disabled")
	}
	if !hasAccountCapability(acc.Capabilities, "alerts") {
		return apperr.New(apperr.CodeInvalidArgument, "integration account does not declare alerts capability")
	}
	return nil
}

func resolveDeliveryMode(srcType domain.AlertSourceType, accountID string, requested domain.AlertSourceDeliveryMode) (domain.AlertSourceDeliveryMode, error) {
	if requested == "" {
		if srcType == domain.SourceHuaweiCES && strings.TrimSpace(accountID) != "" {
			return domain.DeliveryModePull, nil
		}
		return domain.DeliveryModeWebhook, nil
	}
	switch requested {
	case domain.DeliveryModePull:
		if srcType != domain.SourceHuaweiCES {
			return "", apperr.New(apperr.CodeInvalidArgument, "pull delivery is only supported for huawei_ces")
		}
		return domain.DeliveryModePull, nil
	case domain.DeliveryModeWebhook:
		return domain.DeliveryModeWebhook, nil
	default:
		return "", apperr.New(apperr.CodeInvalidArgument, "invalid delivery_mode")
	}
}

func hashOptionalSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	return domain.HashWebhookSecret(secret)
}

func hasAccountCapability(caps []string, want string) bool {
	for _, c := range caps {
		if strings.EqualFold(strings.TrimSpace(c), want) {
			return true
		}
	}
	return false
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
