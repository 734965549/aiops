package application

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	httpx "github.com/734965549/aiops/pkg/transport/http"
)

// IngestService 负责 Webhook 接入（ops/alert-contract.md §1 外部告警接收、§3.2 鉴权与幂等）。
//
// §3.2 要求：按 source_id 查启用接入源、校验 X-AIOPS-Webhook-Token（失败 UNAUTHENTICATED）、
// 记录 IP/User-Agent/trace_id、X-Request-ID 短期幂等（生产 RedisStore 跨 Pod；dev 可 MemoryStore）。
// 返回统计由 HTTP 层经 httpx.OK 封装为 §2 统一响应。
type IngestService struct {
	alerts      domain.AlertRepository
	events      domain.AlertEventRepository
	sources     domain.AlertSourceRepository
	assets      AssetMatcher
	idempotency *webhookIdempotencyExecutor
	audit       AuditRecorder
}

// NewIngestService 构造接入服务；assets 为 nil 时使用 NoopAssetMatcher；生产注入 RedisStore，单实例开发可注入 MemoryStore。
func NewIngestService(
	alerts domain.AlertRepository,
	events domain.AlertEventRepository,
	sources domain.AlertSourceRepository,
	assets AssetMatcher,
	idempotency WebhookIdempotencyStore,
	audit AuditRecorder,
) *IngestService {
	if assets == nil {
		assets = NoopAssetMatcher{}
	}
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	var exec *webhookIdempotencyExecutor
	if idempotency != nil {
		exec = &webhookIdempotencyExecutor{store: idempotency}
	}
	return &IngestService{alerts: alerts, events: events, sources: sources, assets: assets, idempotency: exec, audit: audit}
}

// IngestContext Webhook 接入上下文元数据（§3.2：source_id、token、IP、User-Agent；§6.1 X-Request-ID 辅助幂等）。
type IngestContext struct {
	SourceID  string
	Token     string
	RequestID string
	IP        string
	UserAgent string
}

// VerifySource 校验 §3.2：接入源存在且启用、Webhook token 正确；失败返回 NOT_FOUND 或 UNAUTHENTICATED。
func (s *IngestService) VerifySource(ctx context.Context, ingestCtx IngestContext) (*domain.AlertSource, error) {
	if s == nil || s.sources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert ingest is not enabled")
	}
	sourceID := strings.TrimSpace(ingestCtx.SourceID)
	src, err := s.sources.GetByID(ctx, sourceID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "alert source not found")
		}
		return nil, wrapAlertOpError(err, "load alert source failed")
	}
	if src.IsPullSource() {
		// 拉取源不提供公开 Webhook 入口；在校验 token 前按不存在处理（CES 契约 §4.3）。
		return nil, apperr.New(apperr.CodeNotFound, "alert source not found")
	}
	if !src.Enabled {
		return nil, apperr.New(apperr.CodeNotFound, "alert source not found")
	}
	if !domain.VerifyWebhookSecret(ingestCtx.Token, src.SecretHash) {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid webhook token")
	}
	return src, nil
}

// IngestAlertmanager 处理 Prometheus Alertmanager Webhook payload（§6.1）；入口含 §3.2 鉴权与幂等。
func (s *IngestService) IngestAlertmanager(ctx context.Context, ingestCtx IngestContext, payload AlertmanagerWebhook) (*IngestResultDTO, error) {
	src, err := s.VerifySource(ctx, ingestCtx)
	if err != nil {
		return nil, err
	}
	key := ingestIdempotencyKey(ingestCtx.SourceID, ingestCtx.RequestID)
	return s.idempotency.do(ctx, key, func() (*IngestResultDTO, error) {
		defaults := EnvironmentDefaults{Environment: src.Environment, BusinessLine: src.BusinessLine}
		normalized := ParseAlertmanagerWebhook(payload, defaults)
		return s.ingestNormalized(ctx, src, ingestCtx, normalized, IngestOptions{})
	})
}

