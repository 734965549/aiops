package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/execution/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// DispatchService 代理任务领取、日志与结果回传。
type DispatchService struct {
	tasks    domain.AgentTaskRepository
	steps    domain.StepRepository
	leases   domain.LeaseRepository
	logs     domain.LogStreamRepository
	specs    domain.CommandSpecRepository
	media    domain.MediumRepository
	audit    AuditRecorder
	leaseTTL time.Duration
	now      func() time.Time
}

func NewDispatchService(
	tasks domain.AgentTaskRepository,
	steps domain.StepRepository,
	leases domain.LeaseRepository,
	logs domain.LogStreamRepository,
	specs domain.CommandSpecRepository,
	media domain.MediumRepository,
	audit AuditRecorder,
	leaseTTLSeconds int,
) *DispatchService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	if leaseTTLSeconds <= 0 {
		leaseTTLSeconds = 300
	}
	return &DispatchService{
		tasks: tasks, steps: steps, leases: leases, logs: logs,
		specs: specs, media: media, audit: audit,
		leaseTTL: time.Duration(leaseTTLSeconds) * time.Second, now: time.Now,
	}
}

type LeaseTaskResult struct {
	LeaseID string        `json:"lease_id,omitempty"`
	TaskID  string        `json:"task_id,omitempty"`
	StepID  string        `json:"step_id,omitempty"`
	Command *LeaseCommand `json:"command,omitempty"`
}

