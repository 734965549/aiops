package ingest

import "github.com/734965549/aiops/internal/alert/domain"

// NormalizeSeverity 将外部级别归一化为平台 p0-p3/info，委托 domain 层实现（§4.1）。
func NormalizeSeverity(raw string) domain.AlertSeverity {
	return domain.NormalizeSeverity(raw)
}
