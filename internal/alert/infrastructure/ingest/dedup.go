// Package ingest 负责外部告警 payload 解析、级别归一化、去重键与 Webhook 密钥处理。
// 契约见 ops/alert-contract.md §4.1、§6、§7.1。
package ingest

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

// ComputeDedupKey 按契约 §7.1 计算平台去重键。
func ComputeDedupKey(sourceID, externalIDOrFingerprint, ruleName, resourceName string, labels map[string]string) string {
	sourceID = strings.TrimSpace(sourceID)
	ext := strings.TrimSpace(externalIDOrFingerprint)
	if ext != "" {
		return hashParts(sourceID, ext)
	}
	selected := selectedLabels(labels)
	return hashParts(sourceID, strings.TrimSpace(ruleName), strings.TrimSpace(resourceName), selected)
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
