package toolgateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	apperr "github.com/734965549/aiops/pkg/errors"
)

// RateLimitError 表示 provider 返回 429。
// RetryAfter 用于服务端退避（优先 Retry-After 头）；ResetAfter 用于向用户展示预计恢复时间
// （优先响应体配额提示，缺失时回退 Retry-After）。
type RateLimitError struct {
	StatusCode int
	RetryAfter time.Duration
	ResetAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e == nil {
		return "model provider rate limited"
	}
	return fmt.Sprintf("model provider rate limited (status %d)", e.StatusCode)
}

// 流式超时与重试策略：
//   - 首块 60s / 空闲 180s，连续无输出才算挂；总时长由父级回合预算约束；
//   - 首块前失败（429/5xx/网络/首块超时/空响应）可退避重试，最多 2 次；
//   - 已向调用方输出任何 delta 后不再重试，改为返回 Partial 结果保留已生成内容。
const chatStreamMaxAttempts = 3

var (
	chatStreamFirstChunkTimeout = 60 * time.Second
	chatStreamIdleTimeout       = 180 * time.Second
	chatRetryBaseDelay          = time.Second
	// chatRetrySleep 独立成变量便于单测跳过真实退避等待。
	chatRetrySleep = sleepWithContext
)

// streamOnce 执行一次流式请求；retryable/retryAfter 仅在 err 非空且尚未输出时生效。
func (e *ChatCompletionExecutor) streamOnce(
	ctx context.Context,
	cfg ProviderConfig,
	policy OutboundPolicy,
	targetURL string,
	body []byte,
	onDelta ChatModelDelta,
) (*ChatCompletionResult, bool, time.Duration, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var timedOut atomic.Bool
	receivedLine := false
	finishReason := ""
	timer := time.AfterFunc(chatStreamFirstChunkTimeout, func() {
		timedOut.Store(true)
		cancel()
	})
	defer timer.Stop()

	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, false, 0, apperr.Wrap(err, apperr.CodeInternal, "build provider request failed")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	for k, v := range cfg.Headers {
		httpReq.Header.Set(k, v)
	}
	client := newOutboundStreamHTTPClient(policy, chatStreamFirstChunkTimeout)
	resp, err := client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, 0, ctx.Err()
		}
		if timedOut.Load() {
			return nil, true, 0, apperr.New(apperr.CodeUnavailable, "model provider first chunk timeout")
		}
		return nil, true, 0, apperr.Wrap(err, apperr.CodeUnavailable, "model provider request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfterDuration(resp.Header.Get("Retry-After"))
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		resetAfter := parseQuotaResetHint(string(body))
		if resetAfter <= 0 {
			resetAfter = retryAfter
		}
		rateErr := &RateLimitError{StatusCode: resp.StatusCode, RetryAfter: retryAfter, ResetAfter: resetAfter}
		return nil, true, retryAfter, apperr.Wrap(rateErr, apperr.CodeUnavailable, "model provider rate limited")
	}
	if resp.StatusCode >= 500 {
		return nil, true, 0, apperr.Newf(apperr.CodeUnavailable, "model provider returned status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return nil, false, 0, apperr.Newf(apperr.CodeUnavailable, "model provider returned status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(io.LimitReader(resp.Body, chatMaxResponseBytes+1))
	scanner.Buffer(make([]byte, 64*1024), chatMaxResponseBytes)
	var content strings.Builder
	tokens := 0
	totalBytes := 0
	emittedAny := false
	emit := func(phase, text string) error {
		if onDelta == nil || text == "" {
			return nil
		}
		if err := onDelta(phase, text); err != nil {
			return apperr.Wrap(err, apperr.CodeAborted, "client closed the stream")
		}
		emittedAny = true
		return nil
	}
	for scanner.Scan() {
		line := scanner.Bytes()
		totalBytes += len(line) + 1
		if totalBytes > chatMaxResponseBytes {
			return nil, false, 0, apperr.New(apperr.CodeUnavailable, "model provider response exceeds size limit")
		}
		// 任意非空 SSE 行（含 provider 的注释/心跳）都说明连接仍存活，重置空闲计时。
		if len(bytes.TrimSpace(line)) > 0 {
			receivedLine = true
			timer.Reset(e.effectiveStreamIdleTimeout())
		}
		delta, ok := parseChatStreamLine(line)
		if !ok {
			continue
		}
		if delta.FinishReason != "" {
			finishReason = delta.FinishReason
		}
		if delta.Usage > 0 {
			tokens = delta.Usage
		}
		if delta.Done {
			break
		}
		if delta.Reasoning != "" {
			if err := emit("thinking", delta.Reasoning); err != nil {
				return nil, false, 0, err
			}
		}
		if delta.Content != "" {
			content.WriteString(delta.Content)
			if err := emit("answer", delta.Content); err != nil {
				return nil, false, 0, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil, false, 0, ctx.Err()
		}
		if timedOut.Load() {
			reason := "stream_first_chunk_timeout"
			if receivedLine {
				reason = "stream_idle_timeout"
			}
			if emittedAny {
				return partialStreamResult(content.String(), tokens, reason), false, 0, nil
			}
			return nil, true, 0, apperr.Newf(apperr.CodeUnavailable, "model provider %s", reason)
		}
		if emittedAny {
			return partialStreamResult(content.String(), tokens, "stream_read_error"), false, 0, nil
		}
		return nil, true, 0, apperr.Wrap(err, apperr.CodeUnavailable, "read model provider stream failed")
	}
	finalContent := content.String()
	// provider 明确返回 length/max_tokens 时，优先按输出超限保留部分结果；
	// 若正文恰好仍是完整 JSON（极少数 provider 误标），继续走正常解析。
	if isLengthFinishReason(finishReason) && !json.Valid([]byte(strings.TrimSpace(finalContent))) {
		return partialStreamResult(finalContent, tokens, "output_truncated"), false, 0, nil
	}
	if strings.TrimSpace(finalContent) == "" {
		if emittedAny {
			return partialStreamResult(finalContent, tokens, "empty_model_response"), false, 0, nil
		}
		return nil, true, 0, apperr.New(apperr.CodeUnavailable, "empty model response")
	}
	return &ChatCompletionResult{Content: finalContent, Tokens: tokens}, false, 0, nil
}

func isLengthFinishReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length", "max_tokens", "max_output_tokens":
		return true
	default:
		return false
	}
}

