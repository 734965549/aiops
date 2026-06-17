package domain

import "time"

// Task 执行任务主记录。
type Task struct {
	ID                string
	Name              string
	SourceType        SourceType
	SourceID          string
	OperationType     OperationType
	TargetType        string
	TargetID          string
	TargetName        string
	Environment       string
	RiskLevel         RiskLevel
	Status            TaskStatus
	Parameters        map[string]any
	RollbackPlan      map[string]any
	RunbookTemplateID string
	RunbookSnapshot   map[string]any
	DryRun            bool
	ResultSummary     string
	ErrorMessage      string
	CreatedBy         string
	ConfirmedBy       string
	ExecutedBy        string
	ConfirmedAt       *time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Step 执行步骤。
type Step struct {
	ID              string
	TaskID          string
	StepOrder       int
	Name            string
	ActionType      string
	Status          StepStatus
	RunbookStepID   string
	Parameters      map[string]any
	RiskLevel       RiskLevel
	DryRun          bool
	DryRunSupported bool
	RollbackPlan    map[string]any
	TimeoutSeconds  int
	Output          map[string]any
	ErrorMessage    string
	StartedAt       *time.Time
	FinishedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
