package emit

import "github.com/chemaclass/agnostic-ai/internal/spec"

// LastSettingsModel returns the last non-empty portable model setting.
// Settings specs layer in source order, matching Claude's established
// behavior for the same shared field.
func LastSettingsModel(entries []spec.Entry) string {
	var model string
	for _, entry := range entries {
		if value, _ := entry.Meta["model"].(string); value != "" {
			model = value
		}
	}
	return model
}

// SettingsPermissions merges the portable allow, deny, and ask lists in
// source order, removing duplicates while preserving the first spelling.
func SettingsPermissions(entries []spec.Entry) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"allow", "deny", "ask"} {
		seen := map[string]bool{}
		var values []string
		for _, entry := range entries {
			permissions, _ := entry.Meta["permissions"].(map[string]any)
			for _, value := range StringSlice(permissions[key]) {
				if value == "" || seen[value] {
					continue
				}
				seen[value] = true
				values = append(values, value)
			}
		}
		if len(values) > 0 {
			out[key] = values
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
