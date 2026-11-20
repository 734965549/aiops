package domain

import (
	"context"
	"time"
)

const (
	RelationDependsOn       = "depends_on"
	RelationAttachedTo      = "attached_to"
	RelationLocatedIn       = "located_in"
	RelationRoutesTo        = "routes_to"
	RelationOriginManual    = "manual"
	RelationOriginCloudSync = "cloud_sync"
)

type ResourceRelation struct {
	ID               string
	SourceResourceID string
	TargetResourceID string
	RelationType     string
	Origin           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func IsResourceRelationType(value string) bool {
	switch value {
	case RelationDependsOn, RelationAttachedTo, RelationLocatedIn, RelationRoutesTo:
		return true
	default:
		return false
	}
}

type ResourceRelationRepository interface {
	Create(ctx context.Context, relation *ResourceRelation) error
	GetByID(ctx context.Context, id string) (*ResourceRelation, error)
	Delete(ctx context.Context, id string) error
	ListByResourceIDs(ctx context.Context, resourceIDs []string) ([]ResourceRelation, error)
	CountByResourceID(ctx context.Context, resourceID string) (int64, error)
}