// IngestGeneric 处理通用 Webhook（§6.2）；入口含 §3.2 鉴权与幂等。
func (s *IngestService) IngestGeneric(ctx context.Context, ingestCtx IngestContext, payload GenericWebhookPayload) (*IngestResultDTO, error) {
	src, err := s.VerifySource(ctx, ingestCtx)
	if err != nil {
		return nil, err
	}
	key := ingestIdempotencyKey(ingestCtx.SourceID, ingestCtx.RequestID)
	return s.idempotency.do(ctx, key, func() (*IngestResultDTO, error) {
		defaults := EnvironmentDefaults{Environment: src.Environment, BusinessLine: src.BusinessLine}
		normalized := []NormalizedAlert{ParseGenericWebhook(payload, defaults)}
		return s.ingestNormalized(ctx, src, ingestCtx, normalized, IngestOptions{})
	})
}

// IngestOptions 控制接入归一化管道的生命周期语义。
type IngestOptions struct {
	// IgnoreTerminal 为 true 时（拉取式接入），同一 dedup_key 已 recovered/closed 后
	// 再次观测到 firing 不重开，直接计入 ignored；Webhook 保持 false 以维持 §7.2 重开语义。
	IgnoreTerminal bool
	// DedupeByCloudTrigger 为 true 时，仅当 last_alarm_time 比已记录值更新才记 updated，
	// 重复/旧观测计入 ignored（CES 契约 §7.1）。
	DedupeByCloudTrigger bool
	// ContinueOnError 为 true 时单条写入失败不中断整轮，累计 Failed 并继续；
	// 用于拉取式接入的部分落库语义。
	ContinueOnError bool
}

// IngestNormalized 复用 Webhook 归一化管道写入拉取式告警（CES 等），
// 调用方已完成鉴权/账号校验，并显式提供 DedupKey。
func (s *IngestService) IngestNormalized(ctx context.Context, src *domain.AlertSource, ingestCtx IngestContext, items []NormalizedAlert, opts IngestOptions) (*IngestResultDTO, error) {
	return s.ingestNormalized(ctx, src, ingestCtx, items, opts)
}

func (s *IngestService) ingestNormalized(ctx context.Context, src *domain.AlertSource, ingestCtx IngestContext, items []NormalizedAlert, opts IngestOptions) (*IngestResultDTO, error) {
	if s == nil || s.alerts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert ingest is not enabled")
	}
	result := &IngestResultDTO{}
	var firstErr error
	for _, item := range items {
		result.Accepted++
		if strings.TrimSpace(item.Name) == "" {
			result.Ignored++
			continue
		}
		created, updated, recovered, ignored, err := s.processOne(ctx, src, ingestCtx, item, opts)
		if err != nil {
			if !opts.ContinueOnError {
				return nil, err
			}
			result.Failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		result.Created += created
		result.Updated += updated
		result.Recovered += recovered
		result.Ignored += ignored
	}
	logger.From(ctx).Info("alert ingest completed",
		logger.String("source_id", src.ID),
		logger.String("source_name", src.Name),
		logger.Int("accepted", result.Accepted),
		logger.Int("created", result.Created),
		logger.Int("updated", result.Updated),
		logger.Int("recovered", result.Recovered),
		logger.Int("ignored", result.Ignored),
	)
	return result, firstErr
}

