package domain

import "time"

// MatchTargetType 规则命中后绑定目标类型。
type MatchTargetType string

const (
	TargetApplication MatchTargetType = "application"
	TargetResource    MatchTargetType = "resource"
)

func (t MatchTargetType) IsValid() bool {
	return t == TargetApplication || t == TargetResource
}

// MatchSourceType 规则适用的告警接入源；all 表示任意来源。
type MatchSourceType string

const (
	MatchSourceAll                    MatchSourceType = "all"
	MatchSourcePrometheusAlertmanager MatchSourceType = "prometheus_alertmanager"
	MatchSourceHuaweiCES              MatchSourceType = "huawei_ces"
	MatchSourceSigNoz                 MatchSourceType = "signoz"
	MatchSourceZabbix                 MatchSourceType = "zabbix"
	MatchSourceCustomWebhook          MatchSourceType = "custom_webhook"
)

func (t MatchSourceType) IsValid() bool {
	switch t {
	case MatchSourceAll, MatchSourcePrometheusAlertmanager, MatchSourceHuaweiCES,
		MatchSourceSigNoz, MatchSourceZabbix, MatchSourceCustomWebhook:
		return true
	default:
		return false
	}
}

// MatchRule 用户配置的告警标签匹配规则。
type MatchRule struct {
	ID                string
	Name              string
	Enabled           bool
	Priority          int
	TargetType        MatchTargetType
	SourceType        MatchSourceType
	LabelKey          string
	LabelValuePattern string
	ApplicationID     string
	ResourceID        string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
