package application

import (
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
)

// AlertmanagerWebhook 兼容 Alertmanager 默认 Webhook JSON 结构（§6.1）。
type AlertmanagerWebhook struct {
	Receiver          string              `json:"receiver"`
	Status            string              `json:"status"`
	Alerts            []AlertmanagerAlert `json:"alerts"`
	GroupLabels       map[string]string   `json:"groupLabels"`
	CommonLabels      map[string]string   `json:"commonLabels"`
	CommonAnnotations map[string]string   `json:"commonAnnotations"`
	ExternalURL       string              `json:"externalURL"`
}

// AlertmanagerAlert 单条 alerts[] 元素。
type AlertmanagerAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     string            `json:"startsAt"`
	EndsAt       string            `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

// GenericWebhookPayload 通用 Webhook 请求体（§6.2）。
type GenericWebhookPayload struct {
	ExternalID      string            `json:"external_id"`
	Status          string            `json:"status"`
	Name            string            `json:"name"`
	Severity        string            `json:"severity"`
	Summary         string            `json:"summary"`
	Description     string            `json:"description"`
	BusinessLine    string            `json:"business_line"`
	Environment     string            `json:"environment"`
	ApplicationName string            `json:"application_name"`
	ResourceType    string            `json:"resource_type"`
	ResourceName    string            `json:"resource_name"`
	Labels          map[string]string `json:"labels"`
	Annotations     map[string]string `json:"annotations"`
	StartsAt        int64             `json:"starts_at"`
	EndsAt          int64             `json:"ends_at"`
}

// EnvironmentDefaults 接入源默认环境/业务线。
type EnvironmentDefaults struct {
	Environment  string
	BusinessLine string
}

// NormalizedAlert 接入归一化后的中间结构，再由 IngestService 写入 domain.Alert。
type NormalizedAlert struct {
	ExternalID      string
	Fingerprint     string
	Status          string // firing / resolved
	Name            string
	Summary         string
	Description     string
	Severity        domain.AlertSeverity
	RuleName        string
	BusinessLine    string
	Environment     string
	ApplicationName string
	ResourceType    string
	ResourceName    string
	Labels          map[string]string
	Annotations     map[string]string
	FirstSeenAt     time.Time
	RecoveredAt     *time.Time
}
