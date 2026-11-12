package huawei

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/observability/domain"
	cesv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

const (
	alarmHistoryPageLimit   = 100
	alarmHistoryMaxPages    = 100
	alarmHistoryActiveHours = 24
	alarmHistoryOverlap     = 5 * time.Minute
)

// AlarmScopePage 是单 scope 单轮拉取结果；Complete=false 时不得推进游标。
type AlarmScopePage struct {
	Records       []domain.AlarmRecord
	WindowEnd     int64
	Complete      bool
	FailureCode   string
	FailureReason string
}

// AlarmHistoryPullClient 按 scope 拉取 CES V2 告警记录。
type AlarmHistoryPullClient interface {
	PullScope(ctx context.Context, cred AKSKCredential, scope domain.AlarmPullScope, cursor, roundCutoff int64, deadline time.Time) (AlarmScopePage, error)
}

// alarmHistoryAPI 抽象 SDK 调用，便于 fixture 单测。
type alarmHistoryAPI interface {
	ListAlarmHistories(request *cesv2model.ListAlarmHistoriesRequest) (*cesv2model.ListAlarmHistoriesResponse, error)
	ListAlarmRules(request *cesv2model.ListAlarmRulesRequest) (*cesv2model.ListAlarmRulesResponse, error)
}

// CESAlarmHistoryClient 生产实现，独立于资源发现客户端。
type CESAlarmHistoryClient struct {
	newAPI func(cred AKSKCredential, projectID, region string) (alarmHistoryAPI, error)
	rules  *alarmRuleCache
}

func NewCESAlarmHistoryClient() *CESAlarmHistoryClient {
	return &CESAlarmHistoryClient{
		newAPI: func(cred AKSKCredential, projectID, region string) (alarmHistoryAPI, error) {
			client, err := newCESv2Client(cred, projectID, region)
			if err != nil {
				return nil, err
			}
			return client, nil
		},
		rules: &alarmRuleCache{},
	}
}

var _ AlarmHistoryPullClient = (*CESAlarmHistoryClient)(nil)

// PullScope 拉取单个 region/project scope：
//   - 新 scope：先查云端可见 alarm 记录，再读最近 24 小时更新窗口，按 record_id 合并
//   - 增量：读 [cursor-5m, roundCutoff] 窗口
//   - 任一分页失败/不完整返回已读取记录并置 Complete=false，调用方保留旧游标
//   - 记录合并后按 alarm_id 查询 ListAlarmRules 富化缺失的指标/阈值；富化失败不影响 Complete
func (c *CESAlarmHistoryClient) PullScope(ctx context.Context, cred AKSKCredential, scope domain.AlarmPullScope, cursor, roundCutoff int64, deadline time.Time) (AlarmScopePage, error) {
	if c == nil || c.newAPI == nil {
		return AlarmScopePage{}, fmt.Errorf("alarm history client is not configured")
	}
	if roundCutoff <= 0 {
		roundCutoff = time.Now().UTC().Unix()
	}
	api, err := c.newAPI(cred, scope.ProjectID, scope.Region)
	if err != nil {
		return AlarmScopePage{}, err
	}
	byID := map[string]domain.AlarmRecord{}
	var page AlarmScopePage
	if cursor <= 0 {
		active, complete, code := c.listWindow(ctx, api, scope, nil, 0, 0, deadline)
		mergeAlarmRecords(byID, active)
		if !complete {
			page = AlarmScopePage{Records: alarmRecords(byID), Complete: false, FailureCode: code}
		} else {
			from := time.Unix(roundCutoff, 0).UTC().Add(-alarmHistoryActiveHours * time.Hour).Unix()
			recent, recentComplete, recentCode := c.listWindow(ctx, api, scope, nil, from, roundCutoff, deadline)
			mergeAlarmRecords(byID, recent)
			if !recentComplete {
				page = AlarmScopePage{Records: alarmRecords(byID), Complete: false, FailureCode: recentCode}
			} else {
				page = AlarmScopePage{Records: alarmRecords(byID), WindowEnd: roundCutoff, Complete: true}
			}
		}
	} else {
		from := cursor - int64(alarmHistoryOverlap.Seconds())
		if from < 0 {
			from = 0
		}
		records, complete, code := c.listWindow(ctx, api, scope, nil, from, roundCutoff, deadline)
		mergeAlarmRecords(byID, records)
		if !complete {
			page = AlarmScopePage{Records: alarmRecords(byID), Complete: false, FailureCode: code}
		} else {
			page = AlarmScopePage{Records: alarmRecords(byID), WindowEnd: roundCutoff, Complete: true}
		}
	}
	c.enrichRecords(ctx, api, scope, page.Records, deadline)
	return page, nil
}

