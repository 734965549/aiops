package application

import (
	"math"
	"strings"

	apperr "github.com/734965549/aiops/pkg/errors"
)

func validateParameterSchema(schema, value map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	return validateSchemaValue(schema, value, "parameters")
}

func validateSchemaValue(schema map[string]any, value any, path string) error {
	if len(schema) == 0 {
		return nil
	}
	if rawType, ok := schema["type"].(string); ok {
		if err := validateSchemaType(strings.ToLower(strings.TrimSpace(rawType)), value, path); err != nil {
			return err
		}
	}
	if props, ok := schema["properties"].(map[string]any); ok {
		obj, ok := value.(map[string]any)
		if !ok {
			return apperr.Newf(apperr.CodeInvalidArgument, "%s must be an object", path)
		}
		for _, key := range requiredKeys(schema["required"]) {
			if _, exists := obj[key]; !exists {
				return apperr.Newf(apperr.CodeInvalidArgument, "%s.%s is required", path, key)
			}
		}
		for key, propSchema := range props {
			childSchema, ok := propSchema.(map[string]any)
			if !ok {
				continue
			}
			childValue, exists := obj[key]
			if !exists {
				continue
			}
			if err := validateSchemaValue(childSchema, childValue, path+"."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func requiredKeys(raw any) []string {
	values, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, item := range values {
		if key, ok := item.(string); ok && strings.TrimSpace(key) != "" {
			out = append(out, strings.TrimSpace(key))
		}
	}
	return out
}

func validateSchemaType(schemaType string, value any, path string) error {
	switch schemaType {
	case "", "any":
		return nil
	case "object":
		if _, ok := value.(map[string]any); ok {
			return nil
		}
	case "array":
		if _, ok := value.([]any); ok {
			return nil
		}
	case "string":
		if _, ok := value.(string); ok {
			return nil
		}
	case "boolean":
		if _, ok := value.(bool); ok {
			return nil
		}
	case "number":
		if isNumber(value) {
			return nil
		}
	case "integer":
		if isInteger(value) {
			return nil
		}
	default:
		return nil
	}
	return apperr.Newf(apperr.CodeInvalidArgument, "%s must be %s", path, schemaType)
}

func isNumber(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}

func isInteger(v any) bool {
	switch n := v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return math.Trunc(float64(n)) == float64(n)
	case float64:
		return math.Trunc(n) == n
	default:
		return false
	}
}
