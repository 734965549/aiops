package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/734965549/aiops/internal/ai/toolgateway"
	"github.com/gin-gonic/gin"
)

type mockRegistry struct {
	providers []toolgateway.ProviderConfig
	updated   toolgateway.ProviderConfig
	deletedID string
}

func (m *mockRegistry) ListProviders() []toolgateway.ProviderPublic {
	out := make([]toolgateway.ProviderPublic, 0, len(m.providers))
	for _, p := range m.providers {
		out = append(out, p.Public())
	}
	return out
}
func (m *mockRegistry) UpsertProvider(cfg toolgateway.ProviderConfig) error {
	m.updated = cfg
	return nil
}
func (m *mockRegistry) DeleteProvider(id string) { m.deletedID = id }
func (m *mockRegistry) GetProvider(id string) (toolgateway.ProviderConfig, bool) {
	for _, p := range m.providers {
		if p.ID == id {
			return p, true
		}
	}
	return toolgateway.ProviderConfig{}, false
}

type mockGateway struct{}

func (m *mockGateway) Validate(ctx context.Context, req toolgateway.ToolRequest) (*toolgateway.ToolResponse, error) {
	return &toolgateway.ToolResponse{Allowed: true}, nil
}
func (m *mockGateway) Invoke(ctx context.Context, providerID string, req toolgateway.ToolRequest) (*toolgateway.ToolResponse, error) {
	return &toolgateway.ToolResponse{Allowed: true, Provider: providerID, Data: map[string]any{"ok": true}}, nil
}

func TestListProviders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&mockGateway{}, &mockRegistry{providers: []toolgateway.ProviderConfig{{ID: "demo", Name: "Demo", Type: toolgateway.ProviderTypeA, APIKey: "secret-key"}}}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/providers", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.ListProviders(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.Bytes()
	if !bytes.Contains(body, []byte("demo")) {
		t.Fatalf("expected provider id in response: %s", string(body))
	}
	if bytes.Contains(body, []byte(`"api_key"`)) {
		t.Fatalf("must not return api_key in list response: %s", string(body))
	}
	if !bytes.Contains(body, []byte(`"has_api_key":true`)) {
		t.Fatalf("expected has_api_key true: %s", string(body))
	}
	if bytes.Contains(body, []byte("secret-key")) {
		t.Fatalf("must not leak api_key: %s", string(body))
	}
}

func TestUpsertProvider_UpdateKeepsExistingAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := &mockRegistry{}
	reg.providers = []toolgateway.ProviderConfig{{ID: "demo-http-a", Name: "Old", Type: toolgateway.ProviderTypeA, APIKey: "stored-secret"}}
	h := NewHandler(&mockGateway{}, reg, nil, nil, nil)
	body, _ := json.Marshal(providerRequest{ID: "demo-http-a", Name: "Demo HTTP Provider A", Type: "a", BaseURL: "http://127.0.0.1:9000", TimeoutMS: 30000, Enabled: true})
	req := httptest.NewRequest(http.MethodPost, "/providers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.UpsertProvider(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if reg.updated.APIKey != "stored-secret" {
		t.Fatalf("expected preserved api key, got %q", reg.updated.APIKey)
	}
}

func TestUpsertProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := &mockRegistry{}
	h := NewHandler(&mockGateway{}, reg, nil, nil, nil)
	body, _ := json.Marshal(providerRequest{ID: "demo-http-a", Name: "Demo HTTP Provider A", Type: "a", BaseURL: "http://127.0.0.1:9000", APIKey: "secret", TimeoutMS: 30000, Enabled: true})
	req := httptest.NewRequest(http.MethodPost, "/providers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.UpsertProvider(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if reg.updated.ID != "demo-http-a" || reg.updated.Type != toolgateway.ProviderTypeA {
		t.Fatalf("unexpected updated provider: %+v", reg.updated)
	}
}

func TestInvokeTool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&mockGateway{}, &mockRegistry{}, nil, nil, nil)
	body, _ := json.Marshal(invokeRequest{ProviderID: "demo-http-a", ToolCode: "cmdb.search", Resource: "cmdb", Action: "read"})
	req := httptest.NewRequest(http.MethodPost, "/tools/invoke", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.Invoke(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("demo-http-a")) {
		t.Fatalf("expected provider in response: %s", w.Body.String())
	}
}
