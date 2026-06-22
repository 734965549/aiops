package application

import (
	"github.com/734965549/aiops/internal/integration/domain"
)

type AccountDTO struct {
	AccountID      string   `json:"account_id"`
	Name           string   `json:"name"`
	Provider       string   `json:"provider"`
	AuthType       string   `json:"auth_type"`
	Regions        []string `json:"regions,omitempty"`
	ProjectID      string   `json:"project_id,omitempty"`
	HasCredential  bool     `json:"has_credential"`
	Enabled        bool     `json:"enabled"`
	OwnerTeam      string   `json:"owner_team,omitempty"`
	Description    string   `json:"description,omitempty"`
	Capabilities   []string `json:"capabilities,omitempty"`
	LastCheckStatus string  `json:"last_check_status,omitempty"`
	CreatedAt      int64    `json:"created_at"`
	UpdatedAt      int64    `json:"updated_at"`
}

type ConnectivityCheckDTO struct {
	Status       string   `json:"status"`
	Provider     string   `json:"provider"`
	Capabilities []string `json:"capabilities"`
	CheckedAt    int64    `json:"checked_at"`
	Message      string   `json:"message"`
}

func ToAccountDTO(acc domain.IntegrationAccount, caps []domain.Capability, lastCheck *domain.ConnectivityCheck) AccountDTO {
	dto := AccountDTO{
		AccountID:     acc.AccountID,
		Name:          acc.Name,
		Provider:      string(acc.Provider),
		AuthType:      string(acc.AuthType),
		Regions:       append([]string(nil), acc.Regions...),
		ProjectID:     acc.ProjectID,
		HasCredential: acc.CredentialRefID != "",
		Enabled:       acc.Enabled,
		OwnerTeam:     acc.OwnerTeam,
		Description:   acc.Description,
		Capabilities:  capabilitiesToStrings(caps),
		CreatedAt:     acc.CreatedAt.Unix(),
		UpdatedAt:     acc.UpdatedAt.Unix(),
	}
	if lastCheck != nil {
		dto.LastCheckStatus = string(lastCheck.Status)
	}
	return dto
}

func ToConnectivityCheckDTO(check domain.ConnectivityCheck) ConnectivityCheckDTO {
	return ConnectivityCheckDTO{
		Status:       string(check.Status),
		Provider:     string(check.Provider),
		Capabilities: capabilitiesToStrings(check.Capabilities),
		CheckedAt:    check.CheckedAt.Unix(),
		Message:      check.Message,
	}
}

func capabilitiesToStrings(caps []domain.Capability) []string {
	if len(caps) == 0 {
		return nil
	}
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, string(c))
	}
	return out
}
