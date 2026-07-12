package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/execution/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/google/uuid"
)

const confirmTextRequired = "CONFIRM"

// TaskListQuery 列表查询。
type TaskListQuery struct {
	Page       int
	PageSize   int
	Status     string
	SourceType string
	SourceID   string
	Keyword    string
}

// CreateTaskInput 创建任务入参。
type CreateTaskInput struct {
	Name              string
	SourceType        string
	SourceID          string
	OperationType     string
	TargetType        string
	TargetID          string
	TargetName        string
	Environment       string
	Parameters        map[string]any
	RollbackPlan      map[string]any
	RiskLevel         string
	RunbookTemplateID string
	DryRun            bool
	ExecutionMode     string
	MediumID          string
	CommandSpecID     string
	Arguments         map[string]any
}

// ConfirmTaskInput 确认入参。
type ConfirmTaskInput struct {
	Confirm     bool
	ConfirmText string
}

// TaskService 执行任务服务（ops/execution-contract.md）。
type TaskService struct {
	tasks    domain.TaskRepository
	steps    domain.StepRepository
	creator  domain.TaskCreator
	alerts   AlertReader
	timeline AlertTimelineWriter
	audit    AuditRecorder
	runbooks RunbookLoader
	media    domain.MediumRepository
	specs    domain.CommandSpecRepository
	now      func() time.Time
	simulate StepSimulator
}

// StepSimulator 模拟步骤执行。
type StepSimulator interface {
	Run(ctx context.Context, task domain.Task, step *domain.Step) error
}

// DefaultStepSimulator 第一阶段同步模拟成功。
type DefaultStepSimulator struct{}

func (DefaultStepSimulator) Run(_ context.Context, task domain.Task, step *domain.Step) error {
	if task.DryRun || step.DryRun {
		if step.DryRunSupported {
			step.Output = map[string]any{
				"dry_run":   true,
				"simulated": true,
				"message":   fmt.Sprintf("dry-run step %s completed", step.Name),
			}
			return nil
		}
		step.Output = map[string]any{
			"dry_run":                true,
			"skipped_real_execution": true,
			"message":                "step does not support real dry-run, simulated only",
		}
		return nil
	}
	step.Output = map[string]any{
		"simulated": true,
		"message":   fmt.Sprintf("step %s completed", step.Name),
	}
	return nil
}

// NewTaskService 构造服务。
func NewTaskService(
	tasks domain.TaskRepository,
	steps domain.StepRepository,
	creator domain.TaskCreator,
	alerts AlertReader,
	timeline AlertTimelineWriter,
	audit AuditRecorder,
	runbooks RunbookLoader,
	media domain.MediumRepository,
	specs domain.CommandSpecRepository,
) *TaskService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	if runbooks == nil {
		runbooks = NoopRunbookLoader{}
	}
	return &TaskService{
		tasks: tasks, steps: steps, creator: creator, alerts: alerts, timeline: timeline,
		audit: audit, runbooks: runbooks, media: media, specs: specs,
		now: time.Now, simulate: DefaultStepSimulator{},
	}
}

