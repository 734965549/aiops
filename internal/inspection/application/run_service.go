package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/734965549/aiops/internal/inspection/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

type RunService struct {
	policies  domain.PolicyRepository
	runs      domain.RunRepository
	findings  domain.FindingRepository
	recs      domain.RecommendationRepository
	artifacts domain.ArtifactUnitOfWork
	analyzer  Analyzer
	audit     AuditRecorder
}

func NewRunService(
	policies domain.PolicyRepository,
	runs domain.RunRepository,
	findings domain.FindingRepository,
	recs domain.RecommendationRepository,
	analyzer Analyzer,
	audit AuditRecorder,
) *RunService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	if analyzer == nil {
		analyzer = &noopAnalyzer{}
	}
	return &RunService{
		policies: policies, runs: runs, findings: findings, recs: recs,
		analyzer: analyzer, audit: audit,
	}
}

func (s *RunService) SetArtifactUnitOfWork(uow domain.ArtifactUnitOfWork) {
	if s != nil {
		s.artifacts = uow
	}
}

type ListRunsQuery struct {
	Page     int
	PageSize int
	PolicyID string
	Status   string
}

type ListFindingsQuery struct {
	Page      int
	PageSize  int
	RunID     string
	PolicyID  string
	RiskLevel string
}

func (s *RunService) TriggerRun(ctx context.Context, actor Actor, policyID string, trigger domain.TriggerType) (*RunDTO, error) {
	if s == nil || s.runs == nil || s.policies == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "inspection run service is not enabled")
	}
	policy, err := s.policies.GetByID(ctx, policyID)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	if !policy.Enabled {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "policy is disabled")
	}
	runID := "run-" + uuid.NewString()
	run := &domain.InspectionRun{
		RunID:       runID,
		PolicyID:    policyID,
		Status:      domain.RunStatusPending,
		TriggerType: trigger,
		Summary:     "pending",
	}
	run.AppendTimeline("created", fmt.Sprintf("trigger=%s", trigger))
	if err := s.runs.Create(ctx, run); err != nil {
		return nil, mapDomainErr(err)
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "inspection_run", ResourceID: runID, Action: AuditRunCreate, UserID: actor.UserID,
		Payload: map[string]any{"policy_id": policyID, "trigger_type": trigger},
	})
	if err := s.executeRun(ctx, actor, policy, run); err != nil {
		return nil, executionErr(err)
	}
	dto := toRunDTO(run)
	return &dto, nil
}

func (s *RunService) GetRun(ctx context.Context, runID string) (*RunDTO, error) {
	if s == nil || s.runs == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "inspection run service is not enabled")
	}
	run, err := s.runs.GetByID(ctx, runID)
	if err != nil {
		return nil, mapDomainErr(err)
	}
	dto := toRunDTO(run)
	return &dto, nil
}

func (s *RunService) ListRuns(ctx context.Context, q ListRunsQuery) ([]RunDTO, int64, error) {
	if s == nil || s.runs == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "inspection run service is not enabled")
	}
	filter := domain.RunFilter{
		PolicyID: q.PolicyID, Status: q.Status,
		Limit: q.PageSize, Offset: (q.Page - 1) * q.PageSize,
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	items, err := s.runs.List(ctx, filter)
	if err != nil {
		return nil, 0, mapDomainErr(err)
	}
	total, err := s.runs.Count(ctx, filter)
	if err != nil {
		return nil, 0, mapDomainErr(err)
	}
	out := make([]RunDTO, 0, len(items))
	for i := range items {
		out = append(out, toRunDTO(&items[i]))
	}
	return out, total, nil
}

func (s *RunService) ListFindings(ctx context.Context, q ListFindingsQuery) ([]FindingDTO, int64, error) {
	if s == nil || s.findings == nil || s.recs == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "inspection finding service is not enabled")
	}
	filter := domain.FindingFilter{
		RunID: q.RunID, PolicyID: q.PolicyID, RiskLevel: q.RiskLevel,
		Limit: q.PageSize, Offset: (q.Page - 1) * q.PageSize,
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	items, err := s.findings.List(ctx, filter)
	if err != nil {
		return nil, 0, mapDomainErr(err)
	}
	total, err := s.findings.Count(ctx, filter)
	if err != nil {
		return nil, 0, mapDomainErr(err)
	}
	out := make([]FindingDTO, 0, len(items))
	for i := range items {
		recs, err := s.recs.ListByFindingID(ctx, items[i].FindingID)
		if err != nil {
			return nil, 0, mapDomainErr(err)
		}
		out = append(out, toFindingDTO(&items[i], recs))
	}
	return out, total, nil
}

