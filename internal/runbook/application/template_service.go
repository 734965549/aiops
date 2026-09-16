package application

import (
	"context"
	"sort"
	"strings"
	"time"

	rbdomain "github.com/734965549/aiops/internal/runbook/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// TemplateListQuery 列表查询。
type TemplateListQuery struct {
	Page     int
	PageSize int
	Keyword  string
	Enabled  *bool
}

// CreateTemplateInput 创建模板入参。
type CreateTemplateInput struct {
	Name              string
	Description       string
	Enabled           bool
	OperationType     string
	RiskLevel         string
	MatchAlertName    string
	MatchResourceType string
	MatchEnvironment  string
	ParameterSchema   map[string]any
	RollbackPlan      map[string]any
	Steps             []CreateStepInput
}

// CreateStepInput 创建步骤入参。
type CreateStepInput struct {
	StepOrder         int
	Name              string
	ActionType        string
	RiskLevel         string
	DryRunSupported   bool
	DefaultDryRun     bool
	ParameterSchema   map[string]any
	DefaultParameters map[string]any
	RollbackPlan      map[string]any
	TimeoutSeconds    int
}

// UpdateTemplateInput 更新模板入参。
type UpdateTemplateInput struct {
	Name              string
	Description       string
	Enabled           bool
	OperationType     string
	RiskLevel         string
	MatchAlertName    string
	MatchResourceType string
	MatchEnvironment  string
	ParameterSchema   map[string]any
	RollbackPlan      map[string]any
	Steps             []CreateStepInput
}

// TemplateService Runbook 模板服务。
type TemplateService struct {
	templates rbdomain.TemplateRepository
	steps     rbdomain.StepRepository
	alerts    AlertReader
	audit     AuditRecorder
	now       func() time.Time
}

// NewTemplateService 构造服务。
func NewTemplateService(
	templates rbdomain.TemplateRepository,
	steps rbdomain.StepRepository,
	alerts AlertReader,
	audit AuditRecorder,
) *TemplateService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &TemplateService{
		templates: templates,
		steps:     steps,
		alerts:    alerts,
		audit:     audit,
		now:       time.Now,
	}
}

// List 分页列表。
func (s *TemplateService) List(ctx context.Context, q TemplateListQuery) ([]TemplateDTO, int64, error) {
	if s == nil || s.templates == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "runbook service is not enabled")
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
	filter := rbdomain.TemplateFilter{
		Enabled: q.Enabled,
		Keyword: strings.TrimSpace(q.Keyword),
		Limit:   pageSize,
		Offset:  (page - 1) * pageSize,
	}
	rows, err := s.templates.List(ctx, filter)
	if err != nil {
		return nil, 0, wrapRBError(err, "list runbook templates failed")
	}
	total, err := s.templates.Count(ctx, filter)
	if err != nil {
		return nil, 0, wrapRBError(err, "count runbook templates failed")
	}
	out := make([]TemplateDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, ToTemplateDTO(row))
	}
	return out, total, nil
}

// GetDetail 模板详情。
func (s *TemplateService) GetDetail(ctx context.Context, templateID string) (*TemplateDetailDTO, error) {
	if s == nil || s.templates == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "runbook service is not enabled")
	}
	tpl, steps, err := s.loadTemplateWithSteps(ctx, templateID)
	if err != nil {
		return nil, err
	}
	stepDTOs := make([]StepDTO, 0, len(steps))
	for _, st := range steps {
		stepDTOs = append(stepDTOs, ToStepDTO(st))
	}
	return &TemplateDetailDTO{Template: ToTemplateDTO(*tpl), Steps: stepDTOs}, nil
}

// GetForExecution 加载模板与步骤供 Execution 创建任务。
func (s *TemplateService) GetForExecution(ctx context.Context, templateID string) (*rbdomain.TemplateWithSteps, error) {
	tpl, steps, err := s.loadTemplateWithSteps(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if !tpl.Enabled {
		return nil, apperr.New(apperr.CodeInvalidArgument, "runbook template is disabled")
	}
	if len(steps) == 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "runbook template has no steps")
	}
	return &rbdomain.TemplateWithSteps{Template: *tpl, Steps: steps}, nil
}

