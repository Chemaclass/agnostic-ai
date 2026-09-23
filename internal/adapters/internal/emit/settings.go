package emit

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

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

// SettingsEffort returns the repository effort for target from the last
// settings spec that resolves one: a scalar, else the per-target map's
// target entry, else its default. Nil when none does.
func SettingsEffort(entries []spec.Entry, target string) any {
	var effort any
	for _, entry := range entries {
		raw, ok := entry.Meta["effort"]
		if !ok {
			continue
		}
		resolved := map[string]any{"effort": raw}
		keys := []string{"effort"}
		collapseTargetMap(resolved, &keys, "effort", target)
		switch v := resolved["effort"].(type) {
		case string:
			if v != "" {
				effort = v
			}
		case int, int64, float64:
			effort = v
		}
	}
	return effort
}

// SettingsEffortLevel returns the repository effort for target when
// levels accepts it, where nil levels accept any string. A value it
// rejects raises a coverage note and returns "".
func SettingsEffortLevel(entries []spec.Entry, target string, levels []string) string {
	effort := SettingsEffort(entries, target)
	if effort == nil {
		return ""
	}
	if level, ok := effort.(string); ok && (levels == nil || slices.Contains(levels, level)) {
		return level
	}
	accepted := "a string"
	if levels != nil {
		accepted = strings.Join(levels, ", ")
	}
	NoteFieldNoOp(target, spec.KindSettings, "effort", 1, fmt.Sprintf("%v is not a repository effort it accepts: %s", effort, accepted))
	return ""
}
