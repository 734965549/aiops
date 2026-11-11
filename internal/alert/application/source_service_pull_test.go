package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type sourceTestRepo struct {
	byID map[string]*domain.AlertSource
}

func newSourceTestRepo() *sourceTestRepo {
	return &sourceTestRepo{byID: map[string]*domain.AlertSource{}}
}
func (r *sourceTestRepo) Create(_ context.Context, src *domain.AlertSource) error {
	cp := *src
	r.byID[src.ID] = &cp
	return nil
}
func (r *sourceTestRepo) Update(_ context.Context, src *domain.AlertSource) error {
	cp := *src
	r.byID[src.ID] = &cp
	return nil
}
func (r *sourceTestRepo) GetByID(_ context.Context, id string) (*domain.AlertSource, error) {
	src, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *src
	return &cp, nil
}
func (r *sourceTestRepo) List(context.Context) ([]domain.AlertSource, error) { return nil, nil }
func (r *sourceTestRepo) Delete(context.Context, string) error               { return nil }

type sourceTestAccounts struct {
	info *IntegrationAccountInfo
	err  error
}

func (a *sourceTestAccounts) GetAccount(context.Context, string) (*IntegrationAccountInfo, error) {
	return a.info, a.err
}

func validAccounts() *sourceTestAccounts {
	return &sourceTestAccounts{info: &IntegrationAccountInfo{
		AccountID: "acc-1", Provider: "huawei_cloud", AuthType: "ak_sk",
		Regions: []string{"cn-north-4"}, ProjectID: "proj-1",
		Capabilities: []string{"alerts"}, Enabled: true,
	}}
}

func newPullSourceService(t *testing.T, accounts *sourceTestAccounts) (*SourceService, *sourceTestRepo) {
	t.Helper()
	repo := newSourceTestRepo()
	svc := NewSourceService(repo, NoopAuditRecorder{})
	svc.SetAccountReader(accounts)
	svc.SetAllowFakeAccounts(true)
	return svc, repo
}

func TestSourceService_CreatePullSource(t *testing.T) {
	svc, _ := newPullSourceService(t, validAccounts())
	out, err := svc.Create(context.Background(), Actor{UserID: "u-1"}, CreateSourceInput{
		ID: "ces-src", Name: "CES", Type: "huawei_ces",
		DeliveryMode: domain.DeliveryModePull, AccountID: "acc-1",
		PullConfig: &domain.PullConfig{PullIntervalSeconds: 300, RegionFilter: []string{"CN-North-4"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.DeliveryMode != string(domain.DeliveryModePull) || out.AccountID != "acc-1" {
		t.Fatalf("unexpected dto: %+v", out)
	}
	if out.PullConfig == nil || out.PullConfig.PullIntervalSeconds != 300 {
		t.Fatalf("expected default interval 300, got %+v", out.PullConfig)
	}
	if len(out.PullConfig.RegionFilter) != 1 || out.PullConfig.RegionFilter[0] != "cn-north-4" {
		t.Fatalf("unexpected region filter: %+v", out.PullConfig.RegionFilter)
	}
}

func TestSourceService_CreatePullSourceDefaultInterval(t *testing.T) {
	svc, _ := newPullSourceService(t, validAccounts())
	out, err := svc.Create(context.Background(), Actor{}, CreateSourceInput{
		ID: "ces-src", Name: "CES", Type: "huawei_ces",
		DeliveryMode: domain.DeliveryModePull, AccountID: "acc-1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.PullConfig == nil || out.PullConfig.PullIntervalSeconds != 300 {
		t.Fatalf("expected default interval 300, got %+v", out.PullConfig)
	}
}

func TestSourceService_PullValidationRejectsInvalidScopeAndAuth(t *testing.T) {
	svc, _ := newPullSourceService(t, validAccounts())
	cases := []CreateSourceInput{
		{ID: "s1", Name: "n", Type: "huawei_ces", DeliveryMode: domain.DeliveryModePull, AccountID: "acc-1",
			PullConfig: &domain.PullConfig{RegionFilter: []string{"cn-south-1"}}},
		{ID: "s2", Name: "n", Type: "huawei_ces", DeliveryMode: domain.DeliveryModePull, AccountID: "acc-1", Secret: "x"},
	}
	for _, in := range cases {
		if _, err := svc.Create(context.Background(), Actor{}, in); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidArgument {
			t.Fatalf("expected INVALID_ARGUMENT for %+v, got %v", in, err)
		}
	}
	agency := &sourceTestAccounts{info: &IntegrationAccountInfo{
		AccountID: "acc-1", Provider: "huawei_cloud", AuthType: "agency",
		Regions: []string{"cn-north-4"}, ProjectID: "proj-1", Capabilities: []string{"alerts"}, Enabled: true,
	}}
	svc2, _ := newPullSourceService(t, agency)
	if _, err := svc2.Create(context.Background(), Actor{}, CreateSourceInput{
		ID: "s3", Name: "n", Type: "huawei_ces", DeliveryMode: domain.DeliveryModePull, AccountID: "acc-1",
	}); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("expected INVALID_ARGUMENT for agency auth, got %v", err)
	}
}

func TestSourceService_WebhookRejectsPullConfig(t *testing.T) {
	svc, _ := newPullSourceService(t, validAccounts())
	_, err := svc.Create(context.Background(), Actor{}, CreateSourceInput{
		ID: "am", Name: "AM", Secret: "secret",
		PullConfig: &domain.PullConfig{RegionFilter: []string{"cn-north-4"}},
	})
	if err == nil || apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", err)
	}
}

func TestSourceService_PullImmutability(t *testing.T) {
	svc, _ := newPullSourceService(t, validAccounts())
	ctx := context.Background()
	if _, err := svc.Create(ctx, Actor{}, CreateSourceInput{
		ID: "ces-src", Name: "CES", Type: "huawei_ces",
		DeliveryMode: domain.DeliveryModePull, AccountID: "acc-1",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	mode := domain.DeliveryModeWebhook
	if _, err := svc.Update(ctx, "ces-src", Actor{}, UpdateSourceInput{DeliveryMode: &mode}); err == nil {
		t.Fatal("expected delivery_mode change rejected")
	}
	account := "acc-2"
	if _, err := svc.Update(ctx, "ces-src", Actor{}, UpdateSourceInput{AccountID: &account}); err == nil {
		t.Fatal("expected account_id change rejected")
	}
	typ := "prometheus_alertmanager"
	if _, err := svc.Update(ctx, "ces-src", Actor{}, UpdateSourceInput{Type: &typ}); err == nil {
		t.Fatal("expected pull type change rejected")
	}
	out, err := svc.Update(ctx, "ces-src", Actor{}, UpdateSourceInput{
		PullConfig: &domain.PullConfig{PullIntervalSeconds: 0},
	})
	if err != nil {
		t.Fatalf("manual-only update: %v", err)
	}
	if out.PullConfig == nil || out.PullConfig.PullIntervalSeconds != 0 {
		t.Fatalf("expected manual-only interval 0, got %+v", out.PullConfig)
	}
}