// Create 创建执行任务。
func (s *TaskService) Create(ctx context.Context, actor Actor, in CreateTaskInput) (*CreateTaskResult, error) {
	if s == nil || s.tasks == nil || s.steps == nil || s.creator == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "execution service is not enabled")
	}
	srcType := domain.SourceType(strings.ToLower(strings.TrimSpace(in.SourceType)))
	if !srcType.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid source_type")
	}
	sourceID := strings.TrimSpace(in.SourceID)
	if srcType == domain.SourceAlert && sourceID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "source_id is required for alert source")
	}

	runbookID := strings.TrimSpace(in.RunbookTemplateID)
	name := strings.TrimSpace(in.Name)
	targetType := strings.TrimSpace(in.TargetType)
	targetID := strings.TrimSpace(in.TargetID)
	targetName := strings.TrimSpace(in.TargetName)
	environment := strings.TrimSpace(in.Environment)

	if srcType == domain.SourceAlert {
		if s.alerts == nil {
			return nil, apperr.New(apperr.CodeUnavailable, "alert reader is not configured")
		}
		alertCtx, err := s.alerts.GetForExecution(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(alertCtx.Status, "processing") {
			return nil, apperr.New(apperr.CodeInvalidArgument, "alert must be in processing status to create execution task")
		}
		if name == "" && runbookID == "" {
			name = fmt.Sprintf("处置 %s", alertCtx.Name)
		}
		if environment == "" {
			environment = alertCtx.Environment
		}
		if targetName == "" {
			targetName = alertCtx.ResourceName
		}
		if targetID == "" {
			targetID = alertCtx.ResourceID
		}
		if targetType == "" {
			targetType = alertCtx.ResourceType
			if targetType == "" && alertCtx.ResourceName != "" {
				targetType = "resource"
			}
		}
	}

	opTypeRaw := strings.ToLower(strings.TrimSpace(in.OperationType))
	var opType domain.OperationType
	var runbookTpl *ExecutableRunbook
	var execSteps []domain.Step
	var runbookSnapshot map[string]any
	var runbookName string

	params := in.Parameters
	if params == nil {
		params = map[string]any{}
	}
	rollback := in.RollbackPlan
	if rollback == nil {
		rollback = map[string]any{}
	}
	taskDryRun := in.DryRun
	now := s.now()
	taskID := uuid.NewString()

	execMode := domain.ExecutionMode(strings.ToLower(strings.TrimSpace(in.ExecutionMode)))
	if execMode == "" {
		execMode = domain.ModeSimulated
	}
	if !execMode.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid execution_mode")
	}
	if execMode == domain.ModeAgent {
		return s.createAgentTask(ctx, actor, in, taskID, now, params, rollback, taskDryRun)
	}

	if runbookID != "" {
		if s.runbooks == nil {
			return nil, apperr.New(apperr.CodeUnavailable, "runbook loader is not configured")
		}
		var err error
		runbookTpl, err = s.runbooks.GetForExecution(ctx, runbookID)
		if err != nil {
			return nil, err
		}
		if runbookTpl == nil {
			return nil, apperr.New(apperr.CodeUnavailable, "runbook loader returned empty template")
		}
		if opTypeRaw == "" {
			opTypeRaw = runbookTpl.OperationType
		}
		opType = domain.OperationType(opTypeRaw)
		if !opType.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid operation_type")
		}
		if name == "" {
			name = runbookTpl.Name
		}
		runbookName = runbookTpl.Name
		if len(rollback) == 0 {
			rollback = cloneMap(runbookTpl.RollbackPlan)
		}
		if err := validateParameterSchema(runbookTpl.ParameterSchema, params); err != nil {
			return nil, apperr.New(apperr.CodeInvalidArgument, err.Error())
		}
		runbookSnapshot = buildRunbookSnapshot(runbookTpl)
		execSteps, err = buildStepsFromRunbook(taskID, runbookTpl, params, taskDryRun, now)
		if err != nil {
			return nil, apperr.New(apperr.CodeInvalidArgument, err.Error())
		}
	} else {
		opType = domain.OperationType(opTypeRaw)
		if !opType.IsValid() {
			return nil, apperr.New(apperr.CodeInvalidArgument, "invalid operation_type")
		}
		if name == "" {
			return nil, apperr.New(apperr.CodeInvalidArgument, "name is required")
		}
		execSteps = []domain.Step{{
			ID:         uuid.NewString(),
			TaskID:     taskID,
			StepOrder:  1,
			Name:       fmt.Sprintf("%s - 主步骤", name),
			ActionType: string(opType),
			Status:     domain.StepPending,
			Parameters: cloneMap(params),
			Output:     map[string]any{},
			CreatedAt:  now,
			UpdatedAt:  now,
		}}
	}

	var risk domain.RiskLevel
	var err error
	if runbookTpl != nil {
		riskLevel, riskErr := resolveRunbookTaskRisk(
			domain.OperationType(runbookTpl.OperationType),
			environment,
			runbookTpl.RiskLevel,
			runbookTpl.Steps,
			in.RiskLevel,
		)
		if riskErr != nil {
			return nil, wrapExecError(riskErr, "invalid risk_level")
		}
		risk = domain.RiskLevel(riskLevel)
	} else {
		risk, err = domain.ResolveRiskLevel(opType, environment, in.RiskLevel)
		if err != nil {
			return nil, wrapExecError(err, "invalid risk_level")
		}
	}

	status := domain.InitialStatusForRisk(risk)
	task := &domain.Task{
		ID:                taskID,
		Name:              name,
		SourceType:        srcType,
		SourceID:          sourceID,
		OperationType:     opType,
		TargetType:        targetType,
		TargetID:          targetID,
		TargetName:        targetName,
		Environment:       environment,
		RiskLevel:         risk,
		Status:            status,
		Parameters:        params,
		RollbackPlan:      rollback,
		RunbookTemplateID: runbookID,
		RunbookSnapshot:   runbookSnapshot,
		DryRun:            taskDryRun,
		CreatedBy:         strings.TrimSpace(actor.UserID),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.creator.CreateWithSteps(ctx, task, execSteps); err != nil {
		return nil, wrapExecError(err, "create execution task failed")
	}

	timelinePayload := map[string]any{
		"execution_id":   taskID,
		"operation_type": string(opType),
		"risk_level":     string(risk),
		"target_name":    targetName,
		"task_name":      name,
		"dry_run":        taskDryRun,
	}
	if runbookID != "" {
		timelinePayload["runbook_template_id"] = runbookID
		timelinePayload["runbook_name"] = runbookName
	}
	auditAction := AuditCreate
	if runbookID != "" {
		auditAction = AuditCreateFromRunbook
	}
	s.recordAudit(ctx, taskID, actor.UserID, auditAction, timelinePayload)
	if srcType == domain.SourceAlert && s.timeline != nil {
		s.recordExecutionTimeline(ctx, "execution_created", sourceID, taskID, func() error {
			return s.timeline.RecordExecutionCreated(ctx, sourceID, actor, taskID, timelinePayload)
		})
		s.recordResourceAudit(ctx, "alert", sourceID, actor.UserID, AuditAlertCreate, map[string]any{
			"execution_id": taskID, "operation_type": string(opType),
			"runbook_template_id": runbookID,
		})
	}

	result := &CreateTaskResult{
		TaskID:    taskID,
		Status:    string(status),
		RiskLevel: string(risk),
	}
	if status == domain.StatusPendingConfirm {
		result.ConfirmURL = fmt.Sprintf("/executions?task_id=%s", taskID)
	}
	return result, nil
}

