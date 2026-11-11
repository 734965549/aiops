package application

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
)

const (
	pullLeaseTTL         = 5 * time.Minute
	firstPullLookback    = 24 * time.Hour
	pullOverlap          = 30 * time.Minute
	failedRetryLookback  = 2 * time.Hour
	syncStatusSuccess    = "success"
	syncStatusPartial    = "partial"
	syncStatusFailed     = "failed"
	auditActionSync      = AuditAction("alert_source_sync")
	systemActorUserID    = "system"
	handlerSourceNameFmt = "source:%s"
)

// AlertSyncService 编排 CES 拉取：租约 -> 拉取 -> 复用 ingest 管道落库 -> 统计/审计/状态。
// 见 ops/huawei-ces-alert-sync-contract.md §3、§7、§8、§9、§11。
type AlertSyncService struct {
	sources domain.AlertSourceRepository
	pull    AlertPullPort
	ingest  *IngestService
	leases  AlertSyncLeaseStore
	audit   AuditRecorder
	ownerID string
	now     func() time.Time
	// inflight 进程内单飞，保证同一实例同一源不会并发进入拉取；跨实例由数据库租约兜底。
	inflight sync.Map
}

// NewAlertSyncService 构造拉取编排服务。ownerID 标识当前 API 实例，用于数据库租约归属。
func NewAlertSyncService(
	sources domain.AlertSourceRepository,
	pull AlertPullPort,
	ingest *IngestService,
	leases AlertSyncLeaseStore,
	audit AuditRecorder,
	ownerID string,
) *AlertSyncService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	if strings.TrimSpace(ownerID) == "" {
		ownerID = fmt.Sprintf("api-%d", time.Now().UnixNano())
	}
	return &AlertSyncService{
		sources: sources, pull: pull, ingest: ingest, leases: leases, audit: audit,
		ownerID: ownerID, now: time.Now,
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
	acquired, err := s.leases.Acquire(ctx, sourceID, s.ownerID, pullLeaseTTL)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeUnavailable, "acquire alert sync lease failed")
	}
	if !acquired {
		return nil, apperr.New(apperr.CodeAborted, "alert source sync is already running on another instance")
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.leases.Release(releaseCtx, sourceID, s.ownerID); err != nil {
			logger.L().Warn("release alert sync lease failed",
				logger.String("source_id", sourceID), logger.Error(err))
		}
	}()

	started := s.now()
	userID := strings.TrimSpace(actor.UserID)
	if userID == "" {
		userID = systemActorUserID
	}
	regionsFilter := src.PullConfig.EffectiveRegionFilter()
	pullReq := AlertPullRequest{
		SourceID:  src.ID,
		AccountID: src.AccountID,
		Regions:   regionsFilter,
		Since:     s.since(src),
	}
	pullResult, pullErr := s.pull.PullAlarms(ctx, pullReq)
	if pullErr != nil {
		result := s.failedResult(src, started, pullErr)
		s.commitAndAudit(ctx, src, userID, result, started, pullErr)
		return result, nil
	}
	if pullResult == nil {
		pullResult = &AlertPullResult{}
	}
	renewed, err := s.leases.Renew(ctx, sourceID, s.ownerID, pullLeaseTTL)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeUnavailable, "renew alert sync lease failed")
	}
	if !renewed {
		return nil, apperr.New(apperr.CodeAborted, "alert sync lease expired before ingest")
	}

	items, preIgnored := s.normalizeAlarms(src, pullResult.Instances)
	result := &AlertSyncResultDTO{
		SourceID:  src.ID,
		AccountID: src.AccountID,
		Regions:   append([]string(nil), pullResult.Regions...),
		Accepted:  len(items) + preIgnored,
		Ignored:   preIgnored,
	}
	if len(items) > 0 {
		ingested, ingestErr := s.ingest.IngestNormalized(ctx, src, IngestContext{SourceID: src.ID}, items, IngestOptions{IgnoreTerminal: true})
		if ingestErr != nil {
			status := syncStatusFailed
			result.Status = status
			result.FailedScopes = append(scopeFailureStrings(pullResult.FailedScopes), "ingest:"+string(apperr.CodeOf(ingestErr)))
			result.DurationMS = s.now().Sub(started).Milliseconds()
			result.LastSyncAt = s.now().Unix()
			s.commitAndAudit(ctx, src, userID, result, started, ingestErr)
			return nil, ingestErr
		}
		result.Created = ingested.Created
		result.Updated = ingested.Updated
		result.Recovered = ingested.Recovered
		result.Ignored += ingested.Ignored
	}
	result.FailedScopes = scopeFailureStrings(pullResult.FailedScopes)
	result.Status = scopeStatus(result.Regions, pullResult.FailedScopes)
	result.DurationMS = s.now().Sub(started).Milliseconds()
	result.LastSyncAt = s.now().Unix()
	s.commitAndAudit(ctx, src, userID, result, started, nil)
	return result, nil
}

func (s *AlertSyncService) since(src *domain.AlertSource) *time.Time {
	now := s.now()
	if src.LastSyncAt == nil {
		t := now.Add(-firstPullLookback)
		return &t
	}
	lookback := pullOverlap
	if src.LastSyncStatus == syncStatusFailed {
		lookback = failedRetryLookback
	}
	t := src.LastSyncAt.Add(-lookback)
	if t.After(now) {
		t = now
	}
	return &t
}

