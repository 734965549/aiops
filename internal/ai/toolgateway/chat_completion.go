package toolgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
)

const (
	chatMaxResponseBytes = 4 << 20
	chatMaxAttempts      = 2
)

// 平台直连模型时的默认 system prompt；analyze-alert 路径由 application 层传入固定提示词，
// 直接工具调用未指定时使用本默认值。payload 不支持覆盖，避免绕过安全护栏。
// 与 application/analysis_prompt.go 的 analysisSystemPrompt 保持一致；修改 prompt 时两处必须同步。
const chatDefaultSystemPrompt = `你是企业运维平台的告警分析助手。只依据输入的告警快照和指标证据进行分析，不臆造未提供的数据。
输出必须是单个 JSON 对象，字段：
- summary: string，简明结论；
- risk_level: string，只能为 low / medium / high / info；
- recommendations: string[]，处置建议；需要执行动作时只写“建议创建 xx 处置任务”，不得输出可直接执行的命令、脚本或云 API 调用；
- references: string[]，引用证据来源，优先使用 evidence_id；
- uncertainties: string[]，说明证据不足或不确定性。
禁止输出凭据、Token 或任何可执行指令。`

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

type analysisOutput struct {
	Summary         string   `json:"summary"`
	RiskLevel       string   `json:"risk_level"`
	Recommendations []string `json:"recommendations"`
	References      []string `json:"references"`
	Uncertainties   []string `json:"uncertainties"`
}

// ChatCompletionExecutor 是 type b 的平台直连模型执行器：
// 构造 chat/completions 请求，解析 choices[0].message.content 并校验分析 JSON。
type ChatCompletionExecutor struct {
	typeName    ProviderType
	requestPath string
}

func NewChatCompletionExecutor(t ProviderType, requestPath string) *ChatCompletionExecutor {
	return &ChatCompletionExecutor{typeName: t, requestPath: requestPath}
}

func (e *ChatCompletionExecutor) Type() ProviderType { return e.typeName }

func (e *ChatCompletionExecutor) Validate(cfg ProviderConfig) error {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "base_url is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "api_key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "model is required for provider type b")
	}
	return nil
}

func (e *ChatCompletionExecutor) Invoke(ctx context.Context, cfg ProviderConfig, req ToolRequest, policy OutboundPolicy) (*ToolResponse, error) {
	if err := e.Validate(cfg); err != nil {
		return nil, err
	}
	systemPrompt, userPrompt := buildChatPrompts(req)
	body, err := json.Marshal(chatRequest{
		Model:    cfg.Model,
		Messages: []chatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}},
		ResponseFormat: struct {
			Type string `json:"type"`
		}{Type: "json_object"},
		Temperature: 0.2,
	})
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "build model request failed")
	}
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	targetURL, err := joinProviderURL(cfg.BaseURL, e.requestPath)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeFailedPrecondition, "provider base_url is invalid")
	}
	if err := ValidateOutboundURL(ctx, targetURL, policy); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeFailedPrecondition, "provider target url is invalid")
	}

	started := time.Now()
	var lastErr error
	for attempt := 0; attempt < chatMaxAttempts; attempt++ {
		content, tokens, retry, err := e.callModel(ctx, cfg, policy, timeout, targetURL, body)
		if err != nil {
			lastErr = err
			if retry {
				continue
			}
			return nil, err
		}
		output, parseErr := parseAnalysis(content)
		if parseErr != nil {
			lastErr = parseErr
			continue
		}
		output.References = appendMissingReferences(output.References, payloadEvidenceReferences(req.Payload))
		logger.From(ctx).Info("ai model analysis completed",
			logger.String("provider_id", cfg.ID),
			logger.String("model", cfg.Model),
			logger.Int64("duration_ms", time.Since(started).Milliseconds()),
			logger.Int("tokens", tokens),
		)
		return &ToolResponse{Allowed: true, Provider: cfg.ID, Data: analysisData(output)}, nil
	}
	if lastErr == nil {
		lastErr = errors.New("model request failed")
	}
	logger.From(ctx).Warn("ai model analysis degraded",
		logger.String("provider_id", cfg.ID),
		logger.String("model", cfg.Model),
		logger.Int64("duration_ms", time.Since(started).Milliseconds()),
		logger.Error(lastErr),
	)
	output := fallbackAnalysis(req.Payload)
	data := analysisData(output)
	data["degraded"] = true
	data["degraded_reason"] = "model_unavailable"
	return &ToolResponse{Allowed: true, Provider: cfg.ID, Data: data}, nil
}

