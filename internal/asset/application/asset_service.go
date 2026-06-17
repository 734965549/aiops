// Package application 实现 Asset 用例：注册表 CRUD 与 Alert §9.1 标签匹配。
package application

import (
	"context"
	"strings"

	"github.com/734965549/aiops/internal/asset/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// Actor 操作者。
type Actor struct {
	UserID      string
	DisplayName string
}

// AssetService 管理应用与资源注册表。
type AssetService struct {
	apps      domain.ApplicationRepository
	resources domain.ResourceRepository
	rules     domain.MatchRuleRepository
	audit     AuditRecorder
}

// NewAssetService 构造资产管理服务。
func NewAssetService(apps domain.ApplicationRepository, resources domain.ResourceRepository, rules domain.MatchRuleRepository, audit AuditRecorder) *AssetService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &AssetService{apps: apps, resources: resources, rules: rules, audit: audit}
}

type CreateApplicationInput struct {
	ID          string
	Name        string
	Environment string
	Namespace   string
	Description string
}

type CreateResourceInput struct {
	ID            string
	ApplicationID string
	Name          string
	ResourceType  string
	Namespace     string
	Pod           string
	Node          string
	Instance      string
}

type UpdateApplicationInput struct {
	Name        string
	Environment string
	Namespace   string
	Description string
}

type UpdateResourceInput struct {
	Name         string
	ResourceType string
	Namespace    string
	Pod          string
	Node         string
	Instance     string
}

type ApplicationDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Environment string `json:"environment,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Description string `json:"description,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type ResourceDTO struct {
	ID            string `json:"id"`
	ApplicationID string `json:"application_id"`
	Name          string `json:"name,omitempty"`
	ResourceType  string `json:"resource_type,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	Pod           string `json:"pod,omitempty"`
	Node          string `json:"node,omitempty"`
	Instance      string `json:"instance,omitempty"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

func (s *AssetService) ListApplications(ctx context.Context) ([]ApplicationDTO, error) {
	if s == nil || s.apps == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
	}
	rows, err := s.apps.List(ctx)
	if err != nil {
		return nil, wrapAssetError(err, "list applications failed")
	}
	out := make([]ApplicationDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, toApplicationDTO(row))
	}
	return out, nil
}

func (s *AssetService) CreateApplication(ctx context.Context, actor Actor, in CreateApplicationInput) (*ApplicationDTO, error) {
	if s == nil || s.apps == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = uuid.NewString()
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "name is required")
	}
	app := &domain.Application{
		ID:          id,
		Name:        name,
		Environment: strings.TrimSpace(in.Environment),
		Namespace:   strings.TrimSpace(in.Namespace),
		Description: strings.TrimSpace(in.Description),
	}
	if err := s.apps.Create(ctx, app); err != nil {
		return nil, wrapAssetError(err, "create application failed")
	}
	s.recordAudit(ctx, "application", app.ID, actor.UserID, AuditCreateApplication, map[string]any{
		"name": app.Name, "environment": app.Environment, "result": "success",
	})
	dto := toApplicationDTO(*app)
	return &dto, nil
}

func (s *AssetService) UpdateApplication(ctx context.Context, id string, actor Actor, in UpdateApplicationInput) (*ApplicationDTO, error) {
	if s == nil || s.apps == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "id is required")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "name is required")
	}
	app, err := s.apps.GetByID(ctx, id)
	if err != nil {
		return nil, wrapAssetError(err, "load application failed")
	}
	app.Name = name
	app.Environment = strings.TrimSpace(in.Environment)
	app.Namespace = strings.TrimSpace(in.Namespace)
	app.Description = strings.TrimSpace(in.Description)
	if err := s.apps.Update(ctx, app); err != nil {
		return nil, wrapAssetError(err, "update application failed")
	}
	s.recordAudit(ctx, "application", app.ID, actor.UserID, AuditUpdateApplication, map[string]any{
		"name": app.Name, "environment": app.Environment, "result": "success",
	})
	dto := toApplicationDTO(*app)
	return &dto, nil
}

func (s *AssetService) DeleteApplication(ctx context.Context, id string, actor Actor) error {
	if s == nil || s.apps == nil || s.resources == nil {
		return apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return apperr.New(apperr.CodeInvalidArgument, "id is required")
	}
	if _, err := s.apps.GetByID(ctx, id); err != nil {
		return wrapAssetError(err, "load application failed")
	}
	n, err := s.resources.CountByApplicationID(ctx, id)
	if err != nil {
		return wrapAssetError(err, "count application resources failed")
	}
	if n > 0 {
		return apperr.Newf(apperr.CodeFailedPrecondition, "application has %d resource(s), delete resources first", n)
	}
	if s.rules != nil {
		rn, err := s.rules.CountByApplicationID(ctx, id)
		if err != nil {
			return wrapAssetError(err, "count application match rules failed")
		}
		if rn > 0 {
			return apperr.Newf(apperr.CodeFailedPrecondition, "application has %d match rule(s), delete rules first", rn)
		}
	}
	if err := s.apps.Delete(ctx, id); err != nil {
		return wrapAssetError(err, "delete application failed")
	}
	s.recordAudit(ctx, "application", id, actor.UserID, AuditDeleteApplication, map[string]any{"result": "success"})
	return nil
}

func (s *AssetService) ListResources(ctx context.Context, applicationID string) ([]ResourceDTO, error) {
	if s == nil || s.resources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
	}
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "application_id is required")
	}
	rows, err := s.resources.ListByApplicationID(ctx, applicationID)
	if err != nil {
		return nil, wrapAssetError(err, "list resources failed")
	}
	out := make([]ResourceDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, toResourceDTO(row))
	}
	return out, nil
}

