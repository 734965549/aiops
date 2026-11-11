package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

var stableLabelKeys = []string{
	"alertname", "job", "instance", "cluster", "namespace",
	"pod", "node", "service", "app", "application", "env", "environment",
}

// ComputeDedupKey 按 ops/alert-contract.md §7.1 计算平台去重键。
func ComputeDedupKey(sourceID, externalIDOrFingerprint, ruleName, resourceName string, labels map[string]string) string {
	sourceID = strings.TrimSpace(sourceID)
	ext := strings.TrimSpace(externalIDOrFingerprint)
	if ext != "" {
		return hashParts(sourceID, ext)
	}
	selected := selectedLabels(labels)
	return hashParts(sourceID, strings.TrimSpace(ruleName), strings.TrimSpace(resourceName), selected)
}

// ComputeCESDedupKey 按接入源、账号、区域、项目和 record_id 区分 CES 告警流水，
// 见 ops/huawei-ces-alert-sync-contract.md §6.2。record_id 是 CES 告警记录的唯一流水号；
// 同一规则下不同资源有各自独立的 record_id，因此天然独立触发、独立恢复。
func ComputeCESDedupKey(sourceID, accountID, region, projectID, recordID string) string {
	return hashParts(
		strings.TrimSpace(sourceID),
		strings.TrimSpace(accountID),
		strings.ToLower(strings.TrimSpace(region)),
		strings.TrimSpace(projectID),
		strings.TrimSpace(recordID),
	)
}

func hashParts(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func selectedLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(stableLabelKeys))
	for _, k := range stableLabelKeys {
		if v, ok := labels[k]; ok && strings.TrimSpace(v) != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	return strings.Join(parts, "&")
}
