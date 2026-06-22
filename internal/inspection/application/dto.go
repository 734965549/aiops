package application

import (
	"time"

	"github.com/734965549/aiops/internal/inspection/domain"
)

type PolicyScopeDTO struct {
	Environment    string   `json:"environment"`
	AccountID      string   `json:"account_id"`
	Provider       string   `json:"provider"`
	ApplicationIDs []string `json:"application_ids"`
	ResourceTypes  []string `json:"resource_types"`
}

type PolicyDTO struct {
	PolicyID             string         `json:"policy_id"`
	Name                 string         `json:"name"`
	Enabled              bool           `json:"enabled"`
	Schedule             string         `json:"schedule"`
	Scope                PolicyScopeDTO `json:"scope"`
	Checks               []string       `json:"checks"`
	AgentProfile         string         `json:"agent_profile"`
	NotificationPolicyID string         `json:"notification_policy_id,omitempty"`
	CreatedAt            int64          `json:"created_at"`
	UpdatedAt            int64          `json:"updated_at"`
}

type TimelineEventDTO struct {
	TS     int64  `json:"ts"`
	Event  string `json:"event"`
	Detail string `json:"detail,omitempty"`
}

type RunDTO struct {
	RunID       string             `json:"run_id"`
	PolicyID    string             `json:"policy_id"`
	Status      string             `json:"status"`
	TriggerType string             `json:"trigger_type"`
	Summary     string             `json:"summary"`
	Timeline    []TimelineEventDTO `json:"timeline"`
	StartedAt   *int64             `json:"started_at,omitempty"`
	FinishedAt  *int64             `json:"finished_at,omitempty"`
	CreatedAt   int64              `json:"created_at"`
	UpdatedAt   int64              `json:"updated_at"`
}

type FindingDTO struct {
	FindingID         string                  `json:"finding_id"`
	RunID             string                  `json:"run_id"`
	PolicyID          string                  `json:"policy_id"`
	RiskLevel         string                  `json:"risk_level"`
	Category          string                  `json:"category"`
	Summary           string                  `json:"summary"`
	Detail            string                  `json:"detail,omitempty"`
	AffectedResources []AffectedResourceDTO   `json:"affected_resources"`
	EvidenceRefs      []string                `json:"evidence_refs"`
	Recommendations   []RecommendationDTO     `json:"recommendations"`
	Confidence        float64                 `json:"confidence"`
	Uncertainty       string                  `json:"uncertainty"`
	CreatedAt         int64                   `json:"created_at"`
}

type RecommendationDTO struct {
	RecommendationID   string  `json:"recommendation_id"`
	FindingID          string  `json:"finding_id"`
	RunID              string  `json:"run_id"`
	Title              string  `json:"title"`
	Reason             string  `json:"reason"`
	SuggestedAction    string  `json:"suggested_action"`
	RiskLevel          string  `json:"risk_level"`
	Status             string  `json:"status"`
	CanCreateExecution bool    `json:"can_create_execution"`
	Confidence         float64 `json:"confidence"`
	Uncertainty        string  `json:"uncertainty"`
	CreatedAt          int64   `json:"created_at"`
}

func toPolicyDTO(p *domain.InspectionPolicy) PolicyDTO {
	if p == nil {
		return PolicyDTO{}
	}
	return PolicyDTO{
		PolicyID: p.PolicyID, Name: p.Name, Enabled: p.Enabled, Schedule: p.Schedule,
		Scope: PolicyScopeDTO{
			Environment: p.Scope.Environment, AccountID: p.Scope.AccountID, Provider: p.Scope.Provider,
			ApplicationIDs: p.Scope.ApplicationIDs, ResourceTypes: p.Scope.ResourceTypes,
		},
		Checks: p.Checks, AgentProfile: p.AgentProfile, NotificationPolicyID: p.NotificationPolicyID,
		CreatedAt: p.CreatedAt.Unix(), UpdatedAt: p.UpdatedAt.Unix(),
	}
}

func toRunDTO(r *domain.InspectionRun) RunDTO {
	if r == nil {
		return RunDTO{}
	}
	dto := RunDTO{
		RunID: r.RunID, PolicyID: r.PolicyID, Status: string(r.Status),
		TriggerType: string(r.TriggerType), Summary: r.Summary,
		CreatedAt: r.CreatedAt.Unix(), UpdatedAt: r.UpdatedAt.Unix(),
	}
	for _, ev := range r.Timeline {
		dto.Timeline = append(dto.Timeline, TimelineEventDTO{TS: ev.TS, Event: ev.Event, Detail: ev.Detail})
	}
	if r.StartedAt != nil {
		ts := r.StartedAt.Unix()
		dto.StartedAt = &ts
	}
	if r.FinishedAt != nil {
		ts := r.FinishedAt.Unix()
		dto.FinishedAt = &ts
	}
	return dto
}

func tsPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := t.Unix()
	return &v
}

func toFindingDTO(f *domain.InspectionFinding, recs []domain.Recommendation) FindingDTO {
	if f == nil {
		return FindingDTO{}
	}
	dto := FindingDTO{
		FindingID: f.FindingID, RunID: f.RunID, PolicyID: f.PolicyID,
		RiskLevel: string(f.RiskLevel), Category: f.Category, Summary: f.Summary, Detail: f.Detail,
		EvidenceRefs: f.EvidenceRefs, Confidence: f.Confidence, Uncertainty: f.Uncertainty,
		CreatedAt: f.CreatedAt.Unix(),
	}
	for _, ar := range f.AffectedResources {
		dto.AffectedResources = append(dto.AffectedResources, AffectedResourceDTO{Type: ar.Type, ID: ar.ID, Name: ar.Name})
	}
	for _, rec := range recs {
		dto.Recommendations = append(dto.Recommendations, toRecommendationDTO(&rec))
	}
	return dto
}

func toRecommendationDTO(r *domain.Recommendation) RecommendationDTO {
	if r == nil {
		return RecommendationDTO{}
	}
	return RecommendationDTO{
		RecommendationID: r.RecommendationID, FindingID: r.FindingID, RunID: r.RunID,
		Title: r.Title, Reason: r.Reason, SuggestedAction: r.SuggestedAction,
		RiskLevel: string(r.RiskLevel), Status: string(r.Status),
		CanCreateExecution: r.CanCreateExecution, Confidence: r.Confidence, Uncertainty: r.Uncertainty,
		CreatedAt: r.CreatedAt.Unix(),
	}
}