// listWindow 分页读取一个时间窗口；status 为 nil 表示不过滤状态。
// 返回 complete=false 时 code 为稳定原因码（PAGE_INCOMPLETE/UPSTREAM_TIMEOUT/UPSTREAM_ERROR）。
func (c *CESAlarmHistoryClient) listWindow(ctx context.Context, api alarmHistoryAPI, scope domain.AlarmPullScope, status []cesv2model.ListAlarmHistoriesRequestStatus, from, to int64, deadline time.Time) ([]domain.AlarmRecord, bool, string) {
	var out []domain.AlarmRecord
	seen := map[string]struct{}{}
	offset := int32(0)
	prevPageKey := ""
	for page := 0; page < alarmHistoryMaxPages; page++ {
		if err := ctx.Err(); err != nil {
			return out, false, "UPSTREAM_TIMEOUT"
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return out, false, "ROUND_TIMEOUT"
		}
		req := &cesv2model.ListAlarmHistoriesRequest{
			Offset: &offset,
			Limit:  alarmHistoryLimitPtr(alarmHistoryPageLimit),
		}
		order := cesv2model.GetListAlarmHistoriesRequestOrderByEnum().RECORD_ID
		req.OrderBy = &order
		if status != nil {
			req.Status = &status
			mask := "UN_MASKED"
			req.MaskStatus = &mask
		}
		if from > 0 {
			v := time.Unix(from, 0).UTC().Format(time.RFC3339)
			req.From = &v
		}
		if to > 0 {
			v := time.Unix(to, 0).UTC().Format(time.RFC3339)
			req.To = &v
		}
		resp, err := api.ListAlarmHistories(req)
		if err != nil {
			code := "UPSTREAM_ERROR"
			if isTimeoutError(ctx, err) {
				code = "UPSTREAM_TIMEOUT"
			}
			return out, false, code
		}
		if resp == nil || resp.AlarmHistories == nil || len(*resp.AlarmHistories) == 0 {
			return out, true, ""
		}
		items := *resp.AlarmHistories
		pageKey := strings.TrimSpace(valueOf(items[0].RecordId))
		if pageKey != "" && pageKey == prevPageKey {
			return out, false, "PAGE_INCOMPLETE"
		}
		prevPageKey = pageKey
		duplicateOnly := true
		for _, item := range items {
			rec := mapAlarmHistory(item, scope)
			if rec.RecordID == "" {
				continue
			}
			if _, ok := seen[rec.RecordID]; ok {
				continue
			}
			seen[rec.RecordID] = struct{}{}
			duplicateOnly = false
			out = append(out, rec)
		}
		offset += int32(len(items))
		if resp.Count != nil && offset >= *resp.Count {
			return out, true, ""
		}
		if len(items) < alarmHistoryPageLimit {
			return out, true, ""
		}
		if duplicateOnly {
			return out, false, "PAGE_INCOMPLETE"
		}
	}
	return out, false, "PAGE_INCOMPLETE"
}

func alarmHistoryLimitPtr(v int32) *int32 { return &v }

func valueOf(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// mergeAlarmRecords 按 record_id 合并 active/update 两个查询：
// 明确恢复状态优先；同为 alarm 时取较新的 last_alarm_time。
func mergeAlarmRecords(dst map[string]domain.AlarmRecord, records []domain.AlarmRecord) {
	for _, rec := range records {
		if rec.RecordID == "" {
			continue
		}
		old, ok := dst[rec.RecordID]
		if !ok {
			dst[rec.RecordID] = rec
			continue
		}
		if isRecoveredStatus(rec.Status) && !isRecoveredStatus(old.Status) {
			dst[rec.RecordID] = rec
			continue
		}
		if isRecoveredStatus(rec.Status) == isRecoveredStatus(old.Status) {
			if timeAfter(rec.LastAlarmAt, old.LastAlarmAt) {
				dst[rec.RecordID] = rec
			}
		}
	}
}

func alarmRecords(m map[string]domain.AlarmRecord) []domain.AlarmRecord {
	out := make([]domain.AlarmRecord, 0, len(m))
	for _, rec := range m {
		out = append(out, rec)
	}
	return out
}

func isRecoveredStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ok", "ok_manual":
		return true
	default:
		return false
	}
}

func timeAfter(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}

func isTimeoutError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx.Err() == context.DeadlineExceeded {
		return true
	}
	type timeout interface{ Timeout() bool }
	if t, ok := err.(timeout); ok && t.Timeout() {
		return true
	}
	return false
}

func levelString(level *cesv2model.AlarmHistoryItemV2Level) string {
	if level == nil {
		return ""
	}
	return strconv.Itoa(int(level.Value()))
}

var _ = cesv2.NewCesClient
