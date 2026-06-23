package domain

import "time"

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// AffectedResource 受影响资源摘要。
type AffectedResource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// InspectionFinding 巡检发现项。
type InspectionFinding struct {
	FindingID         string
	RunID             string
	PolicyID          string
	RiskLevel         RiskLevel
	Category          string
	Summary           string
	Detail            string
	AffectedResources []AffectedResource
	EvidenceRefs      []string
	Confidence        float64
	Uncertainty       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
