package application

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	httpx "github.com/734965549/aiops/pkg/transport/http"
)

const (
	pullLeaseTTL        = 5 * time.Minute
	pullRoundBudget     = 20 * time.Second
	pullRoundFinalize   = 5 * time.Second
	syncStatusSuccess   = "success"
	syncStatusPartial   = "partial"
	syncStatusFailed    = "failed"
	auditActionSync     = AuditAction("alert_source_sync")
	systemActorUserID   = "system"
	failureRecordWrite  = "RECORD_WRITE_FAILED"
	failureRoundTimeout = "ROUND_TIMEOUT"
)

// PullAccountRuntimeValidator 校验源绑定账号在运行前仍满足准入条件。
type PullAccountRuntimeValidator interface {
	ValidatePullAccountRuntime(ctx context.Context, accountID, userID string) (*IntegrationAccountInfo, error)
}

// AlertSyncService 编排 CES 拉取：账号校验 -> 共享租约 -> 按 scope 拉取 -> 复用 ingest 管道 -> 游标/状态/审计。
// 见 ops/huawei-ces-alert-sync-contract.md §7、§8、§9、§10、§11。
type AlertSyncService struct {
	sources   domain.AlertSourceRepository
	pull      AlertPullPort
	ingest    *IngestService
	leases    AlertSyncLeaseStore
	validator PullAccountRuntimeValidator
	audit     AuditRecorder
	ownerID   string
	now       func() time.Time
	inflight  sync.Map
}

// NewAlertSyncService 构造拉取编排服务。ownerID 标识当前 API 实例，用于数据库租约归属。
func NewAlertSyncService(
	sources domain.AlertSourceRepository,
	pull AlertPullPort,
	ingest *IngestService,
	leases AlertSyncLeaseStore,
	validator PullAccountRuntimeValidator,
	audit AuditRecorder,
	ownerID string,
) *AlertSyncService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	if strings.TrimSpace(ownerID) == "" {
		ownerID = "api-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	return &AlertSyncService{
		sources: sources, pull: pull, ingest: ingest, leases: leases,
		validator: validator, audit: audit, ownerID: ownerID, now: time.Now,
	}
}