// callModel 发起一次模型请求；retry 表示失败可重试（网络、429、5xx、响应为空）。
func (e *ChatCompletionExecutor) callModel(ctx context.Context, cfg ProviderConfig, policy OutboundPolicy, timeout time.Duration, targetURL string, body []byte) (string, int, bool, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(cctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return "", 0, false, apperr.Wrap(err, apperr.CodeInternal, "build provider request failed")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	for k, v := range cfg.Headers {
		httpReq.Header.Set(k, v)
	}
	client := newOutboundHTTPClient(policy, timeout)
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", 0, true, apperr.Wrap(err, apperr.CodeUnavailable, "model provider request failed")
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, chatMaxResponseBytes+1))
	_ = resp.Body.Close()
	if readErr != nil {
		return "", 0, true, apperr.Wrap(readErr, apperr.CodeUnavailable, "read model provider response failed")
	}
	if len(data) > chatMaxResponseBytes {
		return "", 0, false, apperr.New(apperr.CodeUnavailable, "model provider response exceeds size limit")
	}
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return "", 0, true, apperr.Newf(apperr.CodeUnavailable, "model provider returned status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return "", 0, false, apperr.Newf(apperr.CodeUnavailable, "model provider returned status %d", resp.StatusCode)
	}
	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", 0, true, apperr.Wrap(err, apperr.CodeUnavailable, "decode model provider response failed")
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", parsed.Usage.TotalTokens, true, apperr.New(apperr.CodeUnavailable, "empty model response")
	}
	return parsed.Choices[0].Message.Content, parsed.Usage.TotalTokens, false, nil
}

func buildChatPrompts(req ToolRequest) (string, string) {
	system := strings.TrimSpace(req.SystemPrompt)
	if system == "" {
		system = chatDefaultSystemPrompt
	}
	if user := strings.TrimSpace(req.UserPrompt); user != "" {
		return system, user
	}
	body := map[string]any{
		"alert":    req.Payload["alert"],
		"evidence": req.Payload["evidence"],
		"task":     "基于告警快照和指标证据给出摘要、风险等级、建议、证据引用和不确定性。",
	}
	if req.Payload["alert"] == nil && req.Payload["evidence"] == nil {
		body = req.Payload
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return system, fmt.Sprintf("payload: %v", req.Payload)
	}
	return system, string(raw)
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
	references := payloadEvidenceReferences(payload)
	if references == nil {
		references = []string{}
	}
	return analysisOutput{
		Summary:         "AI 分析暂不可用，请结合告警快照与指标证据人工判断。",
		RiskLevel:       "info",
		Recommendations: []string{},
		References:      references,
		Uncertainties:   []string{"模型或编排服务不可用，已返回保守降级结果"},
	}
}

func analysisData(out analysisOutput) map[string]any {
	raw, err := json.Marshal(out)
	if err != nil {
		return map[string]any{"summary": out.Summary, "risk_level": out.RiskLevel}
	}
	data := map[string]any{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return map[string]any{"summary": out.Summary, "risk_level": out.RiskLevel}
	}
	return data
}

// payloadEvidenceReferences 同时兼容进程内 *MetricEvidenceResult 和 JSON map 两种 evidence 形态。
func payloadEvidenceReferences(payload map[string]any) []string {
	raw, err := json.Marshal(payload["evidence"])
	if err != nil {
		return nil
	}
	var evidence struct {
		Metrics []struct {
			EvidenceID string `json:"evidence_id"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(raw, &evidence); err != nil {
		return nil
	}
	seen := make(map[string]struct{}, len(evidence.Metrics))
	refs := make([]string, 0, len(evidence.Metrics))
	for _, metric := range evidence.Metrics {
		id := strings.TrimSpace(metric.EvidenceID)
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