func (s *AssetService) CreateResource(ctx context.Context, actor Actor, in CreateResourceInput) (*ResourceDTO, error) {
	if s == nil || s.resources == nil || s.apps == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
	}
	appID := strings.TrimSpace(in.ApplicationID)
	if appID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "application_id is required")
	}
	exists, err := s.apps.ExistsByID(ctx, appID)
	if err != nil {
		return nil, wrapAssetError(err, "check application failed")
	}
	if !exists {
		return nil, apperr.New(apperr.CodeNotFound, "application not found")
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = uuid.NewString()
	}
	res := &domain.Resource{
		ID:            id,
		ApplicationID: appID,
		Name:          strings.TrimSpace(in.Name),
		ResourceType:  strings.TrimSpace(in.ResourceType),
		Namespace:     strings.TrimSpace(in.Namespace),
		Pod:           strings.TrimSpace(in.Pod),
		Node:          strings.TrimSpace(in.Node),
		Instance:      strings.TrimSpace(in.Instance),
	}
	if err := s.resources.Create(ctx, res); err != nil {
		return nil, wrapAssetError(err, "create resource failed")
	}
	s.recordAudit(ctx, "resource", res.ID, actor.UserID, AuditCreateResource, map[string]any{
		"application_id": res.ApplicationID, "name": res.Name,
		"resource_type": res.ResourceType, "result": "success",
	})
	dto := toResourceDTO(*res)
	return &dto, nil
}

func (s *AssetService) UpdateResource(ctx context.Context, id string, actor Actor, in UpdateResourceInput) (*ResourceDTO, error) {
	if s == nil || s.resources == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "id is required")
	}
	res, err := s.resources.GetByID(ctx, id)
	if err != nil {
		return nil, wrapAssetError(err, "load resource failed")
	}
	res.Name = strings.TrimSpace(in.Name)
	res.ResourceType = strings.TrimSpace(in.ResourceType)
	res.Namespace = strings.TrimSpace(in.Namespace)
	res.Pod = strings.TrimSpace(in.Pod)
	res.Node = strings.TrimSpace(in.Node)
	res.Instance = strings.TrimSpace(in.Instance)
	if err := s.resources.Update(ctx, res); err != nil {
		return nil, wrapAssetError(err, "update resource failed")
	}
	s.recordAudit(ctx, "resource", res.ID, actor.UserID, AuditUpdateResource, map[string]any{
		"application_id": res.ApplicationID, "name": res.Name, "result": "success",
	})
	dto := toResourceDTO(*res)
	return &dto, nil
}

func (s *AssetService) DeleteResource(ctx context.Context, id string, actor Actor) error {
	if s == nil || s.resources == nil {
		return apperr.New(apperr.CodeUnavailable, "asset service is not enabled")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return apperr.New(apperr.CodeInvalidArgument, "id is required")
	}
	if _, err := s.resources.GetByID(ctx, id); err != nil {
		return wrapAssetError(err, "load resource failed")
	}
	if s.rules != nil {
		rn, err := s.rules.CountByResourceID(ctx, id)
		if err != nil {
			return wrapAssetError(err, "count resource match rules failed")
		}
		if rn > 0 {
			return apperr.Newf(apperr.CodeFailedPrecondition, "resource has %d match rule(s), delete rules first", rn)
		}
	}
	if err := s.resources.Delete(ctx, id); err != nil {
		return wrapAssetError(err, "delete resource failed")
	}
	s.recordAudit(ctx, "resource", id, actor.UserID, AuditDeleteResource, map[string]any{"result": "success"})
	return nil
}

func (s *AssetService) recordAudit(ctx context.Context, resourceType, resourceID, userID string, action AuditAction, payload map[string]any) {
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

func wrapAssetError(err error, op string) error {
	if err == nil {
		return nil
	}
	mapped := apperr.MapSentinels(err, "asset operation failed",
		apperr.Sentinel{Err: domain.ErrNotFound, Code: apperr.CodeNotFound},
		apperr.Sentinel{Err: domain.ErrAlreadyExists, Code: apperr.CodeAlreadyExists},
		apperr.Sentinel{Err: domain.ErrHasResources, Code: apperr.CodeFailedPrecondition},
		apperr.Sentinel{Err: domain.ErrHasMatchRules, Code: apperr.CodeFailedPrecondition},
	)
	if apperr.FromError(mapped).Code != apperr.CodeInternal {
		return mapped
	}
	return apperr.Wrap(err, apperr.CodeInternal, op)
}

func toApplicationDTO(a domain.Application) ApplicationDTO {
	return ApplicationDTO{
		ID:          a.ID,
		Name:        a.Name,
		Environment: a.Environment,
		Namespace:   a.Namespace,
		Description: a.Description,
		CreatedAt:   a.CreatedAt.Unix(),
		UpdatedAt:   a.UpdatedAt.Unix(),
	}
}

func toResourceDTO(r domain.Resource) ResourceDTO {
	return ResourceDTO{
		ID:            r.ID,
		ApplicationID: r.ApplicationID,
		Name:          r.Name,
		ResourceType:  r.ResourceType,
		Namespace:     r.Namespace,
		Pod:           r.Pod,
		Node:          r.Node,
		Instance:      r.Instance,
		CreatedAt:     r.CreatedAt.Unix(),
		UpdatedAt:     r.UpdatedAt.Unix(),
	}
}

func labelValue(labels map[string]string, key string) string {
	if len(labels) == 0 {
		return ""
	}
	return strings.TrimSpace(labels[key])
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
