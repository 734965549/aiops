package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/execution/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// AgentService 执行代理注册与心跳。
type AgentService struct {
	agents   domain.AgentRepository
	media    domain.MediumRepository
	audit    AuditRecorder
	now      func() time.Time
	register func() (string, error)
}

func NewAgentService(agents domain.AgentRepository, media domain.MediumRepository, audit AuditRecorder) *AgentService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &AgentService{
		agents: agents, media: media, audit: audit, now: time.Now,
		register: generateAgentToken,
	}
}

type RegisterAgentInput struct {
	AgentID      string
	MediumID     string
	PublicKey    string
	Version      string
	Capabilities []string
}

type RegisterAgentResult struct {
	AgentID    string `json:"agent_id"`
	MediumID   string `json:"medium_id"`
	AgentToken string `json:"agent_token"`
	Status     string `json:"status"`
}

type HeartbeatInput struct {
	Status       string
	RunningTasks int
	FreeSlots    int
	Version      string
	ObservedAt   int64
}

type AgentDTO struct {
	AgentID       string   `json:"agent_id"`
	MediumID      string   `json:"medium_id"`
	Status        string   `json:"status"`
	Version       string   `json:"version,omitempty"`
	Capabilities  []string `json:"capabilities"`
	RunningTasks  int      `json:"running_tasks"`
	FreeSlots     int      `json:"free_slots"`
	LastHeartbeat int64    `json:"last_heartbeat,omitempty"`
	Disabled      bool     `json:"disabled"`
}

func (s *AgentService) Register(ctx context.Context, in RegisterAgentInput) (*RegisterAgentResult, error) {
	if s == nil || s.agents == nil || s.media == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "agent service is not enabled")
	}
	mediumID := strings.TrimSpace(in.MediumID)
	medium, err := s.media.GetByID(ctx, mediumID)
	if err != nil {
		return nil, wrapExecError(err, "load execution medium failed")
	}
	if !medium.Enabled {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "execution medium is disabled")
	}
	agentID := strings.TrimSpace(in.AgentID)
	if agentID == "" {
		agentID = "agent-" + uuid.NewString()
	}
	token, err := s.register()
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "generate agent token failed")
	}
	now := s.now()
	agent := &domain.ExecutionAgent{
		AgentID: agentID, MediumID: mediumID, Status: domain.AgentRegistered,
		PublicKey: strings.TrimSpace(in.PublicKey), TokenHash: hashAgentToken(token),
		Version: strings.TrimSpace(in.Version), Capabilities: cloneStringSlice(in.Capabilities),
		FreeSlots: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.agents.Create(ctx, agent); err != nil {
		return nil, wrapExecError(err, "register execution agent failed")
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "execution_agent", ResourceID: agentID, Action: AuditAgentRegister, UserID: "",
		Payload: map[string]any{"agent_id": agentID, "medium_id": mediumID},
	})
	return &RegisterAgentResult{
		AgentID: agentID, MediumID: mediumID, AgentToken: token, Status: string(domain.AgentRegistered),
	}, nil
}

func (s *AgentService) Heartbeat(ctx context.Context, agentID string, in HeartbeatInput) (*AgentDTO, error) {
	if s == nil || s.agents == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "agent service is not enabled")
	}
	agent, err := s.agents.GetByID(ctx, agentID)
	if err != nil {
		return nil, wrapExecError(err, "load execution agent failed")
	}
	if agent.Disabled {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "execution agent is disabled")
	}
	status := domain.AgentStatus(strings.ToLower(strings.TrimSpace(in.Status)))
	if !status.IsValid() {
		status = domain.AgentOnline
	}
	now := s.now()
	agent.Status = status
	agent.RunningTasks = in.RunningTasks
	if in.FreeSlots > 0 {
		agent.FreeSlots = in.FreeSlots
	}
	if strings.TrimSpace(in.Version) != "" {
		agent.Version = strings.TrimSpace(in.Version)
	}
	agent.LastHeartbeat = &now
	agent.UpdatedAt = now
	if err := s.agents.Update(ctx, agent); err != nil {
		return nil, wrapExecError(err, "update agent heartbeat failed")
	}
	if medium, mErr := s.media.GetByID(ctx, agent.MediumID); mErr == nil && medium != nil {
		medium.HealthStatus = domain.MediumHealthOnline
		medium.UpdatedAt = now
		_ = s.media.Update(ctx, medium)
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "execution_agent", ResourceID: agent.AgentID, Action: AuditAgentHeartbeat, UserID: "",
		Payload: map[string]any{"status": string(status), "running_tasks": in.RunningTasks},
	})
	return toAgentDTO(*agent), nil
}

func (s *AgentService) AuthenticateByToken(ctx context.Context, token string) (*domain.ExecutionAgent, error) {
	if s == nil || s.agents == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "agent service is not enabled")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "missing agent token")
	}
	agent, err := s.agents.GetByTokenHash(ctx, hashAgentToken(token))
	if err != nil {
		return nil, apperr.New(apperr.CodeUnauthenticated, "invalid agent token")
	}
	if agent.Disabled {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "execution agent is not available")
	}
	return agent, nil
}

func toAgentDTO(agent domain.ExecutionAgent) *AgentDTO {
	dto := &AgentDTO{
		AgentID: agent.AgentID, MediumID: agent.MediumID, Status: string(agent.Status),
		Version: agent.Version, Capabilities: agent.Capabilities,
		RunningTasks: agent.RunningTasks, FreeSlots: agent.FreeSlots, Disabled: agent.Disabled,
	}
	if agent.LastHeartbeat != nil {
		dto.LastHeartbeat = agent.LastHeartbeat.Unix()
	}
	return dto
}

func generateAgentToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "agt_" + hex.EncodeToString(buf), nil
}
