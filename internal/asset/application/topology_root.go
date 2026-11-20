package application

import (
	"context"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

func (s *TopologyService) getRootTopology(ctx context.Context, query TopologyQuery) (*TopologyDTO, error) {
	depth := query.Depth
	if depth == 0 {
		depth = 2
	}
	if depth < 1 || depth > 3 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "depth must be between 1 and 3")
	}
	direction := query.Direction
	if direction == "" {
		direction = "both"
	}
	if direction != "both" && direction != "upstream" && direction != "downstream" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "direction must be both, upstream or downstream")
	}
	if query.RelationType != "" && !domain.IsResourceRelationType(query.RelationType) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid relation_type")
	}
	root, err := s.resources.GetByID(ctx, query.RootResourceID)
	if err != nil {
		return nil, wrapAssetError(err, "load root resource failed")
	}
	rows, err := s.resources.ListAll(ctx)
	if err != nil {
		return nil, wrapAssetError(err, "list topology resources failed")
	}
	resourceByID := make(map[string]domain.Resource, len(rows)+1)
	ids := make([]string, 0, len(rows)+1)
	for _, row := range rows {
		resourceByID[row.ID] = row
		ids = append(ids, row.ID)
	}
	if _, ok := resourceByID[root.ID]; !ok {
		resourceByID[root.ID] = *root
		ids = append(ids, root.ID)
		rows = append(rows, *root)
	}
	manual, err := s.relations.ListByResourceIDs(ctx, ids)
	if err != nil {
		return nil, wrapAssetError(err, "list resource relations failed")
	}
	derived := deriveCloudRelations(rows)
	adjacency := make(map[string][]TopologyEdgeDTO)
	canonical := make([]TopologyEdgeDTO, 0, len(manual)+len(derived.relations))
	seenRelations := make(map[string]struct{})
	for _, relation := range append(manual, derived.relations...) {
		if query.RelationType != "" && relation.RelationType != query.RelationType {
			continue
		}
		if _, ok := resourceByID[relation.SourceResourceID]; !ok {
			continue
		}
		if _, ok := resourceByID[relation.TargetResourceID]; !ok {
			continue
		}
		key := relation.SourceResourceID + "|" + relation.TargetResourceID + "|" + relation.RelationType
		if _, ok := seenRelations[key]; ok {
			continue
		}
		seenRelations[key] = struct{}{}
		edge := toTopologyRelationEdge(relation)
		canonical = append(canonical, edge)
		if direction == "both" || direction == "downstream" {
			adjacency[edge.SourceNodeID] = append(adjacency[edge.SourceNodeID], edge)
		}
		if direction == "both" || direction == "upstream" {
			adjacency[edge.TargetNodeID] = append(adjacency[edge.TargetNodeID], TopologyEdgeDTO{
				EdgeID: edge.EdgeID, SourceNodeID: edge.TargetNodeID, TargetNodeID: edge.SourceNodeID,
				RelationType: edge.RelationType, Origin: edge.Origin,
			})
		}
	}

	visited := map[string]int{"resource:" + root.ID: 0}
	queue := []string{"resource:" + root.ID}
	truncated := false
	for len(queue) > 0 && !truncated {
		current := queue[0]
		queue = queue[1:]
		level := visited[current]
		if level >= depth {
			continue
		}
		for _, edge := range adjacency[current] {
			nextID := strings.TrimPrefix(edge.TargetNodeID, "resource:")
			if _, ok := visited[edge.TargetNodeID]; ok {
				continue
			}
			next, ok := resourceByID[nextID]
			if !ok || !matchesTopologyResource(next, query) {
				continue
			}
			if len(visited) >= maxTopologyResources {
				truncated = true
				break
			}
			visited[edge.TargetNodeID] = level + 1
			queue = append(queue, edge.TargetNodeID)
		}
	}

	nodes := make([]TopologyNodeDTO, 0, len(visited)+1)
	applicationIDs := make(map[string]struct{})
	for nodeID := range visited {
		resourceID := strings.TrimPrefix(nodeID, "resource:")
		row := resourceByID[resourceID]
		nodes = append(nodes, toTopologyResourceNode(row))
		applicationIDs[row.ApplicationID] = struct{}{}
	}
	applications, err := s.apps.List(ctx)
	if err != nil {
		return nil, wrapAssetError(err, "list topology applications failed")
	}
	applicationByID := make(map[string]domain.Application, len(applications))
	for _, app := range applications {
		applicationByID[app.ID] = app
	}
	edges := make([]TopologyEdgeDTO, 0)
	for _, app := range applications {
		if _, ok := applicationIDs[app.ID]; !ok {
			continue
		}
		nodes = append(nodes, TopologyNodeDTO{
			NodeID: "application:" + app.ID, EntityType: "application", EntityID: app.ID,
			Name: app.Name, ApplicationID: app.ID, Environment: app.Environment,
		})
	}
	for nodeID := range visited {
		resourceID := strings.TrimPrefix(nodeID, "resource:")
		row := resourceByID[resourceID]
		if _, ok := applicationByID[row.ApplicationID]; !ok {
			continue
		}
		edges = append(edges, TopologyEdgeDTO{
			EdgeID: "contains:" + resourceID, SourceNodeID: "application:" + row.ApplicationID,
			TargetNodeID: nodeID, RelationType: "contains", Origin: "registry",
		})
	}
	for _, edge := range canonical {
		if _, sourceIncluded := visited[edge.SourceNodeID]; !sourceIncluded {
			continue
		}
		if _, targetIncluded := visited[edge.TargetNodeID]; !targetIncluded {
			continue
		}
		edges = append(edges, edge)
	}
	unresolved := make(map[string]struct{})
	for _, resourceID := range derived.unresolved {
		if _, ok := visited["resource:"+resourceID]; ok {
			unresolved[resourceID] = struct{}{}
		}
	}
	return &TopologyDTO{
		Nodes: nodes,
		Edges: uniqueTopologyEdges(edges),
		Meta: TopologyMetaDTO{
			GeneratedAt: time.Now().UTC().Unix(), Truncated: truncated,
			NodeCount: len(nodes), EdgeCount: len(uniqueTopologyEdges(edges)),
			UnresolvedRelationCount: len(unresolved), RootResourceID: root.ID, Depth: depth, Direction: direction,
		},
	}, nil
}

func uniqueTopologyEdges(edges []TopologyEdgeDTO) []TopologyEdgeDTO {
	seen := make(map[string]struct{}, len(edges))
	out := make([]TopologyEdgeDTO, 0, len(edges))
	for _, edge := range edges {
		key := edge.SourceNodeID + "|" + edge.TargetNodeID + "|" + edge.RelationType + "|" + edge.Origin
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, edge)
	}
	return out
}
