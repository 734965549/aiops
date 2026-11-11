package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

const configLeaseTTL = 30 * time.Second

// SourceService 管理外部告警接入源 CRUD（§1 AlertSource、§5.3、CES 拉取源 §4）。
// secret 仅存哈希，API 返回 secret_masked；HTTP 层经 httpx.OK/Fail 输出（§2）。
type SourceService struct {
	sources      domain.AlertSourceRepository
	accounts     IntegrationAccountReader
	access       AccountAccessChecker
	leases       AlertSyncLeaseStore
	audit        AuditRecorder
	allowFakeAcc bool
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
	// DeliveryMode 省略时默认 webhook（CES 契约 §4.3）。
	DeliveryMode domain.AlertSourceDeliveryMode
	AccountID    string
	// PullConfig 为 nil 表示使用默认值；非 nil 时 0 表示仅手动。
	PullConfig *domain.PullConfig
}

// UpdateSourceInput 更新接入源入参。
// 指针字段：nil 表示省略不修改；非 nil 时写入（environment/business_line/description 可为空字符串表示清空）。
type UpdateSourceInput struct {
	Name         *string
	Type         *string
	Enabled      *bool
	Secret       string
	Environment  *string
	BusinessLine *string
	Description  *string
	// DeliveryMode 只用于校验不可变性，传入不同值会被拒绝。
	DeliveryMode *domain.AlertSourceDeliveryMode
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

// SetAccountAccessChecker 注入账号数据范围检查端口。
func (s *SourceService) SetAccountAccessChecker(checker AccountAccessChecker) {
	if s != nil {
		s.access = checker
	}
}

// SetLeaseStore 注入拉取租约端口，用于源编辑/停用/删除与在途同步互斥。
func (s *SourceService) SetLeaseStore(store AlertSyncLeaseStore) {
	if s != nil {
		s.leases = store
	}
}

// SetAllowFakeAccounts 控制测试环境是否允许 auth_type=none 的拉取账号。
func (s *SourceService) SetAllowFakeAccounts(allow bool) {
	if s != nil {
		s.allowFakeAcc = allow
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
	mode := in.DeliveryMode
	if mode == "" {
		mode = domain.DeliveryModeWebhook
	}
	if mode != domain.DeliveryModePull && mode != domain.DeliveryModeWebhook {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid delivery_mode")
	}
	secret := strings.TrimSpace(in.Secret)
	accountID := strings.TrimSpace(in.AccountID)
	src := &domain.AlertSource{
		ID:           id,
		Name:         name,
		Type:         srcType,
		Enabled:      in.Enabled == nil || *in.Enabled,
		Environment:  strings.TrimSpace(in.Environment),
		BusinessLine: strings.TrimSpace(in.BusinessLine),
		Description:  strings.TrimSpace(in.Description),
		DeliveryMode: mode,
	}
	pullCfg := domain.PullConfig{PullIntervalSeconds: domain.DefaultPullIntervalSeconds}
	if in.PullConfig != nil {
		pullCfg = *in.PullConfig
	}
	if mode == domain.DeliveryModePull {
		if srcType != domain.SourceHuaweiCES {
			return nil, apperr.New(apperr.CodeInvalidArgument, "pull delivery is only supported for huawei_ces")
		}
		if secret != "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "pull source must not submit secret")
		}
		normalized, err := domain.NormalizePullConfig(pullCfg)
		if err != nil {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid pull_config")
		}
		account, err := s.validatePullAccount(ctx, accountID, actor.UserID, false)
		if err != nil {
			return nil, err
		}
		if err := validateRegionScope(account, normalized); err != nil {
			return nil, err
		}
		src.AccountID = accountID
		src.PullConfig = normalized.WithDefaults()
		src.SecretHash = ""
	} else {
		if accountID != "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "account_id is only supported for pull delivery")
		}
		if in.PullConfig != nil && !pullConfigEmpty(*in.PullConfig) {
			return nil, apperr.New(apperr.CodeInvalidArgument, "webhook source must not submit pull_config")
		}
		if secret == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "secret is required")
		}
		src.SecretHash = domain.HashWebhookSecret(secret)
	}
	if err := s.sources.Create(ctx, src); err != nil {
		return nil, wrapAlertOpError(err, "create alert source failed")
	}
	s.recordSourceAudit(ctx, src.ID, actor.UserID, AuditSourceCreate, map[string]any{
		"name": src.Name, "type": string(src.Type), "enabled": src.Enabled,
		"delivery_mode": string(src.DeliveryMode), "account_id": src.AccountID, "result": "success",
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
	release, err := s.lockSourceForConfig(ctx, src)
	if err != nil {
		return nil, err
	}
	if release != nil {
		defer release()
	}
	if in.DeliveryMode != nil && *in.DeliveryMode != "" && *in.DeliveryMode != src.DeliveryMode {
		return nil, apperr.New(apperr.CodeInvalidArgument, "delivery_mode cannot be changed after creation")
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
		if src.IsPullSource() && srcType != src.Type {
			return nil, apperr.New(apperr.CodeInvalidArgument, "pull source type cannot be changed")
		}
		src.Type = srcType
	}
	if in.Enabled != nil {
		src.Enabled = *in.Enabled
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
	secret := strings.TrimSpace(in.Secret)
	if src.IsPullSource() {
		if in.AccountID != nil && strings.TrimSpace(*in.AccountID) != src.AccountID {
			return nil, apperr.New(apperr.CodeInvalidArgument, "pull source account_id cannot be changed")
		}
		if secret != "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "pull source must not submit secret")
		}
		if in.PullConfig != nil {
			candidate := *in.PullConfig
			if candidate.PullIntervalSeconds == 0 && src.PullConfig.PullIntervalSeconds == 0 {
				// 仍为仅手动
			}
			normalized, err := domain.NormalizePullConfig(candidate)
			if err != nil {
				return nil, apperr.New(apperr.CodeInvalidArgument, "invalid pull_config")
			}
			account, err := s.validatePullAccount(ctx, src.AccountID, actor.UserID, false)
			if err != nil {
				return nil, err
			}
			if err := validateRegionScope(account, normalized); err != nil {
				return nil, err
			}
			src.PullConfig = normalized.WithDefaults()
		}
	} else {
		if in.AccountID != nil && strings.TrimSpace(*in.AccountID) != "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "account_id is only supported for pull delivery")
		}
		if in.PullConfig != nil && !pullConfigEmpty(*in.PullConfig) {
			return nil, apperr.New(apperr.CodeInvalidArgument, "webhook source must not submit pull_config")
		}
		if secret != "" {
			src.SecretHash = domain.HashWebhookSecret(secret)
		}
		if strings.TrimSpace(src.SecretHash) == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "secret is required for webhook delivery")
		}
	}
	if err := s.sources.Update(ctx, src); err != nil {
		return nil, wrapAlertOpError(err, "update alert source failed")
	}
	s.recordSourceAudit(ctx, src.ID, actor.UserID, AuditSourceUpdate, map[string]any{
		"name": src.Name, "enabled": src.Enabled,
		"delivery_mode": string(src.DeliveryMode), "account_id": src.AccountID, "result": "success",
	})
	masked := domain.MaskSecret(secret)
	dto := ToAlertSourceDTO(*src, masked)
	return &dto, nil
}

