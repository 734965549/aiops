package domain

// Capability Provider 能力声明。
type Capability string

const (
	CapabilityMetrics  Capability = "metrics"
	CapabilityLogs     Capability = "logs"
	CapabilityTraces   Capability = "traces"
	CapabilityTopology Capability = "topology"
	CapabilityAlerts   Capability = "alerts"
	CapabilityAssets   Capability = "assets"
)

func (c Capability) IsValid() bool {
	switch c {
	case CapabilityMetrics, CapabilityLogs, CapabilityTraces, CapabilityTopology, CapabilityAlerts, CapabilityAssets:
		return true
	default:
		return false
	}
}

// DefaultCapabilitiesForProvider 返回 Provider 占位能力声明。
func DefaultCapabilitiesForProvider(provider ProviderType) []Capability {
	switch provider {
	case ProviderHuaweiCloud:
		return []Capability{
			CapabilityAssets, CapabilityMetrics, CapabilityLogs, CapabilityTraces, CapabilityTopology, CapabilityAlerts,
		}
	case ProviderSigNoz:
		return []Capability{CapabilityMetrics, CapabilityLogs, CapabilityTraces, CapabilityTopology, CapabilityAlerts}
	case ProviderPrometheus:
		return []Capability{CapabilityMetrics, CapabilityAlerts}
	default:
		return nil
	}
}