func buildStepsFromRunbook(
	taskID string,
	tpl *ExecutableRunbook,
	userParams map[string]any,
	taskDryRun bool,
	now time.Time,
) ([]domain.Step, error) {
	steps := make([]domain.Step, 0, len(tpl.Steps))
	for _, rs := range tpl.Steps {
		stepDryRun := taskDryRun && rs.DryRunSupported
		parameters := mergeStepParameters(rs.DefaultParameters, userParams)
		if err := validateParameterSchema(rs.ParameterSchema, parameters); err != nil {
			return nil, fmt.Errorf("step %d %s: %w", rs.StepOrder, rs.Name, err)
		}
		steps = append(steps, domain.Step{
			ID:              uuid.NewString(),
			TaskID:          taskID,
			StepOrder:       rs.StepOrder,
			Name:            rs.Name,
			ActionType:      rs.ActionType,
			Status:          domain.StepPending,
			RunbookStepID:   rs.StepID,
			Parameters:      parameters,
			RiskLevel:       domain.RiskLevel(strings.ToLower(strings.TrimSpace(rs.RiskLevel))),
			DryRun:          stepDryRun,
			DryRunSupported: rs.DryRunSupported,
			RollbackPlan:    cloneMap(rs.RollbackPlan),
			TimeoutSeconds:  rs.TimeoutSeconds,
			Output:          map[string]any{},
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}
	return steps, nil
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// List 分页列表。
func (s *TaskService) List(ctx context.Context, q TaskListQuery) ([]TaskDTO, int64, error) {
	if s == nil || s.tasks == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "execution service is not enabled")
	}
	filter, err := buildTaskFilter(q)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.tasks.List(ctx, filter)
	if err != nil {
		return nil, 0, wrapExecError(err, "list execution tasks failed")
	}
	total, err := s.tasks.Count(ctx, filter)
	if err != nil {
		return nil, 0, wrapExecError(err, "count execution tasks failed")
	}
	out := make([]TaskDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, ToTaskDTO(row))
	}
	return out, total, nil
}

