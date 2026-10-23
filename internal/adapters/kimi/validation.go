package kimi

import (
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"
)

func nativeInteger(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		if int64(int(v)) == v {
			return int(v), true
		}
	case float64:
		if !math.IsNaN(v) && !math.IsInf(v, 0) && v == math.Trunc(v) && v >= 0 && v <= 2147483647 {
			return int(v), true
		}
	}
	return 0, false
}

func nativeStringList(value any, scalar bool) bool {
	switch v := value.(type) {
	case string:
		return scalar
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) == "" {
				return false
			}
		}
		return true
	case []any:
		for _, raw := range v {
			s, ok := raw.(string)
			if !ok || strings.TrimSpace(s) == "" {
				return false
			}
		}
		return true
	}
	return false
}
func validateAgent(meta map[string]any) error {
	if d, ok := meta["description"].(string); !ok || strings.TrimSpace(d) == "" {
		return fmt.Errorf("description must be a nonempty string")
	}
	if v, ok := meta["override"]; ok {
		if _, valid := v.(bool); !valid {
			return fmt.Errorf("override must be a boolean")
		}
	}
	for _, key := range []string{"tools", "disallowedTools", "subagents"} {
		if v, ok := meta[key]; ok && v != nil && !nativeStringList(v, true) {
			return fmt.Errorf("%s must be a string list", key)
		}
	}
	return nil
}
func ValidateHookRecord(record map[string]any) error {
	for key := range record {
		if !slices.Contains([]string{"event", "matcher", "command", "timeout"}, key) {
			return fmt.Errorf("Kimi hook field %s is unsupported", key)
		}
	}
	event, _ := record["event"].(string)
	if !slices.Contains(HookEvents(), event) {
		return fmt.Errorf("Kimi hook event %q is unsupported", event)
	}
	if command, ok := record["command"].(string); !ok || strings.TrimSpace(command) == "" {
		return fmt.Errorf("Kimi hook requires a nonempty command")
	}
	if matcher, ok := record["matcher"]; ok {
		if _, valid := matcher.(string); !valid {
			return fmt.Errorf("Kimi hook matcher must be a string")
		}
	}
	if value, ok := record["timeout"]; ok {
		n, valid := nativeInteger(value)
		if !valid || n < 1 || n > 600 {
			return fmt.Errorf("Kimi hook timeout must be an integer from 1 to 600 seconds")
		}
	}
	return nil
}
func validateMCP(meta map[string]any) error {
	for _, key := range []string{"enabled", "deferred"} {
		if value, ok := meta[key]; ok {
			if _, valid := value.(bool); !valid {
				return fmt.Errorf("%s must be a boolean", key)
			}
		}
	}
	for _, key := range []string{"args", "enabledTools", "disabledTools"} {
		if value, ok := meta[key]; ok && !nativeStringArray(value) {
			return fmt.Errorf("%s must be a string list", key)
		}
	}
	for _, key := range []string{"env", "headers"} {
		if value, ok := meta[key]; ok {
			switch values := value.(type) {
			case map[string]any:
				for _, raw := range values {
					if _, valid := raw.(string); !valid {
						return fmt.Errorf("%s values must be strings", key)
					}
				}
			case map[string]string:
			default:
				return fmt.Errorf("%s must be a string map", key)
			}
		}
	}
	if auth, ok := meta["auth"]; ok && auth != "oauth" {
		return fmt.Errorf("auth must be oauth")
	}
	if value, ok := meta["bearerTokenEnvVar"]; ok {
		if name, valid := value.(string); !valid || strings.TrimSpace(name) == "" {
			return fmt.Errorf("bearerTokenEnvVar must name an environment variable")
		}
	}
	if meta["transport"] == "stdio" {
		for _, key := range []string{"url", "headers", "auth", "bearerTokenEnvVar"} {
			if _, ok := meta[key]; ok {
				return fmt.Errorf("%s requires a remote transport", key)
			}
		}
	} else {
		raw, _ := meta["url"].(string)
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("url must be absolute")
		}
	}
	return nil
}

func nativeStringArray(value any) bool {
	switch values := value.(type) {
	case []string:
		return true
	case []any:
		for _, v := range values {
			if _, ok := v.(string); !ok {
				return false
			}
		}
		return true
	}
	return false
}
