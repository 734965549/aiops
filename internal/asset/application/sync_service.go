package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	integdomain "github.com/734965549/aiops/internal/integration/domain"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

var defaultCloudResourceTypes = []string{"ecs", "cce", "rds", "elb"}

// SyncService 云资源同步到 Asset 注册表。
type SyncService struct {
	apps      domain.ApplicationRepository
	resources domain.ResourceRepository
	batches   domain.SyncBatchRepository
	discovery CloudDiscoveryPort
	accounts  IntegrationAccountPort
	audit     AuditRecorder
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
	}
}

type TriggerSyncInput struct {
	AccountID string
}

type SyncBatchDTO struct {
	BatchID              string `json:"batch_id"`
	IntegrationAccountID string `json:"integration_account_id"`
	Provider             string `json:"provider"`
	Status               string `json:"status"`
	CreatedCount         int    `json:"created_count"`
	UpdatedCount         int    `json:"updated_count"`
	StaleCount           int    `json:"stale_count"`
	FailedCount          int    `json:"failed_count"`
	Message              string `json:"message,omitempty"`
	ApplicationID        string `json:"application_id,omitempty"`
	StartedAt            int64  `json:"started_at"`
	FinishedAt           int64  `json:"finished_at,omitempty"`
	CreatedAt            int64  `json:"created_at"`
	UpdatedAt            int64  `json:"updated_at"`
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
	batch := &domain.SyncBatch{
		BatchID:              batchID,
		IntegrationAccountID: accountID,
		Provider:             provider,
		Status:               domain.SyncBatchStatusRunning,
		StartedAt:            now,
	}
	if err := s.batches.Create(ctx, batch); err != nil {
		return nil, wrapAssetError(err, "create sync batch failed")
	}

	appID, err := s.ensureCloudApplication(ctx, accountID, provider)
	if err != nil {
		s.finishBatchFailed(ctx, batch, err)
		return nil, err
	}

	obsActor := obsapp.Actor{UserID: actor.UserID, DisplayName: actor.DisplayName}
	var partialErrs []string
	var summaryLines []string
	successScopes := make([]discoveredScope, 0, len(regions)*len(defaultCloudResourceTypes))
	if provider == string(integdomain.ProviderHuaweiCloud) {
		successScopes, summaryLines, partialErrs = s.syncHuaweiCES(ctx, obsActor, provider, regions, appID, accountID, batchID, now, batch)
	} else {
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
				successScopes = append(successScopes, discoveredScope{Region: region, ResourceType: resType})
				if result == nil {
					continue
				}
				for _, cloud := range result.Resources {
					created, upsertErr := s.upsertCloudResource(ctx, appID, accountID, batchID, now, cloud)
					if upsertErr != nil {
						batch.FailedCount++
						partialErrs = append(partialErrs, upsertErr.Error())
						continue
					}
					if created {
						batch.CreatedCount++
					} else {
						batch.UpdatedCount++
					}
				}
			}
		}
	}

	for _, scope := range successScopes {
		staleCount, err := s.resources.MarkStaleByAccountScopeExceptBatch(ctx, accountID, scope.Region, scope.ResourceType, batchID)
		if err != nil {
			batch.FailedCount++
			partialErrs = append(partialErrs, fmt.Sprintf("%s/%s: mark stale failed", scope.Region, scope.ResourceType))
			continue
		}
		batch.StaleCount += int(staleCount)
	}

	finished := time.Now().UTC()
	batch.FinishedAt = &finished
	allLines := append(append([]string(nil), summaryLines...), partialErrs...)
	message := strings.Join(allLines, "; ")
	switch {
	case batch.FailedCount > 0 && batch.CreatedCount+batch.UpdatedCount == 0 && len(successScopes) == 0:
		batch.Status = domain.SyncBatchStatusFailed
		batch.Message = truncateMessage(message)
	case batch.FailedCount > 0:
		batch.Status = domain.SyncBatchStatusPartial
		batch.Message = truncateMessage(message)
	default:
		batch.Status = domain.SyncBatchStatusSuccess
		if strings.TrimSpace(message) == "" {
			message = "ok"
		}
		batch.Message = truncateMessage(message)
	}
	if err := s.batches.Update(ctx, batch); err != nil {
		return nil, wrapAssetError(err, "update sync batch failed")
	}

	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "asset_sync_batch",
		ResourceID:   batchID,
		Action:       AuditAssetSync,
		UserID:       actor.UserID,
		Payload: map[string]any{
			"account_id": accountID, "provider": provider, "status": batch.Status,
			"created_count": batch.CreatedCount, "updated_count": batch.UpdatedCount,
			"stale_count": batch.StaleCount, "failed_count": batch.FailedCount,
		},
	})

	dto := toSyncBatchDTO(*batch)
	dto.ApplicationID = appID
	return &dto, nil
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

