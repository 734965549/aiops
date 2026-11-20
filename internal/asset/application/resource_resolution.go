package application

import (
	"context"
	"strings"

	apperr "github.com/734965549/aiops/pkg/errors"
)

type ResolveResourceQuery struct {
	CloudResourceID   string
	AccountID         string
	Region            string
	CloudResourceType string
}

type ResolvedResourceDTO struct {
	ResourceID        string `json:"resource_id"`
	ApplicationID     string `json:"application_id"`
	ApplicationName   string `json:"application_name,omitempty"`
	Name              string `json:"name"`
	ResourceType      string `json:"resource_type,omitempty"`
	CloudResourceType string `json:"cloud_resource_type,omitempty"`
	CloudResourceID   string `json:"cloud_resource_id"`
	Region            string `json:"region,omitempty"`
	SyncStatus        string `json:"sync_status,omitempty"`
}

type ResourceResolutionDTO struct {
	Status     string                `json:"status"`
	Candidates []ResolvedResourceDTO `json:"candidates"`
	Total      int                   `json:"total"`
}

func (s *TopologyService) ResolveResource(ctx context.Context, query ResolveResourceQuery) (*ResourceResolutionDTO, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	cloudResourceID := strings.TrimSpace(query.CloudResourceID)
	accountID := strings.TrimSpace(query.AccountID)
	region := strings.TrimSpace(query.Region)
	cloudResourceType := strings.TrimSpace(query.CloudResourceType)
	if cloudResourceID == "" || accountID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "cloud_resource_id and account_id are required")
	}
	rows, err := s.resources.ListByCloudResourceID(ctx, cloudResourceID)
	if err != nil {
		return nil, wrapAssetError(err, "resolve cloud resource failed")
	}
	applications, err := s.apps.List(ctx)
	if err != nil {
		return nil, wrapAssetError(err, "list resource applications failed")
	}
	applicationNames := make(map[string]string, len(applications))
	for _, app := range applications {
		applicationNames[app.ID] = app.Name
	}
	candidates := make([]ResolvedResourceDTO, 0, len(rows))
	for _, row := range rows {
		if row.IntegrationAccountID != accountID {
			continue
		}
		if region != "" && row.Region != region {
			continue
		}
		if cloudResourceType != "" && row.CloudResourceType != cloudResourceType {
			continue
		}
		candidates = append(candidates, ResolvedResourceDTO{
			ResourceID: row.ID, ApplicationID: row.ApplicationID,
			ApplicationName: applicationNames[row.ApplicationID], Name: topologyResourceName(row),
			ResourceType: row.ResourceType, CloudResourceType: row.CloudResourceType,
			CloudResourceID: row.CloudResourceID, Region: row.Region, SyncStatus: row.SyncStatus,
		})
	}
	status := "unregistered"
	if len(candidates) == 1 {
		status = "matched"
	} else if len(candidates) > 1 {
		status = "ambiguous"
	}
	return &ResourceResolutionDTO{Status: status, Candidates: candidates, Total: len(candidates)}, nil
}
