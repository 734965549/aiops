package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeRelationRepo struct {
	rows []domain.ResourceRelation
}

func (r *fakeRelationRepo) Create(_ context.Context, relation *domain.ResourceRelation) error {
	for _, row := range r.rows {
		if row.SourceResourceID == relation.SourceResourceID && row.TargetResourceID == relation.TargetResourceID && row.RelationType == relation.RelationType {
			return domain.ErrAlreadyExists
		}
	}
	if relation.CreatedAt.IsZero() {
		now := time.Now().UTC()
		relation.CreatedAt = now
		relation.UpdatedAt = now
	}
	r.rows = append(r.rows, *relation)
	return nil
}

func (r *fakeRelationRepo) GetByID(_ context.Context, id string) (*domain.ResourceRelation, error) {
	for _, row := range r.rows {
		if row.ID == id {
			cp := row
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeRelationRepo) Delete(_ context.Context, id string) error {
	for i, row := range r.rows {
		if row.ID == id {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeRelationRepo) ListByResourceIDs(_ context.Context, ids []string) ([]domain.ResourceRelation, error) {
	included := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		included[id] = struct{}{}
	}
	out := make([]domain.ResourceRelation, 0)
	for _, row := range r.rows {
		_, source := included[row.SourceResourceID]
		_, target := included[row.TargetResourceID]
		if source || target {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *fakeRelationRepo) CountByResourceID(_ context.Context, id string) (int64, error) {
	var count int64
	for _, row := range r.rows {
		if row.SourceResourceID == id || row.TargetResourceID == id {
			count++
		}
	}
	return count, nil
}

func topologyTestRepos() (*fakeAppRepo, *fakeResRepo, *fakeRelationRepo) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{
		"payment": {ID: "app-1", Name: "payment", Environment: "prod"},
	}}
	resources := &fakeResRepo{rows: []domain.Resource{
		{ID: "res-1", ApplicationID: "app-1", Name: "gateway", ResourceType: "service", Region: "cn-north-4", SyncStatus: "active"},
		{ID: "res-2", ApplicationID: "app-1", Name: "database", ResourceType: "rds", CloudResourceType: "rds", Region: "cn-north-4", SyncStatus: "stale"},
	}}
	relations := &fakeRelationRepo{rows: []domain.ResourceRelation{{
		ID: "rel-1", SourceResourceID: "res-1", TargetResourceID: "res-2",
		RelationType: domain.RelationDependsOn, Origin: domain.RelationOriginManual,
	}}}
	return apps, resources, relations
}

func TestTopologyService_GetTopologyUsesRegistryAndManualRelations(t *testing.T) {
	apps, resources, relations := topologyTestRepos()
	svc := NewTopologyService(apps, resources, relations, NoopAuditRecorder{})

	withoutStale, err := svc.GetTopology(context.Background(), TopologyQuery{ApplicationID: "app-1"})
	if err != nil {
		t.Fatalf("get topology: %v", err)
	}
	if len(withoutStale.Nodes) != 2 || len(withoutStale.Edges) != 1 {
		t.Fatalf("stale resource should be excluded: %+v", withoutStale)
	}

	topology, err := svc.GetTopology(context.Background(), TopologyQuery{ApplicationID: "app-1", IncludeStale: true})
	if err != nil {
		t.Fatalf("get topology with stale: %v", err)
	}
	if len(topology.Nodes) != 3 || len(topology.Edges) != 3 || topology.Meta.Truncated {
		t.Fatalf("unexpected topology: %+v", topology)
	}
	last := topology.Edges[len(topology.Edges)-1]
	if last.RelationType != domain.RelationDependsOn || last.Origin != domain.RelationOriginManual {
		t.Fatalf("manual relation missing: %+v", last)
	}

	filtered, err := svc.GetTopology(context.Background(), TopologyQuery{ApplicationID: "app-1", Keyword: "gateway"})
	if err != nil {
		t.Fatalf("filter topology: %v", err)
	}
	if len(filtered.Nodes) != 2 || filtered.Nodes[1].EntityID != "res-1" {
		t.Fatalf("unexpected filtered topology: %+v", filtered)
	}
}

func TestTopologyService_TruncatesLargeApplications(t *testing.T) {
	apps := &fakeAppRepo{apps: map[string]domain.Application{"payment": {ID: "app-1", Name: "payment"}}}
	resources := &fakeResRepo{}
	for i := 0; i < maxTopologyResources+1; i++ {
		resources.rows = append(resources.rows, domain.Resource{ID: fmt.Sprintf("res-%d", i), ApplicationID: "app-1"})
	}
	svc := NewTopologyService(apps, resources, &fakeRelationRepo{}, NoopAuditRecorder{})
	topology, err := svc.GetTopology(context.Background(), TopologyQuery{ApplicationID: "app-1"})
	if err != nil {
		t.Fatalf("get topology: %v", err)
	}
	if !topology.Meta.Truncated || len(topology.Nodes) != maxTopologyResources+1 {
		t.Fatalf("expected explicit truncation: meta=%+v nodes=%d", topology.Meta, len(topology.Nodes))
	}
}

func TestTopologyService_CreateAndDeleteRelation(t *testing.T) {
	apps, resources, relations := topologyTestRepos()
	resources.rows[1].SyncStatus = "active"
	relations.rows = nil
	audit := &capturingAssetAudit{}
	svc := NewTopologyService(apps, resources, relations, audit)

	created, err := svc.CreateRelation(context.Background(), Actor{UserID: "user-1"}, CreateResourceRelationInput{
		SourceResourceID: "res-1", TargetResourceID: "res-2", RelationType: domain.RelationDependsOn,
	})
	if err != nil {
		t.Fatalf("create relation: %v", err)
	}
	if created.Origin != domain.RelationOriginManual || created.SourceName != "gateway" || created.TargetName != "database" {
		t.Fatalf("unexpected relation dto: %+v", created)
	}
	if len(audit.rows) != 1 || audit.rows[0].ResourceType != "resource_relation" {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
	if _, err := svc.CreateRelation(context.Background(), Actor{}, CreateResourceRelationInput{
		SourceResourceID: "res-1", TargetResourceID: "res-2", RelationType: domain.RelationDependsOn,
	}); apperr.FromError(err).Code != apperr.CodeAlreadyExists {
		t.Fatalf("expected duplicate conflict, got %v", err)
	}
	if err := svc.DeleteRelation(context.Background(), created.ID, Actor{UserID: "user-1"}); err != nil {
		t.Fatalf("delete relation: %v", err)
	}
	if len(relations.rows) != 0 || len(audit.rows) != 2 {
		t.Fatalf("relation or delete audit missing: rows=%d audit=%d", len(relations.rows), len(audit.rows))
	}
}

func TestTopologyService_RejectsInvalidRelations(t *testing.T) {
	apps, resources, relations := topologyTestRepos()
	svc := NewTopologyService(apps, resources, relations, NoopAuditRecorder{})

	for name, input := range map[string]CreateResourceRelationInput{
		"self": {SourceResourceID: "res-1", TargetResourceID: "res-1", RelationType: domain.RelationDependsOn},
		"type": {SourceResourceID: "res-1", TargetResourceID: "res-2", RelationType: "calls"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.CreateRelation(context.Background(), Actor{}, input)
			if apperr.FromError(err).Code != apperr.CodeInvalidArgument {
				t.Fatalf("expected invalid argument, got %v", err)
			}
		})
	}
}

func TestAssetService_DeleteResourceBlockedByRelation(t *testing.T) {
	apps, resources, relations := topologyTestRepos()
	svc := NewAssetService(apps, resources, nil, nil, nil, NoopAuditRecorder{}, relations)
	err := svc.DeleteResource(context.Background(), "res-1", Actor{UserID: "user-1"})
	if apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
	if len(resources.rows) != 2 {
		t.Fatal("related resource must not be deleted")
	}
}
