package application

import (
	"context"
	"strings"
	"time"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// AlarmPullQuery 是告警记录拉取的应用层查询。
type AlarmPullQuery struct {
	AccountID   string
	Provider    string
	Regions     []string
	Cursors     map[string]int64
	RoundCutoff int64
	Deadline    time.Time
}

// PullAlarmRecords 解析账号/能力/区域/项目，调用 Provider 按 scope 拉取 CES 告警记录。
// 账号级错误返回 error；区域不可用、缺 project 等以 Failures 返回，供调用方生成 scope 失败摘要。
func (s *QueryService) PullAlarmRecords(ctx context.Context, _ Actor, q AlarmPullQuery) (*domain.AlarmPullResult, error) {
	pctx, entry, err := s.resolveEntry(ctx, q.AccountID, q.Provider, integdomain.CapabilityAlerts)
	if err != nil {
		return nil, err
	}
	port, ok := entry.(AlarmRecordPullPort)
	if !ok {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider does not support alarm record pull")
	}
	accountRegions := normalizeRegionSet(pctx.Account.Regions)
	requested := q.Regions
	if len(requested) == 0 {
		requested = pctx.Account.Regions
	}
	result := &domain.AlarmPullResult{
		PlannedRegions: make([]string, 0, len(requested)),
	}
	seenRegion := map[string]struct{}{}
	cfg := ParseSyncModeConfig(pctx.Account.ExtraConfig)
	scopes := make([]domain.AlarmPullScope, 0, len(requested))
	for _, raw := range requested {
		region := strings.ToLower(strings.TrimSpace(raw))
		if region == "" {
			continue
		}
		if _, dup := seenRegion[region]; dup {
			continue
		}
		seenRegion[region] = struct{}{}
		result.PlannedRegions = append(result.PlannedRegions, region)
		if _, ok := accountRegions[region]; !ok {
			result.Failures = append(result.Failures, domain.AlarmScopeFailure{
				Region: region, Reason: "REGION_UNAVAILABLE",
			})
			continue
		}
		projectID := strings.TrimSpace(cfg.ResolveProjectID(region, pctx.Account.ProjectID))
		if projectID == "" {
			result.Failures = append(result.Failures, domain.AlarmScopeFailure{
				Region: region, Reason: "PROJECT_ID_MISSING",
			})
			continue
		}
		scopes = append(scopes, domain.AlarmPullScope{Region: region, ProjectID: projectID})
	}
	if len(scopes) == 0 {
		return result, nil
	}
	cutoff := q.RoundCutoff
	if cutoff <= 0 {
		cutoff = time.Now().UTC().Unix()
	}
	deadline := q.Deadline
	if deadline.IsZero() {
		deadline = time.Now().Add(20 * time.Second)
	}
	pulled, err := port.PullAlarmRecords(ctx, pctx, domain.AlarmPullQuery{
		Scopes:      scopes,
		Cursors:     q.Cursors,
		RoundCutoff: cutoff,
		Deadline:    deadline,
	})
	if err != nil {
		return nil, err
	}
	if pulled == nil {
		pulled = &domain.AlarmPullResult{}
	}
	result.Scopes = pulled.Scopes
	result.Failures = append(result.Failures, pulled.Failures...)
	return result, nil
}

func normalizeRegionSet(regions []string) map[string]struct{} {
	out := make(map[string]struct{}, len(regions))
	for _, r := range regions {
		v := strings.ToLower(strings.TrimSpace(r))
		if v != "" {
			out[v] = struct{}{}
		}
	}
	return out
}
