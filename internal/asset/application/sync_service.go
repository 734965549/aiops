package application

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/google/uuid"
)

var defaultCloudResourceTypes = []string{"ecs", "cce", "rds", "elb"}

// syncBatchLeaseTTL 是单次续租窗口有效期。后台 goroutine 每 syncLeaseRenewInterval 续租一次，
// 把 lease_expires_at 推进到 now+TTL；只要心跳正常，批次不会被 reap。
// 进程崩溃后心跳停止，TTL 到期由下一次同步 ReapExpiredRunning 收尾，实现自愈。
// 不依赖 Redis，与 redis.required=false 部署姿态一致。
// 这些时长在测试中可被覆盖（如缩短心跳间隔/硬超时以加速用例），故用 var 而非 const。
var (
	syncBatchLeaseTTL        = 5 * time.Minute
	syncLeaseRenewInterval   = 60 * time.Second
	syncHardTimeout          = 30 * time.Minute
	syncTerminalCtxTimeout   = 10 * time.Second
	syncLeaseRenewCtxTimeout = 5 * time.Second
)

// SyncService 云资源同步到 Asset 注册表。
type SyncService struct {
	apps        domain.ApplicationRepository
	resources   domain.ResourceRepository
	batches     domain.SyncBatchRepository
	discovery   CloudDiscoveryPort
	accounts    IntegrationAccountPort
	audit       AuditRecorder
	shutdownCtx context.Context // 后台 goroutine 派生 runCtx 的父 context；默认 background，由 main.go 注入
	wg          sync.WaitGroup  // 跟踪在途同步 goroutine，供关闭时 Wait
}

func NewSyncService(
	apps domain.ApplicationRepository,
	resources domain.ResourceRepository,
	batches domain.SyncBatchRepository,
	discovery CloudDiscoveryPort,
	accounts IntegrationAccountPort,
	audit AuditRecorder,
) *SyncService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &SyncService{
		apps: apps, resources: resources, batches: batches,
		discovery: discovery, accounts: accounts, audit: audit,
		shutdownCtx: context.Background(),
	}
}

// SetLifecycle 注入进程级 context，后台同步 goroutine 的 runCtx 派生自它；
// 关闭时取消该 context 可让在途同步尽快进入 finalize 落终态。由 main.go 装配。
func (s *SyncService) SetLifecycle(ctx context.Context) {
	if ctx != nil {
		s.shutdownCtx = ctx
	}
}

// Wait 等待所有在途同步 goroutine 收尾（finalize 落终态）。
// 应在 HTTP server 关闭后调用，确保进程退出前不留卡 running 的批次。
func (s *SyncService) Wait() { s.wg.Wait() }

// WaitContext 等待所有在途同步 goroutine 收尾，ctx 取消时返回 false，避免关闭流程无限阻塞。
func (s *SyncService) WaitContext(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

type TriggerSyncInput struct {
	AccountID string
}

// SyncBatchSummaryDTO 是同步批次的机器可读全量摘要。
// 字段分组：
// - 展示：sync_mode、resource_group_name、resource_group_id、projects、regions
// - 对账：ces_total、discovered_count、enriched_count、unknown_namespace_count、invalid_resource_count
// - 门控：max_resources_reached、product_names_empty、query_failed_types、conversion_failed_types
// - 诊断：failed_scopes、enrichment_failed_types、partial_reason
//
// 供审计、排障、验收与详情页使用；主列表应读取轻量展示层，不要展开全量字段。
// 这里保留全量 summary，避免 message 再承担半结构化协议职责。
type SyncBatchSummaryDTO struct {
	SyncMode              string   `json:"sync_mode,omitempty"`
	ResourceGroupName     string   `json:"resource_group_name,omitempty"`
	ResourceGroupID       string   `json:"resource_group_id,omitempty"`
	Projects              []string `json:"projects,omitempty"`
	Regions               []string `json:"regions,omitempty"`
	CESTotal              int      `json:"ces_total,omitempty"`
	DiscoveredCount       int      `json:"discovered_count,omitempty"`
	FailedScopes          []string `json:"failed_scopes,omitempty"`
	EnrichedCount         int      `json:"enriched_count,omitempty"`
	EnrichmentFailedTypes []string `json:"enrichment_failed_types,omitempty"`
	UnknownNamespaceCount int      `json:"unknown_namespace_count,omitempty"`
	InvalidResourceCount  int      `json:"invalid_resource_count,omitempty"`
	MaxResourcesReached   bool     `json:"max_resources_reached,omitempty"`
	ProductNamesEmpty     bool     `json:"product_names_empty,omitempty"`
	PartialReason         string   `json:"partial_reason,omitempty"`
	QueryFailedTypes      []string `json:"query_failed_types,omitempty"`
	ConversionFailedTypes []string `json:"conversion_failed_types,omitempty"`
}

type SyncBatchDTO struct {
	BatchID              string               `json:"batch_id"`
	IntegrationAccountID string               `json:"integration_account_id"`
	Provider             string               `json:"provider"`
	Status               string               `json:"status"`
	CreatedCount         int                  `json:"created_count"`
	UpdatedCount         int                  `json:"updated_count"`
	StaleCount           int                  `json:"stale_count"`
	FailedCount          int                  `json:"failed_count"`
	Message              string               `json:"message,omitempty"`
	Summary              *SyncBatchSummaryDTO `json:"summary,omitempty"`
	ApplicationID        string               `json:"application_id,omitempty"`
	StartedAt            int64                `json:"started_at"`
	FinishedAt           int64                `json:"finished_at,omitempty"`
	CreatedAt            int64                `json:"created_at"`
	UpdatedAt            int64                `json:"updated_at"`
}

type ListSyncBatchesQuery struct {
	Page                 int
	PageSize             int
	IntegrationAccountID string
}

type discoveredScope struct {
	Region       string
	ResourceType string
}

// negativeStaleScope 表示一个需要反向 stale 标记的 region：标记该 account+region 下所有
// active 的 cloud_sync 资源（排除当前批次）为 stale，但跳过 ExceptTypes 中的类型，见 §13.1。
// 用于 CES/hybrid 权威 scope：从资源组移除的类型不在 ExceptTypes 中，会被标记 stale；
// ExceptTypes 收集不确定类型（查询失败/转换失败/持久化失败），保持 active 避免误标。
type negativeStaleScope struct {
	Region      string
	ExceptTypes []string
}

type staleScopeCollector struct {
	successfulTypes       map[string]struct{}
	queryFailedTypes      map[string]struct{}
	conversionFailedTypes map[string]struct{}
	persistFailedTypes    map[string]struct{}
	upsertedTypes         map[string]struct{}
}

func newStaleScopeCollector(successfulTypes, queryFailedTypes, conversionFailedTypes, persistFailedTypes, upsertedTypes []string) staleScopeCollector {
	return staleScopeCollector{
		successfulTypes:       lowerStringSet(successfulTypes),
		queryFailedTypes:      lowerStringSet(queryFailedTypes),
		conversionFailedTypes: lowerStringSet(conversionFailedTypes),
		persistFailedTypes:    lowerStringSet(persistFailedTypes),
		upsertedTypes:         lowerStringSet(upsertedTypes),
	}
}

func (c staleScopeCollector) scopeSnapshot() staleScopeSnapshot {
	return staleScopeSnapshot{
		SuccessfulTypes:       mapStringSetKeys(c.successfulTypes),
		QueryFailedTypes:      mapStringSetKeys(c.queryFailedTypes),
		ConversionFailedTypes: mapStringSetKeys(c.conversionFailedTypes),
		PersistFailedTypes:    mapStringSetKeys(c.persistFailedTypes),
		UpsertedTypes:         mapStringSetKeys(c.upsertedTypes),
	}
}

type staleScopeSnapshot struct {
	SuccessfulTypes       []string
	QueryFailedTypes      []string
	ConversionFailedTypes []string
	PersistFailedTypes    []string
	UpsertedTypes         []string
}

func (s staleScopeSnapshot) eligibleSuccessTypes() map[string]struct{} {
	out := map[string]struct{}{}
	for _, t := range s.SuccessfulTypes {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if sliceContainsString(s.ConversionFailedTypes, t) || sliceContainsString(s.PersistFailedTypes, t) {
			continue
		}
		out[t] = struct{}{}
	}
	if len(out) > 0 {
		return out
	}
	for _, t := range s.UpsertedTypes {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || sliceContainsString(s.PersistFailedTypes, t) {
			continue
		}
		out[t] = struct{}{}
	}
	return out
}

func (s staleScopeSnapshot) authoritativeExceptTypes() []string {
	return mergeLowerStrings(s.QueryFailedTypes, s.ConversionFailedTypes, s.PersistFailedTypes)
}

func (c *staleScopeCollector) collect(summary obsapp.CloudSyncSummary) {
	for _, t := range summary.SuccessfulTypes {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			c.successfulTypes[t] = struct{}{}
		}
	}
	for _, t := range summary.QueryFailedTypes {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			c.queryFailedTypes[t] = struct{}{}
		}
	}
	for _, t := range summary.ConversionFailedTypes {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			c.conversionFailedTypes[t] = struct{}{}
		}
	}
}