func (s *AlertSyncService) Sync(ctx context.Context, sourceID string, actor Actor) (*AlertSyncResultDTO, error) {
	sourceID = strings.TrimSpace(sourceID)
	if s == nil || s.sources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert sync service is not enabled")
	}
	if s.pull == nil || s.ingest == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert pull port is not configured")
	}
	if s.leases == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert sync lease store is not configured")
	}
	lockKey := "sync:" + sourceID
	if _, loaded := s.inflight.LoadOrStore(lockKey, struct{}{}); loaded {
		return nil, apperr.New(apperr.CodeAborted, "alert source sync is already running")
	}
	defer s.inflight.Delete(lockKey)

	src, err := s.sources.GetByID(ctx, sourceID)
	if err != nil {
		return nil, wrapAlertOpError(err, "load alert source failed")
	}
	if !src.Enabled {
		return nil, apperr.New(apperr.CodeInvalidArgument, "alert source is disabled; enable it before syncing")
	}
	if !src.IsPullSource() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "alert source is not a pull source")
	}
	userID := strings.TrimSpace(actor.UserID)
	if userID == "" {
		userID = systemActorUserID
	}
	plannedRegions := src.PullConfig.EffectiveRegionFilter()
	if s.validator != nil {
		info, err := s.validator.ValidatePullAccountRuntime(ctx, src.AccountID, strings.TrimSpace(actor.UserID))
		if err != nil {
			return nil, err
		}
		if len(plannedRegions) == 0 && info != nil {
			for _, r := range info.Regions {
				if v := strings.ToLower(strings.TrimSpace(r)); v != "" {
					plannedRegions = append(plannedRegions, v)
				}
			}
		}
	}
	acquired, err := s.leases.Acquire(ctx, sourceID, s.ownerID, pullLeaseTTL)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeUnavailable, "acquire alert sync lease failed")
	}
	if !acquired {
		return nil, apperr.New(apperr.CodeAborted, "alert source sync is already running on another instance")
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), pullRoundFinalize)
		defer cancel()
		if err := s.leases.Release(releaseCtx, sourceID, s.ownerID); err != nil {
			logger.L().Warn("release alert sync lease failed",
				logger.String("source_id", sourceID), logger.Error(err))
		}
	}()

	started := s.now()
	roundCutoff := started.UTC().Unix()
	deadline := started.Add(pullRoundBudget)
	cursors := cursorUnix(src.SyncState)
	pullResult, pullErr := s.pull.PullAlarms(ctx, AlertPullRequest{
		SourceID:    src.ID,
		AccountID:   src.AccountID,
		Regions:     plannedRegions,
		Cursors:     cursors,
		RoundCutoff: roundCutoff,
		Deadline:    deadline,
	})
	result := &AlertSyncResultDTO{
		SourceID:     src.ID,
		AccountID:    src.AccountID,
		Regions:      append([]string(nil), plannedRegions...),
		FailedScopes: []ScopeFailureDTO{},
		LastSyncAt:   roundCutoff,
	}
	if pullErr != nil {
		result.Status = syncStatusFailed
		result.FailedScopes = append(result.FailedScopes, ScopeFailureDTO{Reason: string(apperr.CodeOf(pullErr))})
		result.DurationMS = s.now().Sub(started).Milliseconds()
		if err := s.commit(ctx, src, result, cloneScopeState(src.SyncState)); err != nil {
			return nil, err
		}
		s.recordRoundAudit(ctx, src, userID, result)
		return result, nil
	}
	if pullResult == nil {
		pullResult = &AlertPullResult{}
	}
	if len(pullResult.Regions) > 0 {
		result.Regions = append([]string(nil), pullResult.Regions...)
	}
	state := cloneScopeState(src.SyncState)
	completedScopes := 0
	submitted := 0
	for _, failure := range pullResult.Failures {
		result.FailedScopes = append(result.FailedScopes, ScopeFailureDTO{
			Region: failure.Region, ProjectID: failure.ProjectID, Reason: failure.Reason,
		})
	}
	for _, scope := range pullResult.Scopes {
		scopeKey := domain.ScopeKey(scope.Region, scope.ProjectID)
		items, preIgnored := s.normalizeAlarms(src, scope)
		result.Accepted += len(items) + preIgnored
		result.Ignored += preIgnored
		scopeFailed := false
		if strings.TrimSpace(scope.FailureCode) != "" || !scope.Complete {
			scopeFailed = true
		}
		if len(items) > 0 {
			renewed, err := s.leases.Renew(ctx, src.ID, s.ownerID, pullLeaseTTL)
			if err != nil {
				return nil, apperr.Wrap(err, apperr.CodeUnavailable, "renew alert sync lease failed")
			}
			if !renewed {
				return nil, apperr.New(apperr.CodeAborted, "alert sync lease expired before ingest")
			}
			ingested, ingestErr := s.ingest.IngestNormalized(ctx, src, IngestContext{SourceID: src.ID}, items, IngestOptions{
				IgnoreTerminal:       true,
				DedupeByCloudTrigger: true,
				ContinueOnError:      true,
			})
			scopeCreated, scopeUpdated, scopeRecovered := 0, 0, 0
			if ingested != nil {
				scopeCreated = ingested.Created
				scopeUpdated = ingested.Updated
				scopeRecovered = ingested.Recovered
				result.Created += ingested.Created
				result.Updated += ingested.Updated
				result.Recovered += ingested.Recovered
				result.Ignored += ingested.Ignored
				result.FailedRecords += ingested.Failed
			}
			if ingestErr != nil {
				scopeFailed = true
				if ingested == nil {
					result.FailedRecords += len(items)
				}
			}
			submitted += scopeCreated + scopeUpdated + scopeRecovered
		}
		if scopeFailed {
			reason := strings.TrimSpace(scope.FailureCode)
			if reason == "" {
				reason = failureRecordWrite
			}
			result.FailedScopes = append(result.FailedScopes, ScopeFailureDTO{
				Region: scope.Region, ProjectID: scope.ProjectID, Reason: reason,
			})
			continue
		}
		if scope.Complete && scope.WindowEnd > 0 {
			state[scopeKey] = domain.ScopeCursor{CursorAt: time.Unix(scope.WindowEnd, 0).UTC()}
			completedScopes++
		}
	}
	result.Accepted = result.Created + result.Updated + result.Recovered + result.Ignored + result.FailedRecords
	result.Status = syncStatusFor(result.Regions, len(pullResult.Scopes), completedScopes, len(result.FailedScopes), submitted, result.Accepted)
	result.DurationMS = s.now().Sub(started).Milliseconds()
	if err := s.commit(ctx, src, result, state); err != nil {
		return nil, err
	}
	s.recordRoundAudit(ctx, src, userID, result)
	return result, nil
}

