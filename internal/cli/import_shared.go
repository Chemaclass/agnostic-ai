package cli

import (
	"maps"
	"slices"
	"strconv"
)

// importNextStepsOff hides the post-import guidance while a command such
// as `use` runs the next steps itself. Sequential use only.
var importNextStepsOff bool

// importReportsNoChange lets the import command replace the sync steps
// with a no-change line. Init and use write other files first, so sync
// stays their next step.
var importReportsNoChange bool

// printImportNextSteps emits the post-import guidance block. It always
// suggests the sync workflow first, then surfaces up to three other
// detected CLIs as `import <name>` hints so users discover the rest of
// the migration path without rereading the docs. Sources in
// justImported, the ones this run imported, are never suggested.
func printImportNextSteps(root string, justImported ...string) {
	if importNextStepsOff {
		return
	}
	summaryf("\n")
	if importReportsNoChange && importSandbox == "" && importTxn != nil && !importTxn.changed() {
		summaryf("no spec changed, so sync has nothing new to write\n")
	} else {
		summaryf("next steps:\n")
		summaryf("  agnostic-ai sync --plan    # preview what changes\n")
		summaryf("  agnostic-ai sync           # write to configured targets\n")
	}

	detected, _ := detectImportSources(root)
	hints := make([]string, 0, len(detected))
	for _, d := range detected {
		if slices.Contains(justImported, d) {
			continue
		}
		hints = append(hints, d)
	}
	if len(hints) == 0 {
		return
	}
	shown := hints
	extra := 0
	if len(hints) > 3 {
		shown = hints[:3]
		extra = len(hints) - 3
	}
	for _, h := range shown {
		summaryf("  also detected %s/ - run 'agnostic-ai import %s' to import it\n", h, h)
	}
	if extra > 0 {
		summaryf("  (and %d more detected targets)\n", extra)
	}
}

// stringSliceFromAny coerces a YAML-unmarshaled `any` value into a
// []string, dropping non-string elements. Returns nil for missing or
// non-list values.
func stringSliceFromAny(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// nonStringKeyPath names the first field of value, in sorted key order,
// that holds a map with non-string keys, or rootLabel when value itself
// is one. It returns "" when every key is a string.
func nonStringKeyPath(value any, path, rootLabel string) string {
	switch value := value.(type) {
	case map[any]any:
		if path == "" {
			return rootLabel
		}
		return path
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(value)) {
			next := key
			if path != "" {
				next = path + "." + key
			}
			if field := nonStringKeyPath(value[key], next, rootLabel); field != "" {
				return field
			}
		}
	case []any:
		for i, item := range value {
			if field := nonStringKeyPath(item, path+"["+strconv.Itoa(i)+"]", rootLabel); field != "" {
				return field
			}
		}
	}
	return ""
}