func (s *SourceService) Delete(ctx context.Context, sourceID string, actor Actor) error {
	if s == nil || s.sources == nil {
		return apperr.New(apperr.CodeUnavailable, "alert source service is not enabled")
	}
	src, err := s.sources.GetByID(ctx, strings.TrimSpace(sourceID))
	if err != nil {
		return wrapAlertOpError(err, "load alert source failed")
	}
	release, err := s.lockSourceForConfig(ctx, src)
	if err != nil {
		return err
	}
	if release != nil {
		defer release()
	}
	if err := s.sources.Delete(ctx, src.ID); err != nil {
		return wrapAlertOpError(err, "delete alert source failed")
	}
	s.recordSourceAudit(ctx, src.ID, actor.UserID, AuditSourceDelete, map[string]any{"result": "success"})
	return nil
}

// ValidatePullAccountRuntime 供同步用例在每轮开始前重新校验账号准入条件。
func (s *SourceService) ValidatePullAccountRuntime(ctx context.Context, accountID, userID string) (*IntegrationAccountInfo, error) {
	return s.validatePullAccount(ctx, accountID, userID, true)
}

func (s *SourceService) validatePullAccount(ctx context.Context, accountID, userID string, runtime bool) (*IntegrationAccountInfo, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "account_id is required for pull delivery")
	}
	if s == nil || s.accounts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "integration account reader is not configured")
	}
	acc, err := s.accounts.GetAccount(ctx, accountID)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) {
			return nil, err
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "resolve integration account failed")
	}
	if acc == nil {
		return nil, apperr.New(apperr.CodeNotFound, "integration account not found")
	}
	code := apperr.CodeInvalidArgument
	if runtime {
		code = apperr.CodeFailedPrecondition
	}
	if !acc.Enabled {
		return nil, apperr.New(code, "integration account is disabled")
	}
	if acc.Provider != "huawei_cloud" {
		return nil, apperr.New(code, "account provider must be huawei_cloud for huawei_ces pull")
	}
	if !hasAccountCapability(acc.Capabilities, "alerts") {
		return nil, apperr.New(code, "integration account does not declare alerts capability")
	}
	switch acc.AuthType {
	case "ak_sk":
	case "none":
		if !s.allowFakeAcc {
			return nil, apperr.New(code, "auth_type=none is only allowed in test environments")
		}
	default:
		return nil, apperr.New(code, "pull source requires ak_sk auth")
	}
	if strings.TrimSpace(userID) != "" && s.access != nil {
		if err := s.access.CheckAccountAccess(ctx, userID, acc); err != nil {
			return nil, err
		}
	}
	return acc, nil
}