// commit 在租约仍有效时原子写入 last_sync_*/sync_state；过期持有者返回 ABORTED。
func (s *AlertSyncService) commit(ctx context.Context, src *domain.AlertSource, result *AlertSyncResultDTO, state map[string]domain.ScopeCursor) error {
	// 浏览器断开时仍用有界收尾上下文记录本轮结果（CES 契约 §8）。
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pullRoundFinalize)
	defer cancel()
	committed, err := s.leases.CommitSyncState(commitCtx, src.ID, s.ownerID, time.Unix(result.LastSyncAt, 0).UTC(), result.Status, state)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeInternal, "persist alert sync result failed")
	}
	if !committed {
		return apperr.New(apperr.CodeAborted, "alert sync lease expired before commit")
	}
	at := time.Unix(result.LastSyncAt, 0).UTC()
	src.LastSyncAt = &at
	src.LastSyncStatus = result.Status
	src.SyncState = state
	return nil
}

func cloneScopeState(in map[string]domain.ScopeCursor) map[string]domain.ScopeCursor {
	out := make(map[string]domain.ScopeCursor, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *AlertSyncService) recordRoundAudit(ctx context.Context, src *domain.AlertSource, userID string, result *AlertSyncResultDTO) {
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pullRoundFinalize)
	defer cancel()
	payload := map[string]any{
		"trace_id":       httpx.TraceIDFromCtx(auditCtx),
		"source_id":      src.ID,
		"account_id":     src.AccountID,
		"delivery_mode":  string(src.DeliveryMode),
		"regions":        result.Regions,
		"status":         result.Status,
		"accepted":       result.Accepted,
		"created":        result.Created,
		"updated":        result.Updated,
		"recovered":      result.Recovered,
		"ignored":        result.Ignored,
		"failed_records": result.FailedRecords,
		"failed_scopes":  result.FailedScopes,
		"duration_ms":    result.DurationMS,
		"lease_owner":    s.ownerID,
	}
	_ = s.audit.Record(auditCtx, AuditRecord{
		ResourceType: "alert_source",
		ResourceID:   src.ID,
		Action:       auditActionSync,
		UserID:       userID,
		Payload:      payload,
	})
	logger.From(auditCtx).Info("alert source sync completed",
		logger.String("source_id", src.ID),
		logger.String("status", result.Status),
		logger.Any("regions", result.Regions),
		logger.Int("created", result.Created),
		logger.Int("updated", result.Updated),
		logger.Int("recovered", result.Recovered),
		logger.Int("ignored", result.Ignored),
		logger.Int("failed_records", result.FailedRecords),
		logger.Int64("duration_ms", result.DurationMS),
	)
}