type LeaseCommand struct {
	Argv           []string       `json:"argv"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	WorkingDir     string         `json:"working_dir"`
	Env            map[string]any `json:"env"`
}

type RedactionDTO struct {
	Patterns []string `json:"patterns,omitempty"`
}

type AppendLogInput struct {
	LeaseID    string
	StepID     string
	Stream     string
	Sequence   int
	Content    string
	Truncated  bool
	ObservedAt int64
}

type ReportResultInput struct {
	LeaseID       string
	StepID        string
	Status        string
	ExitCode      int
	ResultSummary string
	StartedAt     int64
	FinishedAt    int64
}

func (s *DispatchService) Lease(ctx context.Context, agent *domain.ExecutionAgent) (*LeaseTaskResult, error) {
	if s == nil || s.tasks == nil || agent == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "dispatch service is not enabled")
	}
	if !agent.Status.CanLease() || agent.Disabled {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "execution agent is not available")
	}
	task, err := s.tasks.FindDispatchableTask(ctx, agent.MediumID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return &LeaseTaskResult{}, nil
		}
		return nil, wrapExecError(err, "find dispatchable task failed")
	}
	if !domain.CanExecute(task.Status) || task.DispatchStatus != domain.DispatchPending {
		return &LeaseTaskResult{}, nil
	}
	steps, err := s.steps.ListByTaskID(ctx, task.ID)
	if err != nil {
		return nil, wrapExecError(err, "list execution steps failed")
	}
	if len(steps) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "execution task has no steps")
	}
	step := steps[0]
	spec, err := loadEnabledCommandSpec(ctx, s.specs, step.CommandSpecID)
	if err != nil {
		return nil, wrapExecError(err, "load command spec failed")
	}
	argv, err := BuildCommandArgv(step.CommandTemplate, step.Arguments)
	if err != nil {
		return nil, err
	}

	now := s.now()
	leaseID := "lease-" + uuid.NewString()
	lease := &domain.ExecutionLease{
		LeaseID: leaseID, TaskID: task.ID, StepID: step.ID,
		AgentID: agent.AgentID, MediumID: agent.MediumID,
		Status: domain.LeaseActive, ExpiresAt: now.Add(s.leaseTTL),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.leases.Create(ctx, lease); err != nil {
		return nil, wrapExecError(err, "create execution lease failed")
	}

	to, err := domain.TransitionExecute(task.Status)
	if err != nil {
		return nil, wrapExecError(err, "mark task leased failed")
	}
	task.Status = to
	task.DispatchStatus = domain.DispatchLeased
	task.AgentID = agent.AgentID
	task.LeaseID = leaseID
	task.StartedAt = &now
	task.UpdatedAt = now
	if err := s.tasks.Update(ctx, task); err != nil {
		return nil, wrapExecError(err, "mark task leased failed")
	}

	step.Status = domain.StepRunning
	step.StartedAt = &now
	step.UpdatedAt = now
	if err := s.steps.Update(ctx, &step); err != nil {
		return nil, wrapExecError(err, "mark step running failed")
	}

	agent.RunningTasks++
	if agent.FreeSlots > 0 {
		agent.FreeSlots--
	}
	agent.UpdatedAt = now
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "execution", ResourceID: task.ID, Action: AuditDispatch, UserID: "",
		Payload: map[string]any{
			"task_id": task.ID, "step_id": step.ID, "lease_id": leaseID,
			"agent_id": agent.AgentID, "medium_id": agent.MediumID, "command_spec_id": spec.CommandSpecID,
		},
	})

	return &LeaseTaskResult{
		LeaseID: leaseID, TaskID: task.ID, StepID: step.ID,
		Command: &LeaseCommand{
			Argv: argv, TimeoutSeconds: spec.TimeoutSeconds, WorkingDir: step.WorkingDir, Env: map[string]any{},
		},
	}, nil
}

func (s *DispatchService) AppendLog(ctx context.Context, agent *domain.ExecutionAgent, taskID string, in AppendLogInput) error {
	if s == nil || s.logs == nil || agent == nil {
		return apperr.New(apperr.CodeUnavailable, "dispatch service is not enabled")
	}
	lease, step, err := s.validateLease(ctx, agent, taskID, in.LeaseID, in.StepID)
	if err != nil {
		return err
	}
	spec, err := s.specs.GetByID(ctx, step.CommandSpecID)
	if err != nil {
		return wrapExecError(err, "load command spec failed")
	}
	content, redacted := RedactOutput(in.Content, step.OutputRedaction)
	if spec != nil {
		if c2, r2 := RedactOutput(content, spec.OutputRedaction); r2 {
			content, redacted = c2, true
		}
	}
	observedAt := s.now()
	if in.ObservedAt > 0 {
		observedAt = time.Unix(in.ObservedAt, 0)
	}
	entry := &domain.LogStreamEntry{
		LogID: "log-" + uuid.NewString(), LeaseID: lease.LeaseID, TaskID: taskID,
		StepID: in.StepID, AgentID: agent.AgentID, Stream: strings.ToLower(strings.TrimSpace(in.Stream)),
		Sequence: in.Sequence, Content: content, Truncated: in.Truncated, Redacted: redacted,
		ObservedAt: observedAt, CreatedAt: s.now(), UpdatedAt: s.now(),
	}
	if err := s.logs.Create(ctx, entry); err != nil {
		return wrapExecError(err, "append execution log failed")
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "execution", ResourceID: taskID, Action: AuditAgentLog, UserID: "",
		Payload: map[string]any{"lease_id": lease.LeaseID, "step_id": in.StepID, "stream": entry.Stream, "sequence": in.Sequence},
	})
	return nil
}

func (s *DispatchService) ReportResult(ctx context.Context, agent *domain.ExecutionAgent, taskID string, in ReportResultInput) (*TaskDetailDTO, error) {
	if s == nil || s.tasks == nil || s.steps == nil || agent == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "dispatch service is not enabled")
	}
	task, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		return nil, wrapExecError(err, "load execution task failed")
	}
	if task.Status.IsTerminal() {
		return buildTaskDetail(ctx, s.tasks, s.steps, taskID)
	}
	lease, step, err := s.validateLease(ctx, agent, taskID, in.LeaseID, in.StepID)
	if err != nil {
		return nil, err
	}
	spec, _ := s.specs.GetByID(ctx, step.CommandSpecID)

	now := s.now()
	success := strings.EqualFold(strings.TrimSpace(in.Status), "success")
	if spec != nil && len(spec.AllowedExitCodes) > 0 {
		allowed := false
		for _, code := range spec.AllowedExitCodes {
			if code == in.ExitCode {
				allowed = true
				break
			}
		}
		if !allowed {
			success = false
		}
	} else if in.ExitCode != 0 {
		success = false
	}

	step.FinishedAt = &now
	step.UpdatedAt = now
	if success {
		step.Status = domain.StepSuccess
		step.Output = map[string]any{"exit_code": in.ExitCode, "result_summary": in.ResultSummary}
	} else {
		step.Status = domain.StepFailed
		step.ErrorMessage = strings.TrimSpace(in.ResultSummary)
		if step.ErrorMessage == "" {
			step.ErrorMessage = "agent execution failed"
		}
	}
	if err := s.steps.Update(ctx, &step); err != nil {
		return nil, wrapExecError(err, "update execution step result failed")
	}

	task.FinishedAt = &now
	task.UpdatedAt = now
	task.DispatchStatus = domain.DispatchDispatched
	task.ResultSummary = strings.TrimSpace(in.ResultSummary)
	if success {
		task.Status = domain.StatusSuccess
	} else {
		task.Status = domain.StatusFailed
		task.ErrorMessage = step.ErrorMessage
	}
	if err := s.tasks.Update(ctx, task); err != nil {
		return nil, wrapExecError(err, "update execution task result failed")
	}

	lease.Status = domain.LeaseReleased
	lease.ReleasedAt = &now
	lease.UpdatedAt = now
	_ = s.leases.Update(ctx, lease)

	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "execution", ResourceID: taskID, Action: AuditAgentResult, UserID: "",
		Payload: map[string]any{
			"lease_id": lease.LeaseID, "step_id": step.ID, "exit_code": in.ExitCode,
			"status": string(task.Status), "agent_id": agent.AgentID,
		},
	})
	return buildTaskDetail(ctx, s.tasks, s.steps, taskID)
}

func (s *DispatchService) ListLogs(ctx context.Context, taskID, stepID string) ([]LogEntryDTO, error) {
	if s == nil || s.logs == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "dispatch service is not enabled")
	}
	rows, err := s.logs.ListByTaskStep(ctx, taskID, stepID)
	if err != nil {
		return nil, wrapExecError(err, "list execution logs failed")
	}
	out := make([]LogEntryDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, LogEntryDTO{
			LogID: row.LogID, Stream: row.Stream, Sequence: row.Sequence,
			Content: row.Content, Truncated: row.Truncated, Redacted: row.Redacted,
			ObservedAt: row.ObservedAt.Unix(),
		})
	}
	return out, nil
}

type LogEntryDTO struct {
	LogID      string `json:"log_id"`
	Stream     string `json:"stream"`
	Sequence   int    `json:"sequence"`
	Content    string `json:"content"`
	Truncated  bool   `json:"truncated"`
	Redacted   bool   `json:"redacted"`
	ObservedAt int64  `json:"observed_at"`
}

func (s *DispatchService) validateLease(ctx context.Context, agent *domain.ExecutionAgent, taskID, leaseID, stepID string) (*domain.ExecutionLease, domain.Step, error) {
	if strings.TrimSpace(agent.AgentID) == "" {
		return nil, domain.Step{}, apperr.New(apperr.CodeUnauthenticated, "missing agent identity")
	}
	lease, err := s.leases.GetByID(ctx, leaseID)
	if err != nil {
		return nil, domain.Step{}, wrapExecError(err, "load execution lease failed")
	}
	if lease.AgentID != agent.AgentID || lease.TaskID != taskID {
		return nil, domain.Step{}, apperr.New(apperr.CodePermissionDenied, "lease does not belong to agent")
	}
	if lease.Status != domain.LeaseActive || s.now().After(lease.ExpiresAt) {
		return nil, domain.Step{}, apperr.New(apperr.CodeFailedPrecondition, "execution lease is not active")
	}
	steps, err := s.steps.ListByTaskID(ctx, taskID)
	if err != nil {
		return nil, domain.Step{}, wrapExecError(err, "list execution steps failed")
	}
	for _, st := range steps {
		if st.ID == stepID {
			return lease, st, nil
		}
	}
	return nil, domain.Step{}, apperr.New(apperr.CodeNotFound, "execution step not found")
}

func buildTaskDetail(ctx context.Context, tasks domain.TaskRepository, steps domain.StepRepository, taskID string) (*TaskDetailDTO, error) {
	task, err := tasks.GetByID(ctx, taskID)
	if err != nil {
		return nil, wrapExecError(err, "load execution task failed")
	}
	stepDTOs := []StepDTO{}
	if steps != nil {
		rows, listErr := steps.ListByTaskID(ctx, taskID)
		if listErr == nil {
			stepDTOs = make([]StepDTO, 0, len(rows))
			for _, st := range rows {
				stepDTOs = append(stepDTOs, ToStepDTO(st))
			}
		}
	}
	return &TaskDetailDTO{Task: ToTaskDTO(*task), Steps: stepDTOs}, nil
}
