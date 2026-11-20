// Package domain 定义 AI 上下文的持久化实体与仓储契约。
package domain

import (
	"context"
	"errors"
	"time"
)

// ErrProviderNotFound 表示 provider 记录不存在。
var ErrProviderNotFound = errors.New("ai provider not found")

// Provider 是持久化的 AI provider 配置。
// APIKeyCiphertext 仅保存密文，明文只在 application/infrastructure 运行时短暂持有。
type Provider struct {
	ProviderID       string
	Name             string
	Type             string
	BaseURL          string
	Model            string
	Headers          map[string]string
	TimeoutMS        int64
	Enabled          bool
	Description      string
	// ReasoningEffort 思考型模型的推理强度（low/high/max）；空串表示不发送该参数。
	ReasoningEffort  string
	// VisionEnabled 表示类型 b 模型接受图片输入（多模态）；false 时带图消息被拒绝。
	VisionEnabled    bool
	APIKeyCiphertext []byte
	HasAPIKey        bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ProviderRepository 是 provider 配置的持久化端口。
type ProviderRepository interface {
	List(ctx context.Context) ([]Provider, error)
	Get(ctx context.Context, providerID string) (*Provider, error)
	Upsert(ctx context.Context, provider Provider) error
	Delete(ctx context.Context, providerID string) error
}
