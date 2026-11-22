package observability

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	huawei "github.com/734965549/aiops/internal/observability/infrastructure/provider/huawei"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
)

type fakeAKSKResolver struct {
	cred huawei.AKSKCredential
	err  error
}

func (f *fakeAKSKResolver) ResolveAKSK(context.Context, obsdomain.AccountSnapshot) (huawei.AKSKCredential, error) {
	if f.err != nil {
		return huawei.AKSKCredential{}, f.err
	}
	return f.cred, nil
}

type fakeRDSHistory struct {
	sessions       []rdsmodel.Session
	sessionTotal   int32
	sessionErr     error
	sessionOffsets []int32
	events         []rdsmodel.Event
	eventTotal     int32
	eventErr       error
	topSQL         []rdsmodel.TopSql
	topTotal       int32
	topErr         error
	projectID      string
	region         string
	instanceID     string
	startMS        int64
	endMS          int64
}

func (f *fakeRDSHistory) ListSessions(_ context.Context, _ huawei.AKSKCredential, projectID, region, instanceID string, startMS, endMS int64, offset, _ int32) ([]rdsmodel.Session, int32, error) {
	f.projectID, f.region, f.instanceID = projectID, region, instanceID
	f.startMS, f.endMS = startMS, endMS
	f.sessionOffsets = append(f.sessionOffsets, offset)
	if f.sessionErr != nil {
		return nil, 0, f.sessionErr
	}
	if offset == 0 && len(f.sessions) > 100 {
		return f.sessions[:100], f.sessionTotal, nil
	}
	if int(offset) < len(f.sessions) {
		return f.sessions[offset:], f.sessionTotal, nil
	}
	return nil, f.sessionTotal, nil
}

func (f *fakeRDSHistory) ListWaitEvents(_ context.Context, _ huawei.AKSKCredential, _, _, _ string, _, _ int64, _, _ int32) ([]rdsmodel.Event, int32, error) {
	return f.events, f.eventTotal, f.eventErr
}

func (f *fakeRDSHistory) ListTopSQL(_ context.Context, _ huawei.AKSKCredential, _, _, _ string, _, _ int64, _, _ int32) ([]rdsmodel.TopSql, int32, error) {
	return f.topSQL, f.topTotal, f.topErr
}

func TestPostgresCollectorPaginatesAndRedactsHistory(t *testing.T) {
	rawSQL := "select * from customer where phone='13800000000'"
	sessions := make([]rdsmodel.Session, 101)
	for i := range sessions {
		sessions[i] = rdsmodel.Session{SessionId: historyInt32(int32(i + 1))}
	}
	sessions[0].UserName = historyString("app-user")
	sessions[0].ClientIpAddress = historyString("10.1.2.3")
	sessions[0].SqlStatement = historyString(rawSQL)
	sessions[0].QueryId = historyString("query-1")
	history := &fakeRDSHistory{
		sessions: sessions, sessionTotal: 101,
		events: []rdsmodel.Event{{WaitEventType: historyString("Lock"), Count: historyInt32(1)}}, eventTotal: 1,
		topSQL: []rdsmodel.TopSql{{SqlStatement: historyString(rawSQL), QueryId: historyString("query-1")}}, topTotal: 1,
	}
	collector := NewPostgresCollector(
		&fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{AccountID: "acc-1", Provider: "huawei_cloud", AuthType: "ak_sk", ProjectID: "proj-1", OwnerTeam: "sre"}},
		&fakeAKSKResolver{cred: huawei.AKSKCredential{AccessKey: "ak", SecretKey: "sk"}},
		history,
		&fakeAccountAccess{},
		time.Second,
	)
	out, err := collector.Collect(context.Background(), "user-1", aiapp.PostgresEvidenceQuery{
		AccountID: "acc-1", Region: "cn-north-4", InstanceID: "30715ce0000000000000000000000000in03",
		From: 1700000000, To: 1700003600, Limit: 101,
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if out.Status != aiapp.EvidenceStatusOK || len(out.Sessions) != 101 || len(out.WaitEvents) != 1 || len(out.TopSQL) != 1 {
		t.Fatalf("unexpected history result: %+v", out)
	}
	if history.projectID != "proj-1" || history.region != "cn-north-4" || history.startMS != 1700000000000 || history.endMS != 1700003600000 {
		t.Fatalf("unexpected request metadata: %+v", history)
	}
	if len(history.sessionOffsets) != 2 || history.sessionOffsets[0] != 0 || history.sessionOffsets[1] != 100 {
		t.Fatalf("unexpected pagination: %+v", history.sessionOffsets)
	}
	blob := fmt.Sprintf("%+v", out)
	if strings.Contains(blob, "app-user") || strings.Contains(blob, "13800000000") || strings.Contains(blob, "10.1.2.3") || strings.Contains(blob, rawSQL) {
		t.Fatalf("sensitive history fields leaked: %s", blob)
	}
	if out.Sessions[0].ClientAddress != "10.1.2.0/24" || out.Sessions[0].SQLHash == "" || out.Sessions[0].UserHash == "" {
		t.Fatalf("unexpected redaction: %+v", out.Sessions[0])
	}
	if out.Sessions[0].SQLOperation != "SELECT" || out.Sessions[0].SQLSummary != "select * from customer where phone=?" ||
		out.TopSQL[0].SQLOperation != "SELECT" || out.TopSQL[0].SQLSummary != "select * from customer where phone=?" {
		t.Fatalf("unexpected SQL summary: session=%+v top=%+v", out.Sessions[0], out.TopSQL[0])
	}
}

func TestPostgresCollectorKeepsPartialEvidenceVisible(t *testing.T) {
	history := &fakeRDSHistory{
		sessions: []rdsmodel.Session{{SessionId: historyInt32(1), SessionStatus: historyString("active")}}, sessionTotal: 1,
		eventErr: errors.New("provider permission denied"),
	}
	collector := NewPostgresCollector(
		&fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{AccountID: "acc-1", AuthType: "ak_sk", ProjectID: "proj-1"}},
		&fakeAKSKResolver{cred: huawei.AKSKCredential{AccessKey: "ak", SecretKey: "sk"}}, history, nil, time.Second,
	)
	out, err := collector.Collect(context.Background(), "", aiapp.PostgresEvidenceQuery{AccountID: "acc-1", Region: "cn-north-4", InstanceID: "instance-1", From: 1, To: 2})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != aiapp.EvidenceStatusOK || len(out.Sessions) != 1 || len(out.MissingEvidence) != 1 || out.MissingEvidence[0] != "postgres_wait_events" {
		t.Fatalf("partial failure must remain visible: %+v", out)
	}
}