func (c staleScopeCollector) structuredLogFields() map[string]any {
	snap := c.scopeSnapshot()
	return map[string]any{
		"successful_types":        snap.SuccessfulTypes,
		"query_failed_types":      snap.QueryFailedTypes,
		"conversion_failed_types": snap.ConversionFailedTypes,
		"persist_failed_types":    snap.PersistFailedTypes,
		"upserted_types":          snap.UpsertedTypes,
	}
}

const (
	// syncBatchSignalProductNamesEmpty 表示这轮同步的资源组 product_names 为空，属于需要降级到 partial 的结构化信号。
	syncBatchSignalProductNamesEmpty = "product_names_empty"
	// syncBatchSignalMaxResourcesReached 表示 provider 因达到上限而截断，属于需要降级到 partial 的结构化信号。
	syncBatchSignalMaxResourcesReached = "max_resources_reached"
)

type syncBatchPartialState struct {
	LeaseLost   bool
	Signals     []string
	FailedCount int
	Errors      []string
}

func (s *syncBatchPartialState) NoteFailure() { s.FailedCount++ }

func (s *syncBatchPartialState) MarkSignal(signal string) {
	signal = strings.TrimSpace(signal)
	if signal == "" {
		return
	}
	for _, existing := range s.Signals {
		if existing == signal {
			return
		}
	}
	s.Signals = append(s.Signals, signal)
}

func (s *syncBatchPartialState) AddError(msg string) {
	if msg != "" {
		s.Errors = append(s.Errors, msg)
	}
}

type syncBatchFinalizationDecision struct {
	status  string
	message string
	summary *SyncBatchSummaryDTO
	persist func(context.Context) error
}

type syncBatchFinalizationSignals struct {
	leaseLost           bool
	partialLeaseLost    bool
	hasStaleScope       bool
	maxResourcesReached bool
	failedCount         int
	createdCount        int
	updatedCount        int
	failedScopesCount   int
	partialSignals      []string
	summaryIsPartial    bool
}

func buildSyncBatchFinalizationSignals(batch *domain.SyncBatch, summary *SyncBatchSummaryDTO, partialState syncBatchPartialState, leaseLost, hasStaleScope, maxResourcesReached bool) syncBatchFinalizationSignals {
	signals := syncBatchFinalizationSignals{
		leaseLost:           leaseLost,
		partialLeaseLost:    partialState.LeaseLost,
		hasStaleScope:       hasStaleScope,
		maxResourcesReached: maxResourcesReached,
		failedCount:         batch.FailedCount,
		createdCount:        batch.CreatedCount,
		updatedCount:        batch.UpdatedCount,
		partialSignals:      append([]string(nil), partialState.Signals...),
	}
	if summary != nil {
		signals.failedScopesCount = len(summary.FailedScopes)
		signals.summaryIsPartial = summary.ProductNamesEmpty || summary.MaxResourcesReached || len(summary.QueryFailedTypes) > 0 || len(summary.ConversionFailedTypes) > 0 || len(summary.EnrichmentFailedTypes) > 0 || len(summary.FailedScopes) > 0
	}
	return signals
}

func sliceContainsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

func isPartialFinalization(signals syncBatchFinalizationSignals) bool {
	return len(signals.partialSignals) > 0 || signals.failedScopesCount > 0 || signals.failedCount > 0 || signals.maxResourcesReached || signals.summaryIsPartial
}

func classifySyncBatchFinalizationSignals(signals syncBatchFinalizationSignals) (status string, message string) {
	if signals.leaseLost || signals.partialLeaseLost {
		return domain.SyncBatchStatusFailed, "sync cancelled or timed out"
	}
	if signals.failedCount > 0 && signals.createdCount+signals.updatedCount == 0 && !signals.hasStaleScope {
		return domain.SyncBatchStatusFailed, "sync failed"
	}
	if signals.createdCount == 0 && signals.updatedCount == 0 && !signals.hasStaleScope && signals.failedScopesCount == 0 && len(signals.partialSignals) == 0 && !signals.summaryIsPartial {
		return domain.SyncBatchStatusFailed, "sync produced no results"
	}
	if isPartialFinalization(signals) {
		return domain.SyncBatchStatusPartial, "sync completed with partial results"
	}
	return domain.SyncBatchStatusSuccess, "ok"
}

