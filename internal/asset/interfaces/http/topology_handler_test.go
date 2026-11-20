package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	assetapp "github.com/734965549/aiops/internal/asset/application"
	assetdomain "github.com/734965549/aiops/internal/asset/domain"
)

type assetHTTPTestRelationRepo struct {
	rows []assetdomain.ResourceRelation
}

func (r *assetHTTPTestRelationRepo) Create(_ context.Context, relation *assetdomain.ResourceRelation) error {
	r.rows = append(r.rows, *relation)
	return nil
}

func (r *assetHTTPTestRelationRepo) GetByID(_ context.Context, id string) (*assetdomain.ResourceRelation, error) {
	for _, row := range r.rows {
		if row.ID == id {
			cp := row
			return &cp, nil
		}
	}
	return nil, assetdomain.ErrNotFound
}

func (r *assetHTTPTestRelationRepo) Delete(_ context.Context, id string) error {
	for i, row := range r.rows {
		if row.ID == id {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return assetdomain.ErrNotFound
}

func (r *assetHTTPTestRelationRepo) ListByResourceIDs(_ context.Context, ids []string) ([]assetdomain.ResourceRelation, error) {
	included := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		included[id] = struct{}{}
	}
	out := make([]assetdomain.ResourceRelation, 0)
	for _, row := range r.rows {
		_, source := included[row.SourceResourceID]
		_, target := included[row.TargetResourceID]
		if source || target {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *assetHTTPTestRelationRepo) CountByResourceID(_ context.Context, id string) (int64, error) {
	var count int64
	for _, row := range r.rows {
		if row.SourceResourceID == id || row.TargetResourceID == id {
			count++
		}
	}
	return count, nil
}

func TestTopology_UnauthenticatedAndPermissionDenied(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/assets/topology?application_id=app-1", nil)
	engine, _, _, _ := newAssetHTTPEngine(t, &fakeAssetHTTPAuthorizer{allowed: true})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	engine, token, _, _ := newAssetHTTPEngine(t, &fakeAssetHTTPAuthorizer{allowed: false})
	req = httptest.NewRequest(http.MethodGet, "/api/assets/topology?application_id=app-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestTopologyAndManualRelationLifecycle(t *testing.T) {
	engine, token, apps, resources := newAssetHTTPEngine(t, &fakeAssetHTTPAuthorizer{allowed: true})
	apps.apps = append(apps.apps, assetdomain.Application{ID: "app-1", Name: "payment", Environment: "prod"})
	resources.rows = append(resources.rows,
		assetdomain.Resource{ID: "res-1", ApplicationID: "app-1", Name: "gateway", SyncStatus: "active"},
		assetdomain.Resource{ID: "res-2", ApplicationID: "app-1", Name: "database", SyncStatus: "active"},
	)

	topology := getTopologyForTest(t, engine, token)
	if len(topology.Nodes) != 3 || len(topology.Edges) != 2 {
		t.Fatalf("unexpected registry topology: %+v", topology)
	}

	body := []byte(`{"source_resource_id":"res-1","target_resource_id":"res-2","relation_type":"depends_on"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/assets/resource-relations", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var created assetapp.ResourceRelationDTO
	if err := json.Unmarshal(decodeAssetEnvelope(t, w.Body.Bytes()).Data, &created); err != nil {
		t.Fatalf("decode created relation: %v", err)
	}
	if created.ID == "" || created.Origin != assetdomain.RelationOriginManual {
		t.Fatalf("unexpected relation: %+v", created)
	}

	topology = getTopologyForTest(t, engine, token)
	if len(topology.Edges) != 3 {
		t.Fatalf("manual edge missing: %+v", topology)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/assets/resource-relations/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateRelation_ValidationFailure(t *testing.T) {
	engine, token, apps, resources := newAssetHTTPEngine(t, &fakeAssetHTTPAuthorizer{allowed: true})
	apps.apps = append(apps.apps, assetdomain.Application{ID: "app-1", Name: "payment"})
	resources.rows = append(resources.rows, assetdomain.Resource{ID: "res-1", ApplicationID: "app-1"})
	body := []byte(`{"source_resource_id":"res-1","target_resource_id":"res-1","relation_type":"depends_on"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/assets/resource-relations", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestTopology_InvalidDepthReturnsBadRequest(t *testing.T) {
	engine, token, _, _ := newAssetHTTPEngine(t, &fakeAssetHTTPAuthorizer{allowed: true})
	req := httptest.NewRequest(http.MethodGet, "/api/assets/topology?root_resource_id=res-1&depth=abc", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func getTopologyForTest(t *testing.T, engine http.Handler, token string) assetapp.TopologyDTO {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/assets/topology?application_id=app-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("topology status=%d body=%s", w.Code, w.Body.String())
	}
	var topology assetapp.TopologyDTO
	if err := json.Unmarshal(decodeAssetEnvelope(t, w.Body.Bytes()).Data, &topology); err != nil {
		t.Fatalf("decode topology: %v", err)
	}
	return topology
}