// processOne 处理单条归一化告警：firing 新建/更新，resolved 转 recovered，recovered 后 firing 重新打开当前 lifecycle，closed 后 firing 新建 lifecycle。
func (s *IngestService) processOne(ctx context.Context, src *domain.AlertSource, ingestCtx IngestContext, item NormalizedAlert, opts IngestOptions) (created, updated, recovered, ignored int, err error) {
	dedupKey := item.DedupKey
	if strings.TrimSpace(dedupKey) == "" {
		dedupKey = domain.ComputeDedupKey(src.ID, firstNonEmpty(item.Fingerprint, item.ExternalID), item.RuleName, item.ResourceName, item.Labels)
	}
	now := time.Now()
	isResolved := strings.EqualFold(item.Status, "resolved")

	if opts.IgnoreTerminal {
		latest, findErr := s.alerts.FindLatestByDedupKey(ctx, src.ID, dedupKey)
		if findErr != nil && !errors.Is(findErr, domain.ErrNotFound) {
			return 0, 0, 0, 0, wrapAlertOpError(findErr, "find latest alert failed")
		}
		if findErr == nil && latest != nil &&
			(latest.Status == domain.StatusRecovered || latest.Status == domain.StatusClosed) {
			// 拉取式接入：同一 record 已恢复/关闭后，后续观测只可能是旧快照，不得重开。
			ignored = 1
			return
		}
	}

	active, findErr := s.alerts.FindActiveByDedupKey(ctx, src.ID, dedupKey)
	if findErr != nil && !errors.Is(findErr, domain.ErrNotFound) {
		return 0, 0, 0, 0, wrapAlertOpError(findErr, "find active alert failed")
	}

	if errors.Is(findErr, domain.ErrNotFound) {
		if isResolved {
			ignored = 1
			return
		}
		maxSeq, maxErr := s.alerts.MaxLifecycleSeq(ctx, dedupKey)
		if maxErr != nil {
			return 0, 0, 0, 0, wrapAlertOpError(maxErr, "load alert lifecycle sequence failed")
		}
		eventMessage := "告警首次触发"
		if maxSeq > 0 {
			eventMessage = "告警再次触发（新生命周期）"
		}
		return s.createActiveAlert(ctx, src, ingestCtx, item, dedupKey, maxSeq+1, now, eventMessage, opts)
	}

	return s.processExistingActive(ctx, src, ingestCtx, item, active, now, isResolved, opts)
}

func (s *IngestService) createActiveAlert(ctx context.Context, src *domain.AlertSource, ingestCtx IngestContext, item NormalizedAlert, dedupKey string, lifecycleSeq int, now time.Time, eventMessage string, opts IngestOptions) (created, updated, recovered, ignored int, err error) {
	alert := s.newAlertFromNormalized(src, item, dedupKey, lifecycleSeq, now)
	s.applyAssetLinks(ctx, alert, item)
	if err := s.alerts.Create(ctx, alert); err != nil {
		if errors.Is(err, domain.ErrAlreadyExists) {
			active, findErr := s.alerts.FindActiveByDedupKey(ctx, src.ID, dedupKey)
			if findErr != nil {
				return 0, 0, 0, 0, wrapAlertOpError(findErr, "find active alert after create conflict failed")
			}
			return s.processExistingActive(ctx, src, ingestCtx, item, active, now, false, opts)
		}
		return 0, 0, 0, 0, wrapAlertOpError(err, "create alert failed")
	}
	if err := s.recordIntegrationEvent(ctx, alert.ID, src, ingestCtx, domain.EventTriggered, eventMessage, item); err != nil {
		return 0, 0, 0, 0, err
	}
	s.recordIngestAudit(ctx, alert.ID, src.ID, "created", map[string]any{
		"alert_name": alert.Name, "application_id": alert.ApplicationID,
		"resource_id": alert.ResourceID, "source_id": src.ID,
	})
	created = 1
	return
}

