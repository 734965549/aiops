package toolgateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	apperr "github.com/734965549/aiops/pkg/errors"
)

func TestProviderRegistryChatCompletion(t *testing.T) {
	var captured chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(chatCompletionResponse(`{"summary":"ok"}`))
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL+"/api/v3")
	providerID, ok := registry.ChatProviderID()
	if !ok || providerID != "ark" {
		t.Fatalf("ChatProviderID = %q, %v", providerID, ok)
	}
	result, err := registry.Complete(context.Background(), "ark", "系统提示词", `{"question":"系统状态"}`)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if result.Content != `{"summary":"ok"}` || result.Tokens != 42 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(captured.Messages) != 2 || captured.Messages[0].Content != "系统提示词" {
		t.Fatalf("unexpected messages: %+v", captured.Messages)
	}
	if captured.Messages[1].Content != `{"question":"系统状态"}` {
		t.Fatalf("unexpected user prompt: %q", captured.Messages[1].Content)
	}
	if captured.ResponseFormat.Type != "json_object" || captured.Model != "ep-2024" {
		t.Fatalf("unexpected request: %+v", captured)
	}
}

func TestProviderRegistryChatCompletionRequiresTypeB(t *testing.T) {
	registry := NewProviderRegistryWithPolicy(OutboundPolicy{AllowLoopback: true})
	registry.RegisterExecutor(NewHTTPToolExecutor(ProviderTypeA, "/v1/tools/invoke"))
	registry.RegisterExecutor(NewChatCompletionExecutor(ProviderTypeB, "/chat/completions"))
	if err := registry.UpsertProvider(ProviderConfig{
		ID: "http-a", Type: ProviderTypeA, BaseURL: "http://127.0.0.1:9000", APIKey: "secret", Enabled: true,
	}); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	if _, ok := registry.ChatProviderID(); ok {
		t.Fatal("type a provider must not be a chat provider")
	}
	_, err := registry.Complete(context.Background(), "http-a", "sys", "user")
	if err == nil || apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
	if _, err := registry.Complete(context.Background(), "missing", "sys", "user"); err == nil ||
		apperr.FromError(err).Code != apperr.CodeNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestProviderRegistryChatCompletionDisabledProvider(t *testing.T) {
	registry := chatTestRegistry(t, "http://127.0.0.1:9")
	cfg, _ := registry.GetProvider("ark")
	cfg.Enabled = false
	registry.mu.Lock()
	registry.providers["ark"] = cfg
	registry.mu.Unlock()
	if _, ok := registry.ChatProviderID(); ok {
		t.Fatal("disabled provider must not be selectable")
	}
	if _, err := registry.Complete(context.Background(), "ark", "sys", "user"); err == nil ||
		apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}
