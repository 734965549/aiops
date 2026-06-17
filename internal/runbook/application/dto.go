package application

import (
	"time"

	rbdomain "github.com/734965549/aiops/internal/runbook/domain"
)

// Actor 操作者。
type Actor struct {
	UserID      string
	DisplayName string
}

// TemplateDTO 对外模板对象。
type TemplateDTO struct {
	ID                string         `json:"template_id"`
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	Enabled           bool           `json:"enabled"`
	OperationType     string         `json:"operation_type"`
	RiskLevel         string         `json:"risk_level"`
	MatchAlertName    string         `json:"match_alert_name,omitempty"`
	MatchResourceType string         `json:"match_resource_type,omitempty"`
	MatchEnvironment  string         `json:"match_environment,omitempty"`
	ParameterSchema   map[string]any `json:"parameter_schema"`
	RollbackPlan      map[string]any `json:"rollback_plan,omitempty"`
	CreatedBy         string         `json:"created_by,omitempty"`
	CreatedAt         int64          `json:"created_at"`
	UpdatedAt         int64          `json:"updated_at"`
}

// StepDTO 对外步骤模板对象。
type StepDTO struct {
	StepID            string         `json:"step_id"`
	TemplateID        string         `json:"template_id"`
	StepOrder         int            `json:"step_order"`
	Name              string         `json:"name"`
	ActionType        string         `json:"action_type"`
	RiskLevel         string         `json:"risk_level"`
	DryRunSupported   bool           `json:"dry_run_supported"`
	DefaultDryRun     bool           `json:"default_dry_run"`
	ParameterSchema   map[string]any `json:"parameter_schema"`
	DefaultParameters map[string]any `json:"default_parameters"`
	RollbackPlan      map[string]any `json:"rollback_plan,omitempty"`
	TimeoutSeconds    int            `json:"timeout_seconds"`
	CreatedAt         int64          `json:"created_at"`
	UpdatedAt         int64          `json:"updated_at"`
}

// TemplateDetailDTO 模板详情含步骤。
type TemplateDetailDTO struct {
	Template TemplateDTO `json:"template"`
	Steps    []StepDTO   `json:"steps"`
}

// RecommendationDTO 推荐项。
type RecommendationDTO struct {
	TemplateID      string         `json:"template_id"`
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	RiskLevel       string         `json:"risk_level"`
	OperationType   string         `json:"operation_type"`
	MatchedReason   string         `json:"matched_reason"`
	StepsCount      int            `json:"steps_count"`
	DryRunSupported bool           `json:"dry_run_supported"`
	ParameterSchema map[string]any `json:"parameter_schema"`
}

func ToTemplateDTO(t rbdomain.Template) TemplateDTO {
	schema := t.ParameterSchema
	if schema == nil {
		schema = map[string]any{}
	}
	rollback := t.RollbackPlan
	if rollback == nil {
		rollback = map[string]any{}
	}
	return TemplateDTO{
		ID:                t.ID,
		Name:              t.Name,
		Description:       t.Description,
		Enabled:           t.Enabled,
		OperationType:     string(t.OperationType),
		RiskLevel:         string(t.RiskLevel),
		MatchAlertName:    t.MatchAlertName,
		MatchResourceType: t.MatchResourceType,
		MatchEnvironment:  t.MatchEnvironment,
		ParameterSchema:   schema,
		RollbackPlan:      rollback,
		CreatedBy:         t.CreatedBy,
		CreatedAt:         unixOrZero(t.CreatedAt),
		UpdatedAt:         unixOrZero(t.UpdatedAt),
	}
}

func ToStepDTO(s rbdomain.Step) StepDTO {
	schema := s.ParameterSchema
	if schema == nil {
		schema = map[string]any{}
	}
	defaults := s.DefaultParameters
	if defaults == nil {
		defaults = map[string]any{}
	}
	rollback := s.RollbackPlan
	if rollback == nil {
		rollback = map[string]any{}
	}
	return StepDTO{
		StepID:            s.ID,
		TemplateID:        s.TemplateID,
		StepOrder:         s.StepOrder,
		Name:              s.Name,
		ActionType:        string(s.ActionType),
		RiskLevel:         string(s.RiskLevel),
		DryRunSupported:   s.DryRunSupported,
		DefaultDryRun:     s.DefaultDryRun,
		ParameterSchema:   schema,
		DefaultParameters: defaults,
		RollbackPlan:      rollback,
		TimeoutSeconds:    s.TimeoutSeconds,
		CreatedAt:         unixOrZero(s.CreatedAt),
		UpdatedAt:         unixOrZero(s.UpdatedAt),
	}
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