func buildSyncBatchFinalizationDecision(
	batches domain.SyncBatchRepository,
	batch *domain.SyncBatch,
	accountID string,
	summary *SyncBatchSummaryDTO,
	signals syncBatchFinalizationSignals,
	message string,
) syncBatchFinalizationDecision {
	status, fallbackMessage := classifySyncBatchFinalizationSignals(signals)
	message = truncateMessage(defaultIfEmpty(message, fallbackMessage))
	if signals.leaseLost || signals.partialLeaseLost {
		message = truncateMessage("sync cancelled or timed out")
	}
	return syncBatchFinalizationDecision{status: status, message: message, summary: summary, persist: func(termCtx context.Context) error {
		if status == domain.SyncBatchStatusSuccess {
			_, err := batches.FinalizeSuccess(termCtx, batch, accountID, batch.StartedAt)
			return err
		}
		return batches.Update(termCtx, batch)
	}}
}

func buildSyncBatchFinalization(
	batches domain.SyncBatchRepository,
	batch *domain.SyncBatch,
	accountID string,
	message string,
	syncSummaries []obsapp.CloudSyncSummary,
	partialState syncBatchPartialState,
	leaseLost bool,
	hasStaleScope bool,
	maxResourcesReached bool,
) syncBatchFinalizationDecision {
	summary := buildSyncBatchSummaryDTO(syncSummaries)
	signals := buildSyncBatchFinalizationSignals(batch, summary, partialState, leaseLost, hasStaleScope, maxResourcesReached)
	return buildSyncBatchFinalizationDecision(batches, batch, accountID, summary, signals, message)
}

func defaultIfEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func (s *SyncService) TriggerSync(ctx context.Context, actor Actor, in TriggerSyncInput) (*SyncBatchDTO, error) {
	if s == nil || s.batches == nil || s.resources == nil || s.apps == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "asset sync service is not enabled")
	}
	if s.discovery == nil {
		return nil, apperr.Wrap(domain.ErrDiscoveryUnavailable, apperr.CodeUnavailable, "cloud discovery port is not configured")
	}
	accountID := strings.TrimSpace(in.AccountID)
	if accountID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "account_id is required")
	}
	if s.accounts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "integration account port is not configured")
	}
	account, err := s.accounts.ResolveSyncAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	provider := strings.TrimSpace(account.Provider)
	regions := normalizeRegions(account.Regions)
	if len(regions) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "integration account has no regions configured")
	}

	batchID := "sync-" + uuid.NewString()
	now := time.Now().UTC()
	// 账号级互斥：先 reap 本账号租约过期的 running 批次（崩溃批次自愈），
	// 再插入带租约的 running 批次。若账号已有 running 批次，
	// 0028 部分唯一索引 (integration_account_id) WHERE status='running' 会触发唯一冲突，
	// 映射为 409，避免并发批次交错互相标记 stale。见 docs/huawei-ces-asset-sync-plan.md §P1。
	reaped, err := s.batches.ReapExpiredRunning(ctx, accountID, now)
	if err != nil {
		return nil, wrapAssetError(err, "reap expired sync batches failed")
	}
	if len(reaped) > 0 {
		_ = s.audit.Record(ctx, AuditRecord{
			ResourceType: "asset_sync_batch",
			ResourceID:   accountID,
			Action:       AuditAssetSync,
			UserID:       actor.UserID,
			Payload: map[string]any{
				"event":        "reap_expired_running",
				"account_id":   accountID,
				"reaped_count": len(reaped),
				"result":       "success",
			},
		})
	}
	leaseExpires := now.Add(syncBatchLeaseTTL)
	batch := &domain.SyncBatch{
		BatchID:              batchID,
		IntegrationAccountID: accountID,
		Provider:             provider,
		Status:               domain.SyncBatchStatusRunning,
		StartedAt:            now,
		FencingToken:         uuid.NewString(),
		LeaseExpiresAt:       &leaseExpires,
	}
	if err := s.batches.Create(ctx, batch); err != nil {
		// batch_id 为 UUID，此处冲突只能是账号 running 槽位被占用。
		if apperr.CodeOf(err) == apperr.CodeAlreadyExists {
			return nil, apperr.New(apperr.CodeAlreadyExists, "sync already in progress for this account")
		}
		return nil, wrapAssetError(err, "create sync batch failed")
	}

	appID, err := s.ensureCloudApplication(ctx, accountID, provider)
	if err != nil {
		// 前置阶段失败：批次刚创建即失败，用保留请求链路的短 context 落终态，避免请求 ctx 取消导致卡 running。
		detachedCtx := logger.WithContext(context.WithoutCancel(ctx), logger.From(ctx))
		s.finishBatchFailedDetached(detachedCtx, actor, batch, provider, regions, err)
		return nil, err
	}

	// 立即返回 running 批次，同步在后台 goroutine 执行；前端轮询 GetSyncBatch 到终态。
	// runCtx 派生自进程级 shutdownCtx（关闭时取消）+ 硬超时，与 HTTP 请求生命周期解耦。
	runCtx, runCancel := context.WithTimeout(s.shutdownCtx, syncHardTimeout)
	// 把请求 ctx 的 trace_id/user_id 等 logger 字段带入后台 goroutine，避免请求结束丢链路。
	runCtx = logger.WithContext(runCtx, logger.From(ctx))
	// 先构造返回 DTO，再启动后台 goroutine，避免返回路径与后台同步同时读写 batch。
	dto := toSyncBatchDTO(*batch)

	s.wg.Add(1)
	go s.runSync(runCtx, runCancel, actor, batch, appID, regions, provider)

	return &dto, nil
}

