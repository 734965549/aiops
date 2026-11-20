package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

const maxTopologyResources = 299

type TopologyService struct {
	apps      domain.ApplicationRepository
	resources domain.ResourceRepository
	relations domain.ResourceRelationRepository
	audit     AuditRecorder
}

func NewTopologyService(apps domain.ApplicationRepository, resources domain.ResourceRepository, relations domain.ResourceRelationRepository, audit AuditRecorder) *TopologyService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &TopologyService{apps: apps, resources: resources, relations: relations, audit: audit}
}

type TopologyQuery struct {
	ApplicationID     string
	RootResourceID    string
	Depth             int
	Direction         string
	Region            string
	CloudResourceType string
	RelationType      string
	Keyword           string
	IncludeStale      bool
}

type TopologyNodeDTO struct {
	NodeID            string `json:"node_id"`
	EntityType        string `json:"entity_type"`
	EntityID          string `json:"entity_id"`
	Name              string `json:"name"`
	ApplicationID     string `json:"application_id,omitempty"`
	Environment       string `json:"environment,omitempty"`
	ResourceType      string `json:"resource_type,omitempty"`
	CloudResourceType string `json:"cloud_resource_type,omitempty"`
	CloudResourceID   string `json:"cloud_resource_id,omitempty"`
	Region            string `json:"region,omitempty"`
	SyncStatus        string `json:"sync_status,omitempty"`
	Source            string `json:"source,omitempty"`
	LastSyncedAt      int64  `json:"last_synced_at,omitempty"`
}

type TopologyEdgeDTO struct {
	EdgeID       string `json:"edge_id"`
	SourceNodeID string `json:"source_node_id"`
	TargetNodeID string `json:"target_node_id"`
	RelationType string `json:"relation_type"`
	Origin       string `json:"origin"`
}

type TopologyMetaDTO struct {
	GeneratedAt             int64  `json:"generated_at"`
	Truncated               bool   `json:"truncated"`
	NodeCount               int    `json:"node_count"`
	EdgeCount               int    `json:"edge_count"`
	UnresolvedRelationCount int    `json:"unresolved_relation_count"`
	RootResourceID          string `json:"root_resource_id,omitempty"`
	Depth                   int    `json:"depth,omitempty"`
	Direction               string `json:"direction,omitempty"`
}

type TopologyDTO struct {
	Nodes []TopologyNodeDTO `json:"nodes"`
	Edges []TopologyEdgeDTO `json:"edges"`
	Meta  TopologyMetaDTO   `json:"meta"`
}