// GetDetail 任务详情含步骤。
func (s *TaskService) GetDetail(ctx context.Context, taskID string) (*TaskDetailDTO, error) {
	if s == nil || s.tasks == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "execution service is not enabled")
	}
	task, err := s.tasks.GetByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return nil, wrapExecError(err, "load execution task failed")
	}
	stepDTOs := []StepDTO{}
	if s.steps != nil {
		steps, err := s.steps.ListByTaskID(ctx, task.ID)
		if err != nil {
			return nil, wrapExecError(err, "list execution steps failed")
		}
		stepDTOs = make([]StepDTO, 0, len(steps))
		for _, st := range steps {
			stepDTOs = append(stepDTOs, ToStepDTO(st))
		}
	}
	detail := TaskDetailDTO{Task: ToTaskDTO(*task), Steps: stepDTOs}
	return &detail, nil
}

// Confirm 确认或拒绝待执行任务。
func (s *TaskService) Confirm(ctx context.Context, taskID string, actor Actor, in ConfirmTaskInput) (*TaskDTO, error) {
	if s == nil || s.tasks == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "execution service is not enabled")
	}
	if !in.Confirm {
		return s.rejectTask(ctx, taskID, actor)
	}
	if strings.TrimSpace(in.ConfirmText) != confirmTextRequired {
		return nil, apperr.New(apperr.CodeInvalidArgument, "confirm_text must be CONFIRM")
	}
	now := s.now()
	task, err := s.tasks.UpdateStatusIf(ctx, strings.TrimSpace(taskID), domain.StatusPendingConfirm, domain.StatusPendingExecute, func(t *domain.Task) {
		t.ConfirmedBy = strings.TrimSpace(actor.UserID)
		t.ConfirmedAt = &now
		if t.ExecutionMode == domain.ModeAgent {
			t.DispatchStatus = domain.DispatchPending
		}
		t.UpdatedAt = now
	})
	if err != nil {
		return nil, wrapExecError(err, "confirm execution task failed")
	}
	s.recordAudit(ctx, task.ID, actor.UserID, AuditConfirm, map[string]any{"status": string(domain.StatusPendingExecute), "result": "success"})
	dto := ToTaskDTO(*task)
	return &dto, nil
}

func (s *TaskService) rejectTask(ctx context.Context, taskID string, actor Actor) (*TaskDTO, error) {
	now := s.now()
	task, err := s.tasks.UpdateStatusIf(ctx, strings.TrimSpace(taskID), domain.StatusPendingConfirm, domain.StatusCancelled, func(t *domain.Task) {
		t.UpdatedAt = now
	})
	if err != nil {
		return nil, wrapExecError(err, "reject execution task failed")
	}
	s.recordAudit(ctx, task.ID, actor.UserID, AuditReject, map[string]any{"status": string(domain.StatusCancelled), "result": "success"})
	dto := ToTaskDTO(*task)
	return &dto, nil
}

