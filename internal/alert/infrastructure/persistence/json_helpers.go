// json_helpers 提供 labels/annotations/payload 与 JSONB 字节序列化互转。
package persistence

import (
	"encoding/json"
)

func marshalStringMap(m map[string]string) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

func unmarshalStringMap(data []byte) map[string]string {
	if len(data) == 0 {
		return map[string]string{}
	}
	out := map[string]string{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]string{}
	}
	return out
}

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
