package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	maxModelResponseBytes = 4 << 20
	modelRetryAttempts    = 2
)

const systemPrompt = `你是企业运维平台的告警分析助手。只依据输入的告警快照和指标证据进行分析，不臆造未提供的数据。
输出必须是单个 JSON 对象，字段：
- summary: string，简明结论；
- risk_level: string，只能为 low / medium / high / info；
- recommendations: string[]，处置建议；需要执行动作时只写“建议创建 xx 处置任务”，不得输出可直接执行的命令、脚本或云 API 调用；
- references: string[]，引用证据来源，优先使用 evidence_id；
- uncertainties: string[]，说明证据不足或不确定性。
禁止输出凭据、Token 或任何可执行指令。`

type analysisOutput struct {
	Summary         string   `json:"summary"`
	RiskLevel       string   `json:"risk_level"`
	Recommendations []string `json:"recommendations"`
	References      []string `json:"references"`
	Uncertainties   []string `json:"uncertainties"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	ResponseFormat struct {
		Type string `json:"type"`
	} `json:"response_format"`
	Temperature float64 `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (s *analysisServer) analyze(ctx context.Context, payload map[string]any) (analysisOutput, int, error) {
	userPrompt, err := buildUserPrompt(payload)
	if err != nil {
		return analysisOutput{}, 0, err
	}
	content, tokens, err := s.callModel(ctx, systemPrompt, userPrompt)
	if err != nil {
		return analysisOutput{}, tokens, err
	}
	output, err := parseAnalysis(content)
	if err != nil {
		return analysisOutput{}, tokens, err
	}
	output.References = appendMissingReferences(output.References, evidenceReferences(payload))
	return output, tokens, nil
}

func buildUserPrompt(payload map[string]any) (string, error) {
	body := map[string]any{
		"alert":    payload["alert"],
		"evidence": payload["evidence"],
		"task":     "基于告警快照和指标证据给出摘要、风险等级、建议、证据引用和不确定性。",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal analysis prompt: %w", err)
	}
	return string(raw), nil
}

func (s *analysisServer) callModel(ctx context.Context, system, user string) (string, int, error) {
	endpoint := strings.TrimRight(s.cfg.ModelBaseURL, "/") + "/" + strings.TrimLeft(s.cfg.ModelPath, "/")
	reqBody := chatRequest{Model: s.cfg.ModelName, Messages: []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, Temperature: 0.2}
	reqBody.ResponseFormat.Type = "json_object"
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("marshal model request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < modelRetryAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			return "", 0, fmt.Errorf("build model request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.cfg.ModelAPIKey)
		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxModelResponseBytes+1))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if len(data) > maxModelResponseBytes {
			return "", 0, errors.New("model response exceeds size limit")
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("model returned status %d", resp.StatusCode)
			continue
		}
		if resp.StatusCode >= 300 {
			return "", 0, fmt.Errorf("model returned status %d", resp.StatusCode)
		}
		var parsed chatResponse
		if err := json.Unmarshal(data, &parsed); err != nil {
			return "", 0, fmt.Errorf("decode model response: %w", err)
		}
		if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
			return "", parsed.Usage.TotalTokens, errors.New("empty model response")
		}
		return parsed.Choices[0].Message.Content, parsed.Usage.TotalTokens, nil
	}
	if lastErr == nil {
		lastErr = errors.New("model request failed")
	}
	return "", 0, lastErr
}

func parseAnalysis(content string) (analysisOutput, error) {
	var out analysisOutput
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &out); err != nil {
		return analysisOutput{}, fmt.Errorf("decode analysis json: %w", err)
	}
	if strings.TrimSpace(out.Summary) == "" {
		return analysisOutput{}, errors.New("analysis summary is required")
	}
	out.RiskLevel = normalizeRiskLevel(out.RiskLevel)
	if out.Recommendations == nil {
		out.Recommendations = []string{}
	}
	if out.References == nil {
		out.References = []string{}
	}
	if out.Uncertainties == nil {
		out.Uncertainties = []string{}
	}
	return out, nil
}

func normalizeRiskLevel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "low", "medium", "high", "info":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "info"
	}
}

func extractJSONObject(content string) string {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return content[start : end+1]
	}
	return content
}

func fallbackAnalysis(payload map[string]any) analysisOutput {
	return analysisOutput{
		Summary:         "AI 分析暂不可用，请结合告警快照与指标证据人工判断。",
		RiskLevel:       "info",
		Recommendations: []string{},
		References:      evidenceReferences(payload),
		Uncertainties:   []string{"模型或编排服务不可用，已返回保守降级结果"},
	}
}

func evidenceReferences(payload map[string]any) []string {
	evidence, ok := payload["evidence"].(map[string]any)
	if !ok {
		return nil
	}
	metrics, ok := evidence["metrics"].([]any)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	refs := make([]string, 0, len(metrics))
	for _, raw := range metrics {
		metric, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := metric["evidence_id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		refs = append(refs, "observability evidence "+id)
	}
	return refs
}

func appendMissingReferences(existing, extra []string) []string {
	if len(extra) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing))
	for _, ref := range existing {
		seen[strings.TrimSpace(ref)] = struct{}{}
	}
	out := append([]string(nil), existing...)
	for _, ref := range extra {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, exists := seen[ref]; exists {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	return out
}