// Execute 触发同步模拟执行。
func (s *TaskService) Execute(ctx context.Context, taskID string, actor Actor) (*TaskDetailDTO, error) {
	if s == nil || s.tasks == nil || s.steps == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "execution service is not enabled")
	}
	taskID = strings.TrimSpace(taskID)
	task, loadErr := s.tasks.GetByID(ctx, taskID)
	if loadErr != nil {
		return nil, wrapExecError(loadErr, "load execution task failed")
	}
	if task.ExecutionMode == domain.ModeAgent {
		return nil, apperr.New(apperr.CodeFailedPrecondition, "agent mode tasks must be executed by execution agent after confirm")
	}
	now := s.now()
	task, err := s.tasks.UpdateStatusIf(ctx, taskID, domain.StatusPendingExecute, domain.StatusRunning, func(t *domain.Task) {
		t.ExecutedBy = strings.TrimSpace(actor.UserID)
		t.StartedAt = &now
		t.UpdatedAt = now
	})
	if err != nil {
		return nil, wrapExecError(err, "start execution task failed")
	}

	if task.SourceType == domain.SourceAlert && task.SourceID != "" && s.timeline != nil {
		s.recordExecutionTimeline(ctx, "execution_started", task.SourceID, task.ID, func() error {
			return s.timeline.RecordExecutionStarted(ctx, task.SourceID, actor, task.ID, map[string]any{
				"execution_id":   task.ID,
				"operation_type": string(task.OperationType),
			})
		})
	}

	steps, listErr := s.steps.ListByTaskID(ctx, task.ID)

	var execErr error
	if listErr != nil {
		execErr = wrapExecError(listErr, "list execution steps failed")
	} else if len(steps) == 0 {
		execErr = apperr.New(apperr.CodeInvalidArgument, "execution task has no steps")
	} else {
		for i := range steps {
			step := &steps[i]
			step.Status = domain.StepRunning
			step.StartedAt = &now
			step.UpdatedAt = now
			if err := s.steps.Update(ctx, step); err != nil {
				execErr = wrapExecError(err, "mark execution step running failed")
				break
			}

			if err := s.simulate.Run(ctx, *task, step); err != nil {
				step.Status = domain.StepFailed
				step.ErrorMessage = err.Error()
				execErr = err
			} else {
				step.Status = domain.StepSuccess
			}
			fin := s.now()
			step.FinishedAt = &fin
			step.UpdatedAt = fin
			if err := s.steps.Update(ctx, step); err != nil {
				if execErr == nil {
					execErr = wrapExecError(err, "persist execution step result failed")
				}
				break
			}
			if execErr != nil {
				break
			}
		}
	}

	finishedAt := s.now()
	task.FinishedAt = &finishedAt
	task.UpdatedAt = finishedAt
	if execErr != nil {
		task.Status = domain.StatusFailed
		task.ErrorMessage = execErr.Error()
	} else {
		task.Status = domain.StatusSuccess
		task.ResultSummary = fmt.Sprintf("%s 执行成功", task.Name)
	}
	if err := s.tasks.Update(ctx, task); err != nil {
		return nil, wrapExecError(err, "finish execution task failed")
	}

	finishPayload := map[string]any{
		"execution_id":   task.ID,
		"status":         string(task.Status),
		"result_summary": task.ResultSummary,
		"error_message":  task.ErrorMessage,
		"result":         string(task.Status),
	}
	s.recordAudit(ctx, task.ID, actor.UserID, AuditExecute, finishPayload)
	if task.SourceType == domain.SourceAlert && task.SourceID != "" && s.timeline != nil {
		s.recordExecutionTimeline(ctx, "execution_finished", task.SourceID, task.ID, func() error {
			return s.timeline.RecordExecutionFinished(ctx, task.SourceID, actor, task.ID, finishPayload)
		})
	}

	return s.buildExecuteResult(ctx, task.ID)
}

