package application

import (
	"context"
	"testing"

	"github.com/734965549/aiops/internal/execution/domain"
)

type fakeAgentRepo struct {
	byID    map[string]*domain.ExecutionAgent
	byToken map[string]string
}

func newFakeAgentRepo() *fakeAgentRepo {
	return &fakeAgentRepo{byID: map[string]*domain.ExecutionAgent{}, byToken: map[string]string{}}
}

func (r *fakeAgentRepo) Create(_ context.Context, agent *domain.ExecutionAgent) error {
	cp := *agent
	r.byID[agent.AgentID] = &cp
	r.byToken[agent.TokenHash] = agent.AgentID
	return nil
}

func (r *fakeAgentRepo) Update(_ context.Context, agent *domain.ExecutionAgent) error {
	if _, ok := r.byID[agent.AgentID]; !ok {
		return domain.ErrNotFound
	}
	cp := *agent
	r.byID[agent.AgentID] = &cp
	r.byToken[agent.TokenHash] = agent.AgentID
	return nil
}

func (r *fakeAgentRepo) GetByID(_ context.Context, agentID string) (*domain.ExecutionAgent, error) {
	agent, ok := r.byID[agentID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *agent
	return &cp, nil
}

func (r *fakeAgentRepo) GetByTokenHash(_ context.Context, tokenHash string) (*domain.ExecutionAgent, error) {
	agentID, ok := r.byToken[tokenHash]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return r.GetByID(context.Background(), agentID)
}

func (r *fakeAgentRepo) ListByMedium(_ context.Context, mediumID string) ([]domain.ExecutionAgent, error) {
	out := make([]domain.ExecutionAgent, 0)
	for _, agent := range r.byID {
		if agent.MediumID == mediumID {
			out = append(out, *agent)
		}
	}
	return out, nil
}

type fakeMediumRepo struct {
	byID map[string]*domain.ExecutionMedium
}

func newFakeMediumRepo() *fakeMediumRepo {
	return &fakeMediumRepo{byID: map[string]*domain.ExecutionMedium{}}
}

func (r *fakeMediumRepo) Create(_ context.Context, medium *domain.ExecutionMedium) error {
	cp := *medium
	r.byID[medium.MediumID] = &cp
	return nil
}

func (r *fakeMediumRepo) Update(_ context.Context, medium *domain.ExecutionMedium) error {
	if _, ok := r.byID[medium.MediumID]; !ok {
		return domain.ErrNotFound
	}
	cp := *medium
	r.byID[medium.MediumID] = &cp
	return nil
}

func (r *fakeMediumRepo) GetByID(_ context.Context, mediumID string) (*domain.ExecutionMedium, error) {
	medium, ok := r.byID[mediumID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *medium
	return &cp, nil
}

func (r *fakeMediumRepo) List(_ context.Context, _ domain.MediumFilter) ([]domain.ExecutionMedium, error) {
	return nil, nil
}

func (r *fakeMediumRepo) Count(_ context.Context, _ domain.MediumFilter) (int64, error) {
	return 0, nil
}

func TestAgentAuthenticationAllowsRegisteredAgentHeartbeat(t *testing.T) {
	ctx := context.Background()
	agents := newFakeAgentRepo()
	media := newFakeMediumRepo()
	media.byID["med-1"] = &domain.ExecutionMedium{MediumID: "med-1", Enabled: true}

	svc := NewAgentService(agents, media, nil)
	svc.register = func() (string, error) { return "agt_test_token", nil }

	registered, err := svc.Register(ctx, RegisterAgentInput{AgentID: "agent-1", MediumID: "med-1"})
	if err != nil {
		t.Fatalf("register agent: %v", err)
	}
	if registered.Status != string(domain.AgentRegistered) {
		t.Fatalf("expected registered status, got %s", registered.Status)
	}

	agent, err := svc.AuthenticateByToken(ctx, registered.AgentToken)
	if err != nil {
		t.Fatalf("registered agent should authenticate for first heartbeat: %v", err)
	}
	if agent.Status != domain.AgentRegistered {
		t.Fatalf("expected registered agent, got %s", agent.Status)
	}

	heartbeat, err := svc.Heartbeat(ctx, agent.AgentID, HeartbeatInput{Status: string(domain.AgentOnline), FreeSlots: 2})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if heartbeat.Status != string(domain.AgentOnline) {
		t.Fatalf("expected online after heartbeat, got %s", heartbeat.Status)
	}
}