// runSync 后台执行同步主体：心跳续租 + discovery/upsert/stale + finalize 落终态。
// runCtx 取消（关闭/硬超时）时，finalize 用独立短 context 仍能落终态，保证不卡 running。
func (s *SyncService) runSync(
	runCtx context.Context,
	runCancel context.CancelFunc,
	actor Actor,
	batch *domain.SyncBatch,
	appID string,
	regions []string,
	provider string,
) {
	defer s.wg.Done()
	defer runCancel()

	detachedCtx := context.WithoutCancel(runCtx)

	// 心跳：周期续租，受 runCtx 硬超时控制，finalize 时停止。leaseDone 在心跳退出后关闭，
	// finalize 等待它以确保终态 Update 清空 lease 后不会再被心跳写回（避免竞态）。
	leaseCtx, leaseCancel := context.WithCancel(runCtx)
	defer leaseCancel()
	leaseDone := make(chan struct{})
	go s.leaseHeartbeat(leaseCtx, runCancel, batch.BatchID, batch.FencingToken, leaseDone)

	accountID := batch.IntegrationAccountID
	batchID := batch.BatchID
	fencingToken := batch.FencingToken
	now := batch.StartedAt
	obsActor := obsapp.Actor{UserID: actor.UserID, DisplayName: actor.DisplayName}

	var syncSummaries []obsapp.CloudSyncSummary
	var partialState syncBatchPartialState
	var fatalSyncErr error
	var leaseLost bool
	var negativeScopes []negativeStaleScope
	successScopes := make([]discoveredScope, 0, len(regions)*len(defaultCloudResourceTypes))
	collector := newStaleScopeCollector(nil, nil, nil, nil, nil)
	var summaryLines, partialErrs []string
	var maxResourcesReached, productNamesEmpty bool
	var fsErr error
	// 优先使用全量同步端口（CloudFullSyncPort）；provider 不支持时回退通用逐类型路径，
	// 不在 Asset 层硬编码 provider 判断，见 docs/huawei-ces-asset-sync-plan.md §7.2。
	successScopes, negativeScopes, summaryLines, partialErrs, maxResourcesReached, productNamesEmpty, syncSummaries, fsErr = s.syncCloudFullSync(runCtx, obsActor, provider, regions, appID, accountID, batchID, fencingToken, now, batch, &collector)
	partialState.Errors = append(partialState.Errors, partialErrs...)
	if productNamesEmpty {
		partialState.MarkSignal(syncBatchSignalProductNamesEmpty)
	}
	if errors.Is(fsErr, errFullSyncUnsupported) {
		var genericErr error
		successScopes, negativeScopes, summaryLines, partialErrs, maxResourcesReached, productNamesEmpty, syncSummaries, genericErr = s.syncGeneric(runCtx, obsActor, provider, regions, appID, accountID, batchID, fencingToken, now, batch, &collector)
		partialState.Errors = append(partialState.Errors, partialErrs...)
		if maxResourcesReached {
			partialState.MarkSignal(syncBatchSignalMaxResourcesReached)
		}
		if productNamesEmpty {
			partialState.MarkSignal(syncBatchSignalProductNamesEmpty)
		}
		if genericErr != nil {
			fatalSyncErr = genericErr
		}
		fsErr = genericErr
	}
	if fsErr != nil && errors.Is(fsErr, domain.ErrLeaseLost) {
		leaseLost = true
		runCancel()
	} else if fsErr != nil {
		fatalSyncErr = fsErr
	}
	if runCtx.Err() != nil {
		leaseLost = true
	}

	for _, scope := range successScopes {
		staleCount, err := s.resources.MarkStaleByAccountScopeExceptBatchWithLease(runCtx, accountID, scope.Region, scope.ResourceType, batchID, fencingToken)
		if err != nil {
			if errors.Is(err, domain.ErrLeaseLost) {
				partialState.LeaseLost = true
				runCancel()
				break
			}
			partialState.NoteFailure()
			partialState.AddError(fmt.Sprintf("%s/%s: mark stale failed", scope.Region, scope.ResourceType))
			continue
		}
		batch.StaleCount += int(staleCount)
	}
	// CES/hybrid 权威 scope 反向 stale：标记 account+region 下所有 active 资源为 stale，
	// 但跳过不确定类型（ExceptTypes），见 docs/huawei-ces-asset-sync-plan.md §13.1。
	for _, scope := range negativeScopes {
		staleCount, err := s.resources.MarkStaleByAccountRegionExceptTypesWithLease(runCtx, accountID, scope.Region, scope.ExceptTypes, batchID, fencingToken)
		if err != nil {
			if errors.Is(err, domain.ErrLeaseLost) {
				partialState.LeaseLost = true
				runCancel()
				break
			}
			partialState.NoteFailure()
			partialState.AddError(fmt.Sprintf("%s: mark stale failed", scope.Region))
			continue
		}
		batch.StaleCount += int(staleCount)
	}
	// finalize：闭包捕获 successScopes/maxResourcesReached/partialErrs/summaryLines/syncSummaries，
	// 用独立短 ctx 写终态 + 审计，不受 runCtx 取消影响。
	finalize := func() {
		leaseCancel()
		<-leaseDone
		termCtx, termCancel := context.WithTimeout(detachedCtx, syncTerminalCtxTimeout)
		defer termCancel()
		finished := time.Now().UTC()
		batch.FinishedAt = &finished
		messageParts := make([]string, 0, len(summaryLines)+len(partialState.Errors)+1)
		messageParts = append(messageParts, summaryLines...)
		summaryDTO := buildSyncBatchSummaryDTO(syncSummaries)
		if summaryDTO != nil && strings.TrimSpace(summaryDTO.PartialReason) != "" {
			messageParts = append(messageParts, summaryDTO.PartialReason)
		}
		messageParts = append(messageParts, partialState.Errors...)
		message := strings.Join(messageParts, "; ")
		if leaseLost {
			message = "sync cancelled or timed out"
		} else if fatalSyncErr != nil {
			message = fatalSyncErr.Error()
		}
		decision := buildSyncBatchFinalization(s.batches, batch, accountID, message, syncSummaries, partialState, leaseLost, len(successScopes) > 0 || len(negativeScopes) > 0, maxResourcesReached)
		logger.From(termCtx).Info("sync stale scope collector final state",
			logger.String("batch_id", batchID),
			logger.Any("scope_collector", collector.structuredLogFields()),
			logger.Any("finalization_signals", buildSyncBatchFinalizationSignals(batch, decision.summary, partialState, leaseLost, len(successScopes) > 0 || len(negativeScopes) > 0, maxResourcesReached)),
		)
		batch.Status = decision.status
		batch.Message = truncateMessage(decision.message)
		batch.Summary = marshalSyncBatchSummary(decision.summary)
		if err := decision.persist(termCtx); err != nil {
			if errors.Is(err, domain.ErrLeaseLost) {
				logger.From(runCtx).Warn("skip finalizing sync batch after lease lost", logger.String("batch_id", batchID))
				return
			}
			logger.From(runCtx).Error("finalize sync batch failed", logger.String("batch_id", batchID), logger.Error(err))
			return
		}
		if err := s.audit.Record(termCtx, AuditRecord{
			ResourceType: "asset_sync_batch",
			ResourceID:   batchID,
			Action:       AuditAssetSync,
			UserID:       actor.UserID,
			Payload:      buildAssetSyncAuditPayload(accountID, provider, regions, batch, syncSummaries),
		}); err != nil {
			logger.From(termCtx).Warn("record sync batch audit failed", logger.String("batch_id", batchID), logger.Error(err))
		}
	}
	finalize()
}

// leaseHeartbeat 周期续租 running 批次的 lease_expires_at，直到 ctx 取消或租约所有权丢失。
// 续租用独立短 ctx，不依赖 runCtx；DB 临时错误持续重试，确认租约丢失才取消主任务。
// 退出时关闭 done，供 finalize 等待心跳完全停止后再写终态，避免清空 lease 后被写回。
func (s *SyncService) leaseHeartbeat(ctx context.Context, runCancel context.CancelFunc, batchID, fencingToken string, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(syncLeaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewCtx, cancel := context.WithTimeout(ctx, syncLeaseRenewCtxTimeout)
			err := s.batches.RenewLease(renewCtx, batchID, fencingToken, time.Now().UTC(), syncBatchLeaseTTL)
			cancel()
			if err == nil {
				continue
			}
			if errors.Is(err, domain.ErrLeaseLost) {
				logger.From(ctx).Warn("asset sync lease lost", logger.String("batch_id", batchID))
				runCancel()
				return
			}
			logger.From(ctx).Warn("renew asset sync lease failed, will retry", logger.String("batch_id", batchID), logger.Error(err))
		}
	}
}

