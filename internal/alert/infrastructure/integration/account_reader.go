package integration

import (
	"context"
	"strings"

	alertapp "github.com/734965549/aiops/internal/alert/application"
	integapp "github.com/734965549/aiops/internal/integration/application"
)

// AccountReader 将 integration 账号服务适配为 alert 的账号读取端口。
type AccountReader struct {
	service *integapp.AccountService
}

func NewAccountReader(service *integapp.AccountService) *AccountReader {
	return &AccountReader{service: service}
}

var _ alertapp.IntegrationAccountReader = (*AccountReader)(nil)

func (r *AccountReader) GetAccount(ctx context.Context, accountID string) (*alertapp.IntegrationAccountInfo, error) {
	if r == nil || r.service == nil {
		return nil, nil
	}
	dto, err := r.service.Get(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if dto == nil {
		return nil, nil
	}
	return &alertapp.IntegrationAccountInfo{
		AccountID:      dto.AccountID,
		Provider:       dto.Provider,
		AuthType:       dto.AuthType,
		Regions:        append([]string(nil), dto.Regions...),
		ProjectID:      dto.ProjectID,
		RegionProjects: parseRegionProjects(dto.ExtraConfig),
		OwnerTeam:      dto.OwnerTeam,
		Capabilities:   append([]string(nil), dto.Capabilities...),
		Enabled:        dto.Enabled,
	}, nil
}

func parseRegionProjects(extra map[string]any) map[string]string {
	out := map[string]string{}
	if len(extra) == 0 {
		return out
	}
	raw, ok := extra["region_projects"]
	if !ok {
		return out
	}
	items, ok := raw.([]any)
	if !ok {
		return out
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		region, _ := m["region"].(string)
		projectID, _ := m["project_id"].(string)
		region = strings.ToLower(strings.TrimSpace(region))
		projectID = strings.TrimSpace(projectID)
		if region != "" && projectID != "" {
			out[region] = projectID
		}
	}
	return out
}
