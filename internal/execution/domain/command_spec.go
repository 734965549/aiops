package domain

import "time"

// CommandSpec 受控命令规格。
type CommandSpec struct {
	CommandSpecID    string
	Name             string
	ActionType       string
	MediumTypes      []string
	RiskLevel        RiskLevel
	CommandTemplate  string
	ArgumentSchema   map[string]any
	TimeoutSeconds   int
	AllowedExitCodes []int
	OutputRedaction  map[string]any
	RequiredCaps     []string
	Enabled          bool
	Description      string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
