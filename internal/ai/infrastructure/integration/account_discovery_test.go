package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	integapp "github.com/734965549/aiops/internal/integration/application"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeAccountLister struct {
	rows  []integapp.AccountDTO
	total int64
	err   error
	query integapp.ListAccountsQuery
}

func (f *fakeAccountLister) List(_ context.Context, q integapp.ListAccountsQuery) ([]integapp.AccountDTO, int64, error) {
	f.query = q
	return f.rows, f.total, f.err
}

type fakeAccountAccess struct {
	allowed map[string]bool
	err     error
	refs    []aiapp.AccountRef
}

func (f *fakeAccountAccess) Authorize(_ context.Context, _ string, ref aiapp.AccountRef) error {
	f.refs = append(f.refs, ref)
	if f.err != nil {
		return f.err
	}
	if f.allowed == nil {
		return nil
	}
	if f.allowed[ref.AccountID] {
		return nil
	}
	return apperr.New(apperr.CodePermissionDenied, "denied")
}

func TestAccountDiscoveryFiltersCapsAndSanitizes(t *testing.T) {
	rows := make([]integapp.AccountDTO, 0, 15)
	for i := 0; i < 15; i++ {
		rows = append(rows, integapp.AccountDTO{
			AccountID: fmt.Sprintf("acc-%02d", i), Name: "账号", Provider: "huawei_cloud",
			AuthType: "ak_sk", Regions: []string{"cn-north-4"}, OwnerTeam: "sre",
		})
	}
	lister := &fakeAccountLister{rows: rows, total: 42}
	access := &fakeAccountAccess{allowed: map[string]bool{
		"acc-00": true, "acc-01": true, "acc-02": true, "acc-03": true, "acc-04": true,
		"acc-05": true, "acc-06": true, "acc-07": true, "acc-08": true, "acc-09": true,
	}}
	discovery := NewAccountDiscovery(lister, access, time.Second)

	result, err := discovery.List(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if lister.query.Page != 1 || lister.query.PageSize != 100 || lister.query.Enabled == nil || !*lister.query.Enabled {
		t.Fatalf("unexpected list query: %+v", lister.query)
	}
	if result.Status != aiapp.EvidenceStatusOK || result.Total != 42 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Accounts) != accountDiscoveryMaxAccounts {
		t.Fatalf("accounts capped = %d", len(result.Accounts))
	}
	if len(access.refs) == 0 || access.refs[0].OwnerTeam != "sre" {
		t.Fatalf("authorizer not called with account scope: %+v", access.refs)
	}
	if result.Accounts[0].AccountID != "acc-00" || result.Accounts[0].Regions[0] != "cn-north-4" {
		t.Fatalf("account summary = %+v", result.Accounts[0])
	}
}

func TestAccountDiscoverySkipsDeniedAccounts(t *testing.T) {
	lister := &fakeAccountLister{rows: []integapp.AccountDTO{
		{AccountID: "acc-allowed", Name: "可见"},
		{AccountID: "acc-denied", Name: "不可见"},
	}, total: 2}
	access := &fakeAccountAccess{allowed: map[string]bool{"acc-allowed": true}}
	discovery := NewAccountDiscovery(lister, access, time.Second)

	result, err := discovery.List(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(result.Accounts) != 1 || result.Accounts[0].AccountID != "acc-allowed" {
		t.Fatalf("accounts = %+v", result.Accounts)
	}
}

func TestAccountDiscoveryListErrorDegrades(t *testing.T) {
	lister := &fakeAccountLister{err: apperr.New(apperr.CodeInternal, "db down")}
	discovery := NewAccountDiscovery(lister, nil, time.Second)

	result, err := discovery.List(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("list must degrade without error: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusError || result.Reason == "" {
		t.Fatalf("result = %+v", result)
	}
}
