package domain

import "time"

// Template 预案模板。
type Template struct {
	ID                string
	Name              string
	Description       string
	Enabled           bool
	OperationType     OperationType
	RiskLevel         RiskLevel
	MatchAlertName    string
	MatchResourceType string
	MatchEnvironment  string
	ParameterSchema   map[string]any
	RollbackPlan      map[string]any
	CreatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Step 预案步骤模板。
type Step struct {
	ID                string
	TemplateID        string
	StepOrder         int
	Name              string
	ActionType        ActionType
	RiskLevel         RiskLevel
	DryRunSupported   bool
	DefaultDryRun     bool
	ParameterSchema   map[string]any
	DefaultParameters map[string]any
	RollbackPlan      map[string]any
	TimeoutSeconds    int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// TemplateWithSteps 模板含步骤。
type TemplateWithSteps struct {
	Template Template
	Steps    []Step
}
