package observability

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	huawei "github.com/734965549/aiops/internal/observability/infrastructure/provider/huawei"
	apperr "github.com/734965549/aiops/pkg/errors"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
)

type fakeRDSBackupClient struct {
	policy    *rdsmodel.BackupPolicy
	policyErr error
	records   []rdsmodel.BackupForList
	total     int32
	listErr   error
	begin     string
	end       string
	limit     int32
	instance  string
}

func (f *fakeRDSBackupClient) ShowBackupPolicy(_ context.Context, _ huawei.AKSKCredential, _, _, instanceID string) (*rdsmodel.BackupPolicy, error) {
	f.instance = instanceID
	return f.policy, f.policyErr
}

func (f *fakeRDSBackupClient) ListBackups(_ context.Context, _ huawei.AKSKCredential, _, _, instanceID, begin, end string, limit int32) ([]rdsmodel.BackupForList, int32, error) {
	f.instance = instanceID
	f.begin, f.end, f.limit = begin, end, limit
	return f.records, f.total, f.listErr
}

func rdsBackupCollectorForTest(client huawei.RDSBackupClient, snapshot *obsdomain.AccountSnapshot, access *fakeAccountAccess) *RDSBackupCollector {
	collector := NewRDSBackupCollector(
		&fakeAccountResolver{snapshot: snapshot},
		&fakeAKSKResolver{cred: huawei.AKSKCredential{}},
		client, access, time.Second,
	)
	collector.now = func() time.Time {
		return time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	}
	return collector
}

func TestRDSBackupCollectorMapsPolicyAndRecords(t *testing.T) {
	start := "04:00-05:00"
	period := "1,2,3,4,5"
	client := &fakeRDSBackupClient{
		policy: &rdsmodel.BackupPolicy{StartTime: &start, Period: &period, KeepDays: 7},
		records: []rdsmodel.BackupForList{{
			Id: "backup-1", Name: "auto-1", InstanceId: "instance-1",
			BeginTime: "2026-11-20T04:05:00Z", EndTime: "2026-11-20T04:20:00Z",
			Status: rdsmodel.GetBackupForListStatusEnum().COMPLETED,
			Type:   rdsmodel.GetBackupForListTypeEnum().AUTO,
			Size:   1024,
		}},
		total: 1,
	}
	access := &fakeAccountAccess{}
	collector := rdsBackupCollectorForTest(client, &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "ak_sk", OwnerTeam: "sre",
		Regions: []string{"cn-north-4"}, ProjectID: "p-1", Provider: "huawei",
	}, access)
	out, err := collector.Query(context.Background(), "u-1", aiapp.RDSBackupQuery{
		AccountID: "acc-1", Region: "cn-north-4", InstanceID: "instance-1",
		Days: 7, Limit: 10, Timezone: "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if out.Status != aiapp.EvidenceStatusOK || out.InstanceID != "instance-1" || out.Timezone != "Asia/Shanghai" {
		t.Fatalf("result = %+v", out)
	}
	if !access.called {
		t.Fatal("account access not checked")
	}
	if out.Policy == nil || out.Policy.StartTimeUTC != start || out.Policy.WindowLocal != "12:00-13:00" {
		t.Fatalf("policy = %+v", out.Policy)
	}
	if out.Policy.Period != period || out.Policy.KeepDays != 7 {
		t.Fatalf("policy fields = %+v", out.Policy)
	}
	if client.begin == "" || client.end == "" || client.limit != 10 || client.instance != "instance-1" {
		t.Fatalf("backup list request = %+v", client)
	}
	if out.BackupTotal != 1 || len(out.Backups) != 1 {
		t.Fatalf("backups = %+v", out)
	}
	if out.Backups[0].BeginTime != "2026-11-20 12:05" || out.Backups[0].Type != "auto" || out.Backups[0].Status != "COMPLETED" {
		t.Fatalf("backup record = %+v", out.Backups[0])
	}
}

func TestRDSBackupCollectorRejectsAgencyAndPermission(t *testing.T) {
	agency := rdsBackupCollectorForTest(&fakeRDSBackupClient{}, &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "agency",
	}, &fakeAccountAccess{})
	out, err := agency.Query(context.Background(), "u-1", aiapp.RDSBackupQuery{
		AccountID: "acc-1", Region: "cn-north-4", InstanceID: "instance-1",
	})
	if err != nil || out.Status != aiapp.EvidenceStatusUnsupported {
		t.Fatalf("agency = %+v err=%v", out, err)
	}

	denied := rdsBackupCollectorForTest(&fakeRDSBackupClient{}, &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "ak_sk", OwnerTeam: "sre", ProjectID: "p-1",
	}, &fakeAccountAccess{err: apperr.New(apperr.CodePermissionDenied, "denied")})
	out, err = denied.Query(context.Background(), "u-1", aiapp.RDSBackupQuery{
		AccountID: "acc-1", Region: "cn-north-4", InstanceID: "instance-1",
	})
	if err != nil || out.Status != aiapp.EvidenceStatusPermissionDenied {
		t.Fatalf("permission = %+v err=%v", out, err)
	}
}

func TestRDSBackupCollectorKeepsPolicyWhenBackupListFails(t *testing.T) {
	start := "04:00-05:00"
	client := &fakeRDSBackupClient{
		policy:  &rdsmodel.BackupPolicy{StartTime: &start, KeepDays: 7},
		listErr: errors.New("backup list failed"),
	}
	collector := rdsBackupCollectorForTest(client, &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "ak_sk", ProjectID: "p-1", Provider: "huawei",
	}, &fakeAccountAccess{})
	out, err := collector.Query(context.Background(), "u-1", aiapp.RDSBackupQuery{
		AccountID: "acc-1", Region: "cn-north-4", InstanceID: "instance-1",
		Days: 7, Limit: 10, Timezone: "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if out.Status != aiapp.EvidenceStatusOK || out.Policy == nil {
		t.Fatalf("policy should be kept: %+v", out)
	}
	if len(out.CollectionErrors) != 1 || !strings.Contains(out.CollectionErrors[0], "backups:") {
		t.Fatalf("collection errors = %+v", out.CollectionErrors)
	}
}

func TestLocalBackupWindowCrossesMidnight(t *testing.T) {
	if got := localBackupWindow("20:00-21:00", time.FixedZone("CST", 8*3600)); got != "04:00-05:00" {
		t.Fatalf("window = %q", got)
	}
	if got := localBackupWindow("23:00-00:00", time.UTC); got != "23:00-00:00(次日)" {
		t.Fatalf("window = %q", got)
	}
	if got := localBackupWindow("bad", time.UTC); got != "" {
		t.Fatalf("window = %q", got)
	}
}
