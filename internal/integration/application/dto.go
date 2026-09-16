package application

import (
	"bytes"
	"encoding/json"
	"slices"
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

func encodeExtraConfigInput(provider domain.ProviderType, extra map[string]any) ([]byte, error) {
	if len(extra) == 0 {
		if provider == domain.ProviderHuaweiCloud {
			return []byte(`{"sync_mode":"ces"}`), nil
		}
		return []byte("{}"), nil
	}
	if containsSensitiveExtraConfigKey(extra) {
		return nil, apperr.New(apperr.CodeInvalidArgument, "extra_config must not contain secrets")
	}
	if err := validateProviderExtraConfig(provider, extra); err != nil {
		return nil, err
	}
	raw, err := marshalStrictExtraConfig(extra)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func validateProviderExtraConfig(provider domain.ProviderType, extra map[string]any) error {
	if provider != domain.ProviderHuaweiCloud {
		return nil
	}
	return validateHuaweiExtraConfig(extra)
}

func validateHuaweiExtraConfig(extra map[string]any) error {
	allowed := map[string]struct{}{
		"sync_mode": {}, "resource_group_name": {}, "resource_group_id": {},
		"enterprise_project_id": {}, "max_resources": {}, "region_projects": {},
	}
	for key := range extra {
		if _, ok := allowed[key]; !ok && isSensitiveExtraConfigKey(key) {
			return apperr.Newf(apperr.CodeInvalidArgument, "extra_config.%s is not supported", key)
		}
	}
	if mode, ok := extra["sync_mode"]; ok {
		str, ok := mode.(string)
		if !ok {
			return apperr.New(apperr.CodeInvalidArgument, "extra_config.sync_mode must be a string")
		}
		switch strings.ToLower(strings.TrimSpace(str)) {
		case "ces", "hybrid", "native":
		default:
			return apperr.New(apperr.CodeInvalidArgument, "extra_config.sync_mode must be one of ces, hybrid, native")
		}
	}
	for _, key := range []string{"resource_group_name", "resource_group_id", "enterprise_project_id"} {
		if value, ok := extra[key]; ok {
			str, ok := value.(string)
			if !ok {
				return apperr.Newf(apperr.CodeInvalidArgument, "extra_config.%s must be a string", key)
			}
			if strings.TrimSpace(str) == "" {
				continue
			}
		}
	}
	if value, ok := extra["max_resources"]; ok {
		if err := validateHuaweiMaxResources(value); err != nil {
			return err
		}
	}
	if value, ok := extra["region_projects"]; ok {
		items, ok := value.([]any)
		if !ok {
			return apperr.New(apperr.CodeInvalidArgument, "extra_config.region_projects must be an array")
		}
		seen := map[string]struct{}{}
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				return apperr.New(apperr.CodeInvalidArgument, "extra_config.region_projects items must be objects")
			}
			for k := range m {
				if k != "region" && k != "project_id" && k != "resource_group_id" && k != "resource_group_name" {
					return apperr.Newf(apperr.CodeInvalidArgument, "extra_config.region_projects[].%s is not supported", k)
				}
			}
			region, ok := m["region"].(string)
			if !ok || strings.TrimSpace(region) == "" {
				return apperr.New(apperr.CodeInvalidArgument, "extra_config.region_projects[].region is required")
			}
			projectID, ok := m["project_id"].(string)
			if !ok || strings.TrimSpace(projectID) == "" {
				return apperr.New(apperr.CodeInvalidArgument, "extra_config.region_projects[].project_id is required")
			}
			for _, key := range []string{"resource_group_id", "resource_group_name"} {
				if raw, ok := m[key]; ok {
					str, ok := raw.(string)
					if !ok {
						return apperr.Newf(apperr.CodeInvalidArgument, "extra_config.region_projects[].%s must be a string", key)
					}
					if strings.TrimSpace(str) == "" {
						delete(m, key)
					}
				}
			}
			key := strings.ToLower(strings.TrimSpace(region))
			if _, dup := seen[key]; dup {
				return apperr.New(apperr.CodeInvalidArgument, "extra_config.region_projects contains duplicate region")
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}

func marshalStrictExtraConfig(extra map[string]any) ([]byte, error) {
	raw, err := json.Marshal(extra)
	if err != nil {
		return nil, apperr.New(apperr.CodeInvalidArgument, "extra_config must be a valid json object")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return []byte("{}"), nil
	}
	return raw, nil
}

func validateHuaweiMaxResources(value any) error {
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case float32:
		n = float64(v)
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return apperr.New(apperr.CodeInvalidArgument, "extra_config.max_resources must be an integer")
		}
		n = parsed
	default:
		return apperr.New(apperr.CodeInvalidArgument, "extra_config.max_resources must be an integer")
	}
	if n != float64(int(n)) || n <= 0 {
		return apperr.New(apperr.CodeInvalidArgument, "extra_config.max_resources must be a positive integer")
	}
	if int(n) > 20000 {
		return apperr.New(apperr.CodeInvalidArgument, "extra_config.max_resources must not exceed 20000")
	}
	return nil
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
	sensitive := []string{"secret", "password", "token", "credential", "access_key", "secret_key", "api_key", "authorization", "private_key", "encryption_key", "client_key"}
	if slices.ContainsFunc(sensitive, func(marker string) bool {
		return strings.Contains(k, marker)
	}) {
		return true
	}
	return false
}
