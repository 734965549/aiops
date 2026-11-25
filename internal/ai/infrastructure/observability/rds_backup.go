package observability

import (
	"context"
	"strings"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	huawei "github.com/734965549/aiops/internal/observability/infrastructure/provider/huawei"
	apperr "github.com/734965549/aiops/pkg/errors"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
)

const defaultRDSBackupTimeout = 15 * time.Second

// RDSBackupCollector 复用现有云账号查询 RDS 备份策略与最近备份记录（只读）。
type RDSBackupCollector struct {
	accounts      AccountResolver
	credentials   AKSKResolver
	backups       huawei.RDSBackupClient
	accountAccess aiapp.AccountAccessAuthorizer
	timeout       time.Duration
	now           func() time.Time
}

func NewRDSBackupCollector(accounts AccountResolver, credentials AKSKResolver, backups huawei.RDSBackupClient, accountAccess aiapp.AccountAccessAuthorizer, timeout time.Duration) *RDSBackupCollector {
	if backups == nil {
		backups = huawei.NewRDSBackupClient()
	}
	if timeout <= 0 {
		timeout = defaultRDSBackupTimeout
	}
	return &RDSBackupCollector{
		accounts: accounts, credentials: credentials, backups: backups,
		accountAccess: accountAccess, timeout: timeout, now: time.Now,
	}
}

var _ aiapp.RDSBackupPort = (*RDSBackupCollector)(nil)

func (c *RDSBackupCollector) Query(ctx context.Context, userID string, q aiapp.RDSBackupQuery) (*aiapp.RDSBackupResult, error) {
	if c == nil || c.accounts == nil || c.credentials == nil || c.backups == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "rds backup collector is not configured")
	}
	q.AccountID = strings.TrimSpace(q.AccountID)
	q.Region = strings.TrimSpace(q.Region)
	q.InstanceID = strings.TrimSpace(q.InstanceID)
	q.ProjectID = strings.TrimSpace(q.ProjectID)
	if q.Days <= 0 {
		q.Days = aiapp.RDSBackupDefaultDays
	}
	if q.Days > aiapp.RDSBackupMaxDays {
		q.Days = aiapp.RDSBackupMaxDays
	}
	if q.Limit <= 0 {
		q.Limit = aiapp.RDSBackupDefaultLimit
	}
	if q.Limit > aiapp.RDSBackupMaxLimit {
		q.Limit = aiapp.RDSBackupMaxLimit
	}
	if q.AccountID == "" {
		return degradedRDSBackup(aiapp.EvidenceStatusSkipped, "no_account", q), nil
	}
	if q.Region == "" {
		return degradedRDSBackup(aiapp.EvidenceStatusSkipped, "no_region", q), nil
	}
	if q.InstanceID == "" {
		return degradedRDSBackup(aiapp.EvidenceStatusSkipped, "no_instance_id", q), nil
	}
	snapshot, err := c.accounts.ResolveAccount(ctx, q.AccountID)
	if err != nil {
		return degradedRDSBackup(aiapp.EvidenceStatusError, evidenceErrorReason(err, "resolve integration account failed"), q), nil
	}
	if snapshot == nil {
		return degradedRDSBackup(aiapp.EvidenceStatusError, "integration account not found", q), nil
	}
	if !strings.EqualFold(strings.TrimSpace(snapshot.AuthType), "ak_sk") {
		return degradedRDSBackup(aiapp.EvidenceStatusUnsupported, "rds backup query requires ak_sk auth", q), nil
	}
	if c.accountAccess != nil && strings.TrimSpace(userID) != "" {
		ref := aiapp.AccountRef{
			AccountID: snapshot.AccountID, OwnerTeam: snapshot.OwnerTeam,
			Regions: append([]string(nil), snapshot.Regions...),
		}
		if err := c.accountAccess.Authorize(ctx, userID, ref); err != nil {
			return degradedRDSBackup(aiapp.EvidenceStatusPermissionDenied, "no access to integration account for rds backup", q), nil
		}
	}
	q.ProjectID = resolveRDSProjectID(snapshot, q)
	if q.ProjectID == "" {
		return degradedRDSBackup(aiapp.EvidenceStatusError, "project_id is required", q), nil
	}
	cred, err := c.credentials.ResolveAKSK(ctx, *snapshot)
	if err != nil {
		return degradedRDSBackup(aiapp.EvidenceStatusError, evidenceErrorReason(err, "resolve huawei credential failed"), q), nil
	}
	loc := aiapp.EvidenceLocation(q.Timezone)
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	to := now.Unix()
	from := to - int64(q.Days)*24*3600
	begin := time.Unix(from, 0).UTC().Format("2006-01-02T15:04:05Z")
	end := time.Unix(to, 0).UTC().Format("2006-01-02T15:04:05Z")
	out := &aiapp.RDSBackupResult{
		Status: aiapp.EvidenceStatusOK, Provider: snapshot.Provider,
		ProjectID: q.ProjectID, InstanceID: q.InstanceID, Timezone: loc.String(),
		WindowStart: from, WindowEnd: to, CollectedAt: now.Unix(),
	}
	queryCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	policy, err := c.backups.ShowBackupPolicy(queryCtx, cred, q.ProjectID, q.Region, q.InstanceID)
	if err != nil {
		status := rdsBackupStatusForError(err)
		reason := evidenceErrorReason(err, "rds backup policy query failed")
		if status == aiapp.EvidenceStatusPermissionDenied {
			return degradedRDSBackup(status, reason, q), nil
		}
		out.CollectionErrors = append(out.CollectionErrors, "policy:"+reason)
	} else {
		out.Policy = mapRDSBackupPolicy(policy, loc)
	}

	records, total, err := c.backups.ListBackups(queryCtx, cred, q.ProjectID, q.Region, q.InstanceID, begin, end, int32(q.Limit))
	if err != nil {
		status := rdsBackupStatusForError(err)
		reason := evidenceErrorReason(err, "rds backup list query failed")
		out.CollectionErrors = append(out.CollectionErrors, "backups:"+reason)
		if out.Policy == nil {
			out.Status = status
			out.Reason = reason
		}
		return out, nil
	}
	out.Backups = mapRDSBackupRecords(records, loc)
	out.BackupTotal = int(total)
	return out, nil
}