func (s *SourceService) lockSourceForConfig(ctx context.Context, src *domain.AlertSource) (func(), error) {
	if src == nil || !src.IsPullSource() || s.leases == nil {
		return nil, nil
	}
	owner := "config:" + uuid.NewString()
	ok, err := s.leases.Acquire(ctx, src.ID, owner, configLeaseTTL)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeUnavailable, "acquire alert source config lease failed")
	}
	if !ok {
		return nil, apperr.New(apperr.CodeAborted, "alert source is syncing")
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.leases.Release(releaseCtx, src.ID, owner)
	}, nil
}

func validateRegionScope(acc *IntegrationAccountInfo, cfg domain.PullConfig) error {
	if acc == nil {
		return apperr.New(apperr.CodeInvalidArgument, "integration account not found")
	}
	accountRegions := map[string]struct{}{}
	for _, r := range acc.Regions {
		accountRegions[strings.ToLower(strings.TrimSpace(r))] = struct{}{}
	}
	if len(accountRegions) == 0 {
		return apperr.New(apperr.CodeInvalidArgument, "integration account has no regions")
	}
	filter := cfg.EffectiveRegionFilter()
	for _, r := range filter {
		if _, ok := accountRegions[r]; !ok {
			return apperr.Newf(apperr.CodeInvalidArgument, "region_filter contains region not configured on account: %s", r)
		}
	}
	regions := filter
	if len(regions) == 0 {
		for r := range accountRegions {
			regions = append(regions, r)
		}
	}
	for _, r := range regions {
		if _, err := resolveAccountProject(acc, r); err != nil {
			return err
		}
	}
	return nil
}

func resolveAccountProject(acc *IntegrationAccountInfo, region string) (string, error) {
	region = strings.ToLower(strings.TrimSpace(region))
	if pid := strings.TrimSpace(acc.RegionProjects[region]); pid != "" {
		return pid, nil
	}
	if pid := strings.TrimSpace(acc.ProjectID); pid != "" {
		return pid, nil
	}
	return "", apperr.Newf(apperr.CodeInvalidArgument, "scope %s has no project_id", region)
}

func pullConfigEmpty(cfg domain.PullConfig) bool {
	return cfg.PullIntervalSeconds == 0 && len(cfg.SeverityMap) == 0 && len(cfg.RegionFilter) == 0
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