func (s *SyncService) ensureLeaseOwned(ctx context.Context, batchID, fencingToken string) error {
	if err := s.batches.CheckLeaseOwned(ctx, batchID, fencingToken, time.Now().UTC()); err != nil {
		if errors.Is(err, domain.ErrLeaseLost) {
			return err
		}
		return wrapAssetError(err, "check sync lease failed")
	}
	return nil
}

// finishBatchFailedDetached 用于前置阶段失败（如 ensureCloudApplication 失败）：
// 批次刚创建即需落 failed，用保留请求链路的短 ctx 避免请求 ctx 取消导致卡 running。
func (s *SyncService) finishBatchFailedDetached(ctx context.Context, actor Actor, batch *domain.SyncBatch, provider string, regions []string, cause error) {
	if batch == nil || s.batches == nil {
		return
	}
	termCtx, cancel := context.WithTimeout(ctx, syncTerminalCtxTimeout)
	defer cancel()
	finished := time.Now().UTC()
	batch.Status = domain.SyncBatchStatusFailed
	batch.FinishedAt = &finished
	batch.Message = truncateMessage(apperr.FromError(cause).Message)
	if err := s.batches.Update(termCtx, batch); err != nil {
		logger.From(termCtx).Error("finish failed sync batch failed", logger.String("batch_id", batch.BatchID), logger.Error(err))
		return
	}
	if err := s.audit.Record(termCtx, AuditRecord{
		ResourceType: "asset_sync_batch",
		ResourceID:   batch.BatchID,
		Action:       AuditAssetSync,
		UserID:       actor.UserID,
		Payload:      buildAssetSyncAuditPayload(batch.IntegrationAccountID, provider, regions, batch, nil),
	}); err != nil {
		logger.From(termCtx).Warn("record failed sync batch audit failed", logger.String("batch_id", batch.BatchID), logger.Error(err))
	}
}

func (s *SyncService) GetBatch(ctx context.Context, batchID string) (*SyncBatchDTO, error) {
	if s == nil || s.batches == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "asset sync service is not enabled")
	}
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "batch_id is required")
	}
	batch, err := s.batches.GetByID(ctx, batchID)
	if err != nil {
		return nil, wrapAssetError(err, "load sync batch failed")
	}
	dto := toSyncBatchDTO(*batch)
	return &dto, nil
}

func (s *SyncService) ListBatches(ctx context.Context, q ListSyncBatchesQuery) ([]SyncBatchDTO, int64, error) {
	if s == nil || s.batches == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "asset sync service is not enabled")
	}
	page, pageSize := normalizeAssetPage(q.Page, q.PageSize)
	rows, total, err := s.batches.List(ctx, domain.SyncBatchFilter{
		IntegrationAccountID: q.IntegrationAccountID,
		Limit:                pageSize,
		Offset:               (page - 1) * pageSize,
	})
	if err != nil {
		return nil, 0, wrapAssetError(err, "list sync batches failed")
	}
	out := make([]SyncBatchDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSyncBatchDTO(row))
	}
	return out, total, nil
}

func (s *SyncService) ensureCloudApplication(ctx context.Context, accountID, provider string) (string, error) {
	appID := cloudApplicationID(accountID)
	exists, err := s.apps.ExistsByID(ctx, appID)
	if err != nil {
		return "", wrapAssetError(err, "check cloud application failed")
	}
	if exists {
		return appID, nil
	}
	app := &domain.Application{
		ID:          appID,
		Name:        fmt.Sprintf("%s-cloud", provider),
		Environment: "cloud",
		Description: fmt.Sprintf("Auto-created cloud sync application for account %s", accountID),
	}
	if err := s.apps.Create(ctx, app); err != nil {
		if !isAlreadyExists(err) {
			return "", wrapAssetError(err, "create cloud application failed")
		}
	}
	return appID, nil
}

// errFullSyncUnsupported 表示 provider 未实现 CloudFullSyncPort，调用方应回退通用逐类型路径。
var errFullSyncUnsupported = errors.New("provider does not support full sync")

func isFullSyncUnsupported(err error) bool {
	return errors.Is(err, obsdomain.ErrCapabilityUnsupported)
}

