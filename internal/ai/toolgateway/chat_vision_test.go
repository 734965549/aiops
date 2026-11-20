package toolgateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureVisionRequest 解码多模态请求，返回 user 消息的 content 部分。
func captureVisionRequest(t *testing.T, body []byte) []any {
	t.Helper()
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("unexpected messages: %+v", req.Messages)
	}
	if _, ok := req.Messages[0].Content.(string); !ok {
		t.Fatalf("system content must stay plain string: %T", req.Messages[0].Content)
	}
	parts, ok := req.Messages[1].Content.([]any)
	if !ok {
		t.Fatalf("vision user content must be multipart array, got %T", req.Messages[1].Content)
	}
	return parts
}

func assertVisionParts(t *testing.T, parts []any, userPrompt, imageURL string) {
	t.Helper()
	if len(parts) != 2 {
		t.Fatalf("expected text+image parts, got %+v", parts)
	}
	text, _ := parts[0].(map[string]any)
	if text["type"] != "text" || text["text"] != userPrompt {
		t.Fatalf("unexpected text part: %+v", parts[0])
	}
	image, _ := parts[1].(map[string]any)
	imageURLField, _ := image["image_url"].(map[string]any)
	if image["type"] != "image_url" || imageURLField["url"] != imageURL {
		t.Fatalf("unexpected image part: %+v", parts[1])
	}
}

func TestProviderRegistryCompleteVisionBuildsMultipartContent(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(chatCompletionResponse(`{"summary":"ok"}`))
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	cfg, _ := registry.GetProvider("ark")
	cfg.VisionEnabled = true
	if err := registry.UpsertProvider(cfg); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	models := registry.ChatModels()
	if len(models) != 1 || !models[0].VisionEnabled {
		t.Fatalf("vision flag not exposed in chat models: %+v", models)
	}

	image := "data:image/png;base64,iVBORw0KGgo="
	result, err := registry.CompleteVision(context.Background(), "ark", "sys", "分析这张截图", []string{image})
	if err != nil {
		t.Fatalf("complete vision: %v", err)
	}
	if result.Content != `{"summary":"ok"}` {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertVisionParts(t, captureVisionRequest(t, captured), "分析这张截图", image)
}

func TestProviderRegistryCompleteStreamVisionBuildsMultipartContent(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"summary\\\":\\\"ok\\\"}\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	image := "data:image/png;base64,iVBORw0KGgo="
	result, err := registry.CompleteStreamVision(context.Background(), "ark", "sys", "分析这张截图", []string{image}, func(string, string) error { return nil })
	if err != nil {
		t.Fatalf("complete stream vision: %v", err)
	}
	if result.Content != `{"summary":"ok"}` {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertVisionParts(t, captureVisionRequest(t, captured), "分析这张截图", image)
}

func TestCompleteWithoutImagesKeepsPlainStringContent(t *testing.T) {
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(chatCompletionResponse(`{"summary":"ok"}`))
	}))
	defer srv.Close()

	registry := chatTestRegistry(t, srv.URL)
	if _, err := registry.Complete(context.Background(), "ark", "sys", "纯文本问题"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	var req struct {
		Messages []struct {
			Content any `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(captured, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	content, ok := req.Messages[1].Content.(string)
	if !ok || content != "纯文本问题" {
		t.Fatalf("plain request must keep string content, got %T %v", req.Messages[1].Content, req.Messages[1].Content)
	}
}
