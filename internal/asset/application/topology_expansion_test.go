package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/asset/domain"
)

func cloudTopologyResources() []domain.Resource {
	return []domain.Resource{
		{ID: "vpc-1", ApplicationID: "app-1", Name: "core-vpc", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-1", Region: "cn-north-4", CloudResourceType: "vpc", CloudResourceID: "vpc-cloud-1", SyncStatus: "active"},
		{ID: "ecs-1", ApplicationID: "app-1", Name: "gateway", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-1", Region: "cn-north-4", CloudResourceType: "ecs", CloudResourceID: "ecs-cloud-1", SyncStatus: "active", Labels: map[string]string{"vpc_id": "vpc-cloud-1"}},
		{ID: "evs-1", ApplicationID: "app-1", Name: "data-volume", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-1", Region: "cn-north-4", CloudResourceType: "evs", CloudResourceID: "evs-cloud-1", SyncStatus: "active", Labels: map[string]string{"attached_to": "ecs-cloud-1"}},
		{ID: "subnet-1", ApplicationID: "app-1", Name: "core-subnet", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-1", Region: "cn-north-4", CloudResourceType: "subnet", CloudResourceID: "subnet-cloud-1", SyncStatus: "active", Labels: map[string]string{"vpc_id": "vpc-cloud-1"}},
		{ID: "rds-1", ApplicationID: "app-1", Name: "payment-db", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-1", Region: "cn-north-4", CloudResourceType: "rds", CloudResourceID: "rds-cloud-1", SyncStatus: "active", Labels: map[string]string{"subnet_id": "subnet-cloud-1"}},
	}
}

func TestDeriveCloudRelationsUsesVerifiedFieldsOnly(t *testing.T) {
	resources := cloudTopologyResources()
	resources = append(resources, domain.Resource{
		ID: "ecs-other-region", ApplicationID: "app-1", Source: domain.ResourceSourceCloudSync,
		IntegrationAccountID: "acc-1", Region: "cn-south-1", CloudResourceType: "ecs", CloudResourceID: "ecs-other",
		Labels: map[string]string{"vpc_id": "vpc-cloud-1"},
	})
	result := deriveCloudRelations(resources)
	if len(result.relations) != 4 {
		t.Fatalf("expected four verified relations, got %+v", result.relations)
	}
	origins := map[string]int{}
	for _, relation := range result.relations {
		origins[relation.Origin]++
		if relation.SourceResourceID == "ecs-other-region" {
			t.Fatal("cross-region relation must not be inferred")
		}
	}
	if origins[domain.RelationOriginCloudSync] != 4 || len(result.unresolved) != 1 {
		t.Fatalf("unexpected origin or unresolved result: origins=%v unresolved=%v", origins, result.unresolved)
	}
}

func TestTopologyService_ApplicationIncludesCloudDerivedEdges(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{"payment": {ID: "app-1", Name: "payment"}}}
	resources := &fakeResRepo{rows: cloudTopologyResources()}
	svc := NewTopologyService(apps, resources, &fakeRelationRepo{}, NoopAuditRecorder{})
	topology, err := svc.GetTopology(context.Background(), TopologyQuery{ApplicationID: "app-1"})
	if err != nil {
		t.Fatalf("get topology: %v", err)
	}
	if topology.Meta.UnresolvedRelationCount != 0 {
		t.Fatalf("unexpected unresolved relations: %+v", topology.Meta)
	}
	cloudEdges := 0
	for _, edge := range topology.Edges {
		if edge.Origin == domain.RelationOriginCloudSync {
			cloudEdges++
		}
	}
	if cloudEdges != 4 {
		t.Fatalf("expected four cloud edges, got %+v", topology.Edges)
	}
	filtered, err := svc.GetTopology(context.Background(), TopologyQuery{ApplicationID: "app-1", Keyword: "gateway"})
	if err != nil {
		t.Fatalf("filter topology: %v", err)
	}
	if filtered.Meta.UnresolvedRelationCount != 0 {
		t.Fatalf("filtered targets must not be reported as unresolved: %+v", filtered.Meta)
	}
}

func TestTopologyService_CreateCrossApplicationRelation(t *testing.T) {
	apps, resources, relations := topologyTestRepos()
	resources.rows[1].SyncStatus = "active"
	resources.rows = append(resources.rows, domain.Resource{ID: "res-other", ApplicationID: "app-2", Name: "analytics"})
	relations.rows = nil
	svc := NewTopologyService(apps, resources, relations, NoopAuditRecorder{})

	created, err := svc.CreateRelation(context.Background(), Actor{UserID: "user-1"}, CreateResourceRelationInput{
		SourceResourceID: "res-1", TargetResourceID: "res-other", RelationType: domain.RelationDependsOn,
	})
	if err != nil {
		t.Fatalf("create cross-application relation: %v", err)
	}
	if created.SourceApplicationID != "app-1" || created.TargetApplicationID != "app-2" {
		t.Fatalf("application context missing: %+v", created)
	}
	items, err := svc.ListRelations(context.Background(), "app-1", "")
	if err != nil {
		t.Fatalf("list relations: %v", err)
	}
	if len(items) != 1 || items[0].TargetName != "analytics" || items[0].TargetApplicationID != "app-2" {
		t.Fatalf("cross-application relation not visible: %+v", items)
	}
}

func TestTopologyService_RootQueryExpandsAcrossApplications(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment":   {ID: "app-1", Name: "payment"},
		"analytics": {ID: "app-2", Name: "analytics"},
	}}
	resources := &fakeResRepo{rows: []domain.Resource{
		{ID: "res-1", ApplicationID: "app-1", Name: "gateway", SyncStatus: "active"},
		{ID: "res-2", ApplicationID: "app-1", Name: "payment-api", SyncStatus: "active"},
		{ID: "res-3", ApplicationID: "app-2", Name: "analytics-api", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-1", Region: "cn-north-4", CloudResourceType: "ecs", CloudResourceID: "ecs-3", SyncStatus: "active", Labels: map[string]string{"vpc_id": "vpc-cloud-1"}},
		{ID: "vpc-1", ApplicationID: "app-2", Name: "core-vpc", Source: domain.ResourceSourceCloudSync, IntegrationAccountID: "acc-1", Region: "cn-north-4", CloudResourceType: "vpc", CloudResourceID: "vpc-cloud-1", SyncStatus: "active"},
	}}
	relations := &fakeRelationRepo{rows: []domain.ResourceRelation{
		{ID: "rel-1", SourceResourceID: "res-1", TargetResourceID: "res-2", RelationType: domain.RelationDependsOn, Origin: domain.RelationOriginManual},
		{ID: "rel-2", SourceResourceID: "res-2", TargetResourceID: "res-3", RelationType: domain.RelationDependsOn, Origin: domain.RelationOriginManual},
	}}
	svc := NewTopologyService(apps, resources, relations, NoopAuditRecorder{})

	depthTwo, err := svc.GetTopology(context.Background(), TopologyQuery{RootResourceID: "res-1", Depth: 2, Direction: "downstream"})
	if err != nil {
		t.Fatalf("get depth-two topology: %v", err)
	}
	if len(depthTwo.Nodes) != 5 || len(depthTwo.Edges) != 5 || depthTwo.Meta.RootResourceID != "res-1" {
		t.Fatalf("unexpected depth-two topology: nodes=%d edges=%d meta=%+v", len(depthTwo.Nodes), len(depthTwo.Edges), depthTwo.Meta)
	}
	depthThree, err := svc.GetTopology(context.Background(), TopologyQuery{RootResourceID: "res-1", Depth: 3, Direction: "downstream"})
	if err != nil {
		t.Fatalf("get depth-three topology: %v", err)
	}
	if len(depthThree.Nodes) != 6 || len(depthThree.Edges) != 7 {
		t.Fatalf("cloud relation was not expanded at depth three: nodes=%d edges=%d", len(depthThree.Nodes), len(depthThree.Edges))
	}
}
