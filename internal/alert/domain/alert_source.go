package domain

import "time"

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
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
