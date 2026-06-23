package persistence

import (
	"encoding/json"
)

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

func marshalCapabilities(caps []string) ([]byte, error) {
	if caps == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(caps)
}

func unmarshalCapabilities(data []byte) []string {
	return unmarshalStringSlice(data)
}
