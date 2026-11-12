package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestAnalysisServer(modelURL string) *analysisServer {
	return &analysisServer{
		cfg: Config{
			APIKey:       "test-key",
			ModelBaseURL: modelURL,
			ModelAPIKey:  "model-key",
			ModelName:    "test-model",
			ModelPath:    "/v1/chat/completions",
			Timeout:      2 * time.Second,
		},
		client: &http.Client{Timeout: 2 * time.Second},
	}
}

func TestHandleInvokeSuccess(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer model-key" {
			t.Errorf("authorization = %q", got)
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode model request: %v", err)
		}
		if req.ResponseFormat.Type != "json_object" || req.Model != "test-model" {
			t.Errorf("unexpected model request: %+v", req)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]string{
					"role":    "assistant",
					"content": "{\"summary\":\"CPU 持续超阈值\",\"risk_level\":\"high\",\"recommendations\":[\"建议创建 CPU 扩容处置任务\"],\"references\":[],\"uncertainties\":[]}",
				},
			}},
			"usage": map[string]any{"total_tokens": 42},
		})
	}))
	defer model.Close()

	server := newTestAnalysisServer(model.URL)
	body := `{"tool_code":"alarm.analyze","payload":{"alert":{"name":"HighCPU"},"evidence":{"status":"ok","metrics":[{"evidence_id":"ev-1"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/tool/invoke", strings.NewReader(body))
	req.Header.Set("X-API-Key", "test-key")
	rec := httptest.NewRecorder()
	server.handleInvoke(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var out analysisOutput
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Summary != "CPU 持续超阈值" || out.RiskLevel != "high" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if len(out.References) != 1 || out.References[0] != "observability evidence ev-1" {
		t.Fatalf("unexpected references: %+v", out.References)
	}
}

func TestHandleInvokeRejectsBadAPIKey(t *testing.T) {
	router := NewRouter(Config{APIKey: "test-key", ModelBaseURL: "http://127.0.0.1:1", Timeout: time.Second})
	req := httptest.NewRequest(http.MethodPost, "/api/tool/invoke", strings.NewReader(`{"tool_code":"alarm.analyze"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRouterAcceptsPlatformBearerAPIKey(t *testing.T) {
	router := NewRouter(Config{APIKey: "test-key", ModelBaseURL: "http://127.0.0.1:1", Timeout: time.Second})
	req := httptest.NewRequest(http.MethodPost, "/api/tool/invoke", strings.NewReader(`{"tool_code":"other.tool"}`))
	req.Header.Set("X-API-Key", "Bearer test-key")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("auth should pass, status = %d", rec.Code)
	}
}

func TestHandleInvokeRejectsUnsupportedTool(t *testing.T) {
	server := newTestAnalysisServer("http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodPost, "/api/tool/invoke", strings.NewReader(`{"tool_code":"other.tool"}`))
	req.Header.Set("X-API-Key", "test-key")
	rec := httptest.NewRecorder()
	server.handleInvoke(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleInvokeFallsBackOnModelError(t *testing.T) {
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "upstream down", http.StatusInternalServerError)
	}))
	defer model.Close()

	server := newTestAnalysisServer(model.URL)
	body := `{"tool_code":"alarm.analyze","payload":{"alert":{"name":"HighCPU"},"evidence":{"status":"ok","metrics":[{"evidence_id":"ev-2"}]}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/tool/invoke", strings.NewReader(body))
	req.Header.Set("X-API-Key", "test-key")
	rec := httptest.NewRecorder()
	server.handleInvoke(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if calls != modelRetryAttempts {
		t.Fatalf("model calls = %d, want %d", calls, modelRetryAttempts)
	}
	var out analysisOutput
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.RiskLevel != "info" || len(out.Uncertainties) == 0 {
		t.Fatalf("unexpected fallback: %+v", out)
	}
	if len(out.References) != 1 || out.References[0] != "observability evidence ev-2" {
		t.Fatalf("fallback should keep evidence refs: %+v", out.References)
	}
}

func TestParseAnalysisRequiresSummary(t *testing.T) {
	if _, err := parseAnalysis(`{"risk_level":"high"}`); err == nil {
		t.Fatal("expected missing summary error")
	}
}

func TestEvidenceReferencesDeduplicates(t *testing.T) {
	payload := map[string]any{
		"evidence": map[string]any{
			"metrics": []any{
				map[string]any{"evidence_id": "ev-1"},
				map[string]any{"evidence_id": "ev-1"},
				map[string]any{"evidence_id": ""},
			},
		},
	}
	refs := evidenceReferences(payload)
	if len(refs) != 1 || refs[0] != "observability evidence ev-1" {
		t.Fatalf("unexpected refs: %+v", refs)
	}
}
