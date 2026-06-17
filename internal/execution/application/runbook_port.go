package application

import (
	"context"

	apperr "github.com/734965549/aiops/pkg/errors"
)

// ExecutableRunbook is the execution-side view of a Runbook.
type ExecutableRunbook struct {
	TemplateID      string
	Name            string
	Description     string
	OperationType   string
	RiskLevel       string
	RollbackPlan    map[string]any
	ParameterSchema map[string]any
	Steps           []ExecutableRunbookStep
}

// ExecutableRunbookStep is the execution-side view of a Runbook step.
type ExecutableRunbookStep struct {
	StepID            string
	StepOrder         int
	Name              string
	ActionType        string
	RiskLevel         string
	DryRunSupported   bool
	DefaultDryRun     bool
	ParameterSchema   map[string]any
	DefaultParameters map[string]any
	RollbackPlan      map[string]any
	TimeoutSeconds    int
}

// RunbookLoader loads the execution view required to create a task.
type RunbookLoader interface {
	GetForExecution(ctx context.Context, templateID string) (*ExecutableRunbook, error)
}

// NoopRunbookLoader is used when Runbook is not wired.
type NoopRunbookLoader struct{}

func (NoopRunbookLoader) GetForExecution(context.Context, string) (*ExecutableRunbook, error) {
	return nil, apperr.New(apperr.CodeUnavailable, "runbook loader is not configured")
}
