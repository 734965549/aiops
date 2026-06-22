package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/execution/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type agentModel struct {
	database.BaseModel
	AgentID       string     `gorm:"column:agent_id;type:varchar(64);uniqueIndex;not null"`
	MediumID      string     `gorm:"column:medium_id;type:varchar(64);not null;index"`
	Status        string     `gorm:"column:status;type:varchar(16);not null;default:'registered'"`
	PublicKey     string     `gorm:"column:public_key;type:text;not null;default:''"`
	TokenHash     string     `gorm:"column:token_hash;type:varchar(128);not null;default:''"`
	Version       string     `gorm:"column:version;type:varchar(32);not null;default:''"`
	Capabilities  []byte     `gorm:"column:capabilities;type:jsonb;not null;default:'[]'::jsonb"`
	RunningTasks  int        `gorm:"column:running_tasks;not null;default:0"`
	FreeSlots     int        `gorm:"column:free_slots;not null;default:1"`
	LastHeartbeat *time.Time `gorm:"column:last_heartbeat"`
	Disabled      bool       `gorm:"column:disabled;not null;default:false"`
}

func (agentModel) TableName() string { return "exec_agent" }

type AgentRepository struct{ db *gorm.DB }

func NewAgentRepository(db *gorm.DB) *AgentRepository { return &AgentRepository{db: db} }

func (r *AgentRepository) Create(ctx context.Context, agent *domain.ExecutionAgent) error {
	if r == nil || r.db == nil || agent == nil {
		return errors.New("agent repository is not configured")
	}
	m, err := toAgentModel(agent)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	fillAgentFromModel(agent, m)
	return nil
}

func (r *AgentRepository) Update(ctx context.Context, agent *domain.ExecutionAgent) error {
	if r == nil || r.db == nil || agent == nil {
		return errors.New("agent repository is not configured")
	}
	m, err := toAgentModel(agent)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).Model(&agentModel{}).Where("agent_id = ?", agent.AgentID).Updates(m)
	if res.Error != nil {
		return database.MapUniqueViolation(res.Error, domain.ErrAlreadyExists)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *AgentRepository) GetByID(ctx context.Context, agentID string) (*domain.ExecutionAgent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("agent repository is not configured")
	}
	var m agentModel
	if err := r.db.WithContext(ctx).Where("agent_id = ?", strings.TrimSpace(agentID)).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toAgentDomain(&m)
	return &out, nil
}

func (r *AgentRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.ExecutionAgent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("agent repository is not configured")
	}
	var m agentModel
	if err := r.db.WithContext(ctx).Where("token_hash = ? AND disabled = FALSE", strings.TrimSpace(tokenHash)).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	out := toAgentDomain(&m)
	return &out, nil
}

func (r *AgentRepository) ListByMedium(ctx context.Context, mediumID string) ([]domain.ExecutionAgent, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("agent repository is not configured")
	}
	var rows []agentModel
	if err := r.db.WithContext(ctx).Where("medium_id = ?", strings.TrimSpace(mediumID)).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.ExecutionAgent, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAgentDomain(&row))
	}
	return out, nil
}

func toAgentModel(agent *domain.ExecutionAgent) (*agentModel, error) {
	caps, err := marshalStringSlice(agent.Capabilities)
	if err != nil {
		return nil, err
	}
	return &agentModel{
		AgentID: agent.AgentID, MediumID: agent.MediumID, Status: string(agent.Status),
		PublicKey: agent.PublicKey, TokenHash: agent.TokenHash, Version: agent.Version,
		Capabilities: caps, RunningTasks: agent.RunningTasks, FreeSlots: agent.FreeSlots,
		LastHeartbeat: agent.LastHeartbeat, Disabled: agent.Disabled,
	}, nil
}

func toAgentDomain(m *agentModel) domain.ExecutionAgent {
	return domain.ExecutionAgent{
		AgentID: m.AgentID, MediumID: m.MediumID, Status: domain.AgentStatus(m.Status),
		PublicKey: m.PublicKey, TokenHash: m.TokenHash, Version: m.Version,
		Capabilities: unmarshalStringSlice(m.Capabilities), RunningTasks: m.RunningTasks,
		FreeSlots: m.FreeSlots, LastHeartbeat: m.LastHeartbeat, Disabled: m.Disabled,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func fillAgentFromModel(agent *domain.ExecutionAgent, m *agentModel) {
	agent.CreatedAt = m.CreatedAt
	agent.UpdatedAt = m.UpdatedAt
}