func (s *RunService) executeRun(ctx context.Context, actor Actor, policy *domain.InspectionPolicy, run *domain.InspectionRun) error {
	if err := run.TransitionTo(domain.RunStatusRunning); err != nil {
		return mapDomainErr(err)
	}
	run.AppendTimeline("started", "collecting observability evidence")
	if err := s.runs.Update(ctx, run); err != nil {
		return mapDomainErr(err)
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "inspection_run", ResourceID: run.RunID, Action: AuditRunStart, UserID: actor.UserID,
	})

	from, to := defaultTimeWindow()
	service := "payment-service"
	if len(policy.Scope.ApplicationIDs) > 0 {
		service = policy.Scope.ApplicationIDs[0]
	}
	region := "cn-north-4"

	var evidence []EvidenceSummary
	var collectErrors int
	for _, check := range policy.Checks {
		ev, err := s.analyzer.CollectEvidence(ctx, actor, CheckEvidenceInput{
			Check: check, AccountID: policy.Scope.AccountID, Provider: policy.Scope.Provider,
			Region: region, Service: service, From: from, To: to,
		})
		if err != nil {
			collectErrors++
			run.AppendTimeline("evidence_failed", fmt.Sprintf("check=%s reason=%s", check, failureReason(err)))
			continue
		}
		if ev == nil {
			collectErrors++
			run.AppendTimeline("evidence_failed", fmt.Sprintf("check=%s reason=empty_evidence", check))
			continue
		}
		evidence = append(evidence, *ev)
		run.AppendTimeline("evidence_collected", fmt.Sprintf("check=%s evidence_id=%s", check, ev.EvidenceID))
	}

	run.AppendTimeline("analyzing", fmt.Sprintf("evidence_count=%d", len(evidence)))
	results, err := s.analyzer.Analyze(ctx, policy.Checks, evidence)
	if err != nil {
		if finishErr := s.finishRun(ctx, actor, run, domain.RunStatusFailed, "analysis failed"); finishErr != nil {
			return apperr.Wrap(finishErr, apperr.CodeInternal, "analysis failed and finish run failed")
		}
		return err
	}

	var findings []domain.InspectionFinding
	var recs []domain.Recommendation
	for _, ar := range results {
		findingID := "fnd-" + uuid.NewString()
		var affected []domain.AffectedResource
		for _, r := range ar.AffectedResources {
			affected = append(affected, domain.AffectedResource{Type: r.Type, ID: r.ID, Name: r.Name})
		}
		findings = append(findings, domain.InspectionFinding{
			FindingID: findingID, RunID: run.RunID, PolicyID: policy.PolicyID,
			RiskLevel: domain.RiskLevel(ar.RiskLevel), Category: ar.Category,
			Summary: ar.Summary, Detail: ar.Detail, AffectedResources: affected,
			EvidenceRefs: ar.EvidenceRefs, Confidence: ar.Confidence, Uncertainty: ar.Uncertainty,
		})
		for _, draft := range ar.Recommendations {
			recs = append(recs, domain.Recommendation{
				RecommendationID: "rec-" + uuid.NewString(), FindingID: findingID, RunID: run.RunID,
				Title: draft.Title, Reason: draft.Reason, SuggestedAction: draft.SuggestedAction,
				RiskLevel: domain.RiskLevel(draft.RiskLevel), Status: domain.RecommendationOpen,
				CanCreateExecution: draft.CanCreateExecution, Confidence: draft.Confidence, Uncertainty: draft.Uncertainty,
			})
		}
	}

	status := domain.RunStatusSuccess
	summary := fmt.Sprintf("completed with %d findings", len(findings))
	if collectErrors > 0 {
		status = domain.RunStatusPartial
		summary = fmt.Sprintf("partial: %d findings, %d evidence errors", len(findings), collectErrors)
	}
	if len(evidence) == 0 && collectErrors > 0 {
		status = domain.RunStatusFailed
		summary = "failed: no evidence collected"
	}
	if err := s.persistArtifactsAndFinish(ctx, run, status, summary, findings, recs); err != nil {
		if finishErr := s.finishRun(ctx, actor, run, domain.RunStatusFailed, "persist inspection results failed"); finishErr != nil {
			return apperr.Wrap(finishErr, apperr.CodeInternal, "persist inspection results failed and finish run failed")
		}
		return mapDomainErr(err)
	}
	for _, rec := range recs {
		_ = s.audit.Record(ctx, AuditRecord{
			ResourceType: "inspection_recommendation", ResourceID: rec.RecommendationID,
			Action: AuditRecCreate, UserID: actor.UserID,
			Payload: map[string]any{"finding_id": rec.FindingID, "run_id": run.RunID, "risk_level": rec.RiskLevel},
		})
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "inspection_run", ResourceID: run.RunID, Action: AuditRunFinish, UserID: actor.UserID,
		Payload: map[string]any{"status": status, "summary": summary},
	})
	return nil
}

