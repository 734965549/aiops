package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/execution/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type commandSpecModel struct {
	database.BaseModel
	CommandSpecID    string `gorm:"column:command_spec_id;type:varchar(64);uniqueIndex;not null"`
	Name             string `gorm:"column:name;type:varchar(128);not null"`
	ActionType       string `gorm:"column:action_type;type:varchar(32);not null;default:'diagnose'"`
	MediumTypes      []byte `gorm:"column:medium_types;type:jsonb;not null;default:'[]'::jsonb"`
	RiskLevel        string `gorm:"column:risk_level;type:varchar(16);not null;default:'low'"`
	CommandTemplate  string `gorm:"column:command_template;type:text;not null"`
	ArgumentSchema   []byte `gorm:"column:argument_schema;type:jsonb;not null;default:'{}'::jsonb"`
	TimeoutSeconds   int    `gorm:"column:timeout_seconds;not null;default:30"`
	AllowedExitCodes []byte `gorm:"column:allowed_exit_codes;type:jsonb;not null;default:'[0]'::jsonb"`
	OutputRedaction  []byte `gorm:"column:output_redaction;type:jsonb;not null;default:'{}'::jsonb"`
	RequiredCaps     []byte `gorm:"column:required_caps;type:jsonb;not null;default:'[]'::jsonb"`
	Enabled          bool   `gorm:"column:enabled;not null;default:true"`
	Description      string `gorm:"column:description;type:varchar(512);not null;default:''"`
}

func (commandSpecModel) TableName() string { return "exec_command_spec" }

type CommandSpecRepository struct{ db *gorm.DB }

func NewCommandSpecRepository(db *gorm.DB) *CommandSpecRepository {
	return &CommandSpecRepository{db: db}
}

func (r *CommandSpecRepository) Create(ctx context.Context, spec *domain.CommandSpec) error {
	if r == nil || r.db == nil || spec == nil {
		return errors.New("command spec repository is not configured")
	}
	m, err := toCommandSpecModel(spec)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	fillCommandSpecFromModel(spec, m)
	return nil
}

func (r *CommandSpecRepository) Update(ctx context.Context, spec *domain.CommandSpec) error {
	if r == nil || r.db == nil || spec == nil {
		return errors.New("command spec repository is not configured")
	}
	m, err := toCommandSpecModel(spec)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&commandSpecModel{}).Where("command_spec_id = ?", spec.CommandSpecID).Updates(m)
	if res.Error != nil {
		return database.MapUniqueViolation(res.Error, domain.ErrAlreadyExists)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *CommandSpecRepository) GetByID(ctx context.Context, commandSpecID string) (*domain.CommandSpec, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("command spec repository is not configured")
	}
	var m commandSpecModel
	if err := r.db.WithContext(ctx).Where("command_spec_id = ?", strings.TrimSpace(commandSpecID)).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toCommandSpecDomain(&m)
	return &out, nil
}

func (r *CommandSpecRepository) List(ctx context.Context, enabled *bool, limit, offset int) ([]domain.CommandSpec, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("command spec repository is not configured")
	}
	q := r.db.WithContext(ctx).Model(&commandSpecModel{})
	if enabled != nil {
		q = q.Where("enabled = ?", *enabled)
	}
	var rows []commandSpecModel
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.CommandSpec, 0, len(rows))
	for _, row := range rows {
		out = append(out, toCommandSpecDomain(&row))
	}
	return out, nil
}

func (r *CommandSpecRepository) Count(ctx context.Context, enabled *bool) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("command spec repository is not configured")
	}
	q := r.db.WithContext(ctx).Model(&commandSpecModel{})
	if enabled != nil {
		q = q.Where("enabled = ?", *enabled)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func toCommandSpecModel(spec *domain.CommandSpec) (*commandSpecModel, error) {
	mediumTypes, err := marshalStringSlice(spec.MediumTypes)
	if err != nil {
		return nil, err
	}
	schema, err := marshalAnyMap(spec.ArgumentSchema)
	if err != nil {
		return nil, err
	}
	exitCodes, err := marshalIntSlice(spec.AllowedExitCodes)
	if err != nil {
		return nil, err
	}
	redaction, err := marshalAnyMap(spec.OutputRedaction)
	if err != nil {
		return nil, err
	}
	caps, err := marshalStringSlice(spec.RequiredCaps)
	if err != nil {
		return nil, err
	}
	return &commandSpecModel{
		CommandSpecID: spec.CommandSpecID, Name: spec.Name, ActionType: spec.ActionType,
		MediumTypes: mediumTypes, RiskLevel: string(spec.RiskLevel), CommandTemplate: spec.CommandTemplate,
		ArgumentSchema: schema, TimeoutSeconds: spec.TimeoutSeconds, AllowedExitCodes: exitCodes,
		OutputRedaction: redaction, RequiredCaps: caps, Enabled: spec.Enabled, Description: spec.Description,
	}, nil
}

func toCommandSpecDomain(m *commandSpecModel) domain.CommandSpec {
	return domain.CommandSpec{
		CommandSpecID: m.CommandSpecID, Name: m.Name, ActionType: m.ActionType,
		MediumTypes: unmarshalStringSlice(m.MediumTypes), RiskLevel: domain.RiskLevel(m.RiskLevel),
		CommandTemplate: m.CommandTemplate, ArgumentSchema: unmarshalAnyMap(m.ArgumentSchema),
		TimeoutSeconds: m.TimeoutSeconds, AllowedExitCodes: unmarshalIntSlice(m.AllowedExitCodes),
		OutputRedaction: unmarshalAnyMap(m.OutputRedaction), RequiredCaps: unmarshalStringSlice(m.RequiredCaps),
		Enabled: m.Enabled, Description: m.Description, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func fillCommandSpecFromModel(spec *domain.CommandSpec, m *commandSpecModel) {
	spec.CreatedAt = m.CreatedAt
	spec.UpdatedAt = m.UpdatedAt
}
