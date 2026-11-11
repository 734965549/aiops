package domain

import (
	"strings"
	"time"
)

// AlertSource 领域模型见 ops/alert-contract.md §5.3；Webhook 鉴权见 §3.2。

// AlertSourceType 外部接入源类型。
type AlertSourceType string

const (
	SourcePrometheusAlertmanager AlertSourceType = "prometheus_alertmanager"
	SourceHuaweiCES              AlertSourceType = "huawei_ces"
	SourceSigNoz                 AlertSourceType = "signoz"
	SourceZabbix                 AlertSourceType = "zabbix"
	SourceCustomWebhook          AlertSourceType = "custom_webhook"
)

// IsValid 判断是否为契约 §5.3 定义的接入源类型。
func (t AlertSourceType) IsValid() bool {
	switch t {
	case SourcePrometheusAlertmanager, SourceHuaweiCES, SourceSigNoz, SourceZabbix, SourceCustomWebhook:
		return true
	default:
		return false
	}
}

// AlertSource 是外部告警接入源配置，对应 alert_source 表。
// SecretHash 仅存 token 哈希，接口通过 secret_masked 展示掩码。
type AlertSource struct {
	ID           string          // 接入源 ID（Webhook URL 路径参数）
	Name         string          // 显示名
	Type         AlertSourceType // 接入类型
	Enabled      bool            // 是否启用
	SecretHash   string          // Webhook token sha256 hex
	Environment  string          // 默认环境
	BusinessLine string          // 默认业务线
	Description  string          // 备注
	// 拉取式接入扩展字段，见 ops/huawei-ces-alert-sync-contract.md §4.1。
	AccountID      string          // 绑定的 integration account ID
	PullConfig     PullConfig      // 拉取配置（delivery_mode/interval/severity_map/region_filter）
	LastSyncAt     *time.Time      // 最近一轮拉取完成时间
	LastSyncStatus string          // success / partial / failed / 空
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsPullSource 判断是否为平台主动拉取接入源：
// type=huawei_ces、绑定账号且 delivery_mode 不是 webhook。
func (s AlertSource) IsPullSource() bool {
	if s.Type != SourceHuaweiCES || strings.TrimSpace(s.AccountID) == "" {
		return false
	}
	return s.PullConfig.DeliveryMode != DeliveryModeWebhook
}

// IsWebhookSource 判断是否为 Webhook 推送接入源。
func (s AlertSource) IsWebhookSource() bool { return !s.IsPullSource() }
