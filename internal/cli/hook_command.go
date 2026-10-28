package cli

import (
	"fmt"
	"slices"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// hookCommandAliases are keys people write for `command:` that no
// target reads, too far from it for an edit-distance typo.
var hookCommandAliases = []string{"run", "cmd", "exec", "execute", "script", "commands"}

// hookRunnable reports whether a hook spec names something a target
// runs: a command or another handler at the top level or under the
// `x-<target>` block of a tool it reaches, Kiro's `x-kiro.action`, or the
// `hooks` an import writes under `x-gemini`, `x-qwen`, or `x-grok`. A
// hook with none of them is written nowhere.
func hookRunnable(e spec.Entry) bool {
	if hasHookHandler(e.Meta) {
		return true
	}
	for _, target := range adapters.Names() {
		native, ok := e.Meta["x-"+target].(map[string]any)
		if !ok || !e.EmitsTo(target) {
			continue
		}
		if hasHookHandler(native) || target == "kiro" && native["action"] != nil {
			return true
		}
		if target != "gemini" && target != "qwen" && target != "grok" {
			continue
		}
		switch hooks := native["hooks"].(type) {
		case []any:
			if len(hooks) > 0 {
				return true
			}
		case map[string]any:
			if len(hooks) > 0 {
				return true
			}
		}
	}
	return false
}

func hasHookHandler(meta map[string]any) bool {
	if len(globalHookCommands(meta["command"])) > 0 {
		return true
	}
	for _, key := range []string{"commandWindows", "prompt", "url", "server", "tool", "agent"} {
		if value, _ := meta[key].(string); value != "" {
			return true
		}
	}
	return false
}

// hookCommandIssues reports each hook spec with nothing to run, naming
// the key that was probably meant as `command:`.
func hookCommandIssues(hooks []spec.Entry) []validationIssue {
	var out []validationIssue
	for _, e := range hooks {
		if e.Kind != spec.KindHook || hookRunnable(e) {
			continue
		}
		message := "hook has no `command`, so no tool runs it; add `command:` with the shell command to run"
		if key := hookCommandMisspelling(e.Meta); key != "" {
			message += fmt.Sprintf(" (did you mean `command:` instead of `%s:`?)", key)
		}
		out = append(out, validationIssue{Path: e.Path, Field: "command", Message: message, entry: e})
	}
	return out
}

func hookCommandMisspelling(meta map[string]any) string {
	keys := make([]string, 0, len(meta))
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if slices.Contains(hookCommandAliases, key) || suggest.Name(key, []string{"command"}) != "" {
			return key
		}
	}
	return ""
}

// lintHookCommands reports the hookCommandIssues validate reports
// (LINT052, error).
func lintHookCommands(hooks []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, issue := range hookCommandIssues(hooks) {
		out = append(out, lintFinding{Code: "LINT052", Severity: lintError, Path: issue.Path, Message: issue.Message})
	}
	return out
}
