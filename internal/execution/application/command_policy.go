package application

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/734965549/aiops/internal/execution/domain"
)

var shellMetaPattern = regexp.MustCompile(`[;&|$` + "`" + `<>]`)

// ValidateCommandArguments 校验 arguments 是否符合 Command Spec schema。
func ValidateCommandArguments(schema map[string]any, arguments map[string]any) error {
	if arguments == nil {
		arguments = map[string]any{}
	}
	if err := validateParameterSchema(schema, arguments); err != nil {
		return err
	}
	return validateStringPatterns(schema, arguments, "arguments")
}

func validateStringPatterns(schema map[string]any, value any, path string) error {
	if len(schema) == 0 {
		return nil
	}
	rawType, _ := schema["type"].(string)
	switch strings.ToLower(strings.TrimSpace(rawType)) {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		props, _ := schema["properties"].(map[string]any)
		for key, propSchema := range props {
			childSchema, ok := propSchema.(map[string]any)
			if !ok {
				continue
			}
			childValue, exists := obj[key]
			if !exists {
				continue
			}
			if err := validateStringPatterns(childSchema, childValue, path+"."+key); err != nil {
				return err
			}
		}
	case "string":
		s, ok := value.(string)
		if !ok {
			return nil
		}
		if pattern, ok := schema["pattern"].(string); ok && strings.TrimSpace(pattern) != "" {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("%s has invalid pattern in schema", path)
			}
			if !re.MatchString(s) {
				return fmt.Errorf("%s does not match required pattern", path)
			}
		}
		if shellMetaPattern.MatchString(s) {
			return fmt.Errorf("%s contains forbidden shell metacharacters", path)
		}
	}
	return nil
}

// BuildCommandArgv 将受控模板与参数渲染为 argv。
func BuildCommandArgv(template string, arguments map[string]any) ([]string, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return nil, fmt.Errorf("command template is empty")
	}
	rendered := template
	for key, val := range arguments {
		placeholder := "{" + key + "}"
		strVal := fmt.Sprint(val)
		if shellMetaPattern.MatchString(strVal) {
			return nil, fmt.Errorf("argument %s contains forbidden shell metacharacters", key)
		}
		rendered = strings.ReplaceAll(rendered, placeholder, strVal)
	}
	if strings.Contains(rendered, "{") && strings.Contains(rendered, "}") {
		return nil, fmt.Errorf("command template has unresolved placeholders")
	}
	if shellMetaPattern.MatchString(rendered) {
		return nil, fmt.Errorf("rendered command contains forbidden shell metacharacters")
	}
	parts := strings.Fields(rendered)
	if len(parts) == 0 {
		return nil, fmt.Errorf("rendered command is empty")
	}
	return parts, nil
}

// ResolveAgentTaskRisk 计算 agent 模式任务风险（用户只能提高，不能降低）。
func ResolveAgentTaskRisk(medium *domain.ExecutionMedium, spec *domain.CommandSpec, environment, override string) (domain.RiskLevel, error) {
	base := spec.RiskLevel
	if base == "" || !base.IsValid() {
		base = domain.RiskLow
	}
	if medium != nil {
		switch medium.MediumType {
		case domain.MediumTargetHost:
			if domain.RiskRank(base) < domain.RiskRank(domain.RiskHigh) {
				base = domain.RiskHigh
			}
		case domain.MediumCloudRunCmd, domain.MediumDBReadonly:
			if domain.RiskRank(base) < domain.RiskRank(domain.RiskHigh) {
				base = domain.RiskHigh
			}
		}
		if medium.MaxRiskLevel.IsValid() && domain.RiskRank(base) > domain.RiskRank(medium.MaxRiskLevel) {
			return "", domain.ErrInvalidArgument
		}
	}
	env := strings.ToLower(strings.TrimSpace(environment))
	if (env == "prod" || env == "production") && domain.RiskRank(base) < domain.RiskRank(domain.RiskMedium) {
		base = domain.RiskMedium
	}
	override = strings.ToLower(strings.TrimSpace(override))
	if override == "" {
		return base, nil
	}
	requested := domain.RiskLevel(override)
	if !requested.IsValid() {
		return "", domain.ErrInvalidArgument
	}
	if domain.RiskRank(requested) < domain.RiskRank(base) {
		return "", domain.ErrInvalidArgument
	}
	return requested, nil
}

func mediumSupportsSpec(medium *domain.ExecutionMedium, spec *domain.CommandSpec) error {
	if medium == nil || spec == nil {
		return domain.ErrInvalidArgument
	}
	if !medium.Enabled {
		return domain.ErrFailedPrecondition
	}
	supportedType := false
	for _, t := range spec.MediumTypes {
		if strings.EqualFold(t, string(medium.MediumType)) {
			supportedType = true
			break
		}
	}
	if !supportedType {
		return domain.ErrInvalidArgument
	}
	if len(medium.AllowedCommandIDs) > 0 {
		allowed := false
		for _, id := range medium.AllowedCommandIDs {
			if id == spec.CommandSpecID {
				allowed = true
				break
			}
		}
		if !allowed {
			return domain.ErrInvalidArgument
		}
	}
	if len(spec.RequiredCaps) > 0 {
		capSet := map[string]struct{}{}
		for _, c := range medium.Capabilities {
			capSet[c] = struct{}{}
		}
		for _, req := range spec.RequiredCaps {
			if _, ok := capSet[req]; !ok {
				return domain.ErrInvalidArgument
			}
		}
	}
	return nil
}

// RedactOutput 按规则脱敏输出内容。
func RedactOutput(content string, redaction map[string]any) (string, bool) {
	patterns := redactionPatterns(redaction)
	if len(patterns) == 0 {
		return content, false
	}
	redacted := content
	changed := false
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		next := re.ReplaceAllString(redacted, "[REDACTED]")
		if next != redacted {
			changed = true
			redacted = next
		}
	}
	return redacted, changed
}

func redactionPatterns(redaction map[string]any) []string {
	if len(redaction) == 0 {
		return nil
	}
	raw, ok := redaction["patterns"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