func (s *AlertSyncService) normalizeAlarms(src *domain.AlertSource, scope PulledScope) ([]NormalizedAlert, int) {
	if len(scope.Records) == 0 {
		return nil, 0
	}
	items := make([]NormalizedAlert, 0, len(scope.Records))
	ignored := 0
	now := s.now()
	for _, in := range scope.Records {
		recordID := strings.TrimSpace(in.RecordID)
		name := strings.TrimSpace(in.Name)
		if recordID == "" || name == "" {
			ignored++
			continue
		}
		status := strings.ToLower(strings.TrimSpace(in.Status))
		switch status {
		case "alarm":
			status = "firing"
		case "ok", "ok_manual":
			status = "resolved"
		default:
			ignored++
			continue
		}
		severity, ok := src.PullConfig.SeverityFor(in.Level)
		if !ok {
			severity = domain.SeverityInfo
			logger.L().Warn("huawei ces alarm level not mapped; fallback to info",
				logger.String("source_id", src.ID),
				logger.String("record_id", recordID),
				logger.String("level", in.Level),
			)
		}
		labels := cloneLabels(in.Dimensions)
		labels["account_id"] = src.AccountID
		labels["region"] = scope.Region
		labels["project_id"] = scope.ProjectID
		labels["record_id"] = recordID
		if in.AlarmID != "" {
			labels["alarm_id"] = in.AlarmID
		}
		if in.Namespace != "" {
			labels["ces_namespace"] = in.Namespace
		}
		if in.ResourceID != "" {
			labels["cloud_resource_id"] = in.ResourceID
		}
		if in.ResourceType != "" {
			labels["resource_type"] = in.ResourceType
		}
		ruleName := strings.TrimSpace(in.RuleName)
		if ruleName == "" {
			ruleName = name
		}
		if ruleName != "" {
			labels["rule_name"] = ruleName
		}
		firstSeen := now
		if in.FirstSeenAt != nil {
			firstSeen = *in.FirstSeenAt
		}
		annotations := map[string]string{}
		if in.Namespace != "" {
			annotations["ces_namespace"] = in.Namespace
		}
		if in.Level != "" {
			annotations["ces_level"] = in.Level
		}
		if in.RuleDescription != "" {
			annotations["ces_rule_description"] = in.RuleDescription
		}
		if in.RuleType != "" {
			annotations["ces_rule_type"] = in.RuleType
		}
		if in.RuleLookup != "" {
			annotations["ces_rule_lookup"] = in.RuleLookup
		}
		conditionDesc := ""
		if in.Condition != nil {
			conditionDesc = in.Condition.Describe()
			if conditionDesc != "" {
				annotations["ces_condition"] = conditionDesc
			}
		}
		if in.MetricName != "" {
			annotations["ces_metric_name"] = in.MetricName
			labels["metric_name"] = in.MetricName
		}
		resourceName := firstNonEmpty(in.ResourceName, in.ResourceID)
		description := buildCESDescription(in.Namespace, in.MetricName, conditionDesc, resourceName)
		summary := resourceName
		item := NormalizedAlert{
			DedupKey:       domain.ComputeCESDedupKey(src.ID, src.AccountID, scope.Region, scope.ProjectID, recordID),
			ExternalID:     recordID,
			Fingerprint:    strings.Join([]string{src.AccountID, scope.Region, scope.ProjectID, recordID}, "|"),
			Status:         status,
			Name:           name,
			Summary:        summary,
			Description:    description,
			Severity:       severity,
			RuleName:       name,
			BusinessLine:   src.BusinessLine,
			Environment:    src.Environment,
			ResourceType:   in.ResourceType,
			ResourceName:   in.ResourceName,
			Labels:         labels,
			Annotations:    annotations,
			FirstSeenAt:    firstSeen,
			RecoveredAt:    in.RecoveredAt,
			CloudTriggerAt: in.LastAlarmAt,
			SourceStatus:   strings.ToLower(strings.TrimSpace(in.Status)),
		}
		items = append(items, item)
	}
	return items, ignored
}

// buildCESDescription 按既有契约拼装描述：<namespace> <metric> <condition> · <resource>。
// 指标或条件缺失时只拼接可用部分，避免富化失败时丢失资源名。
func buildCESDescription(namespace, metricName, conditionDesc, resourceName string) string {
	head := strings.TrimSpace(strings.TrimSpace(namespace) + " " + strings.TrimSpace(metricName))
	parts := make([]string, 0, 2)
	if head != "" {
		parts = append(parts, head)
	}
	if conditionDesc = strings.TrimSpace(conditionDesc); conditionDesc != "" {
		parts = append(parts, conditionDesc)
	}
	description := strings.Join(parts, " ")
	resourceName = strings.TrimSpace(resourceName)
	if resourceName == "" {
		return description
	}
	if description == "" {
		return resourceName
	}
	return description + " · " + resourceName
}

func cursorUnix(state map[string]domain.ScopeCursor) map[string]int64 {
	out := make(map[string]int64, len(state))
	for key, cursor := range state {
		if !cursor.CursorAt.IsZero() {
			out[key] = cursor.CursorAt.UTC().Unix()
		}
	}
	return out
}

func syncStatusFor(regions []string, scopeCount, completedScopes, failures, submitted, accepted int) string {
	if len(regions) == 0 && scopeCount == 0 {
		return syncStatusFailed
	}
	if failures == 0 && completedScopes == scopeCount && scopeCount > 0 {
		return syncStatusSuccess
	}
	if submitted > 0 || completedScopes > 0 {
		return syncStatusPartial
	}
	return syncStatusFailed
}

var _ = failureRoundTimeout