// syncHuaweiCES 执行华为云 CES 全量同步，见 docs/huawei-ces-asset-sync-plan.md §8.1。
// 对每个 region 调用全量同步端口，收集资源与摘要；返回成功 scope、摘要行与错误行。
func (s *SyncService) syncHuaweiCES(
	ctx context.Context,
	obsActor obsapp.Actor,
	provider string,
	regions []string,
	appID, accountID, batchID string,
	now time.Time,
	batch *domain.SyncBatch,
) (successScopes []discoveredScope, summaryLines, partialErrs []string) {
	for _, region := range regions {
		result, err := s.discovery.ListAllResources(ctx, obsActor, obsapp.AssetFullSyncQuery{
			AccountID: accountID, Provider: provider, Region: region, MaxResources: 20000,
		})
		if err != nil {
			batch.FailedCount++
			partialErrs = append(partialErrs, fmt.Sprintf("%s: %s", region, apperr.FromError(err).Message))
			continue
		}
		if result == nil {
			continue
		}
		regionTypes := map[string]struct{}{}
		upserted := 0
		for _, cloud := range result.Resources {
			created, upsertErr := s.upsertCloudResource(ctx, appID, accountID, batchID, now, cloud)
			if upsertErr != nil {
				batch.FailedCount++
				partialErrs = append(partialErrs, upsertErr.Error())
				continue
			}
			upserted++
			if created {
				batch.CreatedCount++
			} else {
				batch.UpdatedCount++
			}
			if t := strings.ToLower(strings.TrimSpace(cloud.Type)); t != "" {
				regionTypes[t] = struct{}{}
			}
		}
		// 摘要行，见 §8.1。
		summary := result.Summary
		line := fmt.Sprintf("region=%s group=%s ces_total=%d discovered=%d upserted=%d failed_scopes=%d",
			region, summary.ResourceGroupName, summary.CESTotal, summary.Discovered, upserted, len(summary.FailedScopes))
		if summary.ProductNamesEmpty {
			line += " product_names_empty=true"
		}
		if summary.UnknownNamespaceCount > 0 {
			line += fmt.Sprintf(" unknown_namespace=%d", summary.UnknownNamespaceCount)
		}
		if summary.InvalidResourceCount > 0 {
			line += fmt.Sprintf(" invalid_resource=%d", summary.InvalidResourceCount)
		}
		summaryLines = append(summaryLines, line)
		if len(summary.FailedScopes) > 0 {
			batch.FailedCount += len(summary.FailedScopes)
			partialErrs = append(partialErrs, summary.FailedScopes...)
		}
		for _, t := range summary.SuccessfulTypes {
			if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
				regionTypes[t] = struct{}{}
			}
		}
		// Stale scope 以 provider 成功查询的类型为准；旧 adapter 没有 summary 时回退到本轮入库类型。
		for t := range regionTypes {
			successScopes = append(successScopes, discoveredScope{Region: region, ResourceType: t})
		}
	}
	return successScopes, summaryLines, partialErrs
}

func (s *SyncService) upsertCloudResource(
	ctx context.Context,
	appID, accountID, batchID string,
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
		SyncBatchID:          batchID,
	}
	return s.resources.UpsertCloudSync(ctx, res)
}

func (s *SyncService) finishBatchFailed(ctx context.Context, batch *domain.SyncBatch, cause error) {
	if batch == nil || s.batches == nil {
		return
	}
	finished := time.Now().UTC()
	batch.Status = domain.SyncBatchStatusFailed
	batch.FinishedAt = &finished
	batch.Message = truncateMessage(apperr.FromError(cause).Message)
	_ = s.batches.Update(ctx, batch)
}

func cloudApplicationID(accountID string) string {
	id := strings.TrimSpace(accountID)
	if len(id) > 28 {
		id = id[:28]
	}
	return "cloud-" + id
}

func mapCloudResourceToAssetFields(cloud obsdomain.CloudResource) (resourceType, instance string) {
	switch strings.ToLower(strings.TrimSpace(cloud.Type)) {
	case "ecs":
		return "host", cloudProviderRef(cloud)
	case "evs", "obs", "sfs":
		return "storage", cloudProviderRef(cloud)
	case "vpc", "vpcep", "nat":
		return "network", cloudProviderRef(cloud)
	case "rds", "elb", "cce", "apm":
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

func truncateMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) <= 500 {
		return msg
	}
	return msg[:500]
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
		StartedAt:            batch.StartedAt.Unix(),
		CreatedAt:            batch.CreatedAt.Unix(),
		UpdatedAt:            batch.UpdatedAt.Unix(),
	}
	if batch.FinishedAt != nil {
		dto.FinishedAt = batch.FinishedAt.Unix()
	}
	return dto
}
