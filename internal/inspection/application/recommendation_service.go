package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/inspection/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type RecommendationService struct {
	recs      domain.RecommendationRepository
	execTasks ExecutionCreatorPort
	audit     AuditRecorder
}

func NewRecommendationService(recs domain.RecommendationRepository, execTasks ExecutionCreatorPort, audit AuditRecorder) *RecommendationService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &RecommendationService{recs: recs, execTasks: execTasks, audit: audit}
}

type CreateExecutionFromRecommendationInput struct {
	ExecutionMode string
	MediumID      string
	CommandSpecID string
	Arguments     map[string]any
	ConfirmIntent string
	TargetType    string
	TargetID      string
	TargetName    string
	Environment   string
}

type CreateExecutionFromRecommendationResult struct {
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	RiskLevel string `json:"risk_level"`
}

func (s *RecommendationService) CreateExecution(
	ctx context.Context,
	actor Actor,
	recommendationID string,
	in CreateExecutionFromRecommendationInput,
) (*CreateExecutionFromRecommendationResult, error) {
	if s == nil || s.recs == nil || s.execTasks == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "recommendation execution service is not enabled")
	}
	rec, err := s.recs.GetByID(ctx, recommendationID)
	if err != nil {
		return nil, mapInspectionDomainErr(err)
	}
	if !rec.CanCreateExecution {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "recommendation cannot create execution task")
	}
	if rec.Status == domain.RecommendationExecutionCreated {
		return nil, apperr.New(apperr.CodeAlreadyExists, "execution task already created for recommendation")
	}
	mode := strings.ToLower(strings.TrimSpace(in.ExecutionMode))
	if mode == "" {
		mode = "agent"
	}
	if mode != "agent" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "only execution_mode=agent is supported in this phase")
	}
	if strings.TrimSpace(in.ConfirmIntent) != "" && strings.TrimSpace(in.ConfirmIntent) != "create_task_only" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "confirm_intent must be create_task_only")
	}
	commandSpecID := strings.TrimSpace(in.CommandSpecID)
	if commandSpecID == "" {
		commandSpecID = defaultCommandSpecForRecommendation(rec)
	}
	arguments := cloneRecMap(in.Arguments)
	if commandSpecID == "cmd_linux_disk_usage" && len(arguments) == 0 {
		arguments = map[string]any{"mount_point": "/"}
	}
	result, err := s.execTasks.CreateAgentTask(ctx, actor, CreateAgentTaskRequest{
		Name: rec.Title, SourceID: rec.RecommendationID, MediumID: strings.TrimSpace(in.MediumID),
		CommandSpecID: commandSpecID, Arguments: arguments, RiskLevel: string(rec.RiskLevel),
		Environment: strings.TrimSpace(in.Environment), TargetType: strings.TrimSpace(in.TargetType),
		TargetID: strings.TrimSpace(in.TargetID), TargetName: strings.TrimSpace(in.TargetName),
	})
	if err != nil {
		return nil, err
	}
	rec.Status = domain.RecommendationExecutionCreated
	if err := s.recs.Update(ctx, rec); err != nil {
		return nil, mapInspectionDomainErr(err)
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "inspection_recommendation", ResourceID: rec.RecommendationID,
		Action: "create_execution", UserID: actor.UserID,
		Payload: map[string]any{
			"task_id": result.TaskID, "medium_id": in.MediumID, "command_spec_id": commandSpecID,
			"execution_mode": mode, "risk_level": string(rec.RiskLevel),
		},
	})
	return &CreateExecutionFromRecommendationResult{
		TaskID: result.TaskID, Status: result.Status, RiskLevel: result.RiskLevel,
	}, nil
}

func defaultCommandSpecForRecommendation(rec *domain.Recommendation) string {
	title := strings.ToLower(rec.Title)
	if strings.Contains(title, "磁盘") || strings.Contains(title, "disk") {
		return "cmd_linux_disk_usage"
	}
	if strings.Contains(title, "内存") || strings.Contains(title, "memory") {
		return "cmd_linux_memory_snapshot"
	}
	if strings.Contains(title, "cpu") {
		return "cmd_linux_cpu_snapshot"
	}
	return "cmd_linux_disk_usage"
}

func cloneRecMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func mapInspectionDomainErr(err error) error {
	if err == nil {
		return nil
	}
	return apperr.MapSentinels(err, "inspection recommendation",
		apperr.Sentinel{Err: domain.ErrNotFound, Code: apperr.CodeNotFound},
		apperr.Sentinel{Err: domain.ErrInvalidArgument, Code: apperr.CodeInvalidArgument},
	)
}
