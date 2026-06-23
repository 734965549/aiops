package domain

import "time"

const (
	CheckMetricsCPU      = "metrics.cpu"
	CheckMetricsMemory   = "metrics.memory"
	CheckMetricsDisk     = "metrics.disk"
	CheckTracesLatency   = "traces.latency"
	CheckTracesErrorRate = "traces.error_rate"
	CheckLogsErrorBurst  = "logs.error_burst"
)

var supportedInspectionChecks = map[string]struct{}{
	CheckMetricsCPU:      {},
	CheckMetricsMemory:   {},
	CheckMetricsDisk:     {},
	CheckTracesLatency:   {},
	CheckTracesErrorRate: {},
	CheckLogsErrorBurst:  {},
}

func IsSupportedCheck(check string) bool {
	_, ok := supportedInspectionChecks[check]
	return ok
}

// PolicyScope 巡检目标范围；account_id 与 provider 用于观测查询。
type PolicyScope struct {
	Environment    string   `json:"environment"`
	AccountID      string   `json:"account_id"`
	Provider       string   `json:"provider"`
	ApplicationIDs []string `json:"application_ids"`
	ResourceTypes  []string `json:"resource_types"`
}

// InspectionPolicy 巡检策略。
type InspectionPolicy struct {
	PolicyID             string
	Name                 string
	Enabled              bool
	Schedule             string
	Scope                PolicyScope
	Checks               []string
	AgentProfile         string
	NotificationPolicyID string
	Deleted              bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (p *InspectionPolicy) Validate() error {
	if p == nil {
		return ErrInvalidArgument
	}
	if p.Name == "" {
		return ErrInvalidArgument
	}
	if p.Scope.AccountID == "" {
		return ErrScopeIncomplete
	}
	if len(p.Checks) == 0 {
		return ErrInvalidArgument
	}
	for _, check := range p.Checks {
		if !IsSupportedCheck(check) {
			return ErrUnsupportedCheck
		}
	}
	return nil
}
