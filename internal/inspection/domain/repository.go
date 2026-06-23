package domain

import "context"

type PolicyFilter struct {
	Enabled *bool
	Keyword string
	Limit   int
	Offset  int
}

type RunFilter struct {
	PolicyID string
	Status   string
	Limit    int
	Offset   int
}

type FindingFilter struct {
	RunID     string
	PolicyID  string
	RiskLevel string
	Limit     int
	Offset    int
}

type PolicyRepository interface {
	Create(ctx context.Context, policy *InspectionPolicy) error
	Update(ctx context.Context, policy *InspectionPolicy) error
	GetByID(ctx context.Context, policyID string) (*InspectionPolicy, error)
	List(ctx context.Context, filter PolicyFilter) ([]InspectionPolicy, error)
	Count(ctx context.Context, filter PolicyFilter) (int64, error)
	SoftDelete(ctx context.Context, policyID string) error
}

type RunRepository interface {
	Create(ctx context.Context, run *InspectionRun) error
	Update(ctx context.Context, run *InspectionRun) error
	GetByID(ctx context.Context, runID string) (*InspectionRun, error)
	List(ctx context.Context, filter RunFilter) ([]InspectionRun, error)
	Count(ctx context.Context, filter RunFilter) (int64, error)
}

type FindingRepository interface {
	CreateBatch(ctx context.Context, findings []InspectionFinding) error
	List(ctx context.Context, filter FindingFilter) ([]InspectionFinding, error)
	Count(ctx context.Context, filter FindingFilter) (int64, error)
	GetByID(ctx context.Context, findingID string) (*InspectionFinding, error)
}

type RecommendationRepository interface {
	CreateBatch(ctx context.Context, recs []Recommendation) error
	ListByRunID(ctx context.Context, runID string) ([]Recommendation, error)
	ListByFindingID(ctx context.Context, findingID string) ([]Recommendation, error)
	GetByID(ctx context.Context, recommendationID string) (*Recommendation, error)
	Update(ctx context.Context, rec *Recommendation) error
}

type ArtifactRepositories struct {
	Runs            RunRepository
	Findings        FindingRepository
	Recommendations RecommendationRepository
}

type ArtifactUnitOfWork interface {
	WithinArtifactsTransaction(ctx context.Context, fn func(context.Context, ArtifactRepositories) error) error
}
