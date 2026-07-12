package domain

import "time"

type RecommendationStatus string

const (
	RecommendationOpen             RecommendationStatus = "open"
	RecommendationAccepted         RecommendationStatus = "accepted"
	RecommendationDismissed        RecommendationStatus = "dismissed"
	RecommendationExecutionCreated RecommendationStatus = "execution_created"
)

// Recommendation 可追踪建议。
type Recommendation struct {
	RecommendationID   string
	FindingID          string
	RunID              string
	Title              string
	Reason             string
	SuggestedAction    string
	RiskLevel          RiskLevel
	Status             RecommendationStatus
	CanCreateExecution bool
	Confidence         float64
	Uncertainty        string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
