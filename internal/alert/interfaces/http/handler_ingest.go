package http

import (
	"encoding/json"
	"io"
	"strings"

	alertapp "github.com/734965549/aiops/internal/alert/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	httpx "github.com/734965549/aiops/pkg/transport/http"
	"github.com/gin-gonic/gin"
)

// Webhook 接入（ops/alert-contract.md §3.2）：不经 Bearer，使用 X-AIOPS-Webhook-Token。
//
// §3.2 服务端必须：
//   - 按 source_id 查启用接入源（IngestService.VerifySource）
//   - 校验 token，失败 UNAUTHENTICATED（401）
//   - 请求体默认上限 1MB，超限 PAYLOAD_TOO_LARGE（413）
//   - 记录 IP、User-Agent、trace_id（recordIntegrationEvent）
//   - X-Request-ID 短期幂等（生产 RedisStore 跨 Pod；dev 可 MemoryStore）
//
// 后续可扩展 HMAC 签名（§3.2 X-AIOPS-Signature）。
const webhookTokenHeader = "X-AIOPS-Webhook-Token"
const requestIDHeader = "X-Request-ID"
const maxWebhookBodyBytes = 1 << 20 // §3.2 默认 1MB

// IngestHandler Webhook 接入 HTTP 层（§3.2 共享密钥，不用 Bearer token）。
type IngestHandler struct {
	ingest *alertapp.IngestService
}

// NewIngestHandler 构造 Webhook Handler。
func NewIngestHandler(ingest *alertapp.IngestService) *IngestHandler {
	return &IngestHandler{ingest: ingest}
}

// IngestAlertmanager POST /api/alerts/ingest/alertmanager/:source_id（§6.1）。
// 成功 data 字段见 §6.1 响应表；统一 envelope 见 §2。
func (h *IngestHandler) IngestAlertmanager(c *gin.Context) {
	if h.ingest == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert ingest is not enabled")
		return
	}
	body, err := readWebhookBody(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var payload alertapp.AlertmanagerWebhook
	if err := decodeJSON(body, &payload); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid alertmanager payload")
		return
	}
	ctx := buildIngestContext(c)
	out, err := h.ingest.IngestAlertmanager(c.Request.Context(), ctx, payload)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// IngestWebhook POST /api/alerts/ingest/webhook/:source_id（§6.2）。
// 成功 data 为接入统计，响应 envelope 同 §2。
func (h *IngestHandler) IngestWebhook(c *gin.Context) {
	if h.ingest == nil {
		httpx.FailWith(c, apperr.CodeUnavailable, "alert ingest is not enabled")
		return
	}
	body, err := readWebhookBody(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var payload alertapp.GenericWebhookPayload
	if err := decodeJSON(body, &payload); err != nil {
		httpx.FailWith(c, apperr.CodeInvalidArgument, "invalid webhook payload")
		return
	}
	ctx := buildIngestContext(c)
	out, err := h.ingest.IngestGeneric(c.Request.Context(), ctx, payload)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func buildIngestContext(c *gin.Context) alertapp.IngestContext {
	// §3.2 / §6.1：收集鉴权与审计元数据，传给 application 层。
	return alertapp.IngestContext{
		SourceID:  c.Param("source_id"),
		Token:     strings.TrimSpace(c.GetHeader(webhookTokenHeader)),
		RequestID: strings.TrimSpace(c.GetHeader(requestIDHeader)),
		IP:        c.ClientIP(),
		UserAgent: strings.TrimSpace(c.GetHeader("User-Agent")),
	}
}

// readWebhookBody 读取 Webhook 请求体，§3.2 默认上限 1MB，超限返回 PAYLOAD_TOO_LARGE（413）。
func readWebhookBody(c *gin.Context) ([]byte, error) {
	limited := io.LimitReader(c.Request.Body, maxWebhookBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "read webhook body failed")
	}
	if len(data) > maxWebhookBodyBytes {
		return nil, apperr.New(apperr.CodePayloadTooLarge, "webhook payload too large")
	}
	return data, nil
}

func decodeJSON(data []byte, v any) error {
	if len(data) == 0 {
		return apperr.New(apperr.CodeInvalidArgument, "empty body")
	}
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	return nil
}