func (s *TaskService) buildExecuteResult(ctx context.Context, taskID string) (*TaskDetailDTO, error) {
	finishedTask, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		return nil, wrapExecError(err, "load finished execution task failed")
	}
	stepDTOs := []StepDTO{}
	if s.steps != nil {
		steps, listErr := s.steps.ListByTaskID(ctx, taskID)
		if listErr == nil {
			stepDTOs = make([]StepDTO, 0, len(steps))
			for _, st := range steps {
				stepDTOs = append(stepDTOs, ToStepDTO(st))
			}
		}
	}
	detail := TaskDetailDTO{Task: ToTaskDTO(*finishedTask), Steps: stepDTOs}
	return &detail, nil
}

func (s *TaskService) loadTask(ctx context.Context, taskID string) (*domain.Task, error) {
	task, err := s.tasks.GetByID(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return nil, wrapExecError(err, "load execution task failed")
	}
	return task, nil
}

func buildTaskFilter(q TaskListQuery) (domain.TaskFilter, error) {
	statuses, err := parseStatusFilter(q.Status)
	if err != nil {
		return domain.TaskFilter{}, err
	}
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return domain.TaskFilter{
		Statuses:   statuses,
		SourceType: strings.TrimSpace(q.SourceType),
		SourceID:   strings.TrimSpace(q.SourceID),
		Keyword:    strings.TrimSpace(q.Keyword),
		Limit:      pageSize,
		Offset:     (page - 1) * pageSize,
	}, nil
}

func parseStatusFilter(raw string) ([]domain.TaskStatus, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]domain.TaskStatus, 0, len(parts))
	for _, p := range parts {
		st := domain.TaskStatus(strings.ToLower(strings.TrimSpace(p)))
		if !st.IsValid() {
			return nil, apperr.Newf(apperr.CodeInvalidArgument, "invalid status filter: %s", p)
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *TaskService) recordExecutionTimeline(ctx context.Context, eventType, alertID, taskID string, record func() error) {
	if s == nil || s.timeline == nil {
		return
	}
	if err := record(); err != nil {
		logger.From(ctx).Warn("execution timeline write failed",
			logger.String("event_type", eventType),
			logger.String("task_id", taskID),
			logger.String("alert_id", alertID),
			logger.Error(err),
		)
	}
}

func (s *TaskService) recordAudit(ctx context.Context, resourceID, userID string, action AuditAction, payload map[string]any) {
	s.recordResourceAudit(ctx, "execution", resourceID, userID, action, payload)
}

func (s *TaskService) recordResourceAudit(ctx context.Context, resourceType, resourceID, userID string, action AuditAction, payload map[string]any) {
	if s == nil || s.audit == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		UserID:       userID,
		Payload:      payload,
	})
}

func wrapExecError(err error, op string) error {
	if err == nil {
		return nil
	}
	return apperr.MapSentinels(err, op,
		apperr.Sentinel{Err: domain.ErrNotFound, Code: apperr.CodeNotFound},
		apperr.Sentinel{Err: domain.ErrAlreadyExists, Code: apperr.CodeAlreadyExists},
		apperr.Sentinel{Err: domain.ErrInvalidTransition, Code: apperr.CodeInvalidArgument},
		apperr.Sentinel{Err: domain.ErrInvalidArgument, Code: apperr.CodeInvalidArgument},
		apperr.Sentinel{Err: domain.ErrFailedPrecondition, Code: apperr.CodeFailedPrecondition},
	)
}