// Recommend 根据告警推荐预案。
func (s *TemplateService) Recommend(ctx context.Context, actor Actor, alertID string) ([]RecommendationDTO, error) {
	if s == nil || s.templates == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "runbook service is not enabled")
	}
	if s.alerts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert reader is not configured")
	}
	alertCtx, err := s.alerts.GetForExecution(ctx, strings.TrimSpace(alertID))
	if err != nil {
		return nil, err
	}
	matchCtx := rbdomain.AlertMatchContext{
		Name:         alertCtx.Name,
		ResourceType: alertCtx.ResourceType,
		Environment:  alertCtx.Environment,
		Labels:       alertCtx.Labels,
		Annotations:  alertCtx.Annotations,
	}
	templates, err := s.templates.ListEnabled(ctx)
	if err != nil {
		return nil, wrapRBError(err, "list enabled runbooks failed")
	}

	results := make([]rbdomain.MatchResult, 0)
	for _, tpl := range templates {
		ok, score := rbdomain.MatchesTemplate(tpl, matchCtx)
		if !ok {
			continue
		}
		steps, err := s.steps.ListByTemplateID(ctx, tpl.ID)
		if err != nil {
			return nil, wrapRBError(err, "list runbook steps failed")
		}
		results = append(results, rbdomain.MatchResult{
			Template:        tpl,
			StepsCount:      len(steps),
			DryRunSupported: rbdomain.SupportsDryRun(steps),
			MatchedReason:   buildMatchedReason(score),
			Score:           score,
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.Score.Rank() != b.Score.Rank() {
			return a.Score.Rank() > b.Score.Rank()
		}
		if rbdomain.RiskRank(a.Template.RiskLevel) != rbdomain.RiskRank(b.Template.RiskLevel) {
			return rbdomain.RiskRank(a.Template.RiskLevel) < rbdomain.RiskRank(b.Template.RiskLevel)
		}
		return a.Template.UpdatedAt.After(b.Template.UpdatedAt)
	})

	out := make([]RecommendationDTO, 0, len(results))
	for _, r := range results {
		out = append(out, RecommendationDTO{
			TemplateID:      r.Template.ID,
			Name:            r.Template.Name,
			Description:     r.Template.Description,
			RiskLevel:       string(r.Template.RiskLevel),
			OperationType:   string(r.Template.OperationType),
			MatchedReason:   r.MatchedReason,
			StepsCount:      r.StepsCount,
			DryRunSupported: r.DryRunSupported,
			ParameterSchema: cloneAnyMap(r.Template.ParameterSchema),
		})
	}

	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "alert",
		ResourceID:   alertID,
		Action:       AuditRecommend,
		UserID:       actor.UserID,
		Payload: map[string]any{
			"alert_id": alertID,
			"count":    len(out),
		},
	})
	return out, nil
}

func cloneAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func buildMatchedReason(score rbdomain.MatchScore) string {
	parts := []string{}
	if score.NameExact || score.NameFuzzy {
		parts = append(parts, "alert_name")
	}
	if score.ResourceExact {
		parts = append(parts, "resource_type")
	}
	if score.EnvExact {
		parts = append(parts, "environment")
	}
	if len(parts) == 0 {
		return "wildcard matched"
	}
	return strings.Join(parts, "/") + " matched"
}

func (s *TemplateService) loadTemplateWithSteps(ctx context.Context, templateID string) (*rbdomain.Template, []rbdomain.Step, error) {
	tpl, err := s.templates.GetByID(ctx, strings.TrimSpace(templateID))
	if err != nil {
		return nil, nil, wrapRBError(err, "load runbook template failed")
	}
	steps, err := s.steps.ListByTemplateID(ctx, tpl.ID)
	if err != nil {
		return nil, nil, wrapRBError(err, "list runbook steps failed")
	}
	return tpl, steps, nil
}

func wrapRBError(err error, op string) error {
	if err == nil {
		return nil
	}
	return apperr.MapSentinels(err, op,
		apperr.Sentinel{Err: rbdomain.ErrNotFound, Code: apperr.CodeNotFound},
		apperr.Sentinel{Err: rbdomain.ErrAlreadyExists, Code: apperr.CodeAlreadyExists},
		apperr.Sentinel{Err: rbdomain.ErrInvalidArgument, Code: apperr.CodeInvalidArgument},
	)
}

func validateOperationType(raw string) (rbdomain.OperationType, error) {
	op := rbdomain.OperationType(strings.ToLower(strings.TrimSpace(raw)))
	if !op.IsValid() {
		return "", apperr.New(apperr.CodeInvalidArgument, "invalid operation_type")
	}
	return op, nil
}

func validateRiskLevel(raw string) (rbdomain.RiskLevel, error) {
	r := rbdomain.RiskLevel(strings.ToLower(strings.TrimSpace(raw)))
	if !r.IsValid() {
		return "", apperr.New(apperr.CodeInvalidArgument, "invalid risk_level")
	}
	return r, nil
}
