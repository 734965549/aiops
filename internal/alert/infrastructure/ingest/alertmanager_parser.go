package ingest

import (
	"strings"
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

// ParseAlertmanagerWebhook 将 Alertmanager payload 归一化为平台中间结构列表。
func ParseAlertmanagerWebhook(payload AlertmanagerWebhook, defaults EnvironmentDefaults) []NormalizedAlert {
	out := make([]NormalizedAlert, 0, len(payload.Alerts))
	for _, a := range payload.Alerts {
		labels := mergeMaps(payload.CommonLabels, a.Labels)
		annotations := mergeMaps(payload.CommonAnnotations, a.Annotations)
		name := firstNonEmpty(labels["alertname"], "unknown-alert")
		env := firstNonEmpty(labels["env"], labels["environment"], defaults.Environment)
		biz := firstNonEmpty(labels["business_line"], labels["team"], defaults.BusinessLine)
		appName := firstNonEmpty(labels["service"], labels["app"], labels["application"])
		resourceName := firstNonEmpty(labels["instance"], labels["pod"], labels["node"])
		resourceType := ""
		if labels["pod"] != "" {
			resourceType = "pod"
		} else if labels["node"] != "" {
			resourceType = "node"
		} else if labels["instance"] != "" {
			resourceType = "host"
		}
		firstSeen := parseAMTime(a.StartsAt, time.Now())
		var recoveredAt *time.Time
		if strings.EqualFold(a.Status, "resolved") || strings.EqualFold(payload.Status, "resolved") {
			t := parseAMTime(a.EndsAt, time.Now())
			recoveredAt = &t
		}
		status := "firing"
		if strings.EqualFold(a.Status, "resolved") {
			status = "resolved"
		}
		fp := strings.TrimSpace(a.Fingerprint)
		out = append(out, NormalizedAlert{
			ExternalID:      fp,
			Fingerprint:     fp,
			Status:          status,
			Name:            name,
			Summary:         annotations["summary"],
			Description:     annotations["description"],
			Severity:        NormalizeSeverity(labels["severity"]),
			RuleName:        name,
			BusinessLine:    biz,
			Environment:     env,
			ApplicationName: appName,
			ResourceType:    resourceType,
			ResourceName:    resourceName,
			Labels:          labels,
			Annotations:     annotations,
			FirstSeenAt:     firstSeen,
			RecoveredAt:     recoveredAt,
		})
	}
	return out
}

// EnvironmentDefaults 接入源默认环境/业务线。
type EnvironmentDefaults struct {
	Environment  string
	BusinessLine string
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

// ParseGenericWebhook 解析通用 Webhook payload。
func ParseGenericWebhook(payload GenericWebhookPayload, defaults EnvironmentDefaults) NormalizedAlert {
	status := strings.ToLower(strings.TrimSpace(payload.Status))
	if status == "" {
		status = "firing"
	}
	firstSeen := time.Now()
	if payload.StartsAt > 0 {
		firstSeen = time.Unix(payload.StartsAt, 0)
	}
	var recoveredAt *time.Time
	if payload.EndsAt > 0 {
		t := time.Unix(payload.EndsAt, 0)
		recoveredAt = &t
	}
	if status == "resolved" && recoveredAt == nil {
		now := time.Now()
		recoveredAt = &now
	}
	env := firstNonEmpty(payload.Environment, defaults.Environment)
	biz := firstNonEmpty(payload.BusinessLine, defaults.BusinessLine)
	ext := strings.TrimSpace(payload.ExternalID)
	return NormalizedAlert{
		ExternalID:      ext,
		Fingerprint:     ext,
		Status:          status,
		Name:            strings.TrimSpace(payload.Name),
		Summary:         payload.Summary,
		Description:     payload.Description,
		Severity:        NormalizeSeverity(payload.Severity),
		RuleName:        strings.TrimSpace(payload.Name),
		BusinessLine:    biz,
		Environment:     env,
		ApplicationName: payload.ApplicationName,
		ResourceType:    payload.ResourceType,
		ResourceName:    payload.ResourceName,
		Labels:          cloneStringMap(payload.Labels),
		Annotations:     cloneStringMap(payload.Annotations),
		FirstSeenAt:     firstSeen,
		RecoveredAt:     recoveredAt,
	}
}

func mergeMaps(base, override map[string]string) map[string]string {
	out := cloneStringMap(base)
	for k, v := range override {
		out[k] = v
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func parseAMTime(raw string, fallback time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0001-01-01T00:00:00Z" {
		return fallback
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	return fallback
}
