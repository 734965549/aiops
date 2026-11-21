package observability

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	huawei "github.com/734965549/aiops/internal/observability/infrastructure/provider/huawei"
	apperr "github.com/734965549/aiops/pkg/errors"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
)

const (
	defaultPostgresEvidenceTimeout = 15 * time.Second
	postgresHistoryPageLimit       = 100
	postgresHistoryMaxRows         = 300
	postgresHistoryMaxWindow       = 7 * 24 * 60 * 60
)

var postgresInstanceIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,128}$`)

// AKSKResolver 从 Integration 凭据引用解析华为 AK/SK。
type AKSKResolver interface {
	ResolveAKSK(ctx context.Context, account obsdomain.AccountSnapshot) (huawei.AKSKCredential, error)
}

// PostgresCollector 复用现有云账号采集 RDS 历史会话、等待事件和 Top SQL。
type PostgresCollector struct {
	accounts      AccountResolver
	credentials   AKSKResolver
	history       huawei.RDSHistoryClient
	accountAccess aiapp.AccountAccessAuthorizer
	timeout       time.Duration
}

func NewPostgresCollector(accounts AccountResolver, credentials AKSKResolver, history huawei.RDSHistoryClient, accountAccess aiapp.AccountAccessAuthorizer, timeout time.Duration) *PostgresCollector {
	if history == nil {
		history = huawei.NewRDSHistoryClient()
	}
	if timeout <= 0 {
		timeout = defaultPostgresEvidenceTimeout
	}
	return &PostgresCollector{accounts: accounts, credentials: credentials, history: history, accountAccess: accountAccess, timeout: timeout}
}

var _ aiapp.PostgresEvidencePort = (*PostgresCollector)(nil)

func (c *PostgresCollector) Collect(ctx context.Context, userID string, q aiapp.PostgresEvidenceQuery) (*aiapp.PostgresEvidenceResult, error) {
	if c == nil || c.accounts == nil || c.credentials == nil || c.history == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "postgres evidence collector is not configured")
	}
	q = normalizePostgresEvidenceQuery(q)
	if reason := validatePostgresEvidenceQuery(q); reason != "" {
		return degradedPostgresEvidence(aiapp.EvidenceStatusSkipped, reason, q), nil
	}
	snapshot, err := c.accounts.ResolveAccount(ctx, q.AccountID)
	if err != nil {
		return degradedPostgresEvidence(aiapp.EvidenceStatusError, evidenceErrorReason(err, "resolve integration account failed"), q), nil
	}
	if snapshot == nil {
		return degradedPostgresEvidence(aiapp.EvidenceStatusError, "integration account not found", q), nil
	}
	if snapshot.AuthType != "ak_sk" {
		return degradedPostgresEvidence(aiapp.EvidenceStatusUnsupported, "rds history session analysis requires ak_sk auth", q), nil
	}
	if c.accountAccess != nil && strings.TrimSpace(userID) != "" {
		ref := aiapp.AccountRef{AccountID: snapshot.AccountID, OwnerTeam: snapshot.OwnerTeam, Regions: append([]string(nil), snapshot.Regions...)}
		if err := c.accountAccess.Authorize(ctx, userID, ref); err != nil {
			return degradedPostgresEvidence(aiapp.EvidenceStatusPermissionDenied, "no access to integration account for postgres evidence", q), nil
		}
	}
	q.ProjectID = resolvePostgresProjectID(snapshot, q)
	if q.ProjectID == "" {
		return degradedPostgresEvidence(aiapp.EvidenceStatusError, "project_id is required", q), nil
	}
	cred, err := c.credentials.ResolveAKSK(ctx, *snapshot)
	if err != nil {
		return degradedPostgresEvidence(aiapp.EvidenceStatusError, evidenceErrorReason(err, "resolve huawei credential failed"), q), nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	out := &aiapp.PostgresEvidenceResult{
		Status: aiapp.EvidenceStatusOK, Provider: snapshot.Provider,
		ProjectID: q.ProjectID, ResourceID: q.ResourceID, InstanceID: q.InstanceID,
		WindowStart: q.From, WindowEnd: q.To, CollectedAt: time.Now().Unix(),
	}
	sessions, sessionTotal, err := fetchHistoryPages(queryCtx, q.Limit, func(offset, limit int32) ([]rdsmodel.Session, int32, error) {
		return c.history.ListSessions(queryCtx, cred, q.ProjectID, q.Region, q.InstanceID, q.From*1000, q.To*1000, offset, limit)
	})
	if err != nil {
		reason := evidenceErrorReason(err, "rds history session query failed")
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			reason = "rds history session query timeout"
		}
		return degradedPostgresEvidence(postgresStatusForError(err), reason, q), nil
	}
	out.SessionTotalCount = sessionTotal
	for _, item := range sessions {
		out.Sessions = append(out.Sessions, mapPostgresSession(item))
	}
	out.Truncated = postgresHistoryTruncated(sessionTotal, len(sessions), q.Limit)
	waitEvents, waitTotal, err := fetchHistoryPages(queryCtx, q.Limit, func(offset, limit int32) ([]rdsmodel.Event, int32, error) {
		return c.history.ListWaitEvents(queryCtx, cred, q.ProjectID, q.Region, q.InstanceID, q.From*1000, q.To*1000, offset, limit)
	})
	if err != nil {
		reason := evidenceErrorReason(err, "rds history wait event query failed")
		out.MissingEvidence = appendPostgresMissing(out.MissingEvidence, "postgres_wait_events")
		out.CollectionErrors = append(out.CollectionErrors, "wait_events:"+reason)
	} else {
		out.WaitEventTotalCount = waitTotal
		for _, item := range waitEvents {
			out.WaitEvents = append(out.WaitEvents, mapPostgresWaitEvent(item))
		}
		out.Truncated = out.Truncated || postgresHistoryTruncated(waitTotal, len(waitEvents), q.Limit)
	}
	topSQL, topTotal, err := fetchHistoryPages(queryCtx, q.Limit, func(offset, limit int32) ([]rdsmodel.TopSql, int32, error) {
		return c.history.ListTopSQL(queryCtx, cred, q.ProjectID, q.Region, q.InstanceID, q.From*1000, q.To*1000, offset, limit)
	})
	if err != nil {
		reason := evidenceErrorReason(err, "rds history top sql query failed")
		out.MissingEvidence = appendPostgresMissing(out.MissingEvidence, "postgres_top_sql")
		out.CollectionErrors = append(out.CollectionErrors, "top_sql:"+reason)
	} else {
		out.TopSQLTotalCount = topTotal
		for _, item := range topSQL {
			out.TopSQL = append(out.TopSQL, mapPostgresTopSQL(item))
		}
		out.Truncated = out.Truncated || postgresHistoryTruncated(topTotal, len(topSQL), q.Limit)
	}
	return out, nil
}

func normalizePostgresEvidenceQuery(q aiapp.PostgresEvidenceQuery) aiapp.PostgresEvidenceQuery {
	q.AccountID = strings.TrimSpace(q.AccountID)
	q.Region = strings.TrimSpace(q.Region)
	q.ProjectID = strings.TrimSpace(q.ProjectID)
	q.InstanceID = strings.TrimSpace(q.InstanceID)
	q.ResourceID = strings.TrimSpace(q.ResourceID)
	if q.Limit <= 0 || q.Limit > postgresHistoryMaxRows {
		q.Limit = postgresHistoryMaxRows
	}
	return q
}

func validatePostgresEvidenceQuery(q aiapp.PostgresEvidenceQuery) string {
	switch {
	case q.AccountID == "":
		return "no_account"
	case q.Region == "":
		return "no_region"
	case !postgresInstanceIDPattern.MatchString(q.InstanceID):
		return "invalid_rds_instance_id"
	case q.From <= 0 || q.To <= 0 || q.From >= q.To:
		return "invalid_time_range"
	case q.To-q.From > postgresHistoryMaxWindow:
		return "time range exceeds 7 days"
	default:
		return ""
	}
}

func resolvePostgresProjectID(snapshot *obsdomain.AccountSnapshot, q aiapp.PostgresEvidenceQuery) string {
	if q.ProjectID != "" {
		return q.ProjectID
	}
	cfg := obsapp.ParseSyncModeConfig(snapshot.ExtraConfig)
	return strings.TrimSpace(cfg.ResolveProjectID(q.Region, strings.TrimSpace(snapshot.ProjectID)))
}

func degradedPostgresEvidence(status, reason string, q aiapp.PostgresEvidenceQuery) *aiapp.PostgresEvidenceResult {
	return &aiapp.PostgresEvidenceResult{
		Status: status, Reason: reason, ProjectID: q.ProjectID, ResourceID: q.ResourceID,
		InstanceID: q.InstanceID, WindowStart: q.From, WindowEnd: q.To, CollectedAt: time.Now().Unix(),
		MissingEvidence: []string{"postgres_activity", "postgres_wait_events", "postgres_top_sql"},
	}
}

func postgresStatusForError(err error) string {
	reason := strings.ToLower(err.Error())
	if strings.Contains(reason, "permission denied") {
		return aiapp.EvidenceStatusPermissionDenied
	}
	return aiapp.EvidenceStatusError
}

func appendPostgresMissing(existing []string, missing string) []string {
	for _, item := range existing {
		if item == missing {
			return existing
		}
	}
	return append(existing, missing)
}

func postgresHistoryTruncated(total int32, fetched, maxRows int) bool {
	if total > int32(fetched) {
		return true
	}
	return total < 0 && maxRows > 0 && fetched >= maxRows
}

func fetchHistoryPages[T any](ctx context.Context, maxRows int, fetch func(offset, limit int32) ([]T, int32, error)) ([]T, int32, error) {
	if maxRows <= 0 || maxRows > postgresHistoryMaxRows {
		maxRows = postgresHistoryMaxRows
	}
	out := make([]T, 0, maxRows)
	var total int32
	for offset := 0; offset < maxRows; {
		if err := ctx.Err(); err != nil {
			return nil, total, err
		}
		limit := int32(postgresHistoryPageLimit)
		if remaining := maxRows - offset; remaining < int(limit) {
			limit = int32(remaining)
		}
		rows, rowTotal, err := fetch(int32(offset), limit)
		if err != nil {
			return nil, total, err
		}
		total = rowTotal
		out = append(out, rows...)
		if len(rows) < int(limit) || (total > 0 && len(out) >= int(total)) {
			break
		}
		offset += len(rows)
		if len(rows) == 0 {
			break
		}
	}
	return out, total, nil
}
