package application

import (
	"time"

	"github.com/734965549/aiops/internal/execution/domain"
)

// Actor 操作者。
type Actor struct {
	UserID      string
	DisplayName string
}

// TaskDTO 对外任务对象。
type TaskDTO struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	SourceType        string         `json:"source_type"`
	SourceID          string         `json:"source_id,omitempty"`
	OperationType     string         `json:"operation_type"`
	TargetType        string         `json:"target_type,omitempty"`
	TargetID          string         `json:"target_id,omitempty"`
	TargetName        string         `json:"target_name,omitempty"`
	Environment       string         `json:"environment,omitempty"`
	RiskLevel         string         `json:"risk_level"`
	Status            string         `json:"status"`
	Parameters        map[string]any `json:"parameters"`
	RollbackPlan      map[string]any `json:"rollback_plan,omitempty"`
	RunbookTemplateID string         `json:"runbook_template_id,omitempty"`
	RunbookName       string         `json:"runbook_name,omitempty"`
	DryRun            bool           `json:"dry_run"`
	ExecutionMode     string         `json:"execution_mode,omitempty"`
	MediumID          string         `json:"medium_id,omitempty"`
	AgentID           string         `json:"agent_id,omitempty"`
	DispatchStatus    string         `json:"dispatch_status,omitempty"`
	LeaseID           string         `json:"lease_id,omitempty"`
	CommandSpecID     string         `json:"command_spec_id,omitempty"`
	ResultSummary     string         `json:"result_summary,omitempty"`
	ErrorMessage      string         `json:"error_message,omitempty"`
	CreatedBy         string         `json:"created_by,omitempty"`
	ConfirmedBy       string         `json:"confirmed_by,omitempty"`
	ExecutedBy        string         `json:"executed_by,omitempty"`
	CreatedAt         int64          `json:"created_at"`
	ConfirmedAt       int64          `json:"confirmed_at,omitempty"`
	StartedAt         int64          `json:"started_at,omitempty"`
	FinishedAt        int64          `json:"finished_at,omitempty"`
}

// StepDTO 对外步骤对象。
type StepDTO struct {
	ID           string         `json:"id"`
	TaskID       string         `json:"task_id"`
	StepOrder    int            `json:"step_order"`
	Name         string         `json:"name"`
	ActionType   string         `json:"action_type"`
	Status       string         `json:"status"`
	RiskLevel    string         `json:"risk_level,omitempty"`
	DryRun       bool           `json:"dry_run"`
	Parameters   map[string]any `json:"parameters"`
	RollbackPlan map[string]any `json:"rollback_plan,omitempty"`
	Output       map[string]any `json:"output"`
	ErrorMessage string         `json:"error_message,omitempty"`
	StartedAt    int64          `json:"started_at,omitempty"`
	FinishedAt   int64          `json:"finished_at,omitempty"`
}

// TaskDetailDTO 任务详情。
type TaskDetailDTO struct {
	Task  TaskDTO   `json:"task"`
	Steps []StepDTO `json:"steps"`
}

// CreateTaskResult 创建任务响应。
type CreateTaskResult struct {
	TaskID     string `json:"task_id"`
	Status     string `json:"status"`
	RiskLevel  string `json:"risk_level"`
	ConfirmURL string `json:"confirm_url,omitempty"`
}

func ToTaskDTO(t domain.Task) TaskDTO {
	params := t.Parameters
	if params == nil {
		params = map[string]any{}
	}
	rollback := t.RollbackPlan
	if rollback == nil {
		rollback = map[string]any{}
	}
	dto := TaskDTO{
		ID:                t.ID,
		Name:              t.Name,
		SourceType:        string(t.SourceType),
		SourceID:          t.SourceID,
		OperationType:     string(t.OperationType),
		TargetType:        t.TargetType,
		TargetID:          t.TargetID,
		TargetName:        t.TargetName,
		Environment:       t.Environment,
		RiskLevel:         string(t.RiskLevel),
		Status:            string(t.Status),
		Parameters:        params,
		RollbackPlan:      rollback,
		RunbookTemplateID: t.RunbookTemplateID,
		DryRun:            t.DryRun,
		ExecutionMode:     string(t.ExecutionMode),
		MediumID:          t.MediumID,
		AgentID:           t.AgentID,
		DispatchStatus:    string(t.DispatchStatus),
		LeaseID:           t.LeaseID,
		CommandSpecID:     t.CommandSpecID,
		ResultSummary:     t.ResultSummary,
		ErrorMessage:      t.ErrorMessage,
		CreatedBy:         t.CreatedBy,
		ConfirmedBy:       t.ConfirmedBy,
		ExecutedBy:        t.ExecutedBy,
		CreatedAt:         unixOrZero(t.CreatedAt),
		ConfirmedAt:       unixPtr(t.ConfirmedAt),
		StartedAt:         unixPtr(t.StartedAt),
		FinishedAt:        unixPtr(t.FinishedAt),
	}
	if dto.RunbookTemplateID != "" {
		if name, ok := t.RunbookSnapshot["name"].(string); ok && name != "" {
			dto.RunbookName = name
		}
	}
	return dto
}

func ToStepDTO(s domain.Step) StepDTO {
	out := s.Output
	if out == nil {
		out = map[string]any{}
	}
	params := s.Parameters
	if params == nil {
		params = map[string]any{}
	}
	rollback := s.RollbackPlan
	if rollback == nil {
		rollback = map[string]any{}
	}
	return StepDTO{
		ID:           s.ID,
		TaskID:       s.TaskID,
		StepOrder:    s.StepOrder,
		Name:         s.Name,
		ActionType:   s.ActionType,
		Status:       string(s.Status),
		RiskLevel:    string(s.RiskLevel),
		DryRun:       s.DryRun,
		Parameters:   params,
		RollbackPlan: rollback,
		Output:       out,
		ErrorMessage: s.ErrorMessage,
		StartedAt:    unixPtr(s.StartedAt),
		FinishedAt:   unixPtr(s.FinishedAt),
	}
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func unixPtr(t *time.Time) int64 {
	if t == nil || t.IsZero() {
		return 0
	}
	return t.Unix()
}