// syncCloudFullSync 全量同步路径：通过 CloudFullSyncPort 按 region 全量发现，不受交互查询 limit<=500 限制，
// 见 docs/huawei-ces-asset-sync-plan.md §7.2/§8.1。对每个 region 调用全量同步端口，收集资源与摘要；
// 返回逐类型成功 scope（native/generic/fake 用）、反向 stale scope（CES/hybrid 权威 scope 用，见 §13.1）、
// 摘要行与错误行。若 provider 明确返回 ErrCapabilityUnsupported，说明不支持全量同步，
// 返回 errFullSyncUnsupported 供调用方回退 syncGeneric，此时尚未处理任何资源。
func (s *SyncService) syncCloudFullSync(
	ctx context.Context,
	obsActor obsapp.Actor,
	provider string,
	regions []string,
	appID, accountID, batchID, fencingToken string,
	now time.Time,
	batch *domain.SyncBatch,
	collector *staleScopeCollector,
) (successScopes []discoveredScope, negativeScopes []negativeStaleScope, summaryLines, partialErrs []string, maxResourcesReached, productNamesEmpty bool, syncSummaries []obsapp.CloudSyncSummary, fullSyncErr error) {
	for i, region := range regions {
		result, err := s.discovery.ListAllResources(ctx, obsActor, obsapp.AssetFullSyncQuery{
			AccountID: accountID, Provider: provider, Region: region,
		})
		if err != nil {
			// 只有明确的能力不支持 sentinel 才回退通用路径；CES 配置错误等 FAILED_PRECONDITION
			// 必须按当前 region 失败处理，避免误回退 native 查询。
			if i == 0 && isFullSyncUnsupported(err) {
				fullSyncErr = errFullSyncUnsupported
				return
			}
			batch.FailedCount++
			errMsg := apperr.FromError(err).Message
			partialErrs = append(partialErrs, fmt.Sprintf("%s: %s", region, errMsg))
			// 失败 region 也必须生成摘要，否则终态 summary 不覆盖该 region，违反契约。
			syncSummaries = append(syncSummaries, obsapp.CloudSyncSummary{
				Region:       region,
				FailedScopes: []string{errMsg},
			})
			continue
		}
		if result == nil {
			// provider 返回 nil result 且无错误属于契约违规，不能静默跳过，
			// 否则该 region 可能被当作成功而遗漏资源，最终得到 success/ok。
			batch.FailedCount++
			errMsg := "provider returned nil discovery result without error"
			partialErrs = append(partialErrs, fmt.Sprintf("%s: %s", region, errMsg))
			// 失败 region 也必须生成摘要，否则终态 summary 不覆盖该 region，违反契约。
			syncSummaries = append(syncSummaries, obsapp.CloudSyncSummary{
				Region:       region,
				FailedScopes: []string{errMsg},
			})
			continue
		}
		if collector == nil {
			collector = &staleScopeCollector{
				successfulTypes:       map[string]struct{}{},
				queryFailedTypes:      map[string]struct{}{},
				conversionFailedTypes: map[string]struct{}{},
				persistFailedTypes:    map[string]struct{}{},
				upsertedTypes:         map[string]struct{}{},
			}
		}
		upserted := 0
		for _, cloud := range result.Resources {
			created, upsertErr := s.upsertCloudResource(ctx, appID, accountID, batchID, fencingToken, now, cloud)
			if upsertErr != nil {
				if errors.Is(upsertErr, domain.ErrLeaseLost) {
					fullSyncErr = upsertErr
					return
				}
				batch.FailedCount++
				// 原始错误仅写日志，对外只保留脱敏摘要，避免表名/约束名等底层细节泄露，见 §13。
				logger.From(ctx).Warn("upsert cloud resource failed",
					logger.String("region", region), logger.String("cloud_resource_type", cloud.Type),
					logger.Error(upsertErr))
				partialErrs = append(partialErrs, fmt.Sprintf("%s: %s", region, apperr.FromError(upsertErr).Message))
				if t := strings.ToLower(strings.TrimSpace(cloud.Type)); t != "" {
					collector.persistFailedTypes[t] = struct{}{}
				}
				continue
			}
			upserted++
			if created {
				batch.CreatedCount++
			} else {
				batch.UpdatedCount++
			}
			if t := strings.ToLower(strings.TrimSpace(cloud.Type)); t != "" {
				collector.upsertedTypes[t] = struct{}{}
			}
		}
		// 摘要行，见 §8.1。
		summary := result.Summary
		syncSummaries = append(syncSummaries, summary)
		line := fmt.Sprintf("region=%s project=%s group=%s group_id=%s ces_total=%d discovered=%d upserted=%d failed_scopes=%d",
			region, summary.ProjectID, summary.ResourceGroupName, summary.ResourceGroupID, summary.CESTotal, summary.Discovered, upserted, len(summary.FailedScopes))
		if summary.ResourceGroupSelection != "" {
			line += fmt.Sprintf(" selected_resource_group=%s", summary.ResourceGroupSelection)
		}
		if summary.ProductNamesEmpty {
			line += " product_names_empty=true"
		}
		if summary.UnknownNamespaceCount > 0 {
			line += fmt.Sprintf(" unknown_namespace=%d", summary.UnknownNamespaceCount)
		}
		if summary.InvalidResourceCount > 0 {
			line += fmt.Sprintf(" invalid_resource=%d", summary.InvalidResourceCount)
		}
		if len(summary.QueryFailedTypes) > 0 {
			line += fmt.Sprintf(" query_failed_types=%s", strings.Join(summary.QueryFailedTypes, ","))
		}
		if summary.EnrichedCount > 0 || len(summary.EnrichmentFailedTypes) > 0 {
			line += fmt.Sprintf(" enriched=%d", summary.EnrichedCount)
			if len(summary.EnrichmentFailedTypes) > 0 {
				line += fmt.Sprintf(" enrichment_failed=%s", strings.Join(summary.EnrichmentFailedTypes, ","))
			}
		}
		if summary.MaxResourcesReached {
			line += " max_resources_reached=true"
			maxResourcesReached = true
		}
		if summary.ProductNamesEmpty {
			productNamesEmpty = true
		}
		summaryLines = append(summaryLines, line)
		collector.collect(summary)
		if len(summary.FailedScopes) > 0 {
			batch.FailedCount += len(summary.FailedScopes)
			partialErrs = append(partialErrs, summary.FailedScopes...)
		}
		if summary.MaxResourcesReached {
			collector.successfulTypes = map[string]struct{}{}
			continue
		}
		if isAuthoritativeScope(summary.SyncMode) && !summary.ProductNamesEmpty {
			snapshot := collector.scopeSnapshot()
			negativeScopes = append(negativeScopes, negativeStaleScope{
				Region:      region,
				ExceptTypes: snapshot.authoritativeExceptTypes(),
			})
			continue
		}
		eligibleTypes := collector.scopeSnapshot().eligibleSuccessTypes()
		for t := range eligibleTypes {
			successScopes = append(successScopes, discoveredScope{Region: region, ResourceType: t})
		}
	}
	return successScopes, negativeScopes, summaryLines, partialErrs, maxResourcesReached, productNamesEmpty, syncSummaries, nil
}

// syncGeneric 通用逐类型同步路径：按 region × resource_type 调用 ListResources（limit 500），
// 适用于不支持 CloudFullSyncPort 的 provider，见 docs/huawei-ces-asset-sync-plan.md §7.2。
func (s *SyncService) syncGeneric(
	ctx context.Context,
	obsActor obsapp.Actor,
	provider string,
	regions []string,
	appID, accountID, batchID, fencingToken string,
	now time.Time,
	batch *domain.SyncBatch,
	collector *staleScopeCollector,
) (successScopes []discoveredScope, negativeScopes []negativeStaleScope, summaryLines, partialErrs []string, maxResourcesReached, productNamesEmpty bool, syncSummaries []obsapp.CloudSyncSummary, syncErr error) {
	for _, region := range regions {
		for _, resType := range defaultCloudResourceTypes {
			result, err := s.discovery.ListResources(ctx, obsActor, obsdomain.AssetDiscoveryQuery{
				AccountID: accountID, Provider: provider, Region: region,
				ResourceType: resType, Limit: 500,
			})
			if err != nil {
				batch.FailedCount++
				partialErrs = append(partialErrs, fmt.Sprintf("%s/%s: %s", region, resType, apperr.FromError(err).Message))
				continue
			}
			// provider 查询成功；先 upsert，全部资源成功持久化后才把该 scope 纳入 stale 标记，见 §13。
			typePersistFailed := false
			if result != nil {
				for _, cloud := range result.Resources {
					created, upsertErr := s.upsertCloudResource(ctx, appID, accountID, batchID, fencingToken, now, cloud)
					if upsertErr != nil {
						if errors.Is(upsertErr, domain.ErrLeaseLost) {
							syncErr = upsertErr
							return
						}
						batch.FailedCount++
						// 原始错误仅写日志，对外只保留脱敏摘要，避免表名/约束名等底层细节泄露，见 §13。
						logger.From(ctx).Warn("upsert cloud resource failed",
							logger.String("region", region), logger.String("cloud_resource_type", cloud.Type),
							logger.Error(upsertErr))
						partialErrs = append(partialErrs, fmt.Sprintf("%s/%s: %s", region, resType, apperr.FromError(upsertErr).Message))
						typePersistFailed = true
						if t := strings.ToLower(strings.TrimSpace(resType)); t != "" {
							collector.persistFailedTypes[t] = struct{}{}
						}
						continue
					}
					if created {
						batch.CreatedCount++
					} else {
						batch.UpdatedCount++
					}
				}
			}
			if typePersistFailed {
				continue
			}
			// 截断门控：provider 返回 HasMore=true 表示因达到 limit 而截断，云端仍有更多资源，
			// 该类型跳过 stale 标记，避免未返回资源被误标 stale，见 §13.1。
			if result != nil && result.HasMore {
				continue
			}
			collector.successfulTypes[resType] = struct{}{}
			successScopes = append(successScopes, discoveredScope{Region: region, ResourceType: resType})
		}
	}
	return successScopes, negativeScopes, summaryLines, partialErrs, maxResourcesReached, productNamesEmpty, syncSummaries, syncErr
}

