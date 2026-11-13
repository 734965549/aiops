package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	"github.com/734965549/aiops/internal/ai/toolgateway"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/gin-gonic/gin"
)

type mockProviderManager struct {
	providers []toolgateway.ProviderConfig
	upserted  aiapp.UpsertProviderInput
	deletedID string
	listErr   error
	upsertErr error
}

func (m *mockProviderManager) ListProviders(ctx context.Context) ([]toolgateway.ProviderPublic, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	out := make([]toolgateway.ProviderPublic, 0, len(m.providers))
	for _, p := range m.providers {
		out = append(out, p.Public())
	}
	return out, nil
}

func (m *mockProviderManager) UpsertProvider(ctx context.Context, in aiapp.UpsertProviderInput) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	m.upserted = in
	return nil
}

func (m *mockProviderManager) DeleteProvider(ctx context.Context, providerID string) error {
	m.deletedID = providerID
	return nil
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
	mgr := &mockProviderManager{providers: []toolgateway.ProviderConfig{{
		ID: "demo", Name: "Demo", Type: toolgateway.ProviderTypeB, BaseURL: "https://ark.example.com",
		Model: "ep-1", APIKey: "secret-key",
	}}}
	h := NewHandler(&mockGateway{}, mgr, nil, nil, nil)
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
	if !bytes.Contains(body, []byte(`"model":"ep-1"`)) {
		t.Fatalf("expected model in response: %s", string(body))
	}
	if bytes.Contains(body, []byte("secret-key")) {
		t.Fatalf("must not leak api_key: %s", string(body))
	}
}

func TestListProvidersReturnsServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := &mockProviderManager{listErr: apperr.New(apperr.CodeUnavailable, "database down")}
	h := NewHandler(&mockGateway{}, mgr, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/providers", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.ListProviders(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpsertProviderPassesInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := &mockProviderManager{}
	h := NewHandler(&mockGateway{}, mgr, nil, nil, nil)
	body, _ := json.Marshal(providerRequest{
		ID: "ark", Name: "Ark", Type: "b", BaseURL: "https://ark.example.com",
		Model: "ep-1", APIKey: "secret", TimeoutMS: 30000, Enabled: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/providers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.UpsertProvider(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if mgr.upserted.ID != "ark" || mgr.upserted.Model != "ep-1" || mgr.upserted.APIKey != "secret" {
		t.Fatalf("unexpected upsert input: %+v", mgr.upserted)
	}
}

func TestUpsertProviderReturnsServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := &mockProviderManager{upsertErr: apperr.New(apperr.CodeInvalidArgument, "model is required for provider type b")}
	h := NewHandler(&mockGateway{}, mgr, nil, nil, nil)
	body, _ := json.Marshal(providerRequest{ID: "ark", Name: "Ark", Type: "b", BaseURL: "https://ark.example.com", APIKey: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/providers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.UpsertProvider(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := &mockProviderManager{}
	h := NewHandler(&mockGateway{}, mgr, nil, nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/providers/ark", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "ark"}}

	h.DeleteProvider(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if mgr.deletedID != "ark" {
		t.Fatalf("expected deleted id ark, got %q", mgr.deletedID)
	}
}

func TestInvokeTool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&mockGateway{}, &mockProviderManager{}, nil, nil, nil)
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