func (s *AlertSyncService) normalizeAlarms(src *domain.AlertSource, instances []PulledAlarm) ([]NormalizedAlert, int) {
	items := make([]NormalizedAlert, 0, len(instances))
	ignored := 0
	now := s.now()
	for _, in := range instances {
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
		severity, ok := src.PullConfig.SeverityFor(in.SeverityName)
		if !ok {
			severity = domain.SeverityInfo
			logger.L().Warn("huawei ces alarm severity not mapped; fallback to info",
				logger.String("source_id", src.ID),
				logger.String("record_id", recordID),
				logger.String("severity", in.SeverityName),
			)
		}
		labels := cloneLabels(in.Labels)
		labels["account_id"] = src.AccountID
		labels["region"] = in.Region
		labels["project_id"] = in.ProjectID
		labels["record_id"] = recordID
		if in.AlarmID != "" {
			labels["alarm_id"] = in.AlarmID
		}
		firstSeen := now
		if in.FirstSeenAt != nil {
			firstSeen = *in.FirstSeenAt
		}
		item := NormalizedAlert{
			DedupKey:     domain.ComputeCESDedupKey(src.ID, src.AccountID, in.Region, in.ProjectID, recordID),
			ExternalID:   recordID,
			Fingerprint:  recordID,
			Status:       status,
			Name:         name,
			Summary:      in.Summary,
			Description:  in.Description,
			Severity:     severity,
			RuleName:     name,
			BusinessLine: src.BusinessLine,
			Environment:  src.Environment,
			ResourceType: in.ResourceType,
			ResourceName: in.ResourceName,
			Labels:       labels,
			Annotations:  map[string]string{},
			FirstSeenAt:  firstSeen,
		}
		if in.RecoveredAt != nil {
			item.RecoveredAt = in.RecoveredAt
		}
		items = append(items, item)
	}
	return items, ignored
}

func (s *AlertSyncService) failedResult(src *domain.AlertSource, started time.Time, err error) *AlertSyncResultDTO {
	result := &AlertSyncResultDTO{
		SourceID:     src.ID,
		AccountID:    src.AccountID,
		Regions:      []string{},
		Status:       syncStatusFailed,
		FailedScopes: []string{"account:" + string(apperr.CodeOf(err))},
		DurationMS:   s.now().Sub(started).Milliseconds(),
		LastSyncAt:   s.now().Unix(),
	}
	return result
}

func (s *AlertSyncService) commitAndAudit(ctx context.Context, src *domain.AlertSource, userID string, result *AlertSyncResultDTO, started time.Time, runErr error) {
	if result == nil {
		return
	}
	committed, err := s.leases.CommitSyncState(ctx, src.ID, s.ownerID, time.Unix(result.LastSyncAt, 0), result.Status)
	if err != nil {
		logger.L().Warn("commit alert sync state failed",
			logger.String("source_id", src.ID), logger.Error(err))
	} else if !committed {
		logger.L().Warn("discard expired alert sync result (lease lost)",
			logger.String("source_id", src.ID), logger.String("owner_id", s.ownerID))
	}
	payload := map[string]any{
		"source_id":       src.ID,
		"account_id":      src.AccountID,
		"regions":         result.Regions,
		"status":          result.Status,
		"accepted":        result.Accepted,
		"created":         result.Created,
		"updated":         result.Updated,
		"recovered":       result.Recovered,
		"ignored":         result.Ignored,
		"failed_scopes":   result.FailedScopes,
		"duration_ms":     result.DurationMS,
		"lease_owner":     s.ownerID,
		"lease_committed": committed,
	}
	if committed {
		t := time.Unix(result.LastSyncAt, 0).UTC()
		src.LastSyncAt = &t
		src.LastSyncStatus = result.Status
	}
	if runErr != nil {
		payload["error_code"] = string(apperr.CodeOf(runErr))
	}
	if src.PullConfig.DeliveryMode == "" {
		payload["delivery_mode"] = string(domain.DeliveryModePull)
	} else {
		payload["delivery_mode"] = string(src.PullConfig.DeliveryMode)
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "alert_source",
		ResourceID:   src.ID,
		Action:       auditActionSync,
		UserID:       userID,
		Payload:      payload,
	})
	logger.From(ctx).Info("alert source sync completed",
		logger.String("source_id", src.ID),
		logger.String("status", result.Status),
		logger.Any("regions", result.Regions),
		logger.Int("created", result.Created),
		logger.Int("updated", result.Updated),
		logger.Int("recovered", result.Recovered),
		logger.Int("ignored", result.Ignored),
		logger.Int64("duration_ms", result.DurationMS),
	)
}

func scopeStatus(regions []string, failures []AlertPullScopeFailure) string {
	if len(failures) == 0 {
		return syncStatusSuccess
	}
	failed := map[string]struct{}{}
	for _, f := range failures {
		r := strings.ToLower(strings.TrimSpace(f.Region))
		if r == "" {
			r = "account"
		}
		failed[r] = struct{}{}
	}
	successRegions := 0
	seen := map[string]struct{}{}
	for _, r := range regions {
		key := strings.ToLower(strings.TrimSpace(r))
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		if _, bad := failed[key]; !bad {
			successRegions++
		}
	}
	if successRegions == 0 {
		return syncStatusFailed
	}
	return syncStatusPartial
}

func scopeFailureStrings(failures []AlertPullScopeFailure) []string {
	out := make([]string, 0, len(failures))
	for _, f := range failures {
		scope := strings.TrimSpace(f.Region)
		if pid := strings.TrimSpace(f.ProjectID); pid != "" {
			scope = scope + "/" + pid
		}
		if scope == "" {
			scope = "account"
		}
		code := strings.TrimSpace(f.Code)
		if code == "" {
			code = "FAILED"
		}
		out = append(out, scope+":"+code)
	}
	return out
}