// lowerStringSet 将字符串切片归一化为小写去重集合。
func lowerStringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
			out[v] = struct{}{}
		}
	}
	return out
}

// isAuthoritativeScope 判断 sync_mode 是否代表权威 scope（CES 资源分组 product_names 完整定义了
// 资源组内类型集合）。权威 scope 用反向 stale 标记：不在 scope 内的类型视为已移除，见 §13.1。
// native 只覆盖固定 4 类、generic/fake 覆盖范围有限，均非权威，不能反向标记。
func isAuthoritativeScope(syncMode string) bool {
	mode := strings.ToLower(strings.TrimSpace(syncMode))
	return mode == "ces" || mode == "hybrid"
}

// mergeLowerStrings 将多组字符串合并为小写去重切片，用于聚合反向 stale 的 exceptTypes
// （QueryFailedTypes ∪ ConversionFailedTypes ∪ persistFailedTypes），见 §13.1。
func mergeLowerStrings(sets ...[]string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, set := range sets {
		for _, v := range set {
			if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
				if _, ok := seen[v]; ok {
					continue
				}
				seen[v] = struct{}{}
				out = append(out, v)
			}
		}
	}
	return out
}

// mapStringSetKeys 返回 map[string]struct{} 的 key 切片，用于把 persistFailedTypes 转为切片。
func mapStringSetKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (s *SyncService) upsertCloudResource(
	ctx context.Context,
	appID, accountID, batchID, fencingToken string,
	syncedAt time.Time,
	cloud obsdomain.CloudResource,
) (bool, error) {
	cloudType := strings.ToLower(strings.TrimSpace(cloud.Type))
	cloudID := strings.TrimSpace(cloud.ProviderRef)
	if cloudID == "" {
		cloudID = strings.TrimSpace(cloud.ResourceID)
	}
	if cloudType == "" || cloudID == "" {
		return false, apperr.New(apperr.CodeInvalidArgument, "cloud resource missing type or id")
	}
	resourceType, instance := mapCloudResourceToAssetFields(cloud)
	res := &domain.Resource{
		ID:                   uuid.NewString(),
		ApplicationID:        appID,
		Name:                 strings.TrimSpace(cloud.Name),
		ResourceType:         resourceType,
		Instance:             instance,
		Source:               domain.ResourceSourceCloudSync,
		IntegrationAccountID: accountID,
		CloudResourceID:      cloudID,
		CloudResourceType:    cloudType,
		Region:               strings.TrimSpace(cloud.Region),
		SyncStatus:           domain.SyncStatusActive,
		LastSyncedAt:         &syncedAt,
		Labels:               cloud.Labels,
	}
	return s.resources.UpsertCloudSyncWithLease(ctx, res, batchID, fencingToken)
}

func cloudApplicationID(accountID string) string {
	id := strings.TrimSpace(accountID)
	hash := sha1.Sum([]byte(id))
	suffix := hex.EncodeToString(hash[:])[:12]
	if len(id) > 17 {
		id = id[:17]
	}
	return "cloud-" + id + "-" + suffix
}

func mapCloudResourceToAssetFields(cloud obsdomain.CloudResource) (resourceType, instance string) {
	switch strings.ToLower(strings.TrimSpace(cloud.Type)) {
	case "ecs":
		return "host", cloudProviderRef(cloud)
	case "evs", "obs", "sfs":
		return "storage", cloudProviderRef(cloud)
	case "vpc", "vpcep", "nat":
		return "network", cloudProviderRef(cloud)
	case "rds":
		// SYS.RDS -> database，对齐 docs/huawei-ces-asset-sync-plan.md §9.3 namespace 映射表。
		return "database", cloudProviderRef(cloud)
	case "elb", "cce", "apm":
		return "service", cloudProviderRef(cloud)
	case "dcs", "dms":
		return "middleware", cloudProviderRef(cloud)
	case "cbr":
		return "backup", cloudProviderRef(cloud)
	case "ces":
		return "monitor", cloudProviderRef(cloud)
	default:
		return "service", cloudProviderRef(cloud)
	}
}

func cloudProviderRef(cloud obsdomain.CloudResource) string {
	if cloud.Labels != nil {
		if v := strings.TrimSpace(cloud.Labels["instance_id"]); v != "" {
			return v
		}
	}
	if ref := strings.TrimSpace(cloud.ProviderRef); ref != "" {
		return ref
	}
	return strings.TrimSpace(cloud.ResourceID)
}

func normalizeRegions(regions []string) []string {
	out := make([]string, 0, len(regions))
	seen := map[string]struct{}{}
	for _, region := range regions {
		region = strings.TrimSpace(region)
		if region == "" {
			continue
		}
		if _, ok := seen[region]; ok {
			continue
		}
		seen[region] = struct{}{}
		out = append(out, region)
	}
	return out
}

const syncBatchMessageMaxRunes = 2000

func truncateMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	runes := []rune(msg)
	if len(runes) <= syncBatchMessageMaxRunes {
		return msg
	}
	return string(runes[:syncBatchMessageMaxRunes])
}

func buildAssetSyncAuditPayload(accountID, provider string, regions []string, batch *domain.SyncBatch, summaries []obsapp.CloudSyncSummary) map[string]any {
	payload := map[string]any{
		"account_id": accountID, "provider": provider, "status": batch.Status,
		"regions":       append([]string(nil), regions...),
		"created_count": batch.CreatedCount, "updated_count": batch.UpdatedCount,
		"stale_count": batch.StaleCount, "failed_count": batch.FailedCount,
	}
	if len(summaries) == 0 {
		return payload
	}
	payload["sync_mode"] = firstSummaryString(summaries, func(s obsapp.CloudSyncSummary) string { return s.SyncMode })
	payload["resource_group"] = firstSummaryString(summaries, func(s obsapp.CloudSyncSummary) string { return s.ResourceGroupName })
	payload["resource_group_id"] = firstSummaryString(summaries, func(s obsapp.CloudSyncSummary) string { return s.ResourceGroupID })
	payload["projects"] = uniqueSummaryStrings(summaries, func(s obsapp.CloudSyncSummary) string { return s.ProjectID })
	payload["ces_total"] = sumSummaryInt(summaries, func(s obsapp.CloudSyncSummary) int { return s.CESTotal })
	payload["discovered_count"] = sumSummaryInt(summaries, func(s obsapp.CloudSyncSummary) int { return s.Discovered })
	payload["unknown_namespace_count"] = sumSummaryInt(summaries, func(s obsapp.CloudSyncSummary) int { return s.UnknownNamespaceCount })
	payload["invalid_resource_count"] = sumSummaryInt(summaries, func(s obsapp.CloudSyncSummary) int { return s.InvalidResourceCount })
	payload["max_resources_reached"] = anySummaryBool(summaries, func(s obsapp.CloudSyncSummary) bool { return s.MaxResourcesReached })
	return payload
}