func (s *IngestService) processExistingActive(ctx context.Context, src *domain.AlertSource, ingestCtx IngestContext, item NormalizedAlert, active *domain.Alert, now time.Time, isResolved bool, opts IngestOptions) (created, updated, recovered, ignored int, err error) {
	if opts.DedupeByCloudTrigger && !isResolved && item.CloudTriggerAt != nil {
		if prev, ok := parseCloudTrigger(active.Annotations); ok && item.CloudTriggerAt.Unix() <= prev {
			// 同一 record 的重复或旧 firing 观测，不产生状态/时间线变更（CES 契约 §7.1）。
			ignored = 1
			return
		}
	}
	if isResolved {
		if active.Status == domain.StatusRecovered {
			ignored = 1
			return
		}
		next, ok := domain.TransitionStatus(active.Status, domain.ActionExternalRecover)
		if !ok {
			return 0, 0, 0, 0, apperr.Newf(apperr.CodeInternal, "cannot external recover alert in status %s", active.Status)
		}
		recoveredAt := now
		if item.RecoveredAt != nil {
			recoveredAt = *item.RecoveredAt
		}
		active.Status = next
		active.RecoveredAt = &recoveredAt
		active.LastSeenAt = now
		active.SilencedUntil = nil // 外部恢复结束静默，无需先 unsilence
		applyCloudMetadata(active, item)
		if err := s.alerts.Update(ctx, active); err != nil {
			return 0, 0, 0, 0, wrapAlertOpError(err, "recover alert failed")
		}
		if err := s.recordIntegrationEvent(ctx, active.ID, src, ingestCtx, domain.EventRecovered, "外部系统恢复", item); err != nil {
			return 0, 0, 0, 0, err
		}
		s.recordIngestAudit(ctx, active.ID, src.ID, "recovered", map[string]any{
			"alert_name": active.Name, "source_id": src.ID,
		})
		recovered = 1
		return
	}

	reopened := active.Status == domain.StatusRecovered
	if reopened {
		active.Status = domain.StatusNew
		active.RecoveredAt = nil
		active.AcknowledgedAt = nil
		active.OwnerUserID = ""
		active.AssigneeUserID = ""
	}
	active.OccurrenceCount++
	active.LastSeenAt = now
	active.Labels = cloneLabels(item.Labels)
	active.Annotations = cloneLabels(item.Annotations)
	applyCloudMetadata(active, item)
	active.Severity = item.Severity
	active.Summary = item.Summary
	active.Description = item.Description
	s.applyAssetLinks(ctx, active, item)
	if err := s.alerts.Update(ctx, active); err != nil {
		return 0, 0, 0, 0, wrapAlertOpError(err, "update alert failed")
	}
	eventType := domain.EventUpdated
	eventMessage := "告警重复触发或更新"
	if reopened {
		eventType = domain.EventTriggered
		eventMessage = "告警恢复后再次触发"
	}
	if err := s.recordIntegrationEvent(ctx, active.ID, src, ingestCtx, eventType, eventMessage, item); err != nil {
		return 0, 0, 0, 0, err
	}
	s.recordIngestAudit(ctx, active.ID, src.ID, "updated", map[string]any{
		"alert_name": active.Name, "application_id": active.ApplicationID,
		"resource_id": active.ResourceID, "source_id": src.ID, "reopened": reopened,
	})
	updated = 1
	return
}

func (s *IngestService) newAlertFromNormalized(src *domain.AlertSource, item NormalizedAlert, dedupKey string, lifecycleSeq int, now time.Time) *domain.Alert {
	firstSeen := item.FirstSeenAt
	if firstSeen.IsZero() {
		firstSeen = now
	}
	labels := cloneLabels(item.Labels)
	annotations := cloneLabels(item.Annotations)
	alert := &domain.Alert{
		ID:              newAlertID(),
		ExternalID:      item.ExternalID,
		Source:          string(src.Type),
		SourceID:        src.ID,
		SourceName:      src.Name,
		Fingerprint:     item.Fingerprint,
		DedupKey:        dedupKey,
		LifecycleSeq:    lifecycleSeq,
		Name:            item.Name,
		Summary:         item.Summary,
		Description:     item.Description,
		Severity:        item.Severity,
		Status:          domain.StatusNew,
		RuleName:        item.RuleName,
		BusinessLine:    item.BusinessLine,
		Environment:     item.Environment,
		ApplicationName: item.ApplicationName,
		ResourceType:    item.ResourceType,
		ResourceName:    item.ResourceName,
		Labels:          labels,
		Annotations:     annotations,
		OccurrenceCount: 1,
		FirstSeenAt:     firstSeen,
		LastSeenAt:      now,
	}
	applyCloudMetadata(alert, item)
	return alert
}

