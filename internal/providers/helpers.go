package providers

import (
	"encoding/json"
	"strings"
)

func parseToolArgumentsRaw(raw string) map[string]interface{} {
	args := make(map[string]interface{})
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &args)
	}
	args["_raw"] = raw
	return args
}

func parseToolArgumentsJSON(raw []byte) map[string]interface{} {
	args := make(map[string]interface{})
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &args)
	}
	args["_raw"] = string(raw)
	return args
}

func cleanToolArguments(args map[string]interface{}) map[string]interface{} {
	if args == nil {
		return map[string]interface{}{}
	}

	if raw, ok := args["_raw"].(string); ok && strings.TrimSpace(raw) != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil && parsed != nil {
			return parsed
		}
	}

	clean := make(map[string]interface{}, len(args))
	for key, value := range args {
		if key != "_raw" {
			clean[key] = value
		}
	}
	return clean
}

func toStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return nil
	}
}
