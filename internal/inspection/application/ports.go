package application

import (
	"context"
	"time"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
)

// ObservabilityQueryPort 巡检证据采集所需的 Observability 查询端口。
type ObservabilityQueryPort interface {
	QueryMetrics(ctx context.Context, actor obsapp.Actor, q obsdomain.MetricQuery) (*obsapp.MetricQueryResult, error)
	SearchLogs(ctx context.Context, actor obsapp.Actor, q obsdomain.LogQuery) (*obsapp.LogSearchResult, error)
	QueryTraces(ctx context.Context, actor obsapp.Actor, q obsdomain.TraceQuery) (*obsapp.TraceQueryResult, error)
	QueryTopology(ctx context.Context, actor obsapp.Actor, q obsdomain.TopologyQuery) (*obsapp.TopologyQueryResult, error)
}

// CheckEvidenceInput 单次检查项证据采集输入。
type CheckEvidenceInput struct {
	Check     string
	AccountID string
	Provider  string
	Region    string
	Service   string
	From      int64
	To        int64
}

// EvidenceSummary 证据摘要（脱敏后供分析与审计）。
type EvidenceSummary struct {
	Check      string
	Type       string
	EvidenceID string
	Metric     string
	MaxValue   float64
	SpanCount  int
	ErrorSpans int
	EntryCount int
}

// AffectedResourceDTO 受影响资源摘要。
type AffectedResourceDTO struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RecommendationDraft 分析阶段建议草稿。
type RecommendationDraft struct {
	Title              string
	Reason             string
	SuggestedAction    string
	RiskLevel          string
	CanCreateExecution bool
	Confidence         float64
	Uncertainty        string
}

// AnalysisResult 规则/Agent 分析输出。
type AnalysisResult struct {
	Category          string
	RiskLevel         string
	Summary           string
	Detail            string
	AffectedResources []AffectedResourceDTO
	EvidenceRefs      []string
	Confidence        float64
	Uncertainty       string
	Recommendations   []RecommendationDraft
}

// Analyzer 证据采集与分析端口。
type Analyzer interface {
	CollectEvidence(ctx context.Context, actor Actor, input CheckEvidenceInput) (*EvidenceSummary, error)
	Analyze(ctx context.Context, checks []string, evidence []EvidenceSummary) ([]AnalysisResult, error)
}

// CreateAgentTaskRequest 从建议创建 agent 模式执行任务。
type CreateAgentTaskRequest struct {
	Name          string
	SourceID      string
	MediumID      string
	CommandSpecID string
	Arguments     map[string]any
	RiskLevel     string
	Environment   string
	TargetType    string
	TargetID      string
	TargetName    string
}

// CreateAgentTaskResult 创建结果摘要。
type CreateAgentTaskResult struct {
	TaskID    string
	Status    string
	RiskLevel string
}

// ExecutionCreatorPort 创建 Execution Task，不直接执行。
type ExecutionCreatorPort interface {
	CreateAgentTask(ctx context.Context, actor Actor, req CreateAgentTaskRequest) (*CreateAgentTaskResult, error)
}

func defaultTimeWindow() (int64, int64) {
	to := time.Now().Unix()
	return to - 3600, to
}