// applyCloudMetadata 将 CES 原始状态与云端触发时间写入 annotations，
// 供去重判断和后续 API 展示；不覆盖接入源提供的业务 annotation。
func applyCloudMetadata(alert *domain.Alert, item NormalizedAlert) {
	if alert == nil || item.SourceStatus == "" {
		return
	}
	if alert.Annotations == nil {
		alert.Annotations = map[string]string{}
	}
	alert.Annotations["ces_status"] = item.SourceStatus
	if item.CloudTriggerAt != nil {
		next := item.CloudTriggerAt.Unix()
		if prev, ok := parseCloudTrigger(alert.Annotations); !ok || next > prev {
			alert.Annotations["ces_last_alarm_time"] = strconv.FormatInt(next, 10)
		}
	}
}

func parseCloudTrigger(annotations map[string]string) (int64, bool) {
	raw, ok := annotations["ces_last_alarm_time"]
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// applyAssetLinks 按 §9.1 尝试匹配 Asset 注册表，写入 application_id / resource_id；失败仍保留告警。
func (s *IngestService) applyAssetLinks(ctx context.Context, alert *domain.Alert, item NormalizedAlert) {
	if s == nil || s.assets == nil || alert == nil {
		return
	}
	r, err := s.assets.Match(ctx, AssetMatchInput{
		SourceType:      alert.Source,
		ApplicationName: alert.ApplicationName,
		ResourceName:    alert.ResourceName,
		ResourceType:    alert.ResourceType,
		Environment:     alert.Environment,
		Labels:          alert.Labels,
	})
	if err != nil {
		logger.From(ctx).Warn("asset match failed",
			logger.String("alert_name", alert.Name),
			logger.Error(err),
		)
		return
	}
	if r.ApplicationID != "" {
		alert.ApplicationID = r.ApplicationID
	}
	if r.ResourceID != "" {
		alert.ResourceID = r.ResourceID
	}
}

// recordIntegrationEvent 写入接入时间线，payload 含 §3.2 要求的 ip/user_agent/trace_id 及 request_id。
func (s *IngestService) recordIntegrationEvent(ctx context.Context, alertID string, src *domain.AlertSource, ingestCtx IngestContext, eventType domain.AlertEventType, message string, item NormalizedAlert) error {
	if s == nil || s.events == nil {
		return nil
	}
	payload := map[string]any{
		"source_id":   src.ID,
		"request_id":  strings.TrimSpace(ingestCtx.RequestID),
		"trace_id":    httpx.TraceIDFromCtx(ctx),
		"ip":          ingestCtx.IP,
		"user_agent":  ingestCtx.UserAgent,
		"external_id": item.ExternalID,
		"status":      item.Status,
	}
	if item.SourceStatus != "" {
		payload["ces_status"] = item.SourceStatus
	}
	if item.CloudTriggerAt != nil {
		payload["cloud_trigger_at"] = item.CloudTriggerAt.Unix()
	}
	ev := &domain.AlertEvent{
		ID:        newEventID(),
		AlertID:   alertID,
		EventType: eventType,
		ActorType: domain.ActorIntegration,
		ActorID:   src.ID,
		ActorName: src.Name,
		Message:   message,
		Payload:   payload,
	}
	if err := s.events.Create(ctx, ev); err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "record ingest event failed")
	}
	return nil
}

func (s *IngestService) recordIngestAudit(ctx context.Context, alertID, sourceID, result string, payload map[string]any) {
	if s == nil || s.audit == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["result"] = result
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "alert",
		ResourceID:   alertID,
		Action:       AuditIngest,
		UserID:       "source:" + sourceID,
		Payload:      payload,
	})
}

func cloneLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
