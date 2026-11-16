package toolgateway

import (
	"context"
	"encoding/json"
	"errors"
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

// ChatCompleter 由支持对话补全的 provider 执行器实现（当前为类型 b）。
type ChatCompleter interface {
	Complete(ctx context.Context, cfg ProviderConfig, systemPrompt, userPrompt string, policy OutboundPolicy) (*ChatCompletionResult, error)
}

// ChatClient 是 application 层使用的对话补全端口，由 ProviderRegistry 实现。
// ChatProviderID 只返回启用中的类型 b；没有可用模型时由 application 降级。
type ChatClient interface {
	ChatProviderID() (string, bool)
	Complete(ctx context.Context, providerID, systemPrompt, userPrompt string) (*ChatCompletionResult, error)
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
