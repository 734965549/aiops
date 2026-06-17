package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/asset/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// MatchRuleService 管理告警匹配规则 CRUD。
type MatchRuleService struct {
	rules     domain.MatchRuleRepository
	apps      domain.ApplicationRepository
	resources domain.ResourceRepository
	audit     AuditRecorder
}

// NewMatchRuleService 构造匹配规则服务。
func NewMatchRuleService(rules domain.MatchRuleRepository, apps domain.ApplicationRepository, resources domain.ResourceRepository, audit AuditRecorder) *MatchRuleService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &MatchRuleService{rules: rules, apps: apps, resources: resources, audit: audit}
}

type CreateMatchRuleInput struct {
	ID                string
	Name              string
	Enabled           *bool
	Priority          int
	TargetType        string
	SourceType        string
	LabelKey          string
	LabelValuePattern string
	ApplicationID     string
	ResourceID        string
}

type UpdateMatchRuleInput struct {
	Name              string
	Enabled           *bool
	Priority          int
	TargetType        string
	SourceType        string
	LabelKey          string
	LabelValuePattern string
	ApplicationID     string
	ResourceID        string
}

type MatchRuleDTO struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Enabled           bool   `json:"enabled"`
	Priority          int    `json:"priority"`
	TargetType        string `json:"target_type"`
	SourceType        string `json:"source_type"`
	LabelKey          string `json:"label_key"`
	LabelValuePattern string `json:"label_value_pattern"`
	ApplicationID     string `json:"application_id"`
	ResourceID        string `json:"resource_id,omitempty"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
}

func (s *MatchRuleService) List(ctx context.Context) ([]MatchRuleDTO, error) {
	if s == nil || s.rules == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "match rule service is not enabled")
	}
	rows, err := s.rules.List(ctx)
	if err != nil {
		return nil, wrapAssetError(err, "list match rules failed")
	}
	out := make([]MatchRuleDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, toMatchRuleDTO(row))
	}
	return out, nil
}

func (s *MatchRuleService) Create(ctx context.Context, actor Actor, in CreateMatchRuleInput) (*MatchRuleDTO, error) {
	if s == nil || s.rules == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "match rule service is not enabled")
	}
	rule, err := s.buildRuleFromCreate(ctx, in)
	if err != nil {
		return nil, err
	}
	if err := s.rules.Create(ctx, rule); err != nil {
		return nil, wrapAssetError(err, "create match rule failed")
	}
	s.recordAudit(ctx, rule.ID, actor.UserID, AuditCreateMatchRule, map[string]any{
		"name": rule.Name, "label_key": rule.LabelKey, "label_value_pattern": rule.LabelValuePattern,
		"application_id": rule.ApplicationID, "resource_id": rule.ResourceID, "result": "success",
	})
	dto := toMatchRuleDTO(*rule)
	return &dto, nil
}

func (s *MatchRuleService) Update(ctx context.Context, id string, actor Actor, in UpdateMatchRuleInput) (*MatchRuleDTO, error) {
	if s == nil || s.rules == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "match rule service is not enabled")
	}
	id = strings.TrimSpace(id)
	existing, err := s.rules.GetByID(ctx, id)
	if err != nil {
		return nil, wrapAssetError(err, "load match rule failed")
	}
	rule, err := s.buildRuleFromUpdate(ctx, *existing, in)
	if err != nil {
		return nil, err
	}
	if err := s.rules.Update(ctx, rule); err != nil {
		return nil, wrapAssetError(err, "update match rule failed")
	}
	s.recordAudit(ctx, rule.ID, actor.UserID, AuditUpdateMatchRule, map[string]any{
		"name": rule.Name, "enabled": rule.Enabled, "priority": rule.Priority,
		"application_id": rule.ApplicationID, "resource_id": rule.ResourceID, "result": "success",
	})
	dto := toMatchRuleDTO(*rule)
	return &dto, nil
}

func (s *MatchRuleService) Delete(ctx context.Context, id string, actor Actor) error {
	if s == nil || s.rules == nil {
		return apperr.New(apperr.CodeUnavailable, "match rule service is not enabled")
	}
	id = strings.TrimSpace(id)
	if err := s.rules.Delete(ctx, id); err != nil {
		return wrapAssetError(err, "delete match rule failed")
	}
	s.recordAudit(ctx, id, actor.UserID, AuditDeleteMatchRule, map[string]any{"result": "success"})
	return nil
}

func (s *MatchRuleService) recordAudit(ctx context.Context, resourceID, userID string, action AuditAction, payload map[string]any) {
	if s == nil || s.audit == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "match_rule",
		ResourceID:   resourceID,
		Action:       action,
		UserID:       userID,
		Payload:      payload,
	})
}

func (s *MatchRuleService) buildRuleFromCreate(ctx context.Context, in CreateMatchRuleInput) (*domain.MatchRule, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = uuid.NewString()
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	target, source, err := parseMatchRuleTypes(in.TargetType, in.SourceType)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	labelKey := strings.TrimSpace(in.LabelKey)
	pattern := strings.TrimSpace(in.LabelValuePattern)
	appID := strings.TrimSpace(in.ApplicationID)
	resID := strings.TrimSpace(in.ResourceID)
	if name == "" || labelKey == "" || pattern == "" || appID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "name, label_key, label_value_pattern and application_id are required")
	}
	if err := s.validateRuleRefs(ctx, target, appID, resID); err != nil {
		return nil, err
	}
	return &domain.MatchRule{
		ID: id, Name: name, Enabled: enabled, Priority: in.Priority,
		TargetType: target, SourceType: source,
		LabelKey: labelKey, LabelValuePattern: pattern,
		ApplicationID: appID, ResourceID: resID,
	}, nil
}

func (s *MatchRuleService) buildRuleFromUpdate(ctx context.Context, existing domain.MatchRule, in UpdateMatchRuleInput) (*domain.MatchRule, error) {
	target, source, err := parseMatchRuleTypes(in.TargetType, in.SourceType)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	labelKey := strings.TrimSpace(in.LabelKey)
	pattern := strings.TrimSpace(in.LabelValuePattern)
	appID := strings.TrimSpace(in.ApplicationID)
	resID := strings.TrimSpace(in.ResourceID)
	if name == "" || labelKey == "" || pattern == "" || appID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "name, label_key, label_value_pattern and application_id are required")
	}
	enabled := existing.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if err := s.validateRuleRefs(ctx, target, appID, resID); err != nil {
		return nil, err
	}
	existing.Name = name
	existing.Enabled = enabled
	existing.Priority = in.Priority
	existing.TargetType = target
	existing.SourceType = source
	existing.LabelKey = labelKey
	existing.LabelValuePattern = pattern
	existing.ApplicationID = appID
	existing.ResourceID = resID
	return &existing, nil
}

func (s *MatchRuleService) validateRuleRefs(ctx context.Context, target domain.MatchTargetType, appID, resID string) error {
	if s == nil || s.apps == nil {
		return apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
	}
	exists, err := s.apps.ExistsByID(ctx, appID)
	if err != nil {
		return wrapAssetError(err, "check application failed")
	}
	if !exists {
		return apperr.New(apperr.CodeNotFound, "application not found")
	}
	if target == domain.TargetResource {
		if resID == "" {
			return apperr.New(apperr.CodeInvalidArgument, "resource_id is required when target_type is resource")
		}
		if s.resources == nil {
			return apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
		}
		res, err := s.resources.GetByID(ctx, resID)
		if err != nil {
			return wrapAssetError(err, "load resource failed")
		}
		if res.ApplicationID != appID {
			return apperr.New(apperr.CodeInvalidArgument, "resource does not belong to application")
		}
	}
	return nil
}

func parseMatchRuleTypes(targetRaw, sourceRaw string) (domain.MatchTargetType, domain.MatchSourceType, error) {
	target := domain.MatchTargetType(strings.ToLower(strings.TrimSpace(targetRaw)))
	if target == "" {
		target = domain.TargetApplication
	}
	if !target.IsValid() {
		return "", "", apperr.New(apperr.CodeInvalidArgument, "invalid target_type")
	}
	source := domain.MatchSourceType(strings.ToLower(strings.TrimSpace(sourceRaw)))
	if source == "" {
		source = domain.MatchSourceAll
	}
	if !source.IsValid() {
		return "", "", apperr.New(apperr.CodeInvalidArgument, "invalid source_type")
	}
	return target, source, nil
}

func toMatchRuleDTO(r domain.MatchRule) MatchRuleDTO {
	return MatchRuleDTO{
		ID: r.ID, Name: r.Name, Enabled: r.Enabled, Priority: r.Priority,
		TargetType: string(r.TargetType), SourceType: string(r.SourceType),
		LabelKey: r.LabelKey, LabelValuePattern: r.LabelValuePattern,
		ApplicationID: r.ApplicationID, ResourceID: r.ResourceID,
		CreatedAt: r.CreatedAt.Unix(), UpdatedAt: r.UpdatedAt.Unix(),
	}
}