func firstSummaryString(summaries []obsapp.CloudSyncSummary, pick func(obsapp.CloudSyncSummary) string) string {
	for _, s := range summaries {
		if v := strings.TrimSpace(pick(s)); v != "" {
			return v
		}
	}
	return ""
}

func uniqueSummaryStrings(summaries []obsapp.CloudSyncSummary, pick func(obsapp.CloudSyncSummary) string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, s := range summaries {
		v := strings.TrimSpace(pick(s))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func sumSummaryInt(summaries []obsapp.CloudSyncSummary, pick func(obsapp.CloudSyncSummary) int) int {
	total := 0
	for _, s := range summaries {
		total += pick(s)
	}
	return total
}

func anySummaryBool(summaries []obsapp.CloudSyncSummary, pick func(obsapp.CloudSyncSummary) bool) bool {
	for _, s := range summaries {
		if pick(s) {
			return true
		}
	}
	return false
}

func buildSyncBatchSummaryDTO(summaries []obsapp.CloudSyncSummary) *SyncBatchSummaryDTO {
	if len(summaries) == 0 {
		return nil
	}
	dto := &SyncBatchSummaryDTO{
		SyncMode:              firstSummaryString(summaries, func(s obsapp.CloudSyncSummary) string { return s.SyncMode }),
		ResourceGroupName:     firstSummaryString(summaries, func(s obsapp.CloudSyncSummary) string { return s.ResourceGroupName }),
		ResourceGroupID:       firstSummaryString(summaries, func(s obsapp.CloudSyncSummary) string { return s.ResourceGroupID }),
		Projects:              uniqueSummaryStrings(summaries, func(s obsapp.CloudSyncSummary) string { return s.ProjectID }),
		Regions:               uniqueSummaryStrings(summaries, func(s obsapp.CloudSyncSummary) string { return s.Region }),
		CESTotal:              sumSummaryInt(summaries, func(s obsapp.CloudSyncSummary) int { return s.CESTotal }),
		DiscoveredCount:       sumSummaryInt(summaries, func(s obsapp.CloudSyncSummary) int { return s.Discovered }),
		FailedScopes:          uniqueSummaryStringSlices(summaries, func(s obsapp.CloudSyncSummary) []string { return s.FailedScopes }),
		EnrichedCount:         sumSummaryInt(summaries, func(s obsapp.CloudSyncSummary) int { return s.EnrichedCount }),
		EnrichmentFailedTypes: uniqueSummaryStringSlices(summaries, func(s obsapp.CloudSyncSummary) []string { return s.EnrichmentFailedTypes }),
		UnknownNamespaceCount: sumSummaryInt(summaries, func(s obsapp.CloudSyncSummary) int { return s.UnknownNamespaceCount }),
		InvalidResourceCount:  sumSummaryInt(summaries, func(s obsapp.CloudSyncSummary) int { return s.InvalidResourceCount }),
		MaxResourcesReached:   anySummaryBool(summaries, func(s obsapp.CloudSyncSummary) bool { return s.MaxResourcesReached }),
		ProductNamesEmpty:     anySummaryBool(summaries, func(s obsapp.CloudSyncSummary) bool { return s.ProductNamesEmpty }),
		QueryFailedTypes:      uniqueSummaryStringSlices(summaries, func(s obsapp.CloudSyncSummary) []string { return s.QueryFailedTypes }),
		ConversionFailedTypes: uniqueSummaryStringSlices(summaries, func(s obsapp.CloudSyncSummary) []string { return s.ConversionFailedTypes }),
	}
	if dto.ProductNamesEmpty {
		dto.PartialReason = appendReason(dto.PartialReason, "product_names_empty=true")
	}
	if dto.MaxResourcesReached {
		dto.PartialReason = appendReason(dto.PartialReason, "max_resources_reached=true")
	}
	if len(dto.QueryFailedTypes) > 0 {
		dto.PartialReason = appendReason(dto.PartialReason, "query_failed_types="+strings.Join(dto.QueryFailedTypes, ","))
	}
	if len(dto.ConversionFailedTypes) > 0 {
		dto.PartialReason = appendReason(dto.PartialReason, "conversion_failed_types="+strings.Join(dto.ConversionFailedTypes, ","))
	}
	return dto
}

func appendReason(existing, reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return existing
	}
	if strings.TrimSpace(existing) == "" {
		return reason
	}
	return existing + "; " + reason
}

func uniqueSummaryStringSlices(summaries []obsapp.CloudSyncSummary, pick func(obsapp.CloudSyncSummary) []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, s := range summaries {
		for _, v := range pick(s) {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

func marshalSyncBatchSummary(summary *SyncBatchSummaryDTO) []byte {
	if summary == nil {
		return []byte("{}")
	}
	data, err := json.Marshal(summary)
	if err != nil {
		return []byte("{}")
	}
	return data
}

func unmarshalSyncBatchSummary(data []byte) *SyncBatchSummaryDTO {
	if len(data) == 0 || string(data) == "{}" {
		return nil
	}
	var summary SyncBatchSummaryDTO
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil
	}
	return &summary
}

func isAlreadyExists(err error) bool {
	return apperr.CodeOf(err) == apperr.CodeAlreadyExists
}

func toSyncBatchDTO(batch domain.SyncBatch) SyncBatchDTO {
	dto := SyncBatchDTO{
		BatchID:              batch.BatchID,
		IntegrationAccountID: batch.IntegrationAccountID,
		Provider:             batch.Provider,
		Status:               batch.Status,
		CreatedCount:         batch.CreatedCount,
		UpdatedCount:         batch.UpdatedCount,
		StaleCount:           batch.StaleCount,
		FailedCount:          batch.FailedCount,
		Message:              batch.Message,
		Summary:              unmarshalSyncBatchSummary(batch.Summary),
		StartedAt:            batch.StartedAt.Unix(),
		CreatedAt:            batch.CreatedAt.Unix(),
		UpdatedAt:            batch.UpdatedAt.Unix(),
	}
	if accountID := strings.TrimSpace(batch.IntegrationAccountID); accountID != "" {
		dto.ApplicationID = cloudApplicationID(accountID)
	}
	if batch.FinishedAt != nil {
		dto.FinishedAt = batch.FinishedAt.Unix()
	}
	return dto
}
