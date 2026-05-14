package cli

// printImportNextSteps emits the post-import guidance block. It always
// suggests the sync workflow first, then surfaces up to three other
// detected CLIs as `import <name>` hints so users discover the rest of
// the migration path without rereading the docs.
//
// justImported is the source name the caller passed to runImport; it
// is filtered out of the suggested list so users do not see their own
// CLI echoed back.
func printImportNextSteps(root, justImported string) {
	summaryf("\n")
	summaryf("next steps:\n")
	summaryf("  agnostic-ai sync --check   # preview what changes\n")
	summaryf("  agnostic-ai sync           # write to configured targets\n")

	detected := detectExistingTargets(root)
	hints := make([]string, 0, len(detected))
	for _, d := range detected {
		if d == justImported {
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
