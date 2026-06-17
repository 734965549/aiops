package toolgateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	identityapp "github.com/734965549/aiops/internal/identity/application"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type stubExecutor struct {
	typeName ProviderType
	validErr error
	result   *ToolResponse
	called   bool
}

func (s *stubExecutor) Type() ProviderType                { return s.typeName }
func (s *stubExecutor) Validate(cfg ProviderConfig) error { return s.validErr }
func (s *stubExecutor) Invoke(ctx context.Context, cfg ProviderConfig, req ToolRequest, policy OutboundPolicy) (*ToolResponse, error) {
	s.called = true
	return s.result, nil
}

type gatewayAuthorizer struct {
	result *identityapp.AuthorizationResult
	err    error
	last   identityapp.AuthorizationInput
}

func (g *gatewayAuthorizer) Authorize(ctx context.Context, input identityapp.AuthorizationInput) (*identityapp.AuthorizationResult, error) {
	g.last = input
	if g.err != nil {
		return nil, g.err
	}
	return g.result, nil
}

func TestProviderRegistryLifecycle(t *testing.T) {
	r := NewProviderRegistry()
	exec := &stubExecutor{typeName: ProviderTypeA, result: &ToolResponse{Provider: "demo", Data: map[string]any{"ok": true}}}
	r.RegisterExecutor(exec)

	if err := r.UpsertProvider(ProviderConfig{ID: "p1", Type: ProviderTypeA, BaseURL: "http://example.com", APIKey: "k", Enabled: true}); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	cfg, ok := r.GetProvider("p1")
	if !ok || cfg.Type != ProviderTypeA {
		t.Fatalf("expected provider p1, got ok=%v cfg=%+v", ok, cfg)
	}
	listed := r.ListProviders()
	if len(listed) != 1 {
		t.Fatalf("expected 1 provider")
	}
	if !listed[0].HasAPIKey {
		t.Fatalf("expected has_api_key true")
	}
	if listed[0].ID != "p1" {
		t.Fatalf("unexpected listed provider: %+v", listed[0])
	}
	resp, err := r.Invoke(context.Background(), "p1", ToolRequest{ToolCode: "alarm.analyze"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !exec.called || resp == nil || resp.Data["ok"] != true {
		t.Fatalf("unexpected invoke result: exec.called=%v resp=%+v", exec.called, resp)
	}
	r.DeleteProvider("p1")
	if _, ok := r.GetProvider("p1"); ok {
		t.Fatalf("expected provider deleted")
	}
}

func TestProviderRegistryValidationAndErrors(t *testing.T) {
	r := NewProviderRegistry()
	if err := r.UpsertProvider(ProviderConfig{}); appCode(err) != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if _, err := r.Invoke(context.Background(), "missing", ToolRequest{}); appCode(err) != apperr.CodeNotFound {
		t.Fatalf("expected provider not found, got %v", err)
	}
	if err := r.UpsertProvider(ProviderConfig{ID: "p1", Type: ProviderTypeB, BaseURL: "http://example.com", APIKey: "k", Enabled: true}); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	if _, err := r.Invoke(context.Background(), "p1", ToolRequest{}); appCode(err) != apperr.CodeUnavailable {
		t.Fatalf("expected missing executor unavailable, got %v", err)
	}
}

func TestProviderRegistryInvokeRejectsDisabledProvider(t *testing.T) {
	r := NewProviderRegistry()
	r.RegisterExecutor(&stubExecutor{typeName: ProviderTypeA, result: &ToolResponse{}})
	if err := r.UpsertProvider(ProviderConfig{ID: "p1", Type: ProviderTypeA, BaseURL: "http://example.com", APIKey: "k", Enabled: false}); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	if _, err := r.Invoke(context.Background(), "p1", ToolRequest{ToolCode: "alarm.analyze"}); appCode(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestProviderRegistryWrapsExecutorValidation(t *testing.T) {
	r := NewProviderRegistryWithPolicy(OutboundPolicy{AllowLoopback: true})
	r.RegisterExecutor(&stubExecutor{typeName: ProviderTypeA, validErr: errors.New("bad config")})
	err := r.UpsertProvider(ProviderConfig{ID: "p1", Type: ProviderTypeA, BaseURL: "http://127.0.0.1:9000", APIKey: "k"})
	if appCode(err) != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestProviderRegistryBlocksPrivateBaseURL(t *testing.T) {
	r := NewProviderRegistryWithPolicy(DefaultOutboundPolicy())
	r.RegisterExecutor(&stubExecutor{typeName: ProviderTypeA, result: &ToolResponse{}})
	err := r.UpsertProvider(ProviderConfig{ID: "p1", Type: ProviderTypeA, BaseURL: "http://127.0.0.1:9000", APIKey: "k"})
	if err == nil {
		t.Fatal("expected private base_url to be rejected")
	}
}

func TestProviderRegistryAllowsLoopbackInDevPolicy(t *testing.T) {
	r := NewProviderRegistryWithPolicy(OutboundPolicy{AllowLoopback: true})
	r.RegisterExecutor(&stubExecutor{typeName: ProviderTypeA, result: &ToolResponse{}})
	if err := r.UpsertProvider(ProviderConfig{ID: "p1", Type: ProviderTypeA, BaseURL: "http://127.0.0.1:9000", APIKey: "k", Enabled: true}); err != nil {
		t.Fatalf("expected loopback allowed in dev policy: %v", err)
	}
}

func TestHTTPToolExecutorValidate(t *testing.T) {
	exec := NewHTTPToolExecutor(ProviderTypeA, "/v1/tools/invoke")
	if err := exec.Validate(ProviderConfig{}); appCode(err) != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if err := exec.Validate(ProviderConfig{BaseURL: "http://example.com", APIKey: "secret"}); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestProviderRegistryConcurrentReadWrite(t *testing.T) {
	r := NewProviderRegistry()
	r.RegisterExecutor(&stubExecutor{typeName: ProviderTypeA, result: &ToolResponse{Data: map[string]any{"ok": true}}})
	if err := r.UpsertProvider(ProviderConfig{ID: "p1", Type: ProviderTypeA, BaseURL: "http://example.com", APIKey: "k", Enabled: true}); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(3)
		go func(n int) {
			defer wg.Done()
			_ = r.ListProviders()
		}(i)
		go func(n int) {
			defer wg.Done()
			_, _ = r.GetProvider("p1")
		}(i)
		go func(n int) {
			defer wg.Done()
			id := "p-concurrent"
			if n%2 == 0 {
				_ = r.UpsertProvider(ProviderConfig{ID: id, Type: ProviderTypeA, BaseURL: "http://example.com", APIKey: "k", Enabled: true})
			} else {
				r.DeleteProvider(id)
			}
		}(i)
	}
	wg.Wait()
}

func TestGatewayValidateAndErrors(t *testing.T) {
	auth := &gatewayAuthorizer{result: &identityapp.AuthorizationResult{Allowed: true}}
	gw := NewGateway(auth, NewProviderRegistry())
	res, err := gw.Validate(context.Background(), ToolRequest{UserID: "u1", ToolCode: "alarm.analyze", Resource: "alarm", Action: "read", Dept: "platform"})
	if err != nil || res == nil || !res.Allowed {
		t.Fatalf("expected allow, got res=%+v err=%v", res, err)
	}
	if auth.last.UserID != "u1" || auth.last.ToolCode != "alarm.analyze" {
		t.Fatalf("unexpected authorizer input: %+v", auth.last)
	}

	denyGW := NewGateway(&gatewayAuthorizer{result: &identityapp.AuthorizationResult{Allowed: false, Reason: "denied"}}, NewProviderRegistry())
	denyRes, err := denyGW.Validate(context.Background(), ToolRequest{UserID: "u1", ToolCode: "alarm.analyze"})
	if err != nil {
		t.Fatalf("expected denied result without error, got %v", err)
	}
	if denyRes == nil || denyRes.Allowed || denyRes.Reason != "denied" {
		t.Fatalf("expected denied result, got %+v", denyRes)
	}
	if _, err := gw.Validate(context.Background(), ToolRequest{}); err == nil {
		t.Fatalf("expected invalid arg error")
	}
}

func TestGatewayInvokeSkipsProviderWhenDenied(t *testing.T) {
	auth := &gatewayAuthorizer{result: &identityapp.AuthorizationResult{Allowed: false, Reason: "tool permission denied"}}
	gw := NewGateway(auth, NewProviderRegistry())
	res, err := gw.Invoke(context.Background(), "missing", ToolRequest{UserID: "u1", ToolCode: "alarm.analyze"})
	if err != nil {
		t.Fatalf("expected denied result without provider error, got %v", err)
	}
	if res == nil || res.Allowed || res.Reason != "tool permission denied" {
		t.Fatalf("expected denied result, got %+v", res)
	}
}

func TestGatewayInvokeReturnsRegistryAppError(t *testing.T) {
	auth := &gatewayAuthorizer{result: &identityapp.AuthorizationResult{Allowed: true}}
	gw := NewGateway(auth, NewProviderRegistry())
	_, err := gw.Invoke(context.Background(), "missing", ToolRequest{UserID: "u1", ToolCode: "alarm.analyze"})
	if appCode(err) != apperr.CodeNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestHTTPToolExecutorDoesNotExposeProviderBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret provider details", http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewProviderRegistryWithPolicy(OutboundPolicy{AllowLoopback: true})
	r.RegisterExecutor(NewHTTPToolExecutor(ProviderTypeA, "/"))
	if err := r.UpsertProvider(ProviderConfig{ID: "p1", Type: ProviderTypeA, BaseURL: srv.URL, APIKey: "k", Enabled: true}); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	_, err := r.Invoke(context.Background(), "p1", ToolRequest{UserID: "u1", ToolCode: "alarm.analyze"})
	app := apperr.FromError(err)
	if app.Code != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
	if strings.Contains(app.Message, "secret provider details") {
		t.Fatalf("provider body leaked in message: %q", app.Message)
	}
}

func appCode(err error) apperr.Code {
	if err == nil {
		return apperr.CodeOK
	}
	return apperr.FromError(err).Code
}