func mapRDSBackupPolicy(policy *rdsmodel.BackupPolicy, loc *time.Location) *aiapp.RDSBackupPolicyEvidence {
	if policy == nil {
		return nil
	}
	out := &aiapp.RDSBackupPolicyEvidence{
		KeepDays: int(policy.KeepDays),
		Timezone: loc.String(),
	}
	if policy.StartTime != nil {
		out.StartTimeUTC = strings.TrimSpace(*policy.StartTime)
	}
	if policy.Period != nil {
		out.Period = strings.TrimSpace(*policy.Period)
	}
	out.WindowLocal = localBackupWindow(out.StartTimeUTC, loc)
	switch {
	case out.KeepDays <= 0:
		out.Note = "keep_days=0，自动备份可能已关闭"
	case out.StartTimeUTC != "" && out.WindowLocal == "":
		out.Note = "备份窗口本地时间转换失败，请以 start_time_utc 为准"
	}
	return out
}

// localBackupWindow 将华为云 UTC 备份窗口（hh:mm-HH:MM）转换为本地时间窗；
// 转换后跨日时追加“(次日)”。
func localBackupWindow(raw string, loc *time.Location) string {
	parts := strings.Split(strings.TrimSpace(raw), "-")
	if len(parts) != 2 {
		return ""
	}
	start, err1 := time.Parse("15:04", strings.TrimSpace(parts[0]))
	end, err2 := time.Parse("15:04", strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	base := time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC)
	s := base.Add(time.Duration(start.Hour())*time.Hour + time.Duration(start.Minute())*time.Minute).In(loc)
	e := base.Add(time.Duration(end.Hour())*time.Hour + time.Duration(end.Minute())*time.Minute).In(loc)
	suffix := ""
	if !e.After(s) {
		suffix = "(次日)"
	}
	return s.Format("15:04") + "-" + e.Format("15:04") + suffix
}

func mapRDSBackupRecords(records []rdsmodel.BackupForList, loc *time.Location) []aiapp.RDSBackupRecord {
	out := make([]aiapp.RDSBackupRecord, 0, len(records))
	for _, record := range records {
		out = append(out, aiapp.RDSBackupRecord{
			BackupID:  strings.TrimSpace(record.Id),
			Name:      strings.TrimSpace(record.Name),
			Type:      record.Type.Value(),
			Status:    record.Status.Value(),
			BeginTime: localRDSBackupTime(record.BeginTime, loc),
			EndTime:   localRDSBackupTime(record.EndTime, loc),
			SizeKB:    record.Size,
		})
	}
	return out
}

func localRDSBackupTime(raw string, loc *time.Location) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z0700"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.In(loc).Format("2006-01-02 15:04")
		}
	}
	return raw
}

func resolveRDSProjectID(snapshot *obsdomain.AccountSnapshot, q aiapp.RDSBackupQuery) string {
	if q.ProjectID != "" {
		return q.ProjectID
	}
	cfg := obsapp.ParseSyncModeConfig(snapshot.ExtraConfig)
	return strings.TrimSpace(cfg.ResolveProjectID(q.Region, strings.TrimSpace(snapshot.ProjectID)))
}

func degradedRDSBackup(status, reason string, q aiapp.RDSBackupQuery) *aiapp.RDSBackupResult {
	return &aiapp.RDSBackupResult{
		Status: status, Reason: reason, ProjectID: q.ProjectID,
		InstanceID: q.InstanceID, Timezone: aiapp.EvidenceLocation(q.Timezone).String(),
		CollectedAt: time.Now().Unix(),
	}
}

func rdsBackupStatusForError(err error) string {
	if err == nil {
		return aiapp.EvidenceStatusOK
	}
	reason := strings.ToLower(err.Error())
	if strings.Contains(reason, "permission denied") {
		return aiapp.EvidenceStatusPermissionDenied
	}
	return aiapp.EvidenceStatusError
}
