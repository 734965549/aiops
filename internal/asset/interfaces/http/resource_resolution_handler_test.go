package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	assetapp "github.com/734965549/aiops/internal/asset/application"
	assetdomain "github.com/734965549/aiops/internal/asset/domain"
)

func TestResolveResource_SuccessAndValidation(t *testing.T) {
	engine, token, apps, resources := newAssetHTTPEngine(t, &fakeAssetHTTPAuthorizer{allowed: true})
	apps.apps = append(apps.apps, assetdomain.Application{ID: "app-1", Name: "payment-service"})
	resources.rows = append(resources.rows, assetdomain.Resource{
		ID: "rds-1", ApplicationID: "app-1", Name: "payment-db", Source: assetdomain.ResourceSourceCloudSync,
		IntegrationAccountID: "acc-1", Region: "cn-south-1", CloudResourceType: "rds", CloudResourceID: "rds-cloud-1",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/assets/resources/resolve?cloud_resource_id=rds-cloud-1&account_id=acc-1&region=cn-south-1&cloud_resource_type=rds", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var result assetapp.ResourceResolutionDTO
	if err := json.Unmarshal(decodeAssetEnvelope(t, w.Body.Bytes()).Data, &result); err != nil {
		t.Fatalf("decode resolution: %v", err)
	}
	if result.Status != "matched" || result.Candidates[0].ApplicationName != "payment-service" {
		t.Fatalf("unexpected resolution: %+v", result)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/assets/resources/resolve?cloud_resource_id=rds-cloud-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("validation status=%d body=%s", w.Code, w.Body.String())
	}
}
