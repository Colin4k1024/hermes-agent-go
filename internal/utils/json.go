package utils

import "encoding/json"

// ToolError returns a JSON error string for tool handlers.
func ToolError(message string, extra ...map[string]any) string {
	result := map[string]any{"error": message}
	for _, e := range extra {
		for k, v := range e {
			result[k] = v
		}
	}
	b, _ := json.Marshal(result)
	return string(b)
}

// ToolResult returns a JSON result string for tool handlers.
func ToolResult(data map[string]any) string {
	b, _ := json.Marshal(data)
	return string(b)
}

// ToJSON marshals any value to a JSON string.
func ToJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// FromJSON unmarshals a JSON string into a map.
func FromJSON(s string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}
