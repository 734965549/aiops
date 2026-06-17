package toolgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	apperr "github.com/734965549/aiops/pkg/errors"
)

// ProviderRuntimeConfig 是执行器运行时配置。
type ProviderRuntimeConfig struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
	Headers map[string]string
}

// HTTPToolExecutor 是通用 HTTP + API Key 工具执行器。
type HTTPToolExecutor struct {
	typeName ProviderType
	// RequestPath 可由上层按协议约定设置，例如 /v1/tools/invoke
	RequestPath string
	// AuthHeader 默认 Authorization: Bearer <api_key>
	AuthHeader string
}

func NewHTTPToolExecutor(t ProviderType, requestPath string) *HTTPToolExecutor {
	return &HTTPToolExecutor{typeName: t, RequestPath: requestPath, AuthHeader: "Authorization"}
}

func (e *HTTPToolExecutor) Type() ProviderType { return e.typeName }

func (e *HTTPToolExecutor) Validate(cfg ProviderConfig) error {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "base_url is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return apperr.New(apperr.CodeInvalidArgument, "api_key is required")
	}
	return nil
}

func (e *HTTPToolExecutor) Invoke(ctx context.Context, cfg ProviderConfig, req ToolRequest, policy OutboundPolicy) (*ToolResponse, error) {
	if err := e.Validate(cfg); err != nil {
		return nil, err
	}
	payload := map[string]any{"tool_code": req.ToolCode, "resource": req.Resource, "action": req.Action, "user_id": req.UserID, "payload": req.Payload}
	body, _ := json.Marshal(payload)
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	targetURL, err := joinProviderURL(cfg.BaseURL, e.RequestPath)
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
	httpReq.Header.Set(e.AuthHeader, "Bearer "+cfg.APIKey)
	for k, v := range cfg.Headers {
		httpReq.Header.Set(k, v)
	}
	client := newOutboundHTTPClient(policy, timeout)
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeUnavailable, "tool provider request failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, defaultMaxOutboundBodyBytes+1))
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeUnavailable, "read tool provider response failed")
	}
	if len(data) > defaultMaxOutboundBodyBytes {
		return nil, apperr.New(apperr.CodeUnavailable, "tool provider response exceeds size limit")
	}
	if resp.StatusCode >= 300 {
		return nil, apperr.Newf(apperr.CodeUnavailable, "tool provider returned status %d", resp.StatusCode)
	}
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	return &ToolResponse{Allowed: true, Provider: cfg.ID, Data: out}, nil
}

// OpenAICompatibleExecutor 支持类 OpenAI/Claude 的兼容请求，使用统一 API Key。
type OpenAICompatibleExecutor struct{ HTTPToolExecutor }

func NewOpenAICompatibleExecutor(t ProviderType, requestPath string) *OpenAICompatibleExecutor {
	return &OpenAICompatibleExecutor{HTTPToolExecutor{typeName: t, RequestPath: requestPath, AuthHeader: "Authorization"}}
}

// InternalServiceExecutor 支持内部服务协议，可自定义头。
type InternalServiceExecutor struct{ HTTPToolExecutor }

func NewInternalServiceExecutor(t ProviderType, requestPath string) *InternalServiceExecutor {
	return &InternalServiceExecutor{HTTPToolExecutor{typeName: t, RequestPath: requestPath, AuthHeader: "X-API-Key"}}
}
