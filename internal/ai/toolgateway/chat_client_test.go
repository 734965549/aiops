package toolgateway

import (
	"context"
	"encoding/json"
	"errors"
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

func TestProviderRegistryChatCompletionSendsMaxTokens(t *testing.T) {
	var captured chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(chatCompletionResponse(`{"summary":"ok"}`))
	}))
	defer srv.Close()

	exec := NewChatCompletionExecutor(ProviderTypeB, "/chat/completions")
	exec.SetDefaultMaxOutputTokens(2048)
	registry := NewProviderRegistryWithPolicy(OutboundPolicy{AllowLoopback: true})
	registry.RegisterExecutor(exec)
	if err := registry.UpsertProvider(ProviderConfig{
		ID: "ark", Type: ProviderTypeB, BaseURL: srv.URL, Model: "ep-2024", APIKey: "secret",
		Enabled: true, MaxOutputTokens: 1234,
	}); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}

	if _, err := registry.Complete(context.Background(), "ark", "sys", "user"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if captured.MaxTokens != 1234 {
		t.Fatalf("provider max_output_tokens = %d, want 1234", captured.MaxTokens)
	}

	cfg, _ := registry.GetProvider("ark")
	cfg.MaxOutputTokens = 0
	if err := registry.UpsertProvider(cfg); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	if _, err := registry.Complete(context.Background(), "ark", "sys", "user"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if captured.MaxTokens != 2048 {
		t.Fatalf("global default max_output_tokens = %d, want 2048", captured.MaxTokens)
	}

	cfg.ContextWindowTokens = 8000
	cfg.MaxOutputTokens = 6000
	if err := registry.UpsertProvider(cfg); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	if _, err := registry.Complete(context.Background(), "ark", "sys", "user"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if captured.MaxTokens != 4000 {
		t.Fatalf("max_output_tokens must be capped at half window, got %d", captured.MaxTokens)
	}
}

func TestProviderRegistryChatCompletionSendsReasoningEffort(t *testing.T) {
	var captured chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(chatCompletionResponse(`{"summary":"ok"}`))
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL+"/api/v3")
	cfg, _ := registry.GetProvider("ark")
	cfg.ReasoningEffort = "low"
	if err := registry.UpsertProvider(cfg); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	if _, err := registry.Complete(context.Background(), "ark", "sys", "user"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if captured.ReasoningEffort != "low" {
		t.Fatalf("reasoning_effort = %q, want low", captured.ReasoningEffort)
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

func TestParseChatStreamLine(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		ok    bool
		delta chatStreamDelta
	}{
		{name: "reasoning", line: `data: {"choices":[{"delta":{"reasoning_content":"思考中"}}]}`, ok: true, delta: chatStreamDelta{Reasoning: "思考中"}},
		{name: "content", line: `data: {"choices":[{"delta":{"content":"答案"}}]}`, ok: true, delta: chatStreamDelta{Content: "答案"}},
		{name: "usage", line: `data: {"choices":[],"usage":{"total_tokens":42}}`, ok: true, delta: chatStreamDelta{Usage: 42}},
		{name: "done", line: `data: [DONE]`, ok: true, delta: chatStreamDelta{Done: true}},
		{name: "finish length", line: `data: {"choices":[{"delta":{},"finish_reason":"length"}]}`, ok: true, delta: chatStreamDelta{FinishReason: "length"}},
		{name: "comment", line: `: ping`, ok: false},
		{name: "blank", line: ``, ok: false},
		{name: "garbage", line: `data: not-json`, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseChatStreamLine([]byte(tc.line))
			if ok != tc.ok || got != tc.delta {
				t.Fatalf("got %+v ok=%v, want %+v ok=%v", got, ok, tc.delta, tc.ok)
			}
		})
	}
}

func TestProviderRegistryChatModelsAndLookup(t *testing.T) {
	registry := NewProviderRegistryWithPolicy(OutboundPolicy{AllowLoopback: true})
	registry.RegisterExecutor(NewHTTPToolExecutor(ProviderTypeA, "/v1/tools/invoke"))
	registry.RegisterExecutor(NewChatCompletionExecutor(ProviderTypeB, "/chat/completions"))
	for _, cfg := range []ProviderConfig{
		{ID: "provider-b1", Name: "默认模型", Type: ProviderTypeB, BaseURL: "http://127.0.0.1:9", Model: "model-1", APIKey: "secret", Enabled: true},
		{ID: "provider-b2", Name: "备用模型", Type: ProviderTypeB, BaseURL: "http://127.0.0.1:9", Model: "model-2", APIKey: "secret", Enabled: true},
		{ID: "provider-disabled", Name: "禁用模型", Type: ProviderTypeB, BaseURL: "http://127.0.0.1:9", Model: "model-3", APIKey: "secret", Enabled: false},
		{ID: "provider-a", Name: "工具提供方", Type: ProviderTypeA, BaseURL: "http://127.0.0.1:9", APIKey: "secret", Enabled: true},
	} {
		if err := registry.UpsertProvider(cfg); err != nil {
			t.Fatalf("upsert %s: %v", cfg.ID, err)
		}
	}

	models := registry.ChatModels()
	if len(models) != 2 || models[0].ProviderID != "provider-b1" || models[1].ProviderID != "provider-b2" {
		t.Fatalf("models = %+v", models)
	}
	if models[0].Name != "默认模型" || models[1].Model != "model-2" {
		t.Fatalf("model fields = %+v", models)
	}
	if _, ok := registry.ChatProviderByID("provider-b2"); !ok {
		t.Fatal("enabled type b provider must be selectable")
	}
	if _, ok := registry.ChatProviderByID("provider-disabled"); ok {
		t.Fatal("disabled provider must not be selectable")
	}
	if _, ok := registry.ChatProviderByID("provider-a"); ok {
		t.Fatal("type a provider must not be selectable")
	}
	if _, ok := registry.ChatProviderByID("missing"); ok {
		t.Fatal("missing provider must not be selectable")
	}
}

func TestProviderRegistryCompleteStream(t *testing.T) {
	var captured chatRequest
	var accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, line := range []string{
			`data: {"choices":[{"delta":{"reasoning_content":"思考"}}]}`,
			`data: {"choices":[{"delta":{"content":"{\"summary\":"}}]}`,
			`data: {"choices":[{"delta":{"content":"\"ok\"}"}}]}`,
			`data: {"choices":[],"usage":{"total_tokens":42}}`,
			`data: [DONE]`,
		} {
			_, _ = io.WriteString(w, line+"\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	cfg, _ := registry.GetProvider("ark")
	cfg.MaxOutputTokens = 1234
	if err := registry.UpsertProvider(cfg); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	type deltaEvent struct{ phase, text string }
	var deltas []deltaEvent
	result, err := registry.CompleteStream(context.Background(), "ark", "sys", "user", func(phase, text string) error {
		deltas = append(deltas, deltaEvent{phase: phase, text: text})
		return nil
	})
	if err != nil {
		t.Fatalf("complete stream: %v", err)
	}
	if !captured.Stream || accept != "text/event-stream" {
		t.Fatalf("unexpected stream request: stream=%v accept=%q", captured.Stream, accept)
	}
	if captured.MaxTokens != 1234 {
		t.Fatalf("stream max_tokens = %d, want 1234", captured.MaxTokens)
	}
	if result.Content != `{"summary":"ok"}` || result.Tokens != 42 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(deltas) != 3 || deltas[0].phase != "thinking" || deltas[1].phase != "answer" || deltas[1].text != `{"summary":` {
		t.Fatalf("unexpected deltas: %+v", deltas)
	}
}

func TestCompleteStreamAbortsOnDeltaError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"部分"}}]}`+"\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	_, err := registry.CompleteStream(context.Background(), "ark", "sys", "user", func(_, _ string) error {
		return errors.New("client closed")
	})
	if err == nil || apperr.CodeOf(err) != apperr.CodeAborted {
		t.Fatalf("expected aborted error, got %v", err)
	}
}

func TestCompleteStreamEmptyResponse(t *testing.T) {
	withFastRetries(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	_, err := registry.CompleteStream(context.Background(), "ark", "sys", "user", nil)
	if err == nil || apperr.CodeOf(err) != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}
