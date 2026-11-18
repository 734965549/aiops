package toolgateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	apperr "github.com/734965549/aiops/pkg/errors"
)

// ChatCompletionResult 是一次对话补全的结果；Content 为模型原始文本，
// JSON 解析与业务校验由 application 层负责。
type ChatCompletionResult struct {
	Content string
	Tokens  int
}

// ChatModelDelta 是模型流式增量回调；phase 为 thinking（reasoning_content）或 answer（content）。
// 返回 error 表示客户端已断开，调用方必须立即中止模型请求。
type ChatModelDelta func(phase string, text string) error

// ChatCompleter 由支持对话补全的 provider 执行器实现（当前为类型 b）。
type ChatCompleter interface {
	Complete(ctx context.Context, cfg ProviderConfig, systemPrompt, userPrompt string, policy OutboundPolicy) (*ChatCompletionResult, error)
	CompleteStream(ctx context.Context, cfg ProviderConfig, systemPrompt, userPrompt string, policy OutboundPolicy, onDelta ChatModelDelta) (*ChatCompletionResult, error)
}

// ChatClient 是 application 层使用的对话补全端口，由 ProviderRegistry 实现。
// ChatProviderID 只返回启用中的类型 b；没有可用模型时由 application 降级。
type ChatClient interface {
	ChatProviderID() (string, bool)
	Complete(ctx context.Context, providerID, systemPrompt, userPrompt string) (*ChatCompletionResult, error)
	CompleteStream(ctx context.Context, providerID, systemPrompt, userPrompt string, onDelta ChatModelDelta) (*ChatCompletionResult, error)
}

// Complete 是 ChatCompleter 在类型 b 执行器上的实现：
// 复用 callModel 的超时、出站校验与“deadline 不重试”策略。
func (e *ChatCompletionExecutor) Complete(ctx context.Context, cfg ProviderConfig, systemPrompt, userPrompt string, policy OutboundPolicy) (*ChatCompletionResult, error) {
	if err := e.Validate(cfg); err != nil {
		return nil, err
	}
	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == "" {
		systemPrompt = chatDefaultSystemPrompt
	}
	if strings.TrimSpace(userPrompt) == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "chat prompt is required")
	}
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

	var lastErr error
	for attempt := 0; attempt < chatMaxAttempts; attempt++ {
		content, tokens, retry, err := e.callModel(ctx, cfg, policy, timeout, targetURL, body)
		if err != nil {
			lastErr = err
			// 父级 context 已结束或模型超时预算耗尽时不再重试，避免第二次必然超时。
			if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			if retry {
				continue
			}
			return nil, err
		}
		return &ChatCompletionResult{Content: content, Tokens: tokens}, nil
	}
	if lastErr == nil {
		lastErr = errors.New("model request failed")
	}
	return nil, lastErr
}

