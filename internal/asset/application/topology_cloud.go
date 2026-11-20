package application

import (
	"fmt"
	"strings"

	"github.com/734965549/aiops/internal/asset/domain"
)

type cloudRelationRule struct {
	labelKey     string
	targetType   string
	relationType string
	reverse      bool
	sourceTypes  map[string]struct{}
}

var cloudRelationRules = []cloudRelationRule{
	{labelKey: "vpc_id", targetType: "vpc", relationType: domain.RelationLocatedIn, sourceTypes: cloudSourceTypes("ecs", "rds", "dcs", "dms", "subnet", "elb")},
	{labelKey: "subnet_id", targetType: "subnet", relationType: domain.RelationLocatedIn, sourceTypes: cloudSourceTypes("rds")},
	{labelKey: "attached_to", targetType: "ecs", relationType: domain.RelationAttachedTo, sourceTypes: cloudSourceTypes("evs")},
	{labelKey: "bandwidth_id", targetType: "bandwidth", relationType: domain.RelationAttachedTo, sourceTypes: cloudSourceTypes("eip")},
	{labelKey: "request_vpc_id", targetType: "vpc", relationType: domain.RelationRoutesTo, reverse: true, sourceTypes: cloudSourceTypes("peering")},
	{labelKey: "accept_vpc_id", targetType: "vpc", relationType: domain.RelationRoutesTo, sourceTypes: cloudSourceTypes("peering")},
}

type cloudDerivationResult struct {
	relations  []domain.ResourceRelation
	unresolved []string
}

func cloudSourceTypes(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func deriveCloudRelations(resources []domain.Resource) cloudDerivationResult {
	targets := make(map[string][]domain.Resource)
	for _, row := range resources {
		if row.Source != domain.ResourceSourceCloudSync || row.IntegrationAccountID == "" || row.Region == "" || row.CloudResourceID == "" {
			continue
		}
		key := cloudTargetKey(row.IntegrationAccountID, row.Region, row.CloudResourceType, row.CloudResourceID)
		targets[key] = append(targets[key], row)
	}
	result := cloudDerivationResult{relations: []domain.ResourceRelation{}, unresolved: []string{}}
	seen := make(map[string]struct{})
	for _, source := range resources {
		if source.Source != domain.ResourceSourceCloudSync || source.IntegrationAccountID == "" || source.Region == "" {
			continue
		}
		sourceType := strings.ToLower(strings.TrimSpace(source.CloudResourceType))
		for _, rule := range cloudRelationRules {
			if _, ok := rule.sourceTypes[sourceType]; !ok {
				continue
			}
			reference := strings.TrimSpace(source.Labels[rule.labelKey])
			if reference == "" {
				continue
			}
			candidates := targets[cloudTargetKey(source.IntegrationAccountID, source.Region, rule.targetType, reference)]
			if len(candidates) != 1 {
				result.unresolved = append(result.unresolved, source.ID)
				continue
			}
			target := candidates[0]
			sourceID, targetID := source.ID, target.ID
			if rule.reverse {
				sourceID, targetID = targetID, sourceID
			}
			if sourceID == targetID {
				continue
			}
			key := fmt.Sprintf("%s|%s|%s", sourceID, targetID, rule.relationType)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result.relations = append(result.relations, domain.ResourceRelation{
				ID:               "cloud:" + key,
				SourceResourceID: sourceID,
				TargetResourceID: targetID,
				RelationType:     rule.relationType,
				Origin:           domain.RelationOriginCloudSync,
			})
		}
	}
	return result
}

func cloudTargetKey(accountID, region, resourceType, cloudResourceID string) string {
	return fmt.Sprintf("%s|%s|%s|%s", accountID, region, strings.ToLower(strings.TrimSpace(resourceType)), cloudResourceID)
}