func TestPostgresCollectorSessionPermissionDenied(t *testing.T) {
	history := &fakeRDSHistory{sessionErr: errors.New("provider permission denied")}
	collector := NewPostgresCollector(
		&fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{AccountID: "acc-1", AuthType: "ak_sk", ProjectID: "proj-1"}},
		&fakeAKSKResolver{cred: huawei.AKSKCredential{AccessKey: "ak", SecretKey: "sk"}}, history, nil, time.Second,
	)
	out, err := collector.Collect(context.Background(), "", aiapp.PostgresEvidenceQuery{AccountID: "acc-1", Region: "cn-north-4", InstanceID: "instance-1", From: 1, To: 2})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != aiapp.EvidenceStatusPermissionDenied || len(out.MissingEvidence) == 0 {
		t.Fatalf("unexpected permission result: %+v", out)
	}
}

func TestPostgresCollectorRejectsInvalidWindow(t *testing.T) {
	collector := NewPostgresCollector(&fakeAccountResolver{}, &fakeAKSKResolver{}, &fakeRDSHistory{}, nil, time.Second)
	out, err := collector.Collect(context.Background(), "", aiapp.PostgresEvidenceQuery{AccountID: "acc-1", Region: "cn-north-4", InstanceID: "instance-1", From: 1, To: 8*24*3600 + 2})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != aiapp.EvidenceStatusSkipped || out.Reason != "time range exceeds 7 days" {
		t.Fatalf("unexpected window validation: %+v", out)
	}
}

func TestPostgresHistoryTruncatedWhenTotalUnknown(t *testing.T) {
	if !postgresHistoryTruncated(-1, 300, 300) {
		t.Fatal("unknown total with full budget must be marked truncated")
	}
	if postgresHistoryTruncated(-1, 10, 300) {
		t.Fatal("partial page with unknown total must not be marked truncated")
	}
}

func TestFetchHistoryPagesStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := fetchHistoryPages(ctx, 100, func(offset, limit int32) ([]rdsmodel.Session, int32, error) {
		t.Fatal("canceled context must not call RDS")
		return nil, 0, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestSummarizePostgresSQLMasksSensitiveContent(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		operation string
		summary   string
	}{
		{
			name:      "cte strings dollar quotes and numbers",
			raw:       `/* leading */ WITH x AS (SELECT 'hidden' AS v) UPDATE public.orders SET note = E'alice\'secret', payload = $$private$$, amount = 1.2e3 WHERE id = 7`,
			operation: "UPDATE",
			summary:   "WITH x AS (SELECT ? AS v) UPDATE public.orders SET note = ?, payload = ?, amount = ? WHERE id = ?",
		},
		{
			name:      "nested comments",
			raw:       `DELETE FROM sessions WHERE token = 'secret' /* outer /* nested */ still comment */ AND id IN (1, 2)`,
			operation: "DELETE",
			summary:   "DELETE FROM sessions WHERE token = ? AND id IN (?, ?)",
		},
		{
			name:      "line comment and identifier digits",
			raw:       "-- private\nSELECT col1 FROM table2 WHERE id=42 AND note=$tag$hidden$tag$",
			operation: "SELECT",
			summary:   "SELECT col1 FROM table2 WHERE id=? AND note=?",
		},
		{name: "unterminated string", raw: `SELECT 'do-not-leak`, operation: "SELECT", summary: "SELECT ?"},
		{name: "unterminated quoted identifier", raw: `SELECT "do-not-leak`, operation: "SELECT", summary: `SELECT "?"`},
		{name: "empty", raw: " /* only a comment */ ", operation: "", summary: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			operation, summary := summarizePostgresSQL(tc.raw)
			if operation != tc.operation || summary != tc.summary {
				t.Fatalf("summarizePostgresSQL() = (%q, %q), want (%q, %q)", operation, summary, tc.operation, tc.summary)
			}
		})
	}
}

func TestSummarizePostgresSQLTruncatesToThreeHundredRunes(t *testing.T) {
	operation, summary := summarizePostgresSQL("SELECT " + strings.Repeat("表", 400))
	if operation != "SELECT" || len([]rune(summary)) != 300 {
		t.Fatalf("unexpected bounded summary: operation=%q runes=%d", operation, len([]rune(summary)))
	}
}

func historyString(value string) *string { return &value }
func historyInt32(value int32) *int32    { return &value }
