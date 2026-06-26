package application

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/734965549/aiops/internal/integration/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type AccountDTO struct {
	AccountID       string         `json:"account_id"`
	Name            string         `json:"name"`
	Provider        string         `json:"provider"`
	AuthType        string         `json:"auth_type"`
	Regions         []string       `json:"regions,omitempty"`
	ProjectID       string         `json:"project_id,omitempty"`
	HasCredential   bool           `json:"has_credential"`
	Enabled         bool           `json:"enabled"`
	OwnerTeam       string         `json:"owner_team,omitempty"`
	Description     string         `json:"description,omitempty"`
	ExtraConfig     map[string]any `json:"extra_config,omitempty"`
	Capabilities    []string       `json:"capabilities,omitempty"`
	LastCheckStatus string         `json:"last_check_status,omitempty"`
	CreatedAt       int64          `json:"created_at"`
	UpdatedAt       int64          `json:"updated_at"`
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
		ExtraConfig:   safeExtraConfig(acc.ExtraConfig),
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

func encodeExtraConfigInput(extra map[string]any) ([]byte, error) {
	if extra == nil {
		return []byte("{}"), nil
	}
	if containsSensitiveExtraConfigKey(extra) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "extra_config must not contain secrets")
	}
	raw, err := json.Marshal(extra)
	if err != nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, "extra_config must be a valid json object")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return []byte("{}"), nil
	}
	return raw, nil
}

func safeExtraConfig(raw []byte) map[string]any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("{}")) {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return nil
	}
	removeSensitiveExtraConfigKeys(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func containsSensitiveExtraConfigKey(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if isSensitiveExtraConfigKey(key) || containsSensitiveExtraConfigKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsSensitiveExtraConfigKey(child) {
				return true
			}
		}
	}
	return false
}

func removeSensitiveExtraConfigKeys(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if isSensitiveExtraConfigKey(key) {
				delete(v, key)
				continue
			}
			removeSensitiveExtraConfigKeys(child)
		}
	case []any:
		for _, child := range v {
			removeSensitiveExtraConfigKeys(child)
		}
	}
}

func isSensitiveExtraConfigKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	sensitive := []string{"secret", "password", "token", "credential", "access_key", "secret_key", "api_key", "authorization"}
	for _, marker := range sensitive {
		if strings.Contains(k, marker) {
			return true
		}
	}
	return false
}