func (s *RunService) finishRun(ctx context.Context, actor Actor, run *domain.InspectionRun, status domain.RunStatus, summary string) error {
	finalRun, err := finalizedRun(run, status, summary)
	if err != nil {
		return err
	}
	if err := s.runs.Update(ctx, finalRun); err != nil {
		return mapDomainErr(err)
	}
	*run = *finalRun
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "inspection_run", ResourceID: run.RunID, Action: AuditRunFinish, UserID: actor.UserID,
		Payload: map[string]any{"status": status, "summary": summary},
	})
	return nil
}

func (s *RunService) persistArtifactsAndFinish(
	ctx context.Context,
	run *domain.InspectionRun,
	status domain.RunStatus,
	summary string,
	findings []domain.InspectionFinding,
	recs []domain.Recommendation,
) error {
	finalRun, err := finalizedRun(run, status, summary)
	if err != nil {
		return err
	}
	write := func(ctx context.Context, repos domain.ArtifactRepositories) error {
		if len(findings) > 0 {
			if err := repos.Findings.CreateBatch(ctx, findings); err != nil {
				return err
			}
		}
		if len(recs) > 0 {
			if err := repos.Recommendations.CreateBatch(ctx, recs); err != nil {
				return err
			}
		}
		return repos.Runs.Update(ctx, finalRun)
	}
	if s.artifacts != nil {
		if err := s.artifacts.WithinArtifactsTransaction(ctx, write); err != nil {
			return err
		}
	} else if err := write(ctx, domain.ArtifactRepositories{
		Runs: s.runs, Findings: s.findings, Recommendations: s.recs,
	}); err != nil {
		return err
	}
	*run = *finalRun
	return nil
}

func finalizedRun(run *domain.InspectionRun, status domain.RunStatus, summary string) (*domain.InspectionRun, error) {
	finalRun := *run
	finalRun.Timeline = append([]domain.TimelineEvent(nil), run.Timeline...)
	if err := finalRun.TransitionTo(status); err != nil {
		return nil, mapDomainErr(err)
	}
	finalRun.Summary = summary
	finalRun.AppendTimeline("finished", summary)
	return &finalRun, nil
}

func failureReason(err error) string {
	if err == nil {
		return "unknown"
	}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return string(appErr.Code)
	}
	if errors.Is(err, domain.ErrUnsupportedCheck) {
		return "unsupported_check"
	}
	return "internal_error"
}

func executionErr(err error) error {
	if err == nil {
		return nil
	}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return apperr.Wrap(err, apperr.CodeInternal, "inspection run execution failed")
}

type noopAnalyzer struct{}

func (noopAnalyzer) CollectEvidence(context.Context, Actor, CheckEvidenceInput) (*EvidenceSummary, error) {
	return nil, apperr.New(apperr.CodeUnavailable, "inspection analyzer is not configured")
}
func (noopAnalyzer) Analyze(context.Context, []string, []EvidenceSummary) ([]AnalysisResult, error) {
	return nil, nil
}
