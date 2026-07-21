package application

import (
	"strings"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
)

// ParseAlertmanagerWebhook 将 Alertmanager payload 归一化为平台中间结构列表。
func ParseAlertmanagerWebhook(payload AlertmanagerWebhook, defaults EnvironmentDefaults) []NormalizedAlert {
	out := make([]NormalizedAlert, 0, len(payload.Alerts))
	for _, a := range payload.Alerts {
		labels := mergeIngestMaps(payload.CommonLabels, a.Labels)
		annotations := mergeIngestMaps(payload.CommonAnnotations, a.Annotations)
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
		firstSeen := parseAlertmanagerTime(a.StartsAt, time.Now())
		var recoveredAt *time.Time
		if strings.EqualFold(a.Status, "resolved") || strings.EqualFold(payload.Status, "resolved") {
			t := parseAlertmanagerTime(a.EndsAt, time.Now())
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
			Severity:        domain.NormalizeSeverity(labels["severity"]),
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
		Severity:        domain.NormalizeSeverity(payload.Severity),
		RuleName:        strings.TrimSpace(payload.Name),
		BusinessLine:    biz,
		Environment:     env,
		ApplicationName: payload.ApplicationName,
		ResourceType:    payload.ResourceType,
		ResourceName:    payload.ResourceName,
		Labels:          cloneIngestStringMap(payload.Labels),
		Annotations:     cloneIngestStringMap(payload.Annotations),
		FirstSeenAt:     firstSeen,
		RecoveredAt:     recoveredAt,
	}
}

func mergeIngestMaps(base, override map[string]string) map[string]string {
	out := cloneIngestStringMap(base)
	for k, v := range override {
		out[k] = v
	}
	return out
}

func cloneIngestStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func parseAlertmanagerTime(raw string, fallback time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0001-01-01T00:00:00Z" {
		return fallback
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	return fallback
}
