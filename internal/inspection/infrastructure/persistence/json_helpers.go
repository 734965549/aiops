package persistence

import (
	"encoding/json"
)

func marshalJSON(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}

func marshalStringSlice(items []string) ([]byte, error) {
	if items == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(items)
}

func unmarshalStringSlice(data []byte) []string {
	if len(data) == 0 {
		return []string{}
	}
	out := []string{}
	if err := json.Unmarshal(data, &out); err != nil {
		return []string{}
	}
	return out
}