type ResourceRelationDTO struct {
	ID                  string `json:"relation_id"`
	SourceResourceID    string `json:"source_resource_id"`
	SourceName          string `json:"source_name"`
	SourceApplicationID string `json:"source_application_id,omitempty"`
	TargetResourceID    string `json:"target_resource_id"`
	TargetName          string `json:"target_name"`
	TargetApplicationID string `json:"target_application_id,omitempty"`
	RelationType        string `json:"relation_type"`
	Origin              string `json:"origin"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

type CreateResourceRelationInput struct {
	SourceResourceID string
	TargetResourceID string
	RelationType     string
}

func (s *TopologyService) GetTopology(ctx context.Context, query TopologyQuery) (*TopologyDTO, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	query.Region = strings.TrimSpace(query.Region)
	query.CloudResourceType = strings.TrimSpace(query.CloudResourceType)
	query.RelationType = strings.TrimSpace(query.RelationType)
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.RootResourceID = strings.TrimSpace(query.RootResourceID)
	query.Direction = strings.TrimSpace(query.Direction)
	if query.RootResourceID != "" {
		return s.getRootTopology(ctx, query)
	}
	app, rows, err := s.loadApplicationResources(ctx, query.ApplicationID)
	if err != nil {
		return nil, err
	}
	if query.RelationType != "" && !domain.IsResourceRelationType(query.RelationType) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid relation_type")
	}
	filtered := make([]domain.Resource, 0, len(rows))
	for _, row := range rows {
		if matchesTopologyResource(row, query) {
			filtered = append(filtered, row)
		}
	}
	truncated := len(filtered) > maxTopologyResources
	if truncated {
		filtered = filtered[:maxTopologyResources]
	}
	nodes := []TopologyNodeDTO{{
		NodeID: "application:" + app.ID, EntityType: "application", EntityID: app.ID,
		Name: app.Name, ApplicationID: app.ID, Environment: app.Environment,
	}}
	edges := make([]TopologyEdgeDTO, 0, len(filtered))
	resourceIDs := make([]string, 0, len(filtered))
	included := make(map[string]struct{}, len(filtered))
	for _, row := range filtered {
		resourceIDs = append(resourceIDs, row.ID)
		included[row.ID] = struct{}{}
		nodes = append(nodes, toTopologyResourceNode(row))
		edges = append(edges, TopologyEdgeDTO{
			EdgeID: "contains:" + row.ID, SourceNodeID: "application:" + app.ID,
			TargetNodeID: "resource:" + row.ID, RelationType: "contains", Origin: "registry",
		})
	}
	relations, err := s.relations.ListByResourceIDs(ctx, resourceIDs)
	if err != nil {
		return nil, wrapAssetError(err, "list resource relations failed")
	}
	for _, relation := range relations {
		if query.RelationType != "" && relation.RelationType != query.RelationType {
			continue
		}
		_, sourceIncluded := included[relation.SourceResourceID]
		_, targetIncluded := included[relation.TargetResourceID]
		if !sourceIncluded || !targetIncluded {
			continue
		}
		edges = append(edges, toTopologyRelationEdge(relation))
	}
	derived := deriveCloudRelations(rows)
	for _, relation := range derived.relations {
		if query.RelationType != "" && relation.RelationType != query.RelationType {
			continue
		}
		_, sourceIncluded := included[relation.SourceResourceID]
		_, targetIncluded := included[relation.TargetResourceID]
		if !sourceIncluded || !targetIncluded {
			continue
		}
		edges = append(edges, toTopologyRelationEdge(relation))
	}
	unresolved := 0
	for _, resourceID := range derived.unresolved {
		if _, ok := included[resourceID]; ok {
			unresolved++
		}
	}
	return &TopologyDTO{
		Nodes: nodes, Edges: edges,
		Meta: TopologyMetaDTO{
			GeneratedAt: time.Now().UTC().Unix(), Truncated: truncated,
			NodeCount: len(nodes), EdgeCount: len(edges),
			UnresolvedRelationCount: unresolved,
		},
	}, nil
}

func (s *TopologyService) ListRelations(ctx context.Context, applicationID, relationType string) ([]ResourceRelationDTO, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	relationType = strings.TrimSpace(relationType)
	_, rows, err := s.loadApplicationResources(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if relationType != "" && !domain.IsResourceRelationType(relationType) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid relation_type")
	}
	ids := make([]string, 0, len(rows))
	names := make(map[string]string, len(rows))
	applicationIDs := make(map[string]string, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
		names[row.ID] = topologyResourceName(row)
		applicationIDs[row.ID] = row.ApplicationID
	}
	relations, err := s.relations.ListByResourceIDs(ctx, ids)
	if err != nil {
		return nil, wrapAssetError(err, "list resource relations failed")
	}
	resolveResource := func(id string) (string, string, error) {
		if name, ok := names[id]; ok {
			return name, applicationIDs[id], nil
		}
		row, err := s.resources.GetByID(ctx, id)
		if err != nil {
			return "", "", err
		}
		names[id] = topologyResourceName(*row)
		applicationIDs[id] = row.ApplicationID
		return names[id], applicationIDs[id], nil
	}
	out := make([]ResourceRelationDTO, 0, len(relations))
	for _, relation := range relations {
		if relationType != "" && relation.RelationType != relationType {
			continue
		}
		_, sourceIncluded := names[relation.SourceResourceID]
		_, targetIncluded := names[relation.TargetResourceID]
		if !sourceIncluded && !targetIncluded {
			continue
		}
		sourceName, sourceApplicationID, err := resolveResource(relation.SourceResourceID)
		if err != nil {
			return nil, wrapAssetError(err, "load source resource failed")
		}
		targetName, targetApplicationID, err := resolveResource(relation.TargetResourceID)
		if err != nil {
			return nil, wrapAssetError(err, "load target resource failed")
		}
		dto := toResourceRelationDTO(relation, sourceName, targetName)
		dto.SourceApplicationID = sourceApplicationID
		dto.TargetApplicationID = targetApplicationID
		out = append(out, dto)
	}
	return out, nil
}

func (s *TopologyService) CreateRelation(ctx context.Context, actor Actor, input CreateResourceRelationInput) (*ResourceRelationDTO, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	sourceID := strings.TrimSpace(input.SourceResourceID)
	targetID := strings.TrimSpace(input.TargetResourceID)
	relationType := strings.TrimSpace(input.RelationType)
	if sourceID == "" || targetID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "source_resource_id and target_resource_id are required")
	}
	if sourceID == targetID {
		return nil, apperr.New(apperr.CodeInvalidArgument, "source and target resources must be different")
	}
	if !domain.IsResourceRelationType(relationType) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid relation_type")
	}
	source, err := s.resources.GetByID(ctx, sourceID)
	if err != nil {
		return nil, wrapAssetError(err, "load source resource failed")
	}
	target, err := s.resources.GetByID(ctx, targetID)
	if err != nil {
		return nil, wrapAssetError(err, "load target resource failed")
	}
	relation := &domain.ResourceRelation{
		ID: uuid.NewString(), SourceResourceID: sourceID, TargetResourceID: targetID,
		RelationType: relationType, Origin: domain.RelationOriginManual,
	}
	if err := s.relations.Create(ctx, relation); err != nil {
		return nil, wrapAssetError(err, "create resource relation failed")
	}
	s.recordAudit(ctx, relation.ID, actor.UserID, AuditCreateResourceRelation, map[string]any{
		"source_resource_id": sourceID, "target_resource_id": targetID,
		"relation_type": relationType, "result": "success",
	})
	dto := toResourceRelationDTO(*relation, topologyResourceName(*source), topologyResourceName(*target))
	dto.SourceApplicationID = source.ApplicationID
	dto.TargetApplicationID = target.ApplicationID
	return &dto, nil
}

func (s *TopologyService) DeleteRelation(ctx context.Context, id string, actor Actor) error {
	if err := s.ready(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return apperr.New(apperr.CodeInvalidArgument, "relation_id is required")
	}
	relation, err := s.relations.GetByID(ctx, id)
	if err != nil {
		return wrapAssetError(err, "load resource relation failed")
	}
	if err := s.relations.Delete(ctx, id); err != nil {
		return wrapAssetError(err, "delete resource relation failed")
	}
	s.recordAudit(ctx, id, actor.UserID, AuditDeleteResourceRelation, map[string]any{
		"source_resource_id": relation.SourceResourceID, "target_resource_id": relation.TargetResourceID,
		"relation_type": relation.RelationType, "result": "success",
	})
	return nil
}

func (s *TopologyService) ready() error {
	if s == nil || s.apps == nil || s.resources == nil || s.relations == nil {
		return apperr.New(apperr.CodeUnavailable, "asset topology service is not enabled")
	}
	return nil
}

func (s *TopologyService) loadApplicationResources(ctx context.Context, applicationID string) (*domain.Application, []domain.Resource, error) {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return nil, nil, apperr.New(apperr.CodeInvalidArgument, "application_id is required")
	}
	app, err := s.apps.GetByID(ctx, applicationID)
	if err != nil {
		return nil, nil, wrapAssetError(err, "load application failed")
	}
	rows, err := s.resources.ListByApplicationID(ctx, applicationID)
	if err != nil {
		return nil, nil, wrapAssetError(err, "list application resources failed")
	}
	return app, rows, nil
}

func (s *TopologyService) recordAudit(ctx context.Context, relationID, userID string, action AuditAction, payload map[string]any) {
	if s == nil || s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "resource_relation", ResourceID: relationID,
		Action: action, UserID: userID, Payload: payload,
	})
}

func matchesTopologyResource(row domain.Resource, query TopologyQuery) bool {
	if query.Region != "" && row.Region != query.Region {
		return false
	}
	if query.CloudResourceType != "" && row.CloudResourceType != query.CloudResourceType {
		return false
	}
	if row.SyncStatus == domain.SyncStatusStale && !query.IncludeStale {
		return false
	}
	if query.Keyword == "" {
		return true
	}
	keyword := strings.ToLower(query.Keyword)
	for _, value := range []string{row.ID, row.Name, row.CloudResourceID, row.ResourceType} {
		if strings.Contains(strings.ToLower(value), keyword) {
			return true
		}
	}
	return false
}

func toTopologyResourceNode(row domain.Resource) TopologyNodeDTO {
	node := TopologyNodeDTO{
		NodeID: "resource:" + row.ID, EntityType: "resource", EntityID: row.ID,
		Name: topologyResourceName(row), ApplicationID: row.ApplicationID,
		ResourceType: row.ResourceType, CloudResourceType: row.CloudResourceType,
		CloudResourceID: row.CloudResourceID, Region: row.Region,
		SyncStatus: row.SyncStatus, Source: row.Source,
	}
	if node.Source == "" {
		node.Source = domain.ResourceSourceManual
	}
	if row.LastSyncedAt != nil {
		node.LastSyncedAt = row.LastSyncedAt.Unix()
	}
	return node
}

func toTopologyRelationEdge(relation domain.ResourceRelation) TopologyEdgeDTO {
	return TopologyEdgeDTO{
		EdgeID: "relation:" + relation.ID, SourceNodeID: "resource:" + relation.SourceResourceID,
		TargetNodeID: "resource:" + relation.TargetResourceID,
		RelationType: relation.RelationType, Origin: relation.Origin,
	}
}

func toResourceRelationDTO(relation domain.ResourceRelation, sourceName, targetName string) ResourceRelationDTO {
	return ResourceRelationDTO{
		ID: relation.ID, SourceResourceID: relation.SourceResourceID, SourceName: sourceName,
		TargetResourceID: relation.TargetResourceID, TargetName: targetName,
		RelationType: relation.RelationType, Origin: relation.Origin,
		CreatedAt: relation.CreatedAt.Unix(), UpdatedAt: relation.UpdatedAt.Unix(),
	}
}

func topologyResourceName(row domain.Resource) string {
	if name := strings.TrimSpace(row.Name); name != "" {
		return name
	}
	if name := strings.TrimSpace(row.CloudResourceID); name != "" {
		return name
	}
	return fmt.Sprintf("resource-%s", row.ID)
}