func (s *TaskService) createAgentTask(
	ctx context.Context,
	actor Actor,
	in CreateTaskInput,
	taskID string,
	now time.Time,
	params map[string]any,
	rollback map[string]any,
	taskDryRun bool,
) (*CreateTaskResult, error) {
	if s.media == nil || s.specs == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "execution agent dependencies are not configured")
	}
	mediumID := strings.TrimSpace(in.MediumID)
	commandSpecID := strings.TrimSpace(in.CommandSpecID)
	arguments := cloneMap(in.Arguments)
	if mediumID == "" {
		if v, ok := params["medium_id"].(string); ok {
			mediumID = strings.TrimSpace(v)
		}
	}
	if commandSpecID == "" {
		if v, ok := params["command_spec_id"].(string); ok {
			commandSpecID = strings.TrimSpace(v)
		}
	}
	if len(arguments) == 0 {
		if raw, ok := params["arguments"].(map[string]any); ok {
			arguments = cloneMap(raw)
		}
	}
	if mediumID == "" || commandSpecID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "medium_id and command_spec_id are required for agent execution")
	}
	medium, err := s.media.GetByID(ctx, mediumID)
	if err != nil {
		return nil, wrapExecError(err, "load execution medium failed")
	}
	spec, err := loadEnabledCommandSpec(ctx, s.specs, commandSpecID)
	if err != nil {
		return nil, wrapExecError(err, "load command spec failed")
	}
	if err := mediumSupportsSpec(medium, spec); err != nil {
		return nil, wrapExecError(err, "medium does not support command spec")
	}
	if err := ValidateCommandArguments(spec.ArgumentSchema, arguments); err != nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, err.Error())
	}
	if _, err := BuildCommandArgv(spec.CommandTemplate, arguments); err != nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, err.Error())
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = spec.Name
	}
	environment := strings.TrimSpace(in.Environment)
	if environment == "" {
		environment = medium.Environment
	}
	targetType := strings.TrimSpace(in.TargetType)
	targetID := strings.TrimSpace(in.TargetID)
	targetName := strings.TrimSpace(in.TargetName)

	risk, err := ResolveAgentTaskRisk(medium, spec, environment, in.RiskLevel)
	if err != nil {
		return nil, wrapExecError(err, "invalid risk_level")
	}
	status := domain.InitialStatusForRisk(risk)
	dispatchStatus := domain.DispatchStatus("")
	if status == domain.StatusPendingExecute {
		dispatchStatus = domain.DispatchPending
	}

	task := &domain.Task{
		ID: taskID, Name: name,
		SourceType:    domain.SourceType(strings.ToLower(strings.TrimSpace(in.SourceType))),
		SourceID:      strings.TrimSpace(in.SourceID),
		OperationType: domain.OpCommand, TargetType: targetType, TargetID: targetID,
		TargetName: targetName, Environment: environment, RiskLevel: risk, Status: status,
		ExecutionMode: domain.ModeAgent, MediumID: mediumID, CommandSpecID: commandSpecID,
		DispatchStatus: dispatchStatus,
		Parameters: map[string]any{
			"command_spec_id": commandSpecID,
			"arguments":       arguments,
			"medium_id":       mediumID,
		},
		RollbackPlan: rollback, DryRun: taskDryRun,
		CreatedBy: strings.TrimSpace(actor.UserID), CreatedAt: now, UpdatedAt: now,
	}
	step := domain.Step{
		ID: uuid.NewString(), TaskID: taskID, StepOrder: 1, Name: spec.Name,
		ActionType: string(domain.OpCommand), Status: domain.StepPending,
		CommandSpecID: commandSpecID, CommandTemplate: spec.CommandTemplate,
		Arguments: cloneMap(arguments), OutputRedaction: cloneMap(spec.OutputRedaction),
		TimeoutSeconds: spec.TimeoutSeconds, Parameters: cloneMap(arguments),
		RiskLevel: risk, Output: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.creator.CreateWithSteps(ctx, task, []domain.Step{step}); err != nil {
		return nil, wrapExecError(err, "create agent execution task failed")
	}
	s.recordAudit(ctx, taskID, actor.UserID, AuditCreate, map[string]any{
		"execution_mode": "agent", "medium_id": mediumID, "command_spec_id": commandSpecID,
		"risk_level": string(risk), "status": string(status),
	})
	result := &CreateTaskResult{TaskID: taskID, Status: string(status), RiskLevel: string(risk)}
	if status == domain.StatusPendingConfirm {
		result.ConfirmURL = fmt.Sprintf("/executions?task_id=%s", taskID)
	}
	return result, nil
}