// CompleteStream 以 OpenAI 兼容 SSE 读取模型增量；content 聚合为最终 JSON，
// reasoning_content 只透传给 onDelta。onDelta 返回错误时立即中止，不重试。
func (e *ChatCompletionExecutor) CompleteStream(ctx context.Context, cfg ProviderConfig, systemPrompt, userPrompt string, policy OutboundPolicy, onDelta ChatModelDelta) (*ChatCompletionResult, error) {
	if err := e.Validate(cfg); err != nil {
		return nil, err
	}
	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == "" {
		systemPrompt = chatDefaultSystemPrompt
	}
	if strings.TrimSpace(userPrompt) == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "chat prompt is required")
	}
	body, err := json.Marshal(chatRequest{
		Model:    cfg.Model,
		Messages: []chatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}},
		ResponseFormat: struct {
			Type string `json:"type"`
		}{Type: "json_object"},
		Temperature: 0.2,
		Stream:      true,
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

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(cctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "build provider request failed")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	for k, v := range cfg.Headers {
		httpReq.Header.Set(k, v)
	}
	client := newOutboundHTTPClient(policy, timeout)
	resp, err := client.Do(httpReq)
	if err != nil {
		if cctx.Err() != nil {
			return nil, cctx.Err()
		}
		return nil, apperr.Wrap(err, apperr.CodeUnavailable, "model provider request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return nil, apperr.Newf(apperr.CodeUnavailable, "model provider returned status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return nil, apperr.Newf(apperr.CodeUnavailable, "model provider returned status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(io.LimitReader(resp.Body, chatMaxResponseBytes+1))
	scanner.Buffer(make([]byte, 64*1024), chatMaxResponseBytes)
	var content strings.Builder
	tokens := 0
	totalBytes := 0
	emit := func(phase, text string) error {
		if onDelta == nil || text == "" {
			return nil
		}
		if err := onDelta(phase, text); err != nil {
			return apperr.Wrap(err, apperr.CodeAborted, "client closed the stream")
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Bytes()
		totalBytes += len(line) + 1
		if totalBytes > chatMaxResponseBytes {
			return nil, apperr.New(apperr.CodeUnavailable, "model provider response exceeds size limit")
		}
		delta, ok := parseChatStreamLine(line)
		if !ok {
			continue
		}
		if delta.Usage > 0 {
			tokens = delta.Usage
		}
		if delta.Done {
			break
		}
		if delta.Reasoning != "" {
			if err := emit("thinking", delta.Reasoning); err != nil {
				return nil, err
			}
		}
		if delta.Content != "" {
			content.WriteString(delta.Content)
			if err := emit("answer", delta.Content); err != nil {
				return nil, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if cctx.Err() != nil {
			return nil, cctx.Err()
		}
		return nil, apperr.Wrap(err, apperr.CodeUnavailable, "read model provider stream failed")
	}
	finalContent := content.String()
	if strings.TrimSpace(finalContent) == "" {
		return nil, apperr.New(apperr.CodeUnavailable, "empty model response")
	}
	return &ChatCompletionResult{Content: finalContent, Tokens: tokens}, nil
}

// ChatProviderID 返回可用于对话补全的 provider：仅启用中的类型 b。
func (r *ProviderRegistry) ChatProviderID() (string, bool) {
	if r == nil {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.providers))
	for id, p := range r.providers {
		if p.Enabled && p.Type == ProviderTypeB {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return "", false
	}
	sort.Strings(ids)
	return ids[0], true
}

// CompleteStream 通过指定 provider 完成一次流式对话补全。
// 只有类型 b 执行器实现 ChatCompleter；其他类型返回 FAILED_PRECONDITION。
func (r *ProviderRegistry) CompleteStream(ctx context.Context, providerID, systemPrompt, userPrompt string, onDelta ChatModelDelta) (*ChatCompletionResult, error) {
	if r == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "provider registry is not configured")
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "provider id is required")
	}
	r.mu.RLock()
	cfg, ok := r.providers[providerID]
	exec, hasExec := r.executors[cfg.Type]
	policy := r.policy
	r.mu.RUnlock()
	if !ok {
		return nil, apperr.New(apperr.CodeNotFound, "provider not found")
	}
	if !cfg.Enabled {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider is disabled")
	}
	completer, isCompleter := exec.(ChatCompleter)
	if !hasExec || !isCompleter {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider does not support chat completion")
	}
	if err := ValidateOutboundURL(ctx, cfg.BaseURL, policy); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeFailedPrecondition, "provider base_url is invalid")
	}
	return completer.CompleteStream(ctx, cfg, systemPrompt, userPrompt, policy, onDelta)
}

// Complete 通过指定 provider 完成一次对话补全。
// 只有类型 b 执行器实现 ChatCompleter；其他类型返回 FAILED_PRECONDITION。
func (r *ProviderRegistry) Complete(ctx context.Context, providerID, systemPrompt, userPrompt string) (*ChatCompletionResult, error) {
	if r == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "provider registry is not configured")
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "provider id is required")
	}
	r.mu.RLock()
	cfg, ok := r.providers[providerID]
	exec, hasExec := r.executors[cfg.Type]
	policy := r.policy
	r.mu.RUnlock()
	if !ok {
		return nil, apperr.New(apperr.CodeNotFound, "provider not found")
	}
	if !cfg.Enabled {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider is disabled")
	}
	completer, isCompleter := exec.(ChatCompleter)
	if !hasExec || !isCompleter {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "provider does not support chat completion")
	}
	if err := ValidateOutboundURL(ctx, cfg.BaseURL, policy); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeFailedPrecondition, "provider base_url is invalid")
	}
	return completer.Complete(ctx, cfg, systemPrompt, userPrompt, policy)
}