// partialStreamResult 把已输出的部分内容包装为可保留结果；不触发重试。
func partialStreamResult(content string, tokens int, reason string) *ChatCompletionResult {
	return &ChatCompletionResult{
		Content: content, Tokens: tokens,
		Partial: true, PartialReason: reason,
	}
}

// chatRetryDelay 返回指数退避 + 抖动延迟（1s/2s 基础上加最多 50% 抖动）。
func chatRetryDelay(attempt int) time.Duration {
	base := chatRetryBaseDelay
	for i := 0; i < attempt && base < 8*time.Second; i++ {
		base *= 2
	}
	jitter := time.Duration(0)
	if half := int64(base / 2); half > 0 {
		jitter = time.Duration(rand.Int64N(half))
	}
	return base + jitter
}

// parseRetryAfterDuration 解析 Retry-After（秒数或 HTTP 日期）；无法解析返回 0。
func parseRetryAfterDuration(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(raw); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

// quotaResetPattern 解析 provider 429 响应体中的配额恢复提示，例如
// "quota resets in 5h" / "try again in 30m" / "quotaResetDelay: 1h2m"。
var quotaResetPattern = regexp.MustCompile(`(?i)(?:resets?\s+(?:in|after)|reset\s+(?:in|after)|try\s+again\s+in|retry\s+after|quota[_\s-]*reset(?:[_\s-]*delay)?)[^0-9]{0,24}(\d+)\s*(ms|s|sec|secs|seconds?|m|min|mins|minutes?|h|hr|hrs|hours?)`)

// parseQuotaResetHint 从 429 响应体提取预计恢复时长；无法解析返回 0。
// 只解析时长，不保留或记录响应体，避免把 provider 错误细节写入日志。
func parseQuotaResetHint(raw string) time.Duration {
	match := quotaResetPattern.FindStringSubmatch(raw)
	if len(match) != 3 {
		return 0
	}
	value, err := strconv.Atoi(match[1])
	if err != nil || value <= 0 {
		return 0
	}
	switch unit := strings.ToLower(match[2]); {
	case strings.HasPrefix(unit, "ms"):
		return time.Duration(value) * time.Millisecond
	case strings.HasPrefix(unit, "s"):
		return time.Duration(value) * time.Second
	case strings.HasPrefix(unit, "m"):
		return time.Duration(value) * time.Minute
	case strings.HasPrefix(unit, "h"):
		return time.Duration(value) * time.Hour
	default:
		return 0
	}
}

// sleepWithContext 在退避等待期间响应父级取消/回合预算。
func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
