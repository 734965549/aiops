package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/execution/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// CommandSpecService 受控命令规格查询。
type CommandSpecService struct {
	specs domain.CommandSpecRepository
}

func NewCommandSpecService(specs domain.CommandSpecRepository) *CommandSpecService {
	return &CommandSpecService{specs: specs}
}

type CommandSpecDTO struct {
	CommandSpecID   string         `json:"command_spec_id"`
	Name            string         `json:"name"`
	ActionType      string         `json:"action_type"`
	MediumTypes     []string       `json:"medium_types"`
	RiskLevel       string         `json:"risk_level"`
	CommandTemplate string         `json:"command_template"`
	ArgumentSchema  map[string]any `json:"argument_schema"`
	TimeoutSeconds  int            `json:"timeout_seconds"`
	RequiredCaps    []string       `json:"required_caps,omitempty"`
	Enabled         bool           `json:"enabled"`
	Description     string         `json:"description,omitempty"`
}

func (s *CommandSpecService) Get(ctx context.Context, commandSpecID string) (*CommandSpecDTO, error) {
	if s == nil || s.specs == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "command spec service is not enabled")
	}
	spec, err := s.specs.GetByID(ctx, commandSpecID)
	if err != nil {
		return nil, wrapExecError(err, "get command spec failed")
	}
	return toCommandSpecDTO(*spec), nil
}

func (s *CommandSpecService) List(ctx context.Context, enabled *bool, page, pageSize int) ([]CommandSpecDTO, int64, error) {
	if s == nil || s.specs == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "command spec service is not enabled")
	}
	page, pageSize = normalizePage(page, pageSize)
	rows, err := s.specs.List(ctx, enabled, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, wrapExecError(err, "list command specs failed")
	}
	total, err := s.specs.Count(ctx, enabled)
	if err != nil {
		return nil, 0, wrapExecError(err, "count command specs failed")
	}
	out := make([]CommandSpecDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toCommandSpecDTO(row))
	}
	return out, total, nil
}

func toCommandSpecDTO(spec domain.CommandSpec) *CommandSpecDTO {
	schema := spec.ArgumentSchema
	if schema == nil {
		schema = map[string]any{}
	}
	return &CommandSpecDTO{
		CommandSpecID: spec.CommandSpecID, Name: spec.Name, ActionType: spec.ActionType,
		MediumTypes: spec.MediumTypes, RiskLevel: string(spec.RiskLevel),
		CommandTemplate: spec.CommandTemplate, ArgumentSchema: schema,
		TimeoutSeconds: spec.TimeoutSeconds, RequiredCaps: spec.RequiredCaps,
		Enabled: spec.Enabled, Description: spec.Description,
	}
}

func loadEnabledCommandSpec(ctx context.Context, specs domain.CommandSpecRepository, commandSpecID string) (*domain.CommandSpec, error) {
	spec, err := specs.GetByID(ctx, strings.TrimSpace(commandSpecID))
	if err != nil {
		return nil, err
	}
	if spec == nil || !spec.Enabled {
		return nil, domain.ErrFailedPrecondition
	}
	return spec, nil
}
