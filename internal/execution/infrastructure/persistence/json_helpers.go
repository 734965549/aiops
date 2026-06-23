package persistence

import "encoding/json"

func marshalAnyMap(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

func unmarshalAnyMap(data []byte) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func marshalStringSlice(items []string) ([]byte, error) {
	if items == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(items)
}

func unmarshalStringSlice(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func marshalIntSlice(items []int) ([]byte, error) {
	if items == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(items)
}

func unmarshalIntSlice(data []byte) []int {
	if len(data) == 0 {
		return nil
	}
	var out []int
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
