package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/asset/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

func TestTopologyService_ResolveResource(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment": {ID: "app-1", Name: "payment-service"},
		"replica": {ID: "app-2", Name: "replica-service"},
	}}
	resources := &fakeResRepo{rows: []domain.Resource{
		{ID: "rds-unique", ApplicationID: "app-1", Name: "payment-db", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-1", Region: "cn-south-1", CloudResourceType: "rds", CloudResourceID: "unique-cloud-id"},
		{ID: "rds-shared-1", ApplicationID: "app-1", Name: "payment-db", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-1", Region: "cn-south-1", CloudResourceType: "rds", CloudResourceID: "shared-cloud-id"},
		{ID: "rds-shared-2", ApplicationID: "app-2", Name: "replica-db", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-1", Region: "cn-south-1", CloudResourceType: "rds", CloudResourceID: "shared-cloud-id"},
		{ID: "rds-other-region", ApplicationID: "app-1", Name: "other-db", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-1", Region: "cn-north-4", CloudResourceType: "rds", CloudResourceID: "shared-cloud-id"},
		{ID: "rds-other-account", ApplicationID: "app-1", Name: "other-account-db", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-2", Region: "cn-south-1", CloudResourceType: "rds", CloudResourceID: "shared-cloud-id"},
	}}
	svc := NewTopologyService(apps, resources, &fakeRelationRepo{}, NoopAuditRecorder{})

	matched, err := svc.ResolveResource(context.Background(), ResolveResourceQuery{CloudResourceID: "unique-cloud-id", AccountID: "acc-1", Region: "cn-south-1", CloudResourceType: "rds"})
	if err != nil {
		t.Fatalf("resolve unique resource: %v", err)
	}
	if matched.Status != "matched" || matched.Total != 1 || matched.Candidates[0].Name != "payment-db" || matched.Candidates[0].ApplicationName != "payment-service" {
		t.Fatalf("unexpected match: %+v", matched)
	}

	ambiguous, err := svc.ResolveResource(context.Background(), ResolveResourceQuery{CloudResourceID: "shared-cloud-id", AccountID: "acc-1", Region: "cn-south-1", CloudResourceType: "rds"})
	if err != nil {
		t.Fatalf("resolve ambiguous resource: %v", err)
	}
	if ambiguous.Status != "ambiguous" || ambiguous.Total != 2 {
		t.Fatalf("account and region filters were not applied: %+v", ambiguous)
	}

	unregistered, err := svc.ResolveResource(context.Background(), ResolveResourceQuery{CloudResourceID: "missing", AccountID: "acc-1"})
	if err != nil {
		t.Fatalf("resolve missing resource: %v", err)
	}
	if unregistered.Status != "unregistered" || unregistered.Total != 0 || unregistered.Candidates == nil {
		t.Fatalf("unexpected unregistered result: %+v", unregistered)
	}
}

func TestTopologyService_ResolveResourceValidation(t *testing.T) {
	svc := NewTopologyService(&fakeAppRepo{}, &fakeResRepo{}, &fakeRelationRepo{}, NoopAuditRecorder{})
	_, err := svc.ResolveResource(context.Background(), ResolveResourceQuery{})
	if apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
